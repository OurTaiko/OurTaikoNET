// Package distribution selects public resource mirrors using one trusted client-IP policy.
package distribution

import (
	"encoding/hex"
	"net/http"
	"net/netip"
	"net/url"
	"path"
	"strings"

	"ourtaiko.dev/net/internal/web"
)

const CanonicalBase = "https://distrib.ourtaiko.org/"

type Router struct {
	Base           string
	Country        func(netip.Addr) string
	TrustedProxies []netip.Prefix
}

// FromDonCatalog preserves the existing deployment settings. The shared layout has
// 3dchara/, nameplate/, fanmade/ and database/ under one mirror root. A custom Don-only
// catalog outside that layout keeps its existing Don routing without guessing sibling URLs.
func FromDonCatalog(address string, country func(netip.Addr) string, trusted []netip.Prefix) *Router {
	r := &Router{Country: country, TrustedProxies: trusted}
	u, err := url.Parse(address)
	if err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && strings.HasSuffix(u.Path, "/3dchara/catalog.json") {
		r.Base = u.ResolveReference(&url.URL{Path: "../"}).String()
	}
	return r
}

func (r *Router) IsMainland(request *http.Request) bool {
	if r == nil || r.Country == nil {
		return false
	}
	ip, err := netip.ParseAddr(web.ClientIP(request, r.TrustedProxies))
	return err == nil && ip.IsGlobalUnicast() && !ip.IsPrivate() && r.Country(ip) == "CN"
}

// Candidate maps only public, content-addressed song objects. Signed URLs, external
// origins, mutable objects and unknown extensions always keep their canonical route.
func (r *Router) Candidate(request *http.Request, canonical, digest string) string {
	if r == nil || r.Base == "" || !r.IsMainland(request) {
		return canonical
	}
	u, err := url.Parse(canonical)
	if err != nil || u.Scheme != "https" || u.Host != "distrib.ourtaiko.org" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" {
		return canonical
	}
	if !(strings.HasPrefix(u.Path, "/fanmade/") || strings.HasPrefix(u.Path, "/database/")) {
		return canonical
	}
	if len(digest) != 64 {
		return canonical
	}
	if _, err := hex.DecodeString(digest); err != nil {
		return canonical
	}
	name := path.Base(u.Path)
	ext := path.Ext(name)
	if strings.TrimSuffix(name, ext) != digest || path.Clean(u.Path) != u.Path {
		return canonical
	}
	switch ext {
	case ".tja", ".ogg", ".mp3", ".webp", ".png", ".jpg", ".jpeg", ".mp4":
	default:
		return canonical
	}
	return strings.TrimRight(r.Base, "/") + u.Path
}

// Select is the address a request uses: the mirror's for a mainland visitor, otherwise the
// canonical one. The country decides alone; nothing is probed or compared (OSS fetches a missing
// object from the origin on first request, and clients fall back to FallbackURL if it fails).
func (r *Router) Select(request *http.Request, canonical, digest string, _ int64) string {
	return r.Candidate(request, canonical, digest)
}

// MirrorBase is the mirror root (ending in "/") for a mainland visitor, or "" for everyone else.
func (r *Router) MirrorBase(request *http.Request) string {
	if r == nil || r.Base == "" || !r.IsMainland(request) {
		return ""
	}
	return strings.TrimRight(r.Base, "/") + "/"
}
