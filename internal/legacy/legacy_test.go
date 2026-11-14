package legacy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func named(name string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, name+" "+r.URL.Path) })
}

func TestOldHostsReachTheirSource(t *testing.T) {
	h := Handler(Hosts{"fanmade.ourtaiko.org": {API: named("fanmade")}, "database.ourtaiko.org": {API: named("official")}}, named("current"))
	for _, c := range []struct{ host, want string }{
		{"fanmade.ourtaiko.org", "fanmade /api/v1/game/catalog"},
		{"FANMADE.ourtaiko.org:443", "fanmade /api/v1/game/catalog"},
		{"database.ourtaiko.org.", "official /api/v1/game/catalog"},
		{"net.ourtaiko.org", "current /api/v1/game/catalog"},
		{"evil.example", "current /api/v1/game/catalog"},
		{"", "current /api/v1/game/catalog"},
	} {
		r := httptest.NewRequest("GET", "/api/v1/game/catalog", nil)
		r.Host = c.host
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if got := w.Body.String(); got != c.want {
			t.Errorf("host %q: got %q, want %q", c.host, got, c.want)
		}
	}
}

func TestNoOldHostsMeansNoDetour(t *testing.T) {
	r := httptest.NewRequest("GET", "/x", nil)
	r.Host = "fanmade.ourtaiko.org"
	w := httptest.NewRecorder()
	Handler(nil, named("current")).ServeHTTP(w, r)
	if got := w.Body.String(); got != "current /x" {
		t.Fatal(got)
	}
}

func TestInternalRoutesAreNotReachableFromOutside(t *testing.T) {
	h := PublicOnly(named("profile"))
	for path, want := range map[string]int{"/internal/v1/awards": 404, "/api/v1/users/x/nameplate": 200} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("POST", path, nil))
		if w.Code != want {
			t.Errorf("%s: %d, want %d", path, w.Code, want)
		}
	}
}

func TestOldPagesGoToTheNewSite(t *testing.T) {
	const origin = "https://net.ourtaiko.org"
	h := Handler(Hosts{
		"fanmade.ourtaiko.org":  {API: named("fanmade"), Page: Under(origin, "/fanmade")},
		"database.ourtaiko.org": {API: named("official"), Page: Under(origin, "/official")},
		"profile.ourtaiko.org":  {API: named("profile"), Page: Players(origin)},
	}, named("current"))
	for _, c := range []struct{ host, target, want string }{
		{"fanmade.ourtaiko.org", "/", origin + "/fanmade/"},
		{"fanmade.ourtaiko.org", "/charts/251b6cb754e010f1680f3c3622fea1d5", origin + "/fanmade/charts/251b6cb754e010f1680f3c3622fea1d5"},
		{"fanmade.ourtaiko.org", "/charts/abc/comments/def", origin + "/fanmade/charts/abc/comments/def"},
		{"fanmade.ourtaiko.org", "/?q=%E5%A4%AA%E9%BC%93&order=level", origin + "/fanmade/?q=%E5%A4%AA%E9%BC%93&order=level"},
		{"database.ourtaiko.org", "/charts/1540?by=nijiiro", origin + "/official/charts/1540?by=nijiiro"},
		{"database.ourtaiko.org", "/dans", origin + "/official/dans"},
		{"profile.ourtaiko.org", "/users/de000000000000000000000000000001?tab=titles", origin + "/players/de000000000000000000000000000001?tab=titles"},
		{"profile.ourtaiko.org", "/titles", origin + "/titles"},
		{"profile.ourtaiko.org", "/", origin + "/"},
		// A path that tries to name another site stays on this one.
		{"fanmade.ourtaiko.org", "//evil.example/x", origin + "/fanmade//evil.example/x"},
	} {
		r := httptest.NewRequest("GET", c.target, nil)
		r.Host = c.host
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusMovedPermanently || w.Header().Get("Location") != c.want {
			t.Errorf("%s%s: %d to %q, want 301 to %q", c.host, c.target, w.Code, w.Header().Get("Location"), c.want)
		}
	}
}

func TestOldAPIsAreAnsweredNotMoved(t *testing.T) {
	h := Handler(Hosts{"fanmade.ourtaiko.org": {API: named("fanmade"), Page: Under("https://net.ourtaiko.org", "/fanmade")}}, named("current"))
	for _, c := range []struct{ method, path string }{
		{"GET", "/api/v1/game/catalog"},
		{"POST", "/api/v1/game/scores"},
		{"GET", "/healthz"},
		{"GET", "/readyz"},
		{"POST", "/internal/v1/awards"},
		// Only reading a page moves; anything else is the API's to refuse.
		{"POST", "/charts/abc"},
	} {
		r := httptest.NewRequest(c.method, c.path, nil)
		r.Host = "fanmade.ourtaiko.org"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if got := w.Body.String(); w.Code != 200 || got != "fanmade "+c.path {
			t.Errorf("%s %s: %d %q, want the old API's answer", c.method, c.path, w.Code, got)
		}
	}
}
