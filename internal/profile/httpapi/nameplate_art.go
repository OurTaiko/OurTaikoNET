package httpapi

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"ourtaiko.dev/net/internal/kit/distribution"
	"ourtaiko.dev/net/internal/web"
)

type nameplateCatalog struct {
	Version int                   `json:"version"`
	Files   map[string]donFileRef `json:"files"`
	base    string
}

func (s *Server) readNameplateCatalog(ctx context.Context, address string) (*nameplateCatalog, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	client := s.Config.DonClient
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("nameplate catalog HTTP %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, (1<<20)+1))
	if err != nil || len(body) > 1<<20 {
		return nil, errors.New("nameplate catalog exceeds limit")
	}
	var catalog nameplateCatalog
	if json.Unmarshal(body, &catalog) != nil || catalog.Version != 1 || len(catalog.Files) == 0 || len(catalog.Files) > 128 {
		return nil, errors.New("invalid nameplate catalog")
	}
	for _, ref := range catalog.Files {
		_, err := hex.DecodeString(ref.SHA256)
		if err != nil || len(ref.SHA256) != 64 || ref.Size <= 0 || ref.Size > 8<<20 || !fs.ValidPath(ref.Path) || ref.Path == "." || strings.ContainsAny(ref.Path, "\\?#:\x00") || !strings.HasSuffix(ref.Path, ".png") {
			return nil, errors.New("invalid nameplate file")
		}
	}
	u, err := url.Parse(address)
	if err != nil {
		return nil, err
	}
	catalog.base = u.ResolveReference(&url.URL{Path: "."}).String()
	return &catalog, nil
}

func (s *Server) nameplateArtAnswer(w http.ResponseWriter, r *http.Request) {
	router := s.Config.Distribution
	regional := router != nil && router.Base != "" && router.Country != nil
	if regional {
		w.Header().Set("Cache-Control", "private, max-age=60")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=60")
	}
	s.nameplateMu.Lock()
	defer s.nameplateMu.Unlock()
	address := s.Config.NameplateCatalogURL
	if address == "" {
		address = distribution.CanonicalBase + "nameplate/catalog.json"
	}
	if s.nameplateArtAt.IsZero() || time.Since(s.nameplateArtAt) >= donCatalogTTL {
		catalog, err := s.readNameplateCatalog(r.Context(), address)
		if err == nil {
			s.nameplateArtCatalog = catalog
		}
		s.nameplateArtAt = time.Now()
		if err != nil {
			s.nameplateArtAt = s.nameplateArtAt.Add(-donCatalogTTL + donMirrorRetry)
		}
	}
	if s.nameplateArtCatalog == nil {
		problem(w, http.StatusServiceUnavailable, "NAMEPLATE_CATALOG_UNAVAILABLE", "名牌资源清单暂时不可用")
		return
	}
	answer := map[string]any{"version": s.nameplateArtCatalog.Version, "files": s.nameplateArtCatalog.Files, "baseUrl": s.nameplateArtCatalog.base}
	if regional && router.IsMainland(r) && r.URL.Query().Get("origin") != "1" {
		if s.nameplateMirrorAt.IsZero() || time.Since(s.nameplateMirrorAt) >= donCatalogTTL {
			catalog, err := s.readNameplateCatalog(r.Context(), router.Base+"nameplate/catalog.json")
			if err == nil {
				s.nameplateMirror = catalog
			}
			s.nameplateMirrorAt = time.Now()
			if err != nil {
				s.nameplateMirrorAt = s.nameplateMirrorAt.Add(-donCatalogTTL + donMirrorRetry)
			}
		}
		if mirror := s.nameplateMirror; mirror != nil {
			var paths []string
			for key, ref := range s.nameplateArtCatalog.Files {
				if mirror.Files[key] == ref {
					paths = append(paths, ref.Path)
				}
			}
			if len(paths) > 0 {
				sort.Strings(paths)
				answer["mirrorBaseUrl"], answer["mirrorPaths"] = mirror.base, paths
			}
		}
	}
	web.Respond(w, http.StatusOK, answer)
}
