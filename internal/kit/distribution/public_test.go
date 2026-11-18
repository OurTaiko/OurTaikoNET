package distribution

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestRegionalPublicAvatars(t *testing.T) {
	router := testRouter()
	original := CanonicalBase + "sso/avatars/" + strings.Repeat("a", 32) + "/" + strings.Repeat("b", 32) + ".webp"
	for _, ip := range []string{"203.0.113.1", "203.0.113.2", "203.0.113.3", "203.0.113.4", "203.0.113.5"} {
		request := mainland()
		request.Header.Set("X-Real-IP", ip)
		request.URL.Path = "/api/distribution/avatar"
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
		request.URL.Path = "/api/distribution/avatar"
		request.URL.RawQuery = url.Values{"url": {invalid}}.Encode()
		response := httptest.NewRecorder()
		router.Public(response, request)
		if response.Code != 400 {
			t.Fatal("accepted unsafe avatar URL")
		}
	}
	request := mainland()
	request.URL.Path = "/api/distribution/player-build"
	response := httptest.NewRecorder()
	router.Public(response, request)
	if response.Code != 404 {
		t.Fatal("player-build is no longer an endpoint")
	}
}

func TestRegionalSource(t *testing.T) {
	router := testRouter()
	for _, ip := range []string{"203.0.113.1", "203.0.113.2"} {
		request := mainland()
		request.Header.Set("X-Real-IP", ip)
		request.URL.Path = "/api/distribution/source"
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
