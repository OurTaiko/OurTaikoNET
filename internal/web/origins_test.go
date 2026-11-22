package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOriginsByHost(t *testing.T) {
	o, err := NewOrigins("https://net.ourtaiko.org/", []string{" https://OTNET.example.cn ", "", "http://127.0.0.1:8091"})
	if err != nil {
		t.Fatal(err)
	}
	if o.Primary() != "https://net.ourtaiko.org" {
		t.Fatalf("primary %q", o.Primary())
	}
	for _, tc := range []struct{ host, want string }{
		{"net.ourtaiko.org", "https://net.ourtaiko.org"},
		{"otnet.example.cn", "https://otnet.example.cn"},
		{"OTNET.example.cn:443", "https://otnet.example.cn"},
		{"127.0.0.1:8091", "http://127.0.0.1:8091"},
		// Not listed: the site's own, as for an old site's host name.
		{"fanmade.ourtaiko.org", "https://net.ourtaiko.org"},
		{"127.0.0.1:8092", "https://net.ourtaiko.org"},
		{"otnet.example.cn.evil.org", "https://net.ourtaiko.org"},
	} {
		var got string
		r := httptest.NewRequest("GET", "/", nil)
		r.Host = tc.host
		o.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got = Origin(r.Context(), o.Primary())
		})).ServeHTTP(httptest.NewRecorder(), r)
		if got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.host, got, tc.want)
		}
	}
	// Outside Handler, the fallback.
	if got := Origin(httptest.NewRequest("GET", "/", nil).Context(), "http://localhost/"); got != "http://localhost" {
		t.Fatalf("fallback %q", got)
	}
	if others := o.Others(); len(others) != 2 {
		t.Fatalf("others %v", others)
	}
}

func TestOriginsRejected(t *testing.T) {
	for _, others := range [][]string{
		{"otnet.example.cn"},
		{"ftp://otnet.example.cn"},
		{"https://otnet.example.cn/path"},
		{"https://otnet.example.cn?x=1"},
		{"https://user@otnet.example.cn"},
		{"https://otnet.example.cn#x"},
		{"https://net.ourtaiko.org"},
		{"https://a.example.cn", "https://A.example.cn:443"},
	} {
		if _, err := NewOrigins("https://net.ourtaiko.org", others); err == nil {
			t.Errorf("%q accepted", others)
		}
	}
	if o, err := NewOrigins("http://127.0.0.1:5173", nil); err != nil || len(o.Others()) != 0 {
		t.Fatalf("no others: %v %v", o, err)
	}
}
