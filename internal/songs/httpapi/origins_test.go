package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"ourtaiko.dev/net/internal/web"
)

// The site at another of its addresses (web.Origins): sign-in begins and ends there, with the
// account center's callback at that address, and a write is taken only from its own pages.
// The site's own address goes on as before.
func TestSignInAndWritesAtAnotherOrigin(t *testing.T) {
	pool := testDB(t, Fanmade)
	ctx := context.Background()
	client, redirected := accountCenterOIDC(t, "https://net.example/api/auth/sso/callback")
	app := New(pool, Config{Origin: "https://net.example", SSO: client})
	origins, err := web.NewOrigins("https://net.example", []string{"https://cn.example"})
	if err != nil {
		t.Fatal(err)
	}
	h := origins.Handler(app.Handler())
	serve := func(r *http.Request, host string) *httptest.ResponseRecorder {
		r.Host = host
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}

	for _, tc := range []struct{ host, callback string }{
		{"net.example", "https://net.example/api/auth/sso/callback"},
		{"cn.example", "https://cn.example/api/auth/sso/callback"},
		// An address that is not the site's: its own.
		{"other.example", "https://net.example/api/auth/sso/callback"},
	} {
		w := serve(httptest.NewRequest("GET", "/api/auth/sso/login?returnTo=/users", nil), tc.host)
		at, err := url.Parse(w.Header().Get("Location"))
		if w.Code != 303 || err != nil || at.Query().Get("redirect_uri") != tc.callback {
			t.Fatalf("sign-in at %s: %d %s, want the callback %s", tc.host, w.Code, w.Header().Get("Location"), tc.callback)
		}
	}

	// The account center sends the browser back to the address it began at.
	state, binding := web.NewID()+web.NewID(), web.NewID()+web.NewID()
	if _, err := pool.Exec(ctx, `INSERT INTO oidc_flows VALUES($1,$2,'test-nonce','test-verifier','/users',now()+interval '5 minutes')`, web.Hash(state), web.Hash(binding)); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/api/auth/sso/callback?state="+state+"&code="+web.NewID(), nil)
	r.AddCookie(&http.Cookie{Name: "ourtaiko_oidc", Value: binding})
	w := serve(r, "cn.example")
	if w.Code != 303 || w.Header().Get("Location") != "https://cn.example/users" {
		t.Fatal("callback:", w.Code, w.Header(), w.Body.String())
	}
	if *redirected != "https://cn.example/api/auth/sso/callback" {
		t.Fatalf("the code was redeemed for %q", *redirected)
	}
	// A failed one lands on that address's error page.
	w = serve(httptest.NewRequest("GET", "/api/auth/sso/callback", nil), "cn.example")
	if w.Code != 303 || !strings.HasPrefix(w.Header().Get("Location"), "https://cn.example/") {
		t.Fatal("failed callback:", w.Code, w.Header())
	}

	// A write must come from the pages of the address it was sent to.
	for _, tc := range []struct {
		host, origin string
		refused      bool
	}{
		{"cn.example", "https://cn.example", false},
		{"net.example", "https://net.example", false},
		{"net.example", "https://cn.example", true},
		{"cn.example", "https://net.example", true},
		{"other.example", "https://other.example", true},
	} {
		r := httptest.NewRequest("POST", "/api/auth/logout", strings.NewReader("{}"))
		r.Header.Set("Origin", tc.origin)
		w := serve(r, tc.host)
		if refused := w.Code == 403 && strings.Contains(w.Body.String(), "ORIGIN_INVALID"); refused != tc.refused {
			t.Errorf("a write to %s from %s: %d %s", tc.host, tc.origin, w.Code, w.Body.String())
		}
	}
}
