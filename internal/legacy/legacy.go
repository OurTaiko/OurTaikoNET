// Package legacy keeps the old addresses working.
//
// Before OurTaikoNet there were two song sites, each with its own host name and its own API:
// fanmade.ourtaiko.org and database.ourtaiko.org, and the players' site profile.ourtaiko.org.
// Released versions of the game call /api/v1/game/... and the chart, score and replay routes
// on the first two, and read nameplates from the third. This package is the only place that
// knows about them: a request that arrives for an old host is answered by that site's API
// exactly as the old site answered it. The old sites' pages are not kept: a browser that opens
// one is sent to the same page of the new site.
//
// Nothing new belongs here. New clients use the routes of package site. When the game
// versions that need the old addresses are retired, remove this package and the two host
// names from the configuration, and nothing else has to change.
package legacy

import (
	"net"
	"net/http"
	"strings"
)

// Site is what an old host name still gives.
type Site struct {
	// API answers the old site's API as it did.
	API http.Handler
	// Page is where one of the old site's pages is now: the whole address for the page's
	// path (escaped, starting with "/"). Nil: the old site's pages are not sent anywhere.
	Page func(path string) string
}

// Hosts maps an old host name (without port, lower case) to what it still gives.
type Hosts map[string]Site

// Handler answers requests for an old host as that site's API did, sends a browser that
// asks an old host for a page to the page's new address, and passes every other request to
// current.
func Handler(hosts Hosts, current http.Handler) http.Handler {
	if len(hosts) == 0 {
		return current
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		old, ok := hosts[hostname(r.Host)]
		if !ok {
			current.ServeHTTP(w, r)
			return
		}
		if old.Page != nil && !isAPI(r.URL.Path) && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
			to := old.Page(r.URL.EscapedPath())
			if r.URL.RawQuery != "" {
				to += "?" + r.URL.RawQuery
			}
			// Permanent: the page lives at the new address from now on, and links and
			// bookmarks should follow.
			http.Redirect(w, r, to, http.StatusMovedPermanently)
			return
		}
		old.API.ServeHTTP(w, r)
	})
}

// isAPI reports whether a path of an old host belonged to its API (or its health checks)
// rather than to its website. The old sites served nothing else outside these.
func isAPI(path string) bool {
	for _, prefix := range []string{"/api/", "/internal/"} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return path == "/healthz" || path == "/readyz"
}

// Under gives Page for an old site whose pages are the same pages under prefix on the new
// site: fanmade.ourtaiko.org/charts/x is <origin>/fanmade/charts/x.
func Under(origin, prefix string) func(string) string {
	return func(path string) string { return origin + prefix + path }
}

// Players gives Page for the old players' site: a player's page was /users/<id> and is
// /players/<id>; its other pages kept their paths.
func Players(origin string) func(string) string {
	return func(path string) string {
		if id, ok := strings.CutPrefix(path, "/users/"); ok {
			return origin + "/players/" + id
		}
		return origin + path
	}
}

// PublicOnly hides a site's /internal/ routes. The old sites called each other there; nothing
// outside this program has a reason to any more.
func PublicOnly(api http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/internal/") {
			http.NotFound(w, r)
			return
		}
		api.ServeHTTP(w, r)
	})
}

func hostname(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.ToLower(strings.TrimSuffix(host, "."))
}
