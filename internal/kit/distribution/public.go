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
	case "/api/v1/distribution/player-build":
		r.playerAnswer(w, request)
	default:
		http.NotFound(w, request)
	}
}
