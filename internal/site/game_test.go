package site

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// source answers the game's four routes as a song source does: the public ones to anybody,
// the player's own to the session "good" only. It notes what it was asked.
type source struct {
	name  string
	mu    sync.Mutex
	asked []string
	fail  string
}

func (s *source) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.asked = append(s.asked, r.Method+" "+r.URL.RequestURI()+" "+r.Header.Get("Accept-Encoding"))
	fail := s.fail
	s.mu.Unlock()
	personal := r.URL.Path == "/api/v1/game/scores" || r.URL.Path == "/api/v1/game/playlists"
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	switch {
	case r.URL.Path == fail:
		w.WriteHeader(503)
		io.WriteString(w, `{"code":"SERVICE_UNAVAILABLE"}`)
	case personal && r.Header.Get("Authorization") != "Bearer good":
		w.WriteHeader(401)
		io.WriteString(w, `{"code":"UNAUTHORIZED"}`)
	default:
		io.WriteString(w, `{"from":"`+s.name+`","route":"`+strings.TrimPrefix(r.URL.Path, "/api/v1/game/")+`"}`)
	}
}

func TestTheGameLoadsEverySourceInOneAnswer(t *testing.T) {
	fanmade, official := &source{name: "fanmade"}, &source{name: "official"}
	h := Handler([]Source{{"fanmade", "OurTaiko Fanmade", fanmade}, {"official", "OurTaiko Database", official}}, nil)
	type part struct{ From, Route string }
	type listed struct {
		ID, Name                         string
		Catalog, Dans, Scores, Playlists *part
	}
	ask := func(path, session string) (int, []listed, string) {
		t.Helper()
		r := httptest.NewRequest("GET", path, nil)
		if session != "" {
			r.Header.Set("Authorization", "Bearer "+session)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		var body struct{ Sources []listed }
		json.Unmarshal(w.Body.Bytes(), &body)
		return w.Code, body.Sources, w.Body.String()
	}

	// Nobody signed in: the catalogue and the dan courses of both, in the sources' order.
	code, sources, raw := ask("/api/v1/game/sources", "")
	if code != 200 || len(sources) != 2 || sources[0].ID != "fanmade" || sources[1].Name != "OurTaiko Database" {
		t.Fatal(code, raw)
	}
	for i, name := range []string{"fanmade", "official"} {
		s := sources[i]
		if s.Catalog == nil || *s.Catalog != (part{name, "catalog"}) || s.Dans == nil || *s.Dans != (part{name, "dans"}) || s.Scores != nil || s.Playlists != nil {
			t.Fatal(name, raw)
		}
	}
	// Each part was asked of each source once, as the source's own route.
	if len(fanmade.asked) != 2 || len(official.asked) != 2 {
		t.Fatal(fanmade.asked, official.asked)
	}

	// Signed in: the player's scores and playlists too.
	code, sources, raw = ask("/api/v1/game/sources", "good")
	if code != 200 || sources[1].Scores == nil || *sources[1].Scores != (part{"official", "scores"}) || sources[0].Playlists == nil || sources[0].Catalog == nil {
		t.Fatal(code, raw)
	}
	// Only what is named.
	code, sources, raw = ask("/api/v1/game/sources?parts=scores,playlists", "good")
	if code != 200 || sources[0].Catalog != nil || sources[0].Dans != nil || sources[0].Scores == nil || sources[1].Playlists == nil {
		t.Fatal(code, raw)
	}
	if code, sources, raw = ask("/api/v1/game/sources?parts=scores", "good"); code != 200 || sources[0].Playlists != nil || sources[0].Scores == nil {
		t.Fatal(code, raw)
	}
	// A session the sources refuse is refused here, as they refuse it.
	if code, _, raw = ask("/api/v1/game/sources", "expired"); code != 401 || !strings.Contains(raw, "UNAUTHORIZED") {
		t.Fatal(code, raw)
	}
	if code, _, raw = ask("/api/v1/game/sources?parts=scores", ""); code != 401 {
		t.Fatal(code, raw)
	}
	// A part that is none.
	for _, path := range []string{"/api/v1/game/sources?parts=everything", "/api/v1/game/sources?parts=catalog,charts"} {
		if code, _, raw = ask(path, ""); code != 400 || !strings.Contains(raw, "PARTS_INVALID") {
			t.Fatal(path, code, raw)
		}
	}
	// One part failing in one source: no half answer.
	official.fail = "/api/v1/game/dans"
	if code, _, raw = ask("/api/v1/game/sources", ""); code != 503 || strings.Contains(raw, "fanmade") {
		t.Fatal(code, raw)
	}
	official.fail = ""

	// The answer is compressed once, for a client that takes it so; the sources are not asked to.
	fanmade.asked = nil
	r := httptest.NewRequest("GET", "/api/v1/game/sources", nil)
	r.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Header().Get("Content-Encoding") != "gzip" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Header())
	}
	z, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatal(err)
	}
	plain, _ := io.ReadAll(z)
	if !strings.Contains(string(plain), `"catalog":{"from":"fanmade","route":"catalog"}`) {
		t.Fatal(string(plain))
	}
	for _, asked := range fanmade.asked {
		if strings.Contains(asked, "gzip") {
			t.Fatal("a source was asked to compress its part:", asked)
		}
	}
}
