package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"ourtaiko.dev/net/internal/signin"
	"ourtaiko.dev/net/internal/web"
)

func TestUserActivityMigrationAndAuthentication(t *testing.T) {
	pool := testDB(t, Fanmade)
	ctx := context.Background()
	id := web.NewID()
	password, err := bcrypt.GenerateFromPassword([]byte("test-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO test_sso_users(id,username,password_hash) VALUES($1,'ActivityUser',$2)`, id, string(password)); err != nil {
		t.Fatal(err)
	}
	app := testServer(t, pool, Config{Origin: "http://localhost"})
	read := func() (*time.Time, *time.Time) {
		t.Helper()
		var first, last *time.Time
		if err := pool.QueryRow(ctx, `SELECT first_login_at,last_active_at FROM users WHERE id=$1`, id).Scan(&first, &last); err != nil {
			t.Fatal(err)
		}
		return first, last
	}
	if first, last := read(); first != nil || last != nil {
		t.Fatal("fabricated historical login")
	}
	call := func(method, path, body, cookie, bearer string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: "ourtaiko_session", Value: cookie})
		}
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
		}
		w := httptest.NewRecorder()
		app.Handler().ServeHTTP(w, r)
		return w
	}
	w := call("POST", "/api/game/login", `{"username":"ActivityUser","password":"wrong"}`, "", "")
	if w.Code != 401 {
		t.Fatal(w.Code, w.Body.String())
	}
	if _, last := read(); last != nil {
		t.Fatal("failed login marked active")
	}
	cookie := seedWebSession(t, app, id)
	if w = call("GET", "/api/me", "", cookie, ""); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	first, last := read()
	if first != nil || last == nil {
		t.Fatal("authenticated legacy session invented login or missed activity")
	}
	w = call("POST", "/api/game/login", `{"username":"ActivityUser","password":"test-password"}`, "", "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var login struct {
		Token string `json:"accessToken"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &login); err != nil {
		t.Fatal(err)
	}
	first, last = read()
	if first == nil || last == nil || first.After(*last) {
		t.Fatal("successful login not recorded")
	}
	initial := *first
	recent := *last
	for i := 0; i < 3; i++ {
		if w = call("GET", "/api/game/scores", "", "", login.Token); w.Code != 200 {
			t.Fatal(w.Body.String())
		}
	}
	first, last = read()
	if !first.Equal(initial) || !last.Equal(recent) {
		t.Fatal("first login changed or activity writes not throttled")
	}
	// Move both recorded times back; a real website request must advance last only.
	if _, err = pool.Exec(ctx, `UPDATE users SET first_login_at=now()-interval '2 days',last_active_at=now()-interval '2 minutes' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	first, last = read()
	initial = *first
	old := *last
	if w = call("GET", "/api/me", "", cookie, ""); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	first, last = read()
	if !first.Equal(initial) || !last.After(old) {
		t.Fatal("web activity did not advance independently")
	}
	// Failed introspection must not refresh an expired activity timestamp.
	if _, err = pool.Exec(ctx, `UPDATE users SET last_active_at=now()-interval '2 minutes' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	_, last = read()
	old = *last
	app.Config.SSO.HTTP = &http.Client{Transport: failingTransport{}}
	if w = call("GET", "/api/me", "", cookie, ""); w.Code != 503 {
		t.Fatal(w.Code)
	}
	if _, last = read(); !last.Equal(old) {
		t.Fatal("failed SSO auth marked active")
	}
	// Concurrent first observations create one row, retain one first-login value.
	newID := web.NewID()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := signin.RecordActivity(ctx, pool, newID, true, ""); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	var valid bool
	if err = pool.QueryRow(ctx, `SELECT first_login_at IS NOT NULL AND last_active_at>=first_login_at FROM users WHERE id=$1`, newID).Scan(&valid); err != nil || !valid {
		t.Fatal(err)
	}
}
