package site

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"ourtaiko.dev/net/internal/web"
)

// The game used to load each source by itself: its catalogue, its dan courses and, signed
// in, the player's scores and playlists there, four requests a source and a sign-in for each.
// GET /api/v1/game/sources answers all of it for every source at once:
//
//	{"sources": [{"id": "fanmade", "name": "OurTaiko Fanmade",
//	              "catalog": …, "dans": …, "scores": …, "playlists": …}, …]}
//
// Each part is exactly what the source's own route answers (/game/catalog, /game/dans,
// /game/scores, /game/playlists under /api/v1/sources/<id>/), so a client reads it as it
// reads those. scores and playlists are the signed-in player's and need the game session in
// Authorization; one session is good for every source.
//
// ?parts=catalog,dans,scores,playlists names the parts wanted. Without it: the catalogue and
// dan courses, and with a session the player's scores and playlists as well.
//
// An answer is whole or not given: when a part cannot be answered, the route answers what
// that part did (401 for a session the account center refuses, for example).

// gameParts are the parts in the order they are asked for, each with its route in a source
// and whether it is a player's own.
var gameParts = []struct {
	name, path string
	personal   bool
}{
	{"catalog", "/api/v1/game/catalog", false},
	{"dans", "/api/v1/game/dans", false},
	{"scores", "/api/v1/game/scores", true},
	{"playlists", "/api/v1/game/playlists", true},
}

// answer collects what a source's handler writes.
type answer struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (a *answer) Header() http.Header { return a.header }
func (a *answer) WriteHeader(status int) {
	if a.status == 0 {
		a.status = status
	}
}
func (a *answer) Write(p []byte) (int, error) {
	a.WriteHeader(http.StatusOK)
	return a.body.Write(p)
}

func gameSources(sources []Source) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		signedIn := r.Header.Get("Authorization") != ""
		wanted := map[string]bool{}
		if raw := r.URL.Query().Get("parts"); raw != "" {
			for _, name := range strings.Split(raw, ",") {
				wanted[name] = true
			}
		}
		named := len(wanted) != 0
		var parts []int
		for i, part := range gameParts {
			if !named && (!part.personal || signedIn) || wanted[part.name] {
				parts = append(parts, i)
				delete(wanted, part.name)
			}
		}
		if len(wanted) != 0 || len(parts) == 0 {
			web.Respond(w, 400, map[string]string{"code": "PARTS_INVALID", "message": "parts is a list of catalog, dans, scores and playlists"})
			return
		}
		// Every part of every source is asked for at once, as the source's own route.
		answers := make([][]*answer, len(sources))
		var wait sync.WaitGroup
		for s, source := range sources {
			answers[s] = make([]*answer, len(parts))
			for p, part := range parts {
				a := &answer{header: http.Header{}}
				answers[s][p] = a
				inner := r.Clone(r.Context())
				inner.URL.Path, inner.URL.RawPath, inner.URL.RawQuery = gameParts[part].path, "", ""
				inner.RequestURI = inner.URL.RequestURI()
				// The whole answer is compressed once, here.
				inner.Header.Del("Accept-Encoding")
				wait.Add(1)
				go func() {
					defer wait.Done()
					source.API.ServeHTTP(a, inner)
				}()
			}
		}
		wait.Wait()
		type listed struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			// The parts, by name; a part not asked for is absent.
			Catalog   json.RawMessage `json:"catalog,omitempty"`
			Dans      json.RawMessage `json:"dans,omitempty"`
			Scores    json.RawMessage `json:"scores,omitempty"`
			Playlists json.RawMessage `json:"playlists,omitempty"`
		}
		list := make([]listed, len(sources))
		for s, source := range sources {
			list[s] = listed{ID: source.ID, Name: source.Name}
			for p, part := range parts {
				a := answers[s][p]
				if a.status != http.StatusOK || !json.Valid(a.body.Bytes()) {
					status := a.status
					if status == 0 || status == http.StatusOK {
						status = http.StatusBadGateway
					}
					w.Header().Set("Content-Type", "application/json; charset=utf-8")
					w.WriteHeader(status)
					w.Write(a.body.Bytes())
					return
				}
				raw := json.RawMessage(a.body.Bytes())
				switch gameParts[part].name {
				case "catalog":
					list[s].Catalog = raw
				case "dans":
					list[s].Dans = raw
				case "scores":
					list[s].Scores = raw
				case "playlists":
					list[s].Playlists = raw
				}
			}
		}
		out, finish := web.GzipJSON(w, r)
		defer finish()
		// A player's own scores are in it when signed in: never kept by anything in between.
		out.Header().Set("Cache-Control", "no-store")
		web.Respond(out, 200, map[string]any{"sources": list})
	}
}
