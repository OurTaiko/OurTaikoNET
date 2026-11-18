package distribution

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
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
	custom := FromDonCatalog("https://mirror.example/custom.json", router.Country, nil)
	if custom.Base != "" {
		t.Fatal("guessed siblings of custom Don catalog")
	}
}
