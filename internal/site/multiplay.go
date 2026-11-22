package site

import (
	"net/http"

	"ourtaiko.dev/net/internal/web"
)

// Multiplay is where the game finds Multi-Enso, the online rooms (internal/multiplay): a
// gRPC service of its own, not part of this API. The game asks
// GET /api/game/multiplay before it connects:
//
//	{"url": "https://net.ourtaiko.org", "protocol": 1, "maxProtocol": 2, "maxMembers": 5}
//
// url is the address to dial (https: TLS, http: clear text). protocol is the oldest version
// of the protocol a game may speak: a game that knows only older ones asks its player to
// update. maxProtocol is the newest the server speaks: a game that knows newer ones joins
// with this one. maxMembers is how many players a room holds. A server that does not run
// the service answers 404 MULTIPLAY_UNAVAILABLE.
type Multiplay struct {
	URL         string `json:"url"`
	Protocol    int    `json:"protocol"`
	MaxProtocol int    `json:"maxProtocol"`
	MaxMembers  int    `json:"maxMembers"`
	// AtSite is set when URL is the site's own origin: a request that came to another of the
	// site's addresses (web.Origins) is told to dial that one, whose proxy passes the rooms on.
	AtSite bool `json:"-"`
}

// Option is something a site may have besides its sources and players' routes.
type Option func(*options)

type options struct {
	multiplay *Multiplay
}

// WithMultiplay says the server runs Multi-Enso, and where.
func WithMultiplay(m Multiplay) Option {
	return func(o *options) { o.multiplay = &m }
}

func gameMultiplay(m *Multiplay) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Switching the service on or off is seen at once.
		w.Header().Set("Cache-Control", "no-store")
		if m == nil {
			web.Respond(w, 404, map[string]string{"code": "MULTIPLAY_UNAVAILABLE", "message": "Multi-Enso is not available on this server"})
			return
		}
		answer := *m
		if answer.AtSite {
			answer.URL = web.Origin(r.Context(), answer.URL)
		}
		web.Respond(w, 200, answer)
	}
}
