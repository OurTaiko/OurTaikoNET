package httpapi

import (
	"net/http/httptest"
	"testing"

	"ourtaiko.dev/net/internal/web"
)

func TestProxyRateLimitsSeparateClients(t *testing.T) {
	proxies, _ := web.ParseTrustedProxies("127.0.0.1/32")
	h := New(nil, Config{Origin: "https://example.test", TrustedProxies: proxies}).Handler()
	call := func(ip string) int {
		r := httptest.NewRequest("POST", "/unknown", nil)
		r.RemoteAddr = "127.0.0.1:1234"
		r.Header.Set("Origin", "https://example.test")
		r.Header.Set("X-Real-IP", ip)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	for n := 0; n < 40; n++ {
		if code := call("192.0.2.1"); code != 404 {
			t.Fatal(code)
		}
	}
	if code := call("192.0.2.1"); code != 429 {
		t.Fatal(code)
	}
	if code := call("192.0.2.2"); code != 404 {
		t.Fatal("different client shared rate limit", code)
	}
}
