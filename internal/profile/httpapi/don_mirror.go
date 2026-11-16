package httpapi

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"ourtaiko.dev/net/internal/web"
)

// A mirror catalog is published after the objects it lists. The canonical catalog still decides
// which outfits exist and the bytes they require.
type donMirror struct {
	base  *url.URL
	files map[string]donFileRef
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

func (m *donMirror) url(ref donFileRef) string {
	if m == nil {
		return ""
	}
	mirrored, ok := m.files[ref.Path]
	if !ok || mirrored != ref {
		return ""
	}
	return m.base.ResolveReference(&url.URL{Path: ref.Path, RawQuery: "v=" + ref.SHA256[:12]}).String()
}

func (m *donMirror) urls(c *donCatalog) map[string]string {
	urls := make(map[string]string)
	for ref := range c.fileRefs {
		if address := m.url(ref); address != "" {
			urls[ref.Path] = address
		}
	}
	return urls
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
	m := &donMirror{base: u.ResolveReference(&url.URL{Path: "."}), files: make(map[string]donFileRef)}
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

func (s *Server) mirrorForDonRequest(r *http.Request) *donMirror {
	cfg := s.Config
	if cfg.DonMirrorCatalogURL == "" || cfg.DonIPCountry == nil {
		return nil
	}
	ip, err := netip.ParseAddr(web.ClientIP(r, cfg.TrustedProxies))
	if err != nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || cfg.DonIPCountry(ip) != "CN" {
		return nil
	}
	s.donMirrorMu.Lock()
	defer s.donMirrorMu.Unlock()
	if time.Now().Before(s.donMirrorUntil) {
		return s.donMirror
	}
	mirror, err := s.readDonMirror(r.Context(), cfg.DonMirrorCatalogURL)
	if r.Context().Err() != nil {
		return nil
	}
	s.donMirror = mirror
	ttl := donCatalogTTL
	if err != nil {
		// A failed refresh leaves every URL on the canonical origin. Retry soon without
		// making every request wait on an unavailable mirror.
		ttl = 30 * time.Second
		log.Printf("reading Don mirror catalog: %v", err)
	}
	s.donMirrorUntil = time.Now().Add(ttl)
	return mirror
}
