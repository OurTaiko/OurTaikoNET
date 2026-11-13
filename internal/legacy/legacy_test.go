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
	h := Handler(Hosts{"fanmade.ourtaiko.org": named("fanmade"), "database.ourtaiko.org": named("official")}, named("current"))
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
