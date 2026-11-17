package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"time"
)

// donMirror is where a mainland visitor fetches the canonical catalog's files: every file, at
// the mirror's root. The country alone decides; the mirror's own catalog is not read (OSS fetches
// a missing object from the origin, and clients check each file's SHA-256).
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

// How long a failed catalog read waits before the next try.
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

// mirrorForDonRequest is the mirror for a mainland visitor of catalog c, nil for everyone else.
func (s *Server) mirrorForDonRequest(r *http.Request, c *donCatalog) *donMirror {
	if !s.donRegional() || !s.Config.Distribution.IsMainland(r) {
		return nil
	}
	u, err := ParseDonMirrorCatalogURL(s.Config.DonMirrorCatalogURL)
	if err != nil {
		return nil
	}
	s.donMirrorMu.Lock()
	defer s.donMirrorMu.Unlock()
	if m := s.donMirror; m != nil && s.donMirrorOf == c {
		return m
	}
	m := &donMirror{base: u.ResolveReference(&url.URL{Path: "."}).String(), files: make(map[string]donFileRef)}
	for ref := range c.fileRefs {
		m.files[ref.Path] = ref
	}
	s.donMirror, s.donMirrorOf = m, c
	return m
}
