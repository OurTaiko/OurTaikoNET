package distribution

import (
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

var avatarPath = regexp.MustCompile(`^/sso/avatars/[a-f0-9]{32}/[a-f0-9]{32}\.webp$`)

// Avatar accepts only the account center's public, immutable CDN objects. It never
// probes arbitrary URLs, signed requests, or account endpoints.
func (r *Router) Avatar(request *http.Request, original string) string {
	u, err := url.Parse(original)
	if err != nil || u.Scheme != "https" || u.Host != "distrib.ourtaiko.org" || u.User != nil ||
		u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" || !avatarPath.MatchString(u.Path) {
		return original
	}
	if r == nil || r.Base == "" || !r.IsMainland(request) {
		return original
	}
	target := strings.TrimRight(r.Base, "/") + u.Path
	if r.available(request.Context(), target, 0) {
		return target
	}
	return original
}

// SourceAnswer tells a page, once, which root this visitor's public files come from, so it
// can build avatar and other public addresses itself instead of asking for each one.
type SourceAnswer struct {
	Mirror           bool   `json:"mirror"`
	BaseURL          string `json:"baseUrl"`
	CanonicalBaseURL string `json:"canonicalBaseUrl"`
}

// Source uses the same country and trusted-proxy policy as every other mirror choice. It does
// not probe: a page falls back to the canonical address when a mirror object fails to load.
func (r *Router) Source(request *http.Request) SourceAnswer {
	if r == nil || r.Base == "" || !r.IsMainland(request) {
		return SourceAnswer{BaseURL: CanonicalBase, CanonicalBaseURL: CanonicalBase}
	}
	return SourceAnswer{Mirror: true, BaseURL: strings.TrimRight(r.Base, "/") + "/", CanonicalBaseURL: CanonicalBase}
}

// Public serves both browsers (redirect=1) and credential-free game requests.
func (r *Router) Public(w http.ResponseWriter, request *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	switch request.URL.Path {
	case "/api/v1/distribution/avatar":
		original := request.URL.Query().Get("url")
		u, err := url.Parse(original)
		if err != nil || u.Scheme != "https" || u.Host != "distrib.ourtaiko.org" || u.User != nil ||
			u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" || !avatarPath.MatchString(u.Path) {
			http.Error(w, `{"error":"RESOURCE_URL_INVALID"}`, http.StatusBadRequest)
			return
		}
		selected := r.Avatar(request, original)
		if request.URL.Query().Get("redirect") == "1" {
			w.Header().Del("Content-Type")
			http.Redirect(w, request, selected, http.StatusTemporaryRedirect)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"url": selected, "fallbackUrl": original})
	case "/api/v1/distribution/source":
		json.NewEncoder(w).Encode(r.Source(request))
	case "/api/v1/distribution/player-build":
		r.playerAnswer(w, request)
	default:
		http.NotFound(w, request)
	}
}
