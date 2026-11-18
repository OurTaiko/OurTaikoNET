package httpapi

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"ourtaiko.dev/net/internal/files"
	"strings"
	"time"

	"ourtaiko.dev/net/internal/web"
)

// How long a failed read of the nameplate catalog waits before the next try.
const nameplateRetry = 30 * time.Second

type nameplateCatalog struct {
	Version int                   `json:"version"`
	Files   map[string]donFileRef `json:"files"`
	// The catalog's folder under the distribution root ("nameplate/"), which its paths are in.
	folder string
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
	// The catalog is at <root>/nameplate/catalog.json; one read from elsewhere (tests) is taken
	// to stand for that folder.
	if catalog.folder = files.Path(address[:strings.LastIndex(address, "/")+1]); catalog.folder == "" {
		catalog.folder = "nameplate/"
	}
	return &catalog, nil
}

// nameplateSlot is one catalog as last read. A refresh runs in the background, once at a
// time, so a visitor who leaves or a slow source never holds up the others; the last good
// catalog keeps being answered meanwhile.
type nameplateSlot struct {
	catalog *nameplateCatalog
	until   time.Time
	done    chan struct{}
}

// nameplateNow answers the catalog at address, waiting for a read only when none has
// succeeded yet (and no longer than the caller's request).
func (s *Server) nameplateNow(ctx context.Context, slot *nameplateSlot, address string) *nameplateCatalog {
	s.nameplateMu.Lock()
	catalog := slot.catalog
	if time.Now().Before(slot.until) {
		s.nameplateMu.Unlock()
		return catalog
	}
	done := slot.done
	if done == nil {
		done = make(chan struct{})
		slot.done = done
		go s.refreshNameplate(slot, address, done)
	}
	s.nameplateMu.Unlock()
	if catalog != nil {
		return catalog
	}
	select {
	case <-done:
	case <-ctx.Done():
		return nil
	}
	s.nameplateMu.Lock()
	defer s.nameplateMu.Unlock()
	return slot.catalog
}

func (s *Server) refreshNameplate(slot *nameplateSlot, address string, done chan struct{}) {
	catalog, err := s.readNameplateCatalog(context.Background(), address)
	s.nameplateMu.Lock()
	defer s.nameplateMu.Unlock()
	ttl := donCatalogTTL
	if err != nil {
		ttl = nameplateRetry
		log.Printf("reading nameplate catalog %s: %v", address, err)
	} else {
		slot.catalog = catalog
	}
	slot.until = time.Now().Add(ttl)
	slot.done = nil
	close(done)
}

// nameplateArtAnswer is GET /api/nameplate-art/catalog: the bucket's nameplate catalog with
// every file's path under the distribution root (package files).
func (s *Server) nameplateArtAnswer(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "public, max-age=60")
	address := s.Config.NameplateCatalogURL
	if address == "" {
		address = files.Root + "nameplate/catalog.json"
	}
	catalog := s.nameplateNow(r.Context(), &s.nameplateArt, address)
	if catalog == nil {
		problem(w, http.StatusServiceUnavailable, "NAMEPLATE_CATALOG_UNAVAILABLE", "名牌资源清单暂时不可用")
		return
	}
	rooted := make(map[string]donFileRef, len(catalog.Files))
	for key, ref := range catalog.Files {
		ref.Path = catalog.folder + ref.Path
		rooted[key] = ref
	}
	web.Respond(w, http.StatusOK, map[string]any{"version": catalog.Version, "files": rooted})
}
