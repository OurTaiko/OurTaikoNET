package httpapi

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"

	"ourtaiko.dev/net/internal/kit/distribution"
	"ourtaiko.dev/net/internal/kit/objectstore"
)

func TestRegionalResources(t *testing.T) {
	mirror := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Header().Set("Content-Length", "3") }))
	defer mirror.Close()
	router := distribution.FromDonCatalog(mirror.URL+"/ourtaiko/3dchara/catalog.json", func(ip netip.Addr) string { return "CN" }, nil)
	router.Client = mirror.Client()
	remote := &objectstore.S3{}
	if err := remote.SetPublicBaseURL(distribution.CanonicalBase + "fanmade/"); err != nil {
		t.Fatal(err)
	}
	s := New(nil, Config{Objects: remote, Distribution: router})
	digest := strings.Repeat("a", 64)
	for _, ext := range []string{"tja", "ogg", "mp3", "webp", "mp4"} {
		key := digest + "." + ext
		original := distribution.CanonicalBase + "fanmade/" + key
		for _, method := range []string{"GET", "HEAD"} {
			for _, bypass := range []bool{false, true} {
				r := httptest.NewRequest(method, "/file", nil)
				r.RemoteAddr = "203.0.113.1:123"
				if bypass {
					r.URL.RawQuery = "origin=1"
				}
				link := s.routeResource(r, resourceLink{URL: original, HeadURL: original, SHA256: digest, Size: 3})
				want := mirror.URL + "/ourtaiko/fanmade/" + key
				if bypass {
					want = original
				}
				if link.URL != want || (link.FallbackURL == original) == bypass {
					t.Fatalf("manifest = %+v", link)
				}
				w := httptest.NewRecorder()
				s.serveObject(w, r, key, "日本語."+ext, digest, "application/octet-stream", "attachment")
				got, err := url.Parse(w.Header().Get("Location"))
				if err != nil || w.Code != 302 || got.Scheme+"://"+got.Host+got.Path != want || got.Query().Get("response-content-disposition") == "" {
					t.Fatalf("redirect %d %s %v", w.Code, got, err)
				}
			}
		}
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "203.0.113.1:123"
	signed := resourceLink{URL: distribution.CanonicalBase + "fanmade/" + digest + ".ogg?X-Amz-Signature=secret", SHA256: digest, Size: 3}
	if got := s.routeResource(r, signed); got != signed {
		t.Fatal("signed URL modified")
	}
}
