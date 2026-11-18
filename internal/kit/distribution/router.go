// Package distribution decides, by where a request comes from (one trusted client-IP policy),
// which root a client reads the public files from (package files): the canonical CDN, or the
// mirror for a visitor in mainland China.
package distribution

import (
	"errors"
	"net/http"
	"net/netip"
	"net/url"
	"strings"

	"ourtaiko.dev/net/internal/files"
	"ourtaiko.dev/net/internal/web"
)

// CanonicalBase is the canonical root, the CDN in front of the public bucket.
const CanonicalBase = files.Root

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

// MirrorBase is the mirror root (ending in "/") for a mainland visitor, or "" for everyone else.
func (r *Router) MirrorBase(request *http.Request) string {
	if r == nil || r.Base == "" || !r.IsMainland(request) {
		return ""
	}
	return strings.TrimRight(r.Base, "/") + "/"
}

// Regional reports whether the router tells a mainland visitor at all: a mirror root and a
// country lookup are both configured.
func (r *Router) Regional() bool {
	return r != nil && r.Base != "" && r.Country != nil
}

// ParseMirrorCatalogURL checks DON_MIRROR_CATALOG_URL, the mirror's 3D Don catalog, whose folder's
// parent is the mirror's root (FromDonCatalog).
func ParseMirrorCatalogURL(address string) (*url.URL, error) {
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil ||
		u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return nil, errors.New("DON_MIRROR_CATALOG_URL must be an HTTPS URL without credentials, query or fragment")
	}
	return u, nil
}
