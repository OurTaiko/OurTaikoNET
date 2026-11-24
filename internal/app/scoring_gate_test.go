package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// While Start reads the charts' scoring the site answers 503 with Retry-After, its health checks
// excepted, so a release goes ahead; it serves again once the read is done.
func TestNothingButHealthChecksWhileScoringIsRead(t *testing.T) {
	var a App
	h := a.afterScoring(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		return w
	}
	a.scoring.Store(true)
	for _, path := range []string{"/api/v1/game/scores", "/api/v2/sources/fanmade/charts", "/", "/internal/charts/fanmade/x"} {
		if w := get(path); w.Code != http.StatusServiceUnavailable || w.Header().Get("Retry-After") == "" {
			t.Fatalf("%s while reading: %d %q", path, w.Code, w.Header().Get("Retry-After"))
		}
	}
	for _, path := range []string{"/healthz", "/readyz"} {
		if w := get(path); w.Code != http.StatusNoContent {
			t.Fatalf("%s while reading: %d", path, w.Code)
		}
	}
	a.scoring.Store(false)
	if w := get("/api/v1/game/scores"); w.Code != http.StatusNoContent {
		t.Fatalf("after reading: %d", w.Code)
	}
}
