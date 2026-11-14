package site

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestTheWebsiteIsServedInFrontOfTheAPI(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "index.html", "<!doctype html>page")
	write(t, dir, "assets/app-abc123.js", "console.log(1)")
	write(t, dir, "favicon.ico", "icon")
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "api "+r.Method+" "+r.URL.Path) })
	h := Web(dir, api)
	for _, c := range []struct {
		method, path string
		code         int
		body, cache  string
	}{
		{"GET", "/", 200, "<!doctype html>page", "no-cache"},
		// The page draws these addresses itself.
		{"GET", "/fanmade/charts/abc", 200, "<!doctype html>page", "no-cache"},
		{"GET", "/players/de000000000000000000000000000001", 200, "<!doctype html>page", "no-cache"},
		{"GET", "/assets/app-abc123.js", 200, "console.log(1)", "public, max-age=31536000, immutable"},
		{"GET", "/favicon.ico", 200, "icon", "public, max-age=3600"},
		// Missing files are missing, not the page.
		{"GET", "/assets/gone-000000.js", 404, "", ""},
		{"GET", "/robots.txt", 404, "", ""},
		// The API, its health checks and every write stay the API's.
		{"GET", "/api/v1/sources", 200, "api GET /api/v1/sources", ""},
		{"GET", "/healthz", 200, "api GET /healthz", ""},
		{"GET", "/readyz", 200, "api GET /readyz", ""},
		{"POST", "/fanmade/charts/abc", 200, "api POST /fanmade/charts/abc", ""},
		// Nothing outside the directory is reachable.
		{"GET", "/../web.go", 404, "", ""},
		{"GET", "/assets/../../web.go", 404, "", ""},
	} {
		r := httptest.NewRequest(c.method, "http://net.test/", nil)
		r.URL.Path = c.path
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != c.code || (c.body != "" && w.Body.String() != c.body) || (c.cache != "" && w.Header().Get("Cache-Control") != c.cache) {
			t.Errorf("%s %s: %d %q cache %q; want %d %q cache %q", c.method, c.path, w.Code, w.Body.String(), w.Header().Get("Cache-Control"), c.code, c.body, c.cache)
		}
	}
}

func TestANewReleaseIsServedWithoutARestart(t *testing.T) {
	root := t.TempDir()
	write(t, root, "releases/one/index.html", "one")
	write(t, root, "releases/two/index.html", "two")
	current := filepath.Join(root, "current")
	if err := os.Symlink(filepath.Join("releases", "one"), current); err != nil {
		t.Skip("no symbolic links here:", err)
	}
	h := Web(current, http.NotFoundHandler())
	get := func() string {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
		return w.Body.String()
	}
	if got := get(); got != "one" {
		t.Fatal(got)
	}
	// As a release does it: a new link, moved over the old one.
	next := filepath.Join(root, "current.new")
	if err := os.Symlink(filepath.Join("releases", "two"), next); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(next, current); err != nil {
		t.Fatal(err)
	}
	if got := get(); got != "two" {
		t.Fatal(got)
	}
}

func TestWithoutAWebsiteThereIsOnlyTheAPI(t *testing.T) {
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "api") })
	w := httptest.NewRecorder()
	Web("", api).ServeHTTP(w, httptest.NewRequest("GET", "/fanmade/", nil))
	if w.Body.String() != "api" {
		t.Fatal(w.Body.String())
	}
	// A directory that has no website yet says so instead of pretending a page is missing.
	empty := t.TempDir()
	for _, address := range []string{"/fanmade/", "/", "/index.html"} {
		w = httptest.NewRecorder()
		Web(empty, api).ServeHTTP(w, httptest.NewRequest("GET", address, nil))
		if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "not installed") {
			t.Fatal(address, w.Code, w.Body.String())
		}
	}
}

// The image the server runs in has no table of file types, so the ones Go does not know by
// itself are named here; with nosniff a browser takes the type as given.
func TestTheManifestIsServedAsAManifest(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "index.html", "<!doctype html>page")
	write(t, dir, "site.webmanifest", `{"name":"OurTaikoNET"}`)
	w := httptest.NewRecorder()
	Web(dir, http.NotFoundHandler()).ServeHTTP(w, httptest.NewRequest("GET", "/site.webmanifest", nil))
	if got := w.Header().Get("Content-Type"); w.Code != 200 || got != "application/manifest+json" {
		t.Fatal(w.Code, got)
	}
}
