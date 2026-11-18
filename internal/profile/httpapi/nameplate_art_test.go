package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// The catalog is answered with each file's path under the distribution root, the same for every
// visitor.
func TestNameplateArtPaths(t *testing.T) {
	shadow := donFileRef{"shadow.png", 3, strings.Repeat("a", 64)}
	site := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(nameplateCatalog{Version: 1, Files: map[string]donFileRef{"shadow": shadow}})
	}))
	defer site.Close()
	server := New(nil, Config{NameplateCatalogURL: site.URL + "/nameplate/catalog.json", DonClient: site.Client()})
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/nameplate-art/catalog", nil))
	var answer struct {
		Version int                   `json:"version"`
		Files   map[string]donFileRef `json:"files"`
		BaseURL *string               `json:"baseUrl"`
	}
	if w.Code != 200 || w.Header().Get("Cache-Control") != "public, max-age=60" || json.Unmarshal(w.Body.Bytes(), &answer) != nil ||
		answer.Version != 1 || answer.BaseURL != nil || answer.Files["shadow"] != (donFileRef{"nameplate/shadow.png", 3, shadow.SHA256}) {
		t.Fatalf("answer %d %s", w.Code, w.Body)
	}
}

func TestNameplateArtFailureThrottle(t *testing.T) {
	var hits atomic.Int32
	site := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1); w.WriteHeader(503) }))
	defer site.Close()
	server := New(nil, Config{NameplateCatalogURL: site.URL + "/catalog.json", DonClient: site.Client()})
	for range 2 {
		w := httptest.NewRecorder()
		server.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/nameplate-art/catalog", nil))
		if w.Code != 503 {
			t.Fatalf("status = %d", w.Code)
		}
	}
	if hits.Load() != 1 {
		t.Fatalf("unavailable catalog fetched %d times", hits.Load())
	}
}

// A visitor who leaves while the first read runs does not turn it into a failure for others.
func TestNameplateArtRefreshOutlivesRequest(t *testing.T) {
	release := make(chan struct{})
	site := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		json.NewEncoder(w).Encode(nameplateCatalog{Version: 1, Files: map[string]donFileRef{"shadow": {"shadow.png", 3, strings.Repeat("a", 64)}}})
	}))
	defer site.Close()
	server := New(nil, Config{NameplateCatalogURL: site.URL + "/catalog.json", DonClient: site.Client()})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/nameplate-art/catalog", nil).WithContext(ctx))
	if w.Code != 503 {
		t.Fatalf("status = %d", w.Code)
	}
	close(release)
	waitNameplateRefresh(t, server)
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/nameplate-art/catalog", nil))
	if w.Code != 200 {
		t.Fatalf("status after refresh = %d", w.Code)
	}
}

func waitNameplateRefresh(t *testing.T, server *Server) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		server.nameplateMu.Lock()
		busy := server.nameplateArt.done != nil
		server.nameplateMu.Unlock()
		if !busy {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("nameplate refresh did not finish")
		}
		time.Sleep(time.Millisecond)
	}
}
