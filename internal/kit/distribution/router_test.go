package distribution

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
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

func TestSelectUsesCountryAlone(t *testing.T) {
	router := testRouter()
	digest := strings.Repeat("a", 64)
	original := CanonicalBase + "fanmade/" + digest + ".ogg"
	// No HTTP client is involved: an unreachable mirror is still chosen for mainland visitors.
	if got := router.Select(mainland(), original, digest, 3); got != router.Base+"fanmade/"+digest+".ogg" {
		t.Fatal(got)
	}
	overseas := mainland()
	overseas.Header.Set("X-Real-IP", "203.0.113.2")
	if got := router.Select(overseas, original, digest, 3); got != original {
		t.Fatal(got)
	}
	if router.MirrorBase(overseas) != "" || router.MirrorBase(mainland()) != router.Base {
		t.Fatal("mirror base policy")
	}
	var none *Router
	if none.Select(mainland(), original, digest, 3) != original || none.MirrorBase(mainland()) != "" {
		t.Fatal("nil router must keep canonical addresses")
	}
}
