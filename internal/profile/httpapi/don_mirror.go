package httpapi

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"time"

	"ourtaiko.dev/net/internal/web"
)

// A mirror catalog is published after the objects it lists. The canonical catalog still decides
// which outfits exist and the bytes they require.
type donMirror struct {
	base  string
	files map[string]donFileRef
}

// donMirrorPaths is what the website catalog adds for a mainland visitor: the mirror's base and
// the paths of the files it holds as the canonical catalog lists them, worked out once per pair
// of catalogs.
type donMirrorPaths struct {
	catalog     *donCatalog
	mirror      *donMirror
	base, paths json.RawMessage
}

// How long a failed mirror catalog read waits before the next try.
const donMirrorRetry = 30 * time.Second

func (c *donCatalog) fileRefs(yield func(donFileRef) bool) {
	for _, ref := range c.Base {
		if !yield(ref) {
			return
		}
	}
	for _, items := range c.Items {
		for _, item := range items {
			if !yield(item.File) || item.Icon != nil && !yield(*item.Icon) {
				return
			}
		}
	}
}

func (m *donMirror) has(ref donFileRef) bool {
	if m == nil {
		return false
	}
	mirrored, ok := m.files[ref.Path]
	return ok && mirrored == ref
}

// url is a file's address on the mirror, made like the canonical one; "" when it is not there.
func (m *donMirror) url(ref donFileRef) string {
	if !m.has(ref) {
		return ""
	}
	return m.base + ref.Path + "?v=" + ref.SHA256[:12]
}

// mirrorPaths is the mirror as the website catalog tells it; nil fields when it holds none of
// the catalog's files.
func (s *Server) mirrorPaths(c *donCatalog, m *donMirror) *donMirrorPaths {
	s.donMirrorMu.Lock()
	defer s.donMirrorMu.Unlock()
	if p := s.donMirrorPaths; p != nil && p.catalog == c && p.mirror == m {
		return p
	}
	var paths []string
	for ref := range c.fileRefs {
		if m.has(ref) {
			paths = append(paths, ref.Path)
		}
	}
	slices.Sort(paths)
	paths = slices.Compact(paths)
	p := &donMirrorPaths{catalog: c, mirror: m}
	if len(paths) > 0 {
		p.base, _ = json.Marshal(m.base)
		p.paths, _ = json.Marshal(paths)
	}
	s.donMirrorPaths = p
	return p
}

func (s *Server) readDonMirror(ctx context.Context, address string) (*donMirror, error) {
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil ||
		u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return nil, errors.New("DON_MIRROR_CATALOG_URL must be an HTTPS URL without credentials, query or fragment")
	}
	timeout, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	c, err := s.readDonCatalog(timeout, address)
	if err != nil {
		return nil, err
	}
	m := &donMirror{base: u.ResolveReference(&url.URL{Path: "."}).String(), files: make(map[string]donFileRef)}
	for ref := range c.fileRefs {
		if _, err = hex.DecodeString(ref.SHA256); !fs.ValidPath(ref.Path) || ref.Path == "." ||
			strings.ContainsAny(ref.Path, "\\\x00") || ref.Size <= 0 || len(ref.SHA256) != 64 || err != nil {
			return nil, fmt.Errorf("invalid Don mirror entry for %q", ref.Path)
		}
		if previous, exists := m.files[ref.Path]; exists && previous != ref {
			return nil, fmt.Errorf("conflicting Don mirror entries for %q", ref.Path)
		}
		m.files[ref.Path] = ref
	}
	return m, nil
}

// donRegional says whether Don files may be routed by where a request comes from.
func (s *Server) donRegional() bool {
	return s.Config.DonMirrorCatalogURL != "" && s.Config.DonIPCountry != nil
}

func (s *Server) mirrorForDonRequest(r *http.Request) *donMirror {
	cfg := s.Config
	if !s.donRegional() {
		return nil
	}
	ip, err := netip.ParseAddr(web.ClientIP(r, cfg.TrustedProxies))
	if err != nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || cfg.DonIPCountry(ip) != "CN" {
		return nil
	}
	return s.donMirrorNow(r.Context())
}

// donMirrorNow is the mirror as last read. When it is due, one refresh runs on its own; requests
// keep the last good mirror meanwhile, and only wait when there has never been one.
func (s *Server) donMirrorNow(ctx context.Context) *donMirror {
	s.donMirrorMu.Lock()
	mirror := s.donMirror
	if time.Now().Before(s.donMirrorUntil) {
		s.donMirrorMu.Unlock()
		return mirror
	}
	done := s.donMirrorDone
	if done == nil {
		done = make(chan struct{})
		s.donMirrorDone = done
		go s.refreshDonMirror(done)
	}
	s.donMirrorMu.Unlock()
	if mirror != nil {
		return mirror
	}
	select {
	case <-done:
	case <-ctx.Done():
		return nil
	}
	s.donMirrorMu.Lock()
	defer s.donMirrorMu.Unlock()
	return s.donMirror
}

func (s *Server) refreshDonMirror(done chan struct{}) {
	mirror, err := s.readDonMirror(context.Background(), s.Config.DonMirrorCatalogURL)
	s.donMirrorMu.Lock()
	ttl := donCatalogTTL
	if err != nil {
		// Keep the last good mirror (every file is still checked against the canonical
		// catalog) and try again soon.
		ttl = donMirrorRetry
		log.Printf("reading Don mirror catalog: %v", err)
	} else {
		s.donMirror = mirror
	}
	s.donMirrorUntil = time.Now().Add(ttl)
	s.donMirrorDone = nil
	s.donMirrorMu.Unlock()
	close(done)
}
