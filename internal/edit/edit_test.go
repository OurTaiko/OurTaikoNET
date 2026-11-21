package edit

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"

	"ourtaiko.dev/net/internal/kit/distribution"
)

const page = `<!DOCTYPE html><html lang="en"><head><meta charset="utf-8" /><script type="module" src="./main.js"></script></head><body></body></html>`

// cdn serves a manifest naming build and that build's page; manifestReads counts the manifest's reads.
func cdn(t *testing.T, build string, manifestReads *atomic.Int32) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/edit-nightly.json":
			manifestReads.Add(1)
			io.WriteString(w, `{"path":"/edit/`+build+`/index.html","version":"0.1.0-1","files":{}}`)
		case "/edit/abababababababab/index.html":
			w.Header().Set("Content-Encoding", "br")
			io.WriteString(w, "\x1b\x00")
		case "/edit/" + build + "/index.html":
			io.WriteString(w, page)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func serve(h http.Handler, method, target, remote string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, nil)
	if remote != "" {
		r.RemoteAddr = remote
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestThePageIsTheCurrentBuildsWithABaseOnTheVisitorsRoot(t *testing.T) {
	var reads atomic.Int32
	origin := cdn(t, "0123456789abcdef", &reads)
	router := &distribution.Router{Base: "https://mirror.test/ourtaiko/", Country: func(ip netip.Addr) string {
		if ip.String() == "1.2.3.4" {
			return "CN"
		}
		return "JP"
	}}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "site "+r.URL.Path) })
	h := Handler(Config{Root: origin.URL, Manifest: "edit-nightly.json", Router: router}, next)

	w := serve(h, "GET", "/edit/app/", "5.6.7.8:1000")
	want := `<head><base href="` + origin.URL + `/edit/0123456789abcdef/">`
	if w.Code != 200 || !strings.Contains(w.Body.String(), want) || w.Header().Get("Cache-Control") != "private, no-cache" {
		t.Fatalf("elsewhere: %d %q %q", w.Code, w.Body.String(), w.Header().Get("Cache-Control"))
	}
	w = serve(h, "GET", "/edit/app/", "1.2.3.4:1000")
	if !strings.Contains(w.Body.String(), `<base href="https://mirror.test/ourtaiko/edit/0123456789abcdef/">`) {
		t.Fatalf("mainland: %q", w.Body.String())
	}
	if n := reads.Load(); n != 1 {
		t.Errorf("the manifest was read %d times within its minute; want 1", n)
	}

	for _, c := range []struct {
		target string
		code   int
		where  string
	}{
		{"/edit/app", 301, "/edit/app/"},
		{"/edit/app?embed=1", 301, "/edit/app/?embed=1"},
		{"/edit/app/main.js", 404, ""},
		// The website's own pages.
		{"/edit", 200, ""},
		{"/edit/", 200, ""},
		{"/players", 200, ""},
	} {
		w := serve(h, "GET", c.target, "")
		if w.Code != c.code || w.Header().Get("Location") != c.where {
			t.Errorf("%s: %d %q; want %d %q", c.target, w.Code, w.Header().Get("Location"), c.code, c.where)
		}
	}
	if w := serve(h, "HEAD", "/edit/app/", ""); w.Code != 200 || w.Body.Len() != 0 {
		t.Errorf("HEAD: %d %d bytes", w.Code, w.Body.Len())
	}
	if w := serve(h, "POST", "/edit/app/", ""); w.Code != 405 {
		t.Errorf("POST: %d", w.Code)
	}
}

func TestNoUsableBuildIsAnError(t *testing.T) {
	var reads atomic.Int32
	for _, build := range []string{"not-a-build", "abababababababab"} {
		origin := cdn(t, build, &reads)
		h := Handler(Config{Root: origin.URL, Manifest: "edit-nightly.json"}, http.NotFoundHandler())
		if w := serve(h, "GET", "/edit/app/", ""); w.Code != 503 {
			t.Errorf("%s: %d %q; want 503", build, w.Code, w.Body.String())
		}
	}
}

func TestAFailedReadKeepsTheBuildLastRead(t *testing.T) {
	var reads atomic.Int32
	origin := cdn(t, "0123456789abcdef", &reads)
	s := Handler(Config{Root: origin.URL, Manifest: "edit-nightly.json"}, http.NotFoundHandler()).(*server)
	if w := serve(s, "GET", "/edit/app/", ""); w.Code != 200 {
		t.Fatal(w.Code)
	}
	origin.Close()
	s.readAt = s.readAt.Add(-2 * manifestTTL)
	if w := serve(s, "GET", "/edit/app/", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "0123456789abcdef") {
		t.Errorf("after the CDN went away: %d %q", w.Code, w.Body.String())
	}
}
