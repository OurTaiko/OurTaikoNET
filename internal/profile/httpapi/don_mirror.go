package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"slices"
	"time"
)

// A mainland visitor gets everything from the mirror, the catalog included: the mirror's own
// catalog.json (its CDN keeps it 60 seconds) and the files it lists, at the mirror's root. The
// country alone decides; nothing is compared with the canonical catalog. The synchronization
// worker publishes that catalog after the files it lists, so they are there when it names them.

// How long the mirror's catalog is used before it is read again (its CDN caches it as long),
// and how long a failed read waits before the next try.
const (
	donMirrorTTL   = time.Minute
	donMirrorRetry = 30 * time.Second
)

// donMirrorPaths is what the website catalog adds for a mainland visitor: the mirror's base and
// every path its catalog lists, worked out once per catalog read.
type donMirrorPaths struct {
	catalog     *donCatalog
	base, paths json.RawMessage
}

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

// mirrorPaths is the mirror catalog m as the website catalog tells it.
func (s *Server) mirrorPaths(m *donCatalog) *donMirrorPaths {
	s.donMirrorMu.Lock()
	defer s.donMirrorMu.Unlock()
	if p := s.donMirrorPaths; p != nil && p.catalog == m {
		return p
	}
	var paths []string
	for ref := range m.fileRefs {
		paths = append(paths, ref.Path)
	}
	slices.Sort(paths)
	p := &donMirrorPaths{catalog: m}
	p.base, _ = json.Marshal(m.baseURL)
	p.paths, _ = json.Marshal(slices.Compact(paths))
	s.donMirrorPaths = p
	return p
}

// ParseDonMirrorCatalogURL checks DON_MIRROR_CATALOG_URL; the server does not start with a bad one.
func ParseDonMirrorCatalogURL(address string) (*url.URL, error) {
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil ||
		u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return nil, errors.New("DON_MIRROR_CATALOG_URL must be an HTTPS URL without credentials, query or fragment")
	}
	return u, nil
}

// donRegional says whether Don files may be routed by where a request comes from.
func (s *Server) donRegional() bool {
	return s.Config.DonMirrorCatalogURL != "" && s.Config.Distribution != nil && s.Config.Distribution.Country != nil
}

// donMirrorFor is the mirror's catalog for a mainland visitor; nil for everyone else, and while
// the mirror's catalog has never been read (the canonical one is used then).
func (s *Server) donMirrorFor(r *http.Request) *donCatalog {
	if !s.donRegional() || !s.Config.Distribution.IsMainland(r) {
		return nil
	}
	return s.donMirrorNow(r.Context())
}

// donMirrorNow is the mirror's catalog as last read. When it is due, one refresh runs on its own;
// requests keep the last good catalog meanwhile, and only wait when there has never been one.
func (s *Server) donMirrorNow(ctx context.Context) *donCatalog {
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	mirror, err := s.readDonCatalog(ctx, s.Config.DonMirrorCatalogURL)
	s.donMirrorMu.Lock()
	ttl := donMirrorTTL
	if err != nil {
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
