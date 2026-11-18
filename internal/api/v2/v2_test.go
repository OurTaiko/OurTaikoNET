package apiv2

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"ourtaiko.dev/net/internal/api/mount"
	"ourtaiko.dev/net/internal/files"
	"ourtaiko.dev/net/internal/kit/distribution"
)

func TestVersion2(t *testing.T) {
	const mirror = "https://mirror.example/ourtaiko/"
	router := distribution.FromDonCatalog(mirror+"3dchara/catalog.json", func(ip netip.Addr) string {
		return map[string]string{"203.0.113.1": "CN"}[ip.String()]
	}, nil)
	body := `{"songCoverPath":"fanmade/a.webp","version":"` + "v" + `"}` + "\n"
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Link", mount.Link(r, "/api/charts/1"))
		w.Write([]byte(body))
	})
	h := Handler(api, Config{Router: router})
	get := func(path, ip string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		r.RemoteAddr = ip + ":1"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	// Answers are the version-free API's, the same for every visitor.
	for _, ip := range []string{"203.0.113.1", "203.0.113.2"} {
		if w := get("/api/v2/sources/fanmade/charts/1", ip); w.Body.String() != body || w.Header().Get("Link") != "/api/v2/charts/1" {
			t.Fatalf("%s: %s %v", ip, w.Body, w.Header())
		}
	}
	// The root is the visitor's, and is no one else's.
	for ip, want := range map[string]distribution.SourceAnswer{
		"203.0.113.1": {Mirror: true, BaseURL: mirror, CanonicalBaseURL: files.Root},
		"203.0.113.2": {BaseURL: files.Root, CanonicalBaseURL: files.Root},
	} {
		w := get("/api/v2/distribution/source", ip)
		var answer distribution.SourceAnswer
		if json.Unmarshal(w.Body.Bytes(), &answer) != nil || answer != want || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s: %s", ip, w.Body)
		}
	}
	// Version 1's own routes are not version 2's; files are served only with files on disk.
	if get("/api/v2/distribution/avatar", "203.0.113.1").Body.String() != body || get("/api/v2/files/fanmade/a.webp", "203.0.113.1").Body.String() != body {
		t.Fatal("not the API's")
	}
}
