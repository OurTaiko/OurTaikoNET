package distribution

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func testRouter() *Router {
	return FromDonCatalog("https://mirror.example/ourtaiko/3dchara/catalog.json", func(ip netip.Addr) string {
		return map[string]string{"203.0.113.1": "CN", "203.0.113.2": "US", "203.0.113.3": "HK", "203.0.113.4": "MO", "203.0.113.5": "TW", "10.0.0.2": "CN"}[ip.String()]
	}, []netip.Prefix{netip.MustParsePrefix("172.20.0.1/32")})
}
func mainland() *http.Request {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "172.20.0.1:123"
	r.Header.Set("X-Real-IP", "203.0.113.1")
	return r
}

func TestRegionalCandidates(t *testing.T) {
	router := testRouter()
	digest := strings.Repeat("a", 64)
	if router.Base != "https://mirror.example/ourtaiko/" {
		t.Fatal(router.Base)
	}
	for _, ip := range []string{"203.0.113.1", "203.0.113.2", "203.0.113.3", "203.0.113.4", "203.0.113.5", "203.0.113.6", "10.0.0.2"} {
		r := mainland()
		r.Header.Set("X-Real-IP", ip)
		if router.IsMainland(r) != (ip == "203.0.113.1") {
			t.Fatalf("country policy for %s", ip)
		}
	}
	spoof := mainland()
	spoof.RemoteAddr = "203.0.113.2:123"
	if router.IsMainland(spoof) {
		t.Fatal("untrusted header accepted")
	}
	for _, collection := range []string{"fanmade", "database"} {
		for _, ext := range []string{"tja", "ogg", "mp3", "webp", "png", "jpg", "jpeg", "mp4"} {
			original := CanonicalBase + collection + "/" + digest + "." + ext
			if got := router.Candidate(mainland(), original, digest); got != router.Base+collection+"/"+digest+"."+ext {
				t.Fatal(got)
			}
		}
	}
	for _, address := range []string{
		CanonicalBase + "fanmade/" + digest + ".ogg?X-Amz-Signature=secret",
		CanonicalBase + "fanmade/" + digest + ".ogg#fragment",
		CanonicalBase + "fanmade/../database/" + digest + ".ogg",
		CanonicalBase + "fanmade/%2e%2e/database/" + digest + ".ogg",
		CanonicalBase + "fanmade/catalog.json", CanonicalBase + "other/" + digest + ".ogg",
		"https://cdn.example/fanmade/" + digest + ".ogg", "http://distrib.ourtaiko.org/fanmade/" + digest + ".ogg",
		CanonicalBase + "fanmade/" + digest + ".exe", CanonicalBase + "fanmade/" + strings.Repeat("b", 64) + ".ogg",
	} {
		if got := router.Candidate(mainland(), address, digest); got != address {
			t.Fatalf("unsafe mapping: %s", got)
		}
	}
	custom := FromDonCatalog("https://mirror.example/custom.json", router.Country, nil)
	if custom.Base != "" {
		t.Fatal("guessed siblings of custom Don catalog")
	}
}

func TestMirrorReadiness(t *testing.T) {
	var hits atomic.Int32
	var status atomic.Int32
	status.Store(200)
	site := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Method != "HEAD" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("unsafe probe")
		}
		w.Header().Set("Content-Length", "3")
		w.WriteHeader(int(status.Load()))
	}))
	defer site.Close()
	router := testRouter()
	router.Base = site.URL + "/ourtaiko/"
	router.Client = site.Client()
	original := CanonicalBase + "fanmade/" + strings.Repeat("a", 64) + ".ogg"
	r := mainland()
	r.Header.Set("Authorization", "Bearer api-secret")
	r.Header.Set("Cookie", "private=secret")
	var wait sync.WaitGroup
	for range 20 {
		wait.Go(func() {
			if got := router.Select(r, original, strings.Repeat("a", 64), 3); got == original {
				t.Error("healthy mirror ignored")
			}
		})
	}
	wait.Wait()
	if hits.Load() != 1 {
		t.Fatalf("coalesced probes = %d", hits.Load())
	}
	// A prior size-free redirect probe cannot validate a size-bearing manifest.
	if got := router.Select(r, original, strings.Repeat("a", 64), 4); got != original {
		t.Fatal("wrong size accepted")
	}
	status.Store(503)
	failed := strings.ReplaceAll(original, strings.Repeat("a", 64), strings.Repeat("b", 64))
	for range 2 {
		if router.Select(r, failed, strings.Repeat("b", 64), 3) != failed {
			t.Fatal("failed mirror accepted")
		}
	}
	if hits.Load() != 3 {
		t.Fatalf("negative cache probes = %d", hits.Load())
	}
	canceled, cancel := context.WithCancel(r.Context())
	cancel()
	before := hits.Load()
	if router.Select(r.WithContext(canceled), original, strings.Repeat("a", 64), 5) != original {
		t.Fatal("cancel ignored")
	}
	if hits.Load() != before {
		t.Fatal("canceled probe sent network request")
	}
}

func TestColdProbeSurvivesResponseDeadline(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	site := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.Header().Set("Content-Length", "3")
	}))
	defer site.Close()
	router := testRouter()
	router.Base = site.URL + "/ourtaiko/"
	router.Client = site.Client()
	digest := strings.Repeat("a", 64)
	original := CanonicalBase + "fanmade/" + digest + ".ogg"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan string)
	go func() { done <- router.Select(mainland().WithContext(ctx), original, digest, 3) }()
	<-started
	cancel()
	if got := <-done; got != original {
		t.Fatal("response cancellation ignored")
	}
	close(release)
	deadline := time.Now().Add(time.Second)
	for {
		router.mu.Lock()
		pending := len(router.flights)
		router.mu.Unlock()
		if pending == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("cold probe did not finish")
		}
		time.Sleep(time.Millisecond)
	}
	if router.Select(mainland(), original, digest, 3) == original {
		t.Fatal("background result lost")
	}
}
