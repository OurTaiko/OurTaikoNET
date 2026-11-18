package site

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func echo(name string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, name+" "+r.Method+" "+r.URL.RequestURI())
	})
}

func TestSourcesAreReachedUnderTheirName(t *testing.T) {
	h := Handler([]Source{{"fanmade", "OurTaiko Fanmade", echo("fanmade")}, {"official", "OurTaiko Database", echo("official")}}, echo("players"))
	for _, c := range []struct {
		method, path string
		status       int
		want         string
	}{
		{"GET", "/api/sources/fanmade/game/catalog", 200, "fanmade GET /api/game/catalog"},
		{"GET", "/api/sources/official/charts/abc?by=number", 200, "official GET /api/charts/abc?by=number"},
		{"POST", "/api/sources/official/game/scores", 200, "official POST /api/game/scores"},
		{"GET", "/api/sources/nowhere/charts", 404, "SOURCE_NOT_FOUND"},
		{"GET", "/api/sources/fanmade/auth/sso/login", 404, "/api/auth/sso/login"},
		{"POST", "/api/sources/official/auth/logout", 404, "/api/auth/sso/login"},
		{"GET", "/api/sources/official/authors", 200, "official GET /api/authors"},
		{"GET", "/api/auth/sso/login", 200, "players GET /api/auth/sso/login"},
		{"GET", "/api/users/abc/nameplate", 200, "players GET /api/users/abc/nameplate"},
		{"POST", "/internal/v1/awards", 404, "NOT_FOUND"},
		{"GET", "/elsewhere", 404, "NOT_FOUND"},
		{"GET", "/api/sources", 200, `{"sources":[{"id":"fanmade","name":"OurTaiko Fanmade"},{"id":"official","name":"OurTaiko Database"}]}`},
		{"GET", "/healthz", 200, `{"ok":true}`},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(c.method, c.path, nil))
		if w.Code != c.status || !strings.Contains(w.Body.String(), c.want) {
			t.Errorf("%s %s: %d %q, want %d containing %q", c.method, c.path, w.Code, w.Body.String(), c.status, c.want)
		}
	}
}

func TestWithoutPlayersOnlySourcesAnswer(t *testing.T) {
	h := Handler([]Source{{"fanmade", "OurTaiko Fanmade", echo("fanmade")}}, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/titles", nil))
	if w.Code != 404 {
		t.Fatal(w.Code, w.Body.String())
	}
}
