package site

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTheGameIsToldWhereMultiEnsoIs(t *testing.T) {
	sources := []Source{{"fanmade", "OurTaiko Fanmade", echo("fanmade")}}
	on := Handler(sources, echo("players"), WithMultiplay(Multiplay{URL: "https://net.example", Protocol: 1, MaxProtocol: 2, MaxMembers: 5}))
	w := httptest.NewRecorder()
	on.ServeHTTP(w, httptest.NewRequest("GET", "/api/game/multiplay", nil))
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"url":"https://net.example","protocol":1,"maxProtocol":2,"maxMembers":5}` {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("content type %q", got)
	}
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("cache control %q", got)
	}

	// A server without the service says so itself: the request is not passed on to the
	// players' routes, with or without them.
	for name, players := range map[string]bool{"with players' routes": true, "without": false} {
		off := Handler(sources, nil)
		if players {
			off = Handler(sources, echo("players"))
		}
		w = httptest.NewRecorder()
		off.ServeHTTP(w, httptest.NewRequest("GET", "/api/game/multiplay", nil))
		if w.Code != 404 || !strings.Contains(w.Body.String(), `"code":"MULTIPLAY_UNAVAILABLE"`) || !strings.Contains(w.Body.String(), `"message":"`) {
			t.Fatalf("%s: %d %s", name, w.Code, w.Body.String())
		}
	}

	// The rest of the site is as it is without the option.
	for path, want := range map[string]string{
		"/api/sources/fanmade/game/catalog": "fanmade GET /api/game/catalog",
		"/api/titles":                       "players GET /api/titles",
		"/api/game/multiplay/rooms":         "players GET /api/game/multiplay/rooms",
	} {
		w = httptest.NewRecorder()
		on.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 || w.Body.String() != want {
			t.Errorf("%s: %d %q", path, w.Code, w.Body.String())
		}
	}
	// What the released game loads is answered as before.
	loads := Handler([]Source{{"fanmade", "OurTaiko Fanmade", &source{name: "fanmade"}}}, nil, WithMultiplay(Multiplay{URL: "https://net.example", Protocol: 1, MaxProtocol: 2, MaxMembers: 5}))
	w = httptest.NewRecorder()
	loads.ServeHTTP(w, httptest.NewRequest("GET", "/api/game/sources?parts=catalog", nil))
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"sources":[{"id":"fanmade","name":"OurTaiko Fanmade","catalog":{"from":"fanmade","route":"catalog"}}]}` {
		t.Errorf("game/sources: %d %q", w.Code, w.Body.String())
	}
}
