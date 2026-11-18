// Package site is the API of net.ourtaiko.org: one address for both song sources.
//
// Each source's API is under /api/sources/<source>/, for example
// /api/sources/official/charts. What comes after the source is, for now, the API the
// source's old site had; the two are being merged into one. Everything else under /api/
// is about players, whichever source they play: titles, nameplates, all their scores.
// /api/game/sources is what the game loads from every source, in one answer (game.go).
// /api/game/multiplay is where the game finds the online rooms (multiplay.go).
package site

import (
	"net/http"
	"strings"

	"ourtaiko.dev/net/internal/api/mount"
	"ourtaiko.dev/net/internal/web"
)

// Source is one song source and the handler of its API, which expects paths under /api/.
type Source struct {
	ID   string
	Name string
	API  http.Handler
}

// Handler serves the sources in the order given, and players' own routes from players
// (nil: there are none). Options add what a site may have besides.
func Handler(sources []Source, players http.Handler, opts ...Option) http.Handler {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	type listed struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	list := make([]listed, 0, len(sources))
	byID := map[string]http.Handler{}
	for _, s := range sources {
		list = append(list, listed{s.ID, s.Name})
		byID[s.ID] = s.API
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { web.Respond(w, 200, map[string]bool{"ok": true}) })
	mux.HandleFunc("GET /api/sources", func(w http.ResponseWriter, r *http.Request) {
		web.Respond(w, 200, map[string]any{"sources": list})
	})
	mux.HandleFunc("GET /api/game/sources", gameSources(sources))
	mux.HandleFunc("GET /api/game/multiplay", gameMultiplay(o.multiplay))
	mux.HandleFunc("/api/sources/{source}/{rest...}", func(w http.ResponseWriter, r *http.Request) {
		api, ok := byID[r.PathValue("source")]
		if !ok {
			web.Respond(w, 404, map[string]string{"code": "SOURCE_NOT_FOUND", "message": "No such song source"})
			return
		}
		// The site has one sign-in, /api/auth/…, and a session opened there is a session in
		// every source. The sign-in routes each source's old site had are not offered.
		if rest := r.PathValue("rest"); rest == "auth" || strings.HasPrefix(rest, "auth/") {
			web.Respond(w, 404, map[string]string{"code": "NOT_FOUND", "message": "Sign in at " + mount.Link(r, "/api/auth/sso/login")})
			return
		}
		// The source's API sees the path it has always had.
		inner := r.Clone(r.Context())
		inner.URL.Path = "/api/" + r.PathValue("rest")
		inner.URL.RawPath = ""
		inner.RequestURI = inner.URL.RequestURI()
		api.ServeHTTP(w, inner)
	})
	notFound := func(w http.ResponseWriter, r *http.Request) {
		web.Respond(w, 404, map[string]string{"code": "NOT_FOUND", "message": "Not found"})
	}
	if players != nil {
		mux.Handle("/api/", players)
	}
	mux.HandleFunc("/", notFound)
	return mux
}
