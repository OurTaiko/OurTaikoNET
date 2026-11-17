package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNameplateArtRouting(t *testing.T) {
	shadow := donFileRef{"shadow.png", 3, strings.Repeat("a", 64)}
	outline := donFileRef{"outline.png", 4, strings.Repeat("b", 64)}
	var mirrorHits atomic.Int32
	site := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/canonical/catalog.json" {
			json.NewEncoder(w).Encode(nameplateCatalog{Version: 1, Files: map[string]donFileRef{"shadow": shadow, "outline": outline}})
			return
		}
		if r.URL.Path == "/ourtaiko/nameplate/catalog.json" {
			mirrorHits.Add(1)
			w.WriteHeader(500)
			return
		}
		http.NotFound(w, r)
	}))
	defer site.Close()
	server := New(nil, Config{NameplateCatalogURL: site.URL + "/canonical/catalog.json", DonMirrorCatalogURL: site.URL + "/ourtaiko/3dchara/catalog.json", DonClient: site.Client(), DonIPCountry: func(ip netip.Addr) string {
		if ip.String() == "203.0.113.1" {
			return "CN"
		}
		return "US"
	}})
	answer := func(ip, query string) map[string]any {
		r := httptest.NewRequest("GET", "/api/v1/nameplate-art/catalog"+query, nil)
		r.RemoteAddr = ip + ":123"
		w := httptest.NewRecorder()
		server.Handler().ServeHTTP(w, r)
		if w.Code != 200 || w.Header().Get("Cache-Control") != "private, max-age=60" {
			t.Fatalf("response %d %s", w.Code, w.Body)
		}
		var result map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	if value := answer("203.0.113.2", ""); value["mirrorBaseUrl"] != nil || mirrorHits.Load() != 0 {
		t.Fatal("overseas fetched mirror")
	}
	// Every canonical file is on the mirror for mainland visitors; the mirror is never read.
	value := answer("203.0.113.1", "")
	paths, _ := value["mirrorPaths"].([]any)
	if len(paths) != 2 || paths[0] != "outline.png" || paths[1] != "shadow.png" || value["mirrorBaseUrl"] != site.URL+"/ourtaiko/nameplate/" {
		t.Fatalf("answer = %v", value)
	}
	if answer("203.0.113.1", "?origin=1")["mirrorBaseUrl"] != nil {
		t.Fatal("origin bypass ignored")
	}
	if mirrorHits.Load() != 0 {
		t.Fatal("the mirror catalog was fetched")
	}
}

func TestNameplateArtFailureThrottle(t *testing.T) {
	var hits atomic.Int32
	site := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1); w.WriteHeader(503) }))
	defer site.Close()
	server := New(nil, Config{NameplateCatalogURL: site.URL + "/catalog.json", DonClient: site.Client()})
	for range 2 {
		w := httptest.NewRecorder()
		server.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/nameplate-art/catalog", nil))
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
	server.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/nameplate-art/catalog", nil).WithContext(ctx))
	if w.Code != 503 {
		t.Fatalf("status = %d", w.Code)
	}
	close(release)
	waitNameplateRefresh(t, server)
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/nameplate-art/catalog", nil))
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
