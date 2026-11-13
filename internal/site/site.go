// Package site is the API of net.ourtaiko.org: one address for both song sources.
//
// Each source's API is under /api/v1/sources/<source>/, for example
// /api/v1/sources/official/charts. What comes after the source is, for now, the API the
// source's old site had; the two are being merged into one. Everything else under /api/v1/
// is about players, whichever source they play: titles, nameplates, all their scores.
package site

import (
	"encoding/json"
	"net/http"
	"strings"
)

// Source is one song source and the handler of its API, which expects paths under /api/v1/.
type Source struct {
	ID   string
	Name string
	API  http.Handler
}

// Handler serves the sources in the order given, and players' own routes from players
// (nil: there are none).
func Handler(sources []Source, players http.Handler) http.Handler {
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
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, map[string]bool{"ok": true}) })
	mux.HandleFunc("GET /api/v1/sources", func(w http.ResponseWriter, r *http.Request) {
		respond(w, 200, map[string]any{"sources": list})
	})
	mux.HandleFunc("/api/v1/sources/{source}/{rest...}", func(w http.ResponseWriter, r *http.Request) {
		api, ok := byID[r.PathValue("source")]
		if !ok {
			respond(w, 404, map[string]string{"code": "SOURCE_NOT_FOUND", "message": "No such song source"})
			return
		}
		// The site has one sign-in, /api/v1/auth/…, and a session opened there is a session in
		// every source. The sign-in routes each source's old site had are not offered.
		if rest := r.PathValue("rest"); rest == "auth" || strings.HasPrefix(rest, "auth/") {
			respond(w, 404, map[string]string{"code": "NOT_FOUND", "message": "Sign in at /api/v1/auth/sso/login"})
			return
		}
		// The source's API sees the path it has always had.
		inner := r.Clone(r.Context())
		inner.URL.Path = "/api/v1/" + r.PathValue("rest")
		inner.URL.RawPath = ""
		inner.RequestURI = inner.URL.RequestURI()
		api.ServeHTTP(w, inner)
	})
	notFound := func(w http.ResponseWriter, r *http.Request) {
		respond(w, 404, map[string]string{"code": "NOT_FOUND", "message": "Not found"})
	}
	if players != nil {
		mux.Handle("/api/v1/", players)
	}
	mux.HandleFunc("/", notFound)
	return mux
}

func respond(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}
