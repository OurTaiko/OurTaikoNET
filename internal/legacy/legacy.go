// Package legacy keeps the old addresses working.
//
// Before OurTaikoNet there were two song sites, each with its own host name and its own API:
// fanmade.ourtaiko.org and database.ourtaiko.org, and the players' site profile.ourtaiko.org.
// Released versions of the game call /api/v1/game/... and the chart, score and replay routes
// on the first two, and read nameplates from the third. This package is the only place that
// knows about them: a request that arrives for an old host is answered by that site's API
// exactly as the old site answered it.
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

// Hosts maps an old host name (without port, lower case) to the API of its song source.
type Hosts map[string]http.Handler

// Handler answers requests for an old host from that host's source and passes every other
// request to current.
func Handler(hosts Hosts, current http.Handler) http.Handler {
	if len(hosts) == 0 {
		return current
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if old, ok := hosts[hostname(r.Host)]; ok {
			old.ServeHTTP(w, r)
			return
		}
		current.ServeHTTP(w, r)
	})
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
