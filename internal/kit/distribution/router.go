// Package distribution selects public resource mirrors using one trusted client-IP policy.
package distribution

import (
	"context"
	"encoding/hex"
	"net/http"
	"net/netip"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"ourtaiko.dev/net/internal/web"
)

const CanonicalBase = "https://distrib.ourtaiko.org/"

type Router struct {
	Base           string
	Country        func(netip.Addr) string
	TrustedProxies []netip.Prefix
	Client         *http.Client
	mu             sync.Mutex
	checks         map[string]check
	flights        map[string]chan struct{}
	probes         chan struct{}
}

type check struct {
	ok    bool
	until time.Time
}

// FromDonCatalog preserves the existing deployment settings. The shared layout has
// 3dchara/, nameplate/, fanmade/ and database/ under one mirror root. A custom Don-only
// catalog outside that layout keeps its existing Don routing without guessing sibling URLs.
func FromDonCatalog(address string, country func(netip.Addr) string, trusted []netip.Prefix) *Router {
	r := &Router{Country: country, TrustedProxies: trusted, checks: make(map[string]check), flights: make(map[string]chan struct{}), probes: make(chan struct{}, 4)}
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

// Select checks cold mirror objects with HEAD, which OSS may satisfy by back-to-origin.
// A failed, unavailable or incorrectly sized mirror uses the original URL. Positive and
// negative checks are bounded and shared across all source endpoints.
func (r *Router) Select(request *http.Request, canonical, digest string, size int64) string {
	target := r.Candidate(request, canonical, digest)
	if target == canonical {
		return canonical
	}
	if r.available(request.Context(), target, size) {
		return target
	}
	return canonical
}

func (r *Router) available(parent context.Context, address string, size int64) bool {
	if parent.Err() != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	key := address + ":" + strconv.FormatInt(size, 10)
	r.mu.Lock()
	if cached, exists := r.checks[key]; exists && time.Now().Before(cached.until) {
		r.mu.Unlock()
		return cached.ok
	}
	done := r.flights[key]
	if done == nil {
		// Bound queued cold probes as well as active connections. A slow cold origin must
		// not hold the API response or create an unbounded background queue.
		if len(r.flights) >= 256 {
			r.mu.Unlock()
			return false
		}
		done = make(chan struct{})
		r.flights[key] = done
		go r.probe(address, size, key, done)
	}
	r.mu.Unlock()
	select {
	case <-done:
		r.mu.Lock()
		cached := r.checks[key]
		r.mu.Unlock()
		return cached.ok
	case <-ctx.Done():
		return false
	}
}

func (r *Router) probe(address string, size int64, key string, done chan struct{}) {
	// Finish cold HEAD even after the caller has used canonical URLs. OSS may need
	// several seconds to contact its origin; later requests reuse this result.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	ok := false
	select {
	case r.probes <- struct{}{}:
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, address, nil)
		if err == nil {
			client := r.Client
			if client == nil {
				client = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
			}
			res, err := client.Do(req)
			if err == nil {
				ok = res.StatusCode == http.StatusOK && res.ContentLength > 0 && (size <= 0 || res.ContentLength == size)
				res.Body.Close()
			}
		}
		<-r.probes
	case <-ctx.Done():
	}
	ttl := 30 * time.Second
	if ok {
		ttl = 5 * time.Minute
	}
	r.mu.Lock()
	if len(r.checks) >= 8192 {
		clear(r.checks)
	}
	r.checks[key] = check{ok, time.Now().Add(ttl)}
	delete(r.flights, key)
	close(done)
	r.mu.Unlock()
}
