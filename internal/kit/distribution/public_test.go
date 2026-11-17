package distribution

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

type publicTransport func(*http.Request) (*http.Response, error)

func (f publicTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRegionalPublicAvatars(t *testing.T) {
	var probes atomic.Int32
	router := testRouter()
	router.Client = &http.Client{Transport: publicTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodHead || r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			t.Error("unsafe avatar probe")
		}
		probes.Add(1)
		return &http.Response{StatusCode: 200, ContentLength: 7, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	original := CanonicalBase + "sso/avatars/" + strings.Repeat("a", 32) + "/" + strings.Repeat("b", 32) + ".webp"
	for _, ip := range []string{"203.0.113.1", "203.0.113.2", "203.0.113.3", "203.0.113.4", "203.0.113.5"} {
		request := mainland()
		request.Header.Set("X-Real-IP", ip)
		request.URL.Path = "/api/v1/distribution/avatar"
		request.URL.RawQuery = url.Values{"url": {original}, "redirect": {"1"}}.Encode()
		response := httptest.NewRecorder()
		router.Public(response, request)
		want := original
		if ip == "203.0.113.1" {
			want = router.Base + strings.TrimPrefix(original, CanonicalBase)
		}
		if response.Code != 307 || response.Header().Get("Location") != want || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("avatar routing for %s", ip)
		}
	}
	for _, invalid := range []string{original + "?signature=x", original + "#x", strings.Replace(original, "distrib.ourtaiko.org", "private.example", 1), CanonicalBase + "sso/avatars/../secret.webp"} {
		request := mainland()
		request.URL.Path = "/api/v1/distribution/avatar"
		request.URL.RawQuery = url.Values{"url": {invalid}}.Encode()
		response := httptest.NewRecorder()
		router.Public(response, request)
		if response.Code != 400 {
			t.Fatal("accepted unsafe avatar URL")
		}
	}
	if probes.Load() != 1 {
		t.Fatal("non-mainland or invalid URLs caused probes")
	}
}

func TestRegionalPlayerBuild(t *testing.T) {
	for _, state := range []string{"same", "stale", "hash-mismatch", "invalid", "down"} {
		t.Run(state, func(t *testing.T) {
			router := testRouter()
			router.Client = &http.Client{Transport: publicTransport(func(request *http.Request) (*http.Response, error) {
				build := playerBuild{Path: "/player/0123456789abcdef/index.html", Files: map[string]string{"index.html": strings.Repeat("a", 64), "Build/Web.wasm.unityweb": strings.Repeat("b", 64)}}
				status := 200
				if request.URL.Host != "distrib.ourtaiko.org" {
					switch state {
					case "stale":
						build.Path = "/player/1111111111111111/index.html"
					case "hash-mismatch":
						build.Files["index.html"] = strings.Repeat("c", 64)
					case "invalid":
						build.Files["../secret"] = strings.Repeat("a", 64)
					case "down":
						status = 503
					}
				}
				body, _ := json.Marshal(build)
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
			})}
			for _, ip := range []string{"203.0.113.1", "203.0.113.2", "203.0.113.3"} {
				request := mainland()
				request.URL.Path = "/api/v1/distribution/player-build"
				request.Header.Set("X-Real-IP", ip)
				response := httptest.NewRecorder()
				router.Public(response, request)
				var result struct{ Path, BaseURL, FallbackBaseURL string }
				if json.Unmarshal(response.Body.Bytes(), &result) != nil || response.Code != 200 {
					t.Fatal("player manifest unavailable")
				}
				want := CanonicalBase
				if state == "same" && ip == "203.0.113.1" {
					want = router.Base
				}
				if result.BaseURL != want || result.FallbackBaseURL != CanonicalBase || result.Path != "/player/0123456789abcdef/index.html" {
					t.Fatalf("incorrect %s player routing: %+v", state, result)
				}
			}
		})
	}
}

func TestRegionalSource(t *testing.T) {
	router := testRouter()
	router.Client = &http.Client{Transport: publicTransport(func(*http.Request) (*http.Response, error) {
		t.Error("source answer must not probe")
		return nil, http.ErrHandlerTimeout
	})}
	for _, ip := range []string{"203.0.113.1", "203.0.113.2"} {
		request := mainland()
		request.Header.Set("X-Real-IP", ip)
		request.URL.Path = "/api/v1/distribution/source"
		response := httptest.NewRecorder()
		router.Public(response, request)
		var answer SourceAnswer
		if response.Code != 200 || response.Header().Get("Cache-Control") != "no-store" || json.Unmarshal(response.Body.Bytes(), &answer) != nil {
			t.Fatalf("source answer for %s", ip)
		}
		want := SourceAnswer{BaseURL: CanonicalBase, CanonicalBaseURL: CanonicalBase}
		if ip == "203.0.113.1" {
			want = SourceAnswer{Mirror: true, BaseURL: strings.TrimRight(router.Base, "/") + "/", CanonicalBaseURL: CanonicalBase}
		}
		if answer != want {
			t.Fatalf("source for %s: %+v", ip, answer)
		}
	}
	var none *Router
	if answer := none.Source(mainland()); answer.Mirror || answer.BaseURL != CanonicalBase {
		t.Fatal("no router must keep the canonical root")
	}
}
