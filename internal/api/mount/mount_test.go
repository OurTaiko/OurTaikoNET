package mount

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMountAndServe(t *testing.T) {
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Path", r.URL.Path)
		w.Header().Set("Version", Version(r))
		w.Header().Set("Link", Link(r, "/api/charts/1/audio"))
	})
	h := Serve(map[string]http.Handler{"v1": Mount("v1", api), "v2": Mount("v2", api)}, api)
	for _, tc := range []struct{ path, inner, version, link string }{
		{"/api/v1/charts/1", "/api/charts/1", "v1", "/api/v1/charts/1/audio"},
		{"/api/v2/charts/1?x=1", "/api/charts/1", "v2", "/api/v2/charts/1/audio"},
		{"/api/v2", "/api", "v2", "/api/v2/charts/1/audio"},
		// Health checks and /internal/ are not versioned.
		{"/healthz", "/healthz", "", "/api/charts/1/audio"},
		{"/internal/v1/awards", "/internal/v1/awards", "", "/api/charts/1/audio"},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
		if w.Code != 200 || w.Header().Get("Path") != tc.inner || w.Header().Get("Version") != tc.version || w.Header().Get("Link") != tc.link {
			t.Fatalf("%s: %d %v", tc.path, w.Code, w.Header())
		}
	}
	// A path without a version, or with one there is not, reaches no API.
	for _, path := range []string{"/api/charts/1", "/api/v3/charts/1", "/api/v1x/charts"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 404 || w.Header().Get("Path") != "" {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
	// Mount alone refuses what is not under its prefix.
	w := httptest.NewRecorder()
	Mount("v1", api).ServeHTTP(w, httptest.NewRequest("GET", "/api/v10/charts", nil))
	if w.Code != 404 {
		t.Fatal(w.Code)
	}
}
