package httpapi

import (
	"ourtaiko.dev/net/internal/files"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ourtaiko.dev/net/internal/account"
	"ourtaiko.dev/net/internal/web"
)

func TestSSOMigrationGuardAndLiveProfile(t *testing.T) {
	pool := testDB(t, Fanmade)
	ctx := context.Background()
	id := web.NewID()
	_, e := pool.Exec(ctx, `INSERT INTO test_sso_users(id,username,nickname,password_hash) VALUES($1,'Private123','公开昵称','unused')`, id)
	if e != nil {
		t.Fatal(e)
	}
	app := testServer(t, pool, Config{Origin: "http://localhost"})
	handler := app.Handler()
	token := seedWebSession(t, app, id)
	// A player's row is their id, when they first signed in and were last active, their user
	// groups, and a copy of what the account center publishes about them (nickname, avatar,
	// when it was last asked): no username, email, password or administrator flag.
	var columns string
	if e = pool.QueryRow(ctx, `SELECT string_agg(column_name::text, ',' ORDER BY column_name) FROM information_schema.columns WHERE table_schema='net' AND table_name='users'`).Scan(&columns); e != nil ||
		columns != "avatar_url,first_login_at,groups,id,last_active_at,nickname,profile_checked_at" {
		t.Fatal("unexpected identity/activity columns", columns, e)
	}
	call := func(method, path, csrf string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(`{}`))
		r.Header.Set("Origin", "http://localhost")
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-CSRF-Token", csrf)
		r.AddCookie(&http.Cookie{Name: "ourtaiko_session", Value: token})
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	w := call("GET", "/api/me", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "公开昵称") {
		t.Fatal(w.Body.String())
	}
	_, e = pool.Exec(ctx, `UPDATE test_sso_users SET nickname='更新昵称',is_admin=true WHERE id=$1`, id)
	if e != nil {
		t.Fatal(e)
	}
	w = call("GET", "/api/me", "")
	if !strings.Contains(w.Body.String(), "更新昵称") || !strings.Contains(w.Body.String(), `"isAdmin":true`) {
		t.Fatal("stale SSO profile", w.Body.String())
	}
	for _, path := range []string{"/api/auth/login", "/api/auth/register", "/api/auth/email-code"} {
		if w := call("POST", path, ""); w.Code != 410 {
			t.Fatal(path, w.Code)
		}
	}
	if w := call("PATCH", "/api/me", "csrf"); w.Code != 410 {
		t.Fatal(w.Code)
	}
	if w := call("POST", "/api/auth/logout", "invalid"); w.Code != 403 {
		t.Fatal("CSRF accepted", w.Code)
	}
	// A remote outage cannot authorize a local cached user, but local logout still succeeds.
	app.Config.SSO.HTTP = &http.Client{Transport: failingTransport{}, Timeout: time.Second}
	if w := call("GET", "/api/me", ""); w.Code != 503 {
		t.Fatal("SSO outage bypass", w.Code)
	}
	if w := call("POST", "/api/auth/logout", "csrf"); w.Code != 200 {
		t.Fatal("outage prevented logout", w.Code)
	}
	w = call("GET", "/api/me", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"user":null`) {
		t.Fatal(w.Code, w.Body.String())
	}
}

type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("offline")
}

func TestSSOCallbackBindingAndEncryption(t *testing.T) {
	pool := testDB(t, Fanmade)
	app := testServer(t, pool, Config{Origin: "http://localhost"})
	h := app.Handler()
	state, binding := web.NewID()+web.NewID(), web.NewID()+web.NewID()
	_, e := pool.Exec(context.Background(), `INSERT INTO oidc_flows VALUES($1,$2,'nonce','verifier','/upload',now()+interval '5 minutes')`, web.Hash(state), web.Hash(binding))
	if e != nil {
		t.Fatal(e)
	}
	call := func(cookie, query string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/api/auth/sso/callback?state="+state+query, nil)
		r.AddCookie(&http.Cookie{Name: "ourtaiko_oidc", Value: cookie})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := call(web.NewID()+web.NewID(), "&code=forged"); w.Code != 303 || w.Header().Get("Location") != "http://localhost/login?error=sso" {
		t.Fatal(w.Code)
	}
	var n int
	_ = pool.QueryRow(context.Background(), `SELECT count(*) FROM oidc_flows`).Scan(&n)
	if n != 1 {
		t.Fatal("mismatched browser consumed flow")
	}
	call(binding, "&error=access_denied")
	_ = pool.QueryRow(context.Background(), `SELECT count(*) FROM oidc_flows`).Scan(&n)
	if n != 0 {
		t.Fatal("cancelled flow reusable")
	}
	if w := call(binding, "&code=replay"); w.Code != 303 {
		t.Fatal(w.Code)
	}
	c := app.Config.SSO
	encrypted := c.Seal("secret", "sessionA")
	if string(encrypted) == "secret" {
		t.Fatal("token not encrypted")
	}
	if _, e := c.Unseal(encrypted, "sessionB"); e == nil {
		t.Fatal("ciphertext can be swapped")
	}
	encrypted[len(encrypted)-1] ^= 1
	if _, e := c.Unseal(encrypted, "sessionA"); e == nil {
		t.Fatal("tampered ciphertext accepted")
	}
	for _, input := range []string{"https://evil.example", "//evil.example", "/\\evil.example", "/x\r\ny"} {
		if New(nil, Config{}).sessions().SafeReturn(input) != "/upload" {
			t.Fatal(input)
		}
	}
	if New(nil, Config{}).sessions().SafeReturn("/me/charts?page=2") != "/me/charts?page=2" {
		t.Fatal("lost valid return path")
	}
}
func TestPublicNamesDoNotExposeLogin(t *testing.T) {
	pool := testDB(t, Fanmade)
	id := web.NewID()
	_, e := pool.Exec(context.Background(), `INSERT INTO test_sso_users(id,username,nickname,password_hash) VALUES($1,'PrivateLogin','公开名','unused')`, id)
	if e != nil {
		t.Fatal(e)
	}
	app := testServer(t, pool, Config{})
	names := app.publicProfiles(context.Background(), []string{id, id})
	data, _ := json.Marshal(names)
	if strings.Contains(string(data), "PrivateLogin") || names[id].Nickname != "公开名" {
		t.Fatal(string(data))
	}
	matches, e := app.Config.SSO.Search(context.Background(), "PrivateLogin")
	if e != nil || len(matches) != 0 {
		t.Fatal("login name searchable", e)
	}
	app.Config.SSO.HTTP = &http.Client{Transport: failingTransport{}}
	if app.publicProfiles(context.Background(), []string{id})[id] != (account.PublicProfile{Nickname: "未知用户"}) {
		t.Fatal("no outage fallback")
	}
}

// Profile preferences are relayed live from SSO, never stored in the business DB.
func TestSSOPreferredLanguageIsLive(t *testing.T) {
	language := "ja"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/v1/web/introspect" || r.Header.Get("X-Service-ID") != "fanmade" {
			t.Error("unexpected identity request")
		}
		json.NewEncoder(w).Encode(map[string]any{"user": User{ID: strings.Repeat("a", 32), PreferredLanguage: language}})
	}))
	defer upstream.Close()
	client := &account.Client{Config: account.Config{Issuer: upstream.URL, ServiceID: "fanmade"}, HTTP: upstream.Client()}
	for _, preference := range []string{"ja", "ko", "en", "zh-hans", ""} {
		language = preference
		user, err := client.Identity(context.Background(), "web", "test-token")
		if err != nil || user.PreferredLanguage != preference {
			t.Fatalf("stale preferred language: %#v, %v", user, err)
		}
		body, err := json.Marshal(user)
		if err != nil || !strings.Contains(string(body), `"preferredLanguage":"`+preference+`"`) {
			t.Fatalf("preferred language missing from API response: %s, %v", body, err)
		}
	}
}

func TestSSOAvatarURLIsPassedThroughOnlyFromIssuer(t *testing.T) {
	id := strings.Repeat("a", 32)
	digest := strings.Repeat("0f", 16)
	avatar := ""
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"user": User{ID: id, AvatarURL: avatar}})
	}))
	defer upstream.Close()
	client := &account.Client{Config: account.Config{Issuer: upstream.URL + "/", ServiceID: "fanmade"}, HTTP: upstream.Client()}
	valid := upstream.URL + "/avatars/" + id + "/" + digest + ".webp"
	for given, want := range map[string]string{
		valid: valid,
		"":    "",
		"https://evil.example/avatars/" + id + "/" + digest + ".webp":                 "",
		upstream.URL + "/avatars/" + strings.Repeat("b", 32) + "/" + digest + ".webp": "",
		upstream.URL + "/avatars/" + id + "/" + digest + ".png":                       "",
		upstream.URL + "/avatars/" + id + "/" + digest + ".webp?x=1":                  "",
		upstream.URL + "/avatars/" + id + "/../" + digest[:29] + ".webp":              "",
		"javascript:alert(1)": "",
	} {
		avatar = given
		user, err := client.Identity(context.Background(), "web", "test-token")
		if err != nil || user.AvatarURL != want {
			t.Fatalf("avatar %q: got %q, %v", given, user.AvatarURL, err)
		}
	}
	avatar = valid
	user, _ := client.Identity(context.Background(), "web", "test-token")
	body, err := json.Marshal(user)
	if err != nil || !strings.Contains(string(body), `"avatarUrl":"`+valid+`"`) {
		t.Fatalf("avatarUrl missing from API response: %s, %v", body, err)
	}
}

func TestSSOProfilesCarryOnlyValidAvatars(t *testing.T) {
	a, b := strings.Repeat("a", 32), strings.Repeat("b", 32)
	digest := strings.Repeat("0f", 16)
	var issuer string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/v1/users/lookup" {
			t.Error("unexpected request", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"users": []map[string]string{
			{"id": a, "nickname": "A", "avatarUrl": issuer + "/avatars/" + a + "/" + digest + ".webp"},
			// Another user's avatar path must not be attributed to b.
			{"id": b, "nickname": "B", "avatarUrl": issuer + "/avatars/" + a + "/" + digest + ".webp"},
		}})
	}))
	defer upstream.Close()
	issuer = upstream.URL
	client := &account.Client{Config: account.Config{Issuer: issuer, ServiceID: "fanmade"}, HTTP: upstream.Client()}
	profiles, err := client.Profiles(context.Background(), []string{a, b})
	if err != nil {
		t.Fatal(err)
	}
	if profiles[a] != (account.PublicProfile{Nickname: "A", AvatarURL: issuer + "/avatars/" + a + "/" + digest + ".webp"}) ||
		profiles[b] != (account.PublicProfile{Nickname: "B"}) {
		t.Fatalf("%#v", profiles)
	}
	// A chart names its author's avatar by its file under the distribution root, where the
	// account center keeps avatars; an address elsewhere names none.
	chart := Chart{OwnerID: a}
	chart.setUploader(account.PublicProfile{Nickname: "A", AvatarURL: files.Root + "sso/avatars/" + a + "/" + digest + ".webp"})
	data, _ := json.Marshal(chart)
	if !strings.Contains(string(data), `"authorAvatarPath":"sso/avatars/`+a+"/"+digest+`.webp"`) {
		t.Fatal(string(data))
	}
	chart.setUploader(profiles[a])
	if data, _ = json.Marshal(chart); !strings.Contains(string(data), `"authorAvatarPath":""`) {
		t.Fatal(string(data))
	}
}
