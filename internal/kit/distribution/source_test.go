package distribution

import (
	"strings"
	"testing"
)

func TestRegionalSource(t *testing.T) {
	router := testRouter()
	for _, ip := range []string{"203.0.113.1", "203.0.113.2"} {
		request := mainland()
		request.Header.Set("X-Real-IP", ip)
		want := SourceAnswer{BaseURL: CanonicalBase, CanonicalBaseURL: CanonicalBase}
		if ip == "203.0.113.1" {
			want = SourceAnswer{Mirror: true, BaseURL: strings.TrimRight(router.Base, "/") + "/", CanonicalBaseURL: CanonicalBase}
		}
		if answer := router.Source(request); answer != want {
			t.Fatalf("source for %s: %+v", ip, answer)
		}
	}
	var none *Router
	if answer := none.Source(mainland()); answer.Mirror || answer.BaseURL != CanonicalBase || none.Regional() {
		t.Fatal("no router must keep the canonical root")
	}
	if !router.Regional() {
		t.Fatal("regional router")
	}
}
