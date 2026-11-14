package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"ourtaiko.dev/net/internal/account"
	"ourtaiko.dev/net/internal/netdb"
	"ourtaiko.dev/net/internal/netdb/netdbtest"
	songs "ourtaiko.dev/net/internal/songs/httpapi"
)

// One program, one database: each source answers under its name at the new address and as
// its old site at the old one, and neither sees the other's charts.
func TestBothSourcesFromOneDatabase(t *testing.T) {
	pool, url := netdbtest.NewURL(t, "fanmade")
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
INSERT INTO net.users(id) VALUES (repeat('a',32));
INSERT INTO fanmade.chart_resources(storage_key,original_filename,sha256,byte_size,extension) VALUES
 (repeat('1',64)||'.tja','a.tja',repeat('1',64),10,'tja'), (repeat('2',64)||'.ogg','a.ogg',repeat('2',64),10,'ogg');
INSERT INTO official.chart_resources(storage_key,original_filename,sha256,byte_size,extension) VALUES
 (repeat('3',64)||'.tja','b.tja',repeat('3',64),10,'tja'), (repeat('4',64)||'.ogg','b.ogg',repeat('4',64),10,'ogg');
INSERT INTO fanmade.charts(id,owner_ids,title,bpm,duration,demo_end,tja_file,audio_file)
 SELECT repeat('f',32), ARRAY[repeat('a',32)], 'Fan song', 120, 60, 10, min(id), max(id) FROM fanmade.chart_resources;
-- The official source lists a chart in the game only once it has a category.
INSERT INTO official.charts(id,owner_ids,title,bpm,duration,demo_end,tja_file,audio_file,category_flags)
 SELECT repeat('d',32), ARRAY[repeat('a',32)], 'Official song', 120, 60, 10, min(id), max(id), 1 FROM official.chart_resources;
INSERT INTO fanmade.chart_difficulties(chart_id,course,side,level,content_hash) VALUES (repeat('f',32),'Oni','single',8,'v2:'||repeat('0',64));
INSERT INTO official.chart_difficulties(chart_id,course,side,level,content_hash) VALUES (repeat('d',32),'Oni','single',9,'v2:'||repeat('1',64));
`); err != nil {
		t.Fatal(err)
	}
	storage := t.TempDir()
	a, err := New(ctx, Config{DatabaseURL: url, Origin: "http://localhost", StorageDir: storage,
		LegacyFanmadeHost: "fanmade.ourtaiko.org", LegacyOfficialHost: "database.ourtaiko.org"})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	h := a.Handler()
	// A page of an old site is the same page of this one now; the old API stays where it was.
	for asked, want := range map[string]string{
		"fanmade.ourtaiko.org/charts/" + strings.Repeat("f", 32): "http://localhost/fanmade/charts/" + strings.Repeat("f", 32),
		"database.ourtaiko.org/playlists?sort=newest":            "http://localhost/official/playlists?sort=newest",
	} {
		host, path, _ := strings.Cut(asked, "/")
		r := httptest.NewRequest("GET", "/"+path, nil)
		r.Host = host
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusMovedPermanently || w.Header().Get("Location") != want {
			t.Errorf("%s: %d to %q, want 301 to %q", asked, w.Code, w.Header().Get("Location"), want)
		}
	}
	// With files on this server's disk, a chart's download links are this site's own routes
	// of the chart's source, whichever address asked, and they answer.
	if err := os.WriteFile(filepath.Join(storage, "fanmade", strings.Repeat("1", 64)+".tja"), []byte("TITLE:Fan"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, asked := range []struct{ host, path string }{
		{"net.ourtaiko.org", "/api/v1/sources/fanmade/charts/" + strings.Repeat("f", 32) + "/resources"},
		{"fanmade.ourtaiko.org", "/api/v1/charts/" + strings.Repeat("f", 32) + "/resources"},
	} {
		r := httptest.NewRequest("GET", asked.path, nil)
		r.Host = asked.host
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		var links struct {
			Resources map[string]struct {
				URL string `json:"url"`
			} `json:"resources"`
		}
		want := "http://localhost/api/v1/sources/fanmade/charts/" + strings.Repeat("f", 32) + "/tja"
		if err := json.Unmarshal(w.Body.Bytes(), &links); err != nil || w.Code != 200 || links.Resources["tja"].URL != want {
			t.Fatalf("%s%s: %d %s", asked.host, asked.path, w.Code, w.Body.String())
		}
		w = httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", strings.TrimPrefix(want, "http://localhost"), nil))
		if w.Code != 200 || w.Body.String() != "TITLE:Fan" {
			t.Fatalf("following %s: %d %s", want, w.Code, w.Body.String())
		}
	}
	titles := func(host, path string) (int, []string) {
		t.Helper()
		r := httptest.NewRequest("GET", path, nil)
		r.Host = host
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		var body struct {
			Charts []struct {
				Title string `json:"title"`
			} `json:"charts"`
		}
		if w.Code == http.StatusOK {
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("%s%s: %v: %s", host, path, err, w.Body.String())
			}
		}
		out := []string{}
		for _, c := range body.Charts {
			out = append(out, c.Title)
		}
		return w.Code, out
	}
	for _, c := range []struct {
		host, path string
		status     int
		want       string
	}{
		{"net.ourtaiko.org", "/api/v1/sources/fanmade/game/catalog", 200, "Fan song"},
		{"net.ourtaiko.org", "/api/v1/sources/official/game/catalog", 200, "Official song"},
		{"fanmade.ourtaiko.org", "/api/v1/game/catalog", 200, "Fan song"},
		{"database.ourtaiko.org", "/api/v1/game/catalog", 200, "Official song"},
		// The new address has no catalog of its own outside a source, and the old addresses
		// do not know the new routes.
		{"net.ourtaiko.org", "/api/v1/game/catalog", 404, ""},
		{"fanmade.ourtaiko.org", "/api/v1/sources/official/game/catalog", 404, ""},
	} {
		status, got := titles(c.host, c.path)
		if status != c.status || (c.want != "" && (len(got) != 1 || got[0] != c.want)) {
			t.Errorf("%s%s: status %d charts %v, want %d [%s]", c.host, c.path, status, got, c.status, c.want)
		}
	}
}

func TestInitRefusesADatabaseThatIsNotEmpty(t *testing.T) {
	_, url := netdbtest.NewURL(t, "fanmade")
	if err := Init(context.Background(), url); err == nil {
		t.Fatal("Init filled a database that already had tables")
	}
}

func TestNewRefusesADatabaseOfAnotherVersion(t *testing.T) {
	pool, url := netdbtest.NewURL(t, "fanmade")
	if _, err := pool.Exec(context.Background(), `INSERT INTO net.schema_migrations(version) VALUES (99)`); err != nil {
		t.Fatal(err)
	}
	if _, err := New(context.Background(), Config{DatabaseURL: url, Origin: "http://localhost", StorageDir: t.TempDir()}); err == nil {
		t.Fatal("New accepted a newer schema")
	}
}

// A release that changes the schema brings the database along as it starts: a database one
// change behind is given that change, once.
func TestNewBringsAnOlderDatabaseUpToDate(t *testing.T) {
	ctx := context.Background()
	pool, url := netdbtest.NewURL(t, "fanmade")
	if _, err := pool.Exec(ctx, `ALTER TABLE net.users DROP COLUMN nickname, DROP COLUMN avatar_url, DROP COLUMN profile_checked_at`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM net.schema_migrations WHERE version>1`); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		a, err := New(ctx, Config{DatabaseURL: url, Origin: "http://localhost", StorageDir: t.TempDir()})
		if err != nil {
			t.Fatal(err)
		}
		a.Close()
	}
	var version, columns int
	if err := pool.QueryRow(ctx, `SELECT (SELECT max(version) FROM net.schema_migrations),
 (SELECT count(*) FROM information_schema.columns WHERE table_schema='net' AND table_name='users' AND column_name IN ('nickname','avatar_url','profile_checked_at'))`).Scan(&version, &columns); err != nil {
		t.Fatal(err)
	}
	if version != SchemaVersion || SchemaVersion < 2 || columns != 3 {
		t.Fatal(version, SchemaVersion, columns)
	}
}

// The players' part reads both sources' scores and the sources ask it for awards, all inside
// one program; from outside, the award route does not exist.
func TestPlayersAcrossSources(t *testing.T) {
	pool, url := netdbtest.NewURL(t, "fanmade")
	ctx := context.Background()
	player := strings.Repeat("a", 32)
	if _, err := pool.Exec(ctx, `
INSERT INTO net.users(id) VALUES (repeat('a',32));
INSERT INTO fanmade.chart_resources(storage_key,original_filename,sha256,byte_size,extension) VALUES
 (repeat('1',64)||'.tja','a.tja',repeat('1',64),10,'tja'), (repeat('2',64)||'.ogg','a.ogg',repeat('2',64),10,'ogg');
INSERT INTO official.chart_resources(storage_key,original_filename,sha256,byte_size,extension) VALUES
 (repeat('3',64)||'.tja','b.tja',repeat('3',64),10,'tja'), (repeat('4',64)||'.ogg','b.ogg',repeat('4',64),10,'ogg');
INSERT INTO fanmade.charts(id,owner_ids,title,bpm,duration,demo_end,tja_file,audio_file)
 SELECT repeat('f',32), ARRAY[repeat('a',32)], 'Fan song', 120, 60, 10, min(id), max(id) FROM fanmade.chart_resources;
INSERT INTO official.charts(id,owner_ids,title,bpm,duration,demo_end,tja_file,audio_file,category_flags)
 SELECT repeat('d',32), ARRAY[repeat('a',32)], 'Official song', 120, 60, 10, min(id), max(id), 1 FROM official.chart_resources;
INSERT INTO fanmade.chart_difficulties(id,chart_id,course,side,level,content_hash) OVERRIDING SYSTEM VALUE VALUES (5,repeat('f',32),'Oni','single',8,'v2:'||repeat('0',64));
INSERT INTO official.chart_difficulties(id,chart_id,course,side,level,content_hash) OVERRIDING SYSTEM VALUE VALUES (5,repeat('d',32),'Oni','single',9,'v2:'||repeat('1',64));
INSERT INTO net.scores(source,id,user_id,good,ok,bad,score,drumroll,max_combo,payload_digest,difficulty_id) VALUES
 ('fanmade',repeat('1',32),repeat('a',32),10,0,0,1000,0,10,repeat('d',64),5),
 ('official',repeat('2',32),repeat('a',32),20,0,0,2000,0,20,repeat('d',64),5);
`); err != nil {
		t.Fatal(err)
	}
	a, err := New(ctx, Config{DatabaseURL: url, Origin: "http://localhost", StorageDir: t.TempDir(), LegacyProfileHost: "profile.ourtaiko.org"})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	h := a.Handler()
	get := func(method, host, path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, nil)
		r.Host = host
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}

	// One list of a player's plays, from both sources, at the new address and the old one.
	for _, host := range []string{"net.ourtaiko.org", "profile.ourtaiko.org"} {
		w := get("GET", host, "/api/v1/users/"+player+"/scores?view=all")
		var list struct {
			Items []struct {
				Source   string `json:"source"`
				Title    string `json:"title"`
				Score    int    `json:"score"`
				ChartURL string `json:"chartUrl"`
			} `json:"items"`
			Sources []struct {
				ID    string `json:"id"`
				Error string `json:"error"`
			} `json:"sources"`
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &list) != nil {
			t.Fatalf("%s: scores %d %s", host, w.Code, w.Body.String())
		}
		for _, s := range list.Sources {
			if s.Error != "" {
				t.Errorf("%s: source %s could not be read: %s", host, s.ID, s.Error)
			}
		}
		got := map[string]string{}
		for _, item := range list.Items {
			got[item.Source] = item.Title + " " + item.ChartURL
		}
		if len(list.Items) != 2 || got["fanmade"] != "Fan song http://localhost/fanmade/charts/"+strings.Repeat("f", 32) ||
			got["official"] != "Official song http://localhost/official/charts/"+strings.Repeat("d", 32) {
			t.Errorf("%s: scores %v from %s", host, got, w.Body.String())
		}
	}

	// What a dan attempt earns is recorded in the transaction the source saves the attempt in,
	// under the source's name: kept when that transaction commits, gone when it does not.
	official1, err := netdb.Open(ctx, url, "official")
	if err != nil {
		t.Fatal(err)
	}
	defer official1.Close()
	grant := func(pool *pgxpool.Pool, commit bool, give func(tx pgx.Tx) (json.RawMessage, error)) {
		t.Helper()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		answer, err := give(tx)
		if err != nil || !strings.Contains(string(answer), `"granted"`) || !strings.Contains(string(answer), `"nameplate"`) {
			t.Fatalf("award: %s %v", answer, err)
		}
		if commit {
			if err = tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
		}
	}
	grant(pool, true, func(tx pgx.Tx) (json.RawMessage, error) {
		return a.Fanmade.Config.Awards.Grant(ctx, tx, songs.AwardRequest{Key: "attempt-f", UserID: player, CourseID: strings.Repeat("c", 32), CourseTitle: "Course", Verdict: 4, Titles: []string{}})
	})
	grant(official1, true, func(tx pgx.Tx) (json.RawMessage, error) {
		return a.Official.Config.Awards.Grant(ctx, tx, songs.AwardRequest{Key: "attempt-o", UserID: strings.Repeat("b", 32), CourseID: strings.Repeat("c", 32), CourseTitle: "Course", Verdict: 4, Titles: []string{}})
	})
	grant(pool, false, func(tx pgx.Tx) (json.RawMessage, error) {
		return a.Fanmade.Config.Awards.Grant(ctx, tx, songs.AwardRequest{Key: "attempt-undone", UserID: player, CourseID: strings.Repeat("c", 32), CourseTitle: "Course", Verdict: 4, Titles: []string{}})
	})
	var recorded string
	if err = pool.QueryRow(ctx, `SELECT string_agg(service||':'||request_key, ',' ORDER BY service) FROM net.awards`).Scan(&recorded); err != nil || recorded != "fanmade:attempt-f,official:attempt-o" {
		t.Fatalf("awards recorded: %q %v", recorded, err)
	}
	// An attempt already used for another player is refused for good, as the source's own kind
	// of refusal, and the transaction goes on.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.Fanmade.Config.Awards.Grant(ctx, tx, songs.AwardRequest{Key: "attempt-f", UserID: strings.Repeat("b", 32), CourseID: strings.Repeat("c", 32), Verdict: 4, Titles: []string{}})
	var refused *songs.ProfileRejection
	if !errors.As(err, &refused) || refused.Code != "AWARD_CONFLICT" {
		t.Fatal("an attempt of another player:", err)
	}
	if _, err = tx.Exec(ctx, `SELECT 1`); err != nil {
		t.Fatal("the transaction is not usable after a refusal:", err)
	}
	tx.Rollback(ctx)
	// The second award was for someone no source had seen: they are a player now.
	var known bool
	if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM net.users WHERE id=repeat('b',32))`).Scan(&known); err != nil || !known {
		t.Fatal("the awarded player is not in net.users", err)
	}
	// The sources still read the title catalog from the players' part.
	if titles, err := a.Fanmade.Config.Profile.Titles(ctx, true); err != nil || titles == nil {
		t.Fatal("title catalog:", titles, err)
	}

	// Nameplates are public; asking for awards is not possible from outside.
	if w := get("GET", "profile.ourtaiko.org", "/api/v1/users/"+player+"/nameplate"); w.Code != 200 {
		t.Errorf("nameplate at the old address: %d %s", w.Code, w.Body.String())
	}
	if w := get("GET", "net.ourtaiko.org", "/api/v1/users/"+player+"/nameplate"); w.Code != 200 {
		t.Errorf("nameplate at the new address: %d %s", w.Code, w.Body.String())
	}
	for _, host := range []string{"net.ourtaiko.org", "profile.ourtaiko.org", "fanmade.ourtaiko.org"} {
		if w := get("POST", host, "/internal/v1/awards"); w.Code != 404 {
			t.Errorf("POST /internal/v1/awards on %s: %d, want 404", host, w.Code)
		}
	}
}

// accountCenter stands in for the account center's internal API: the access token
// "tok:<player id>" is that player's, until it is revoked; "adm:<player id>" is the same for
// someone the account center calls this site's administrator.
const newcomer = "cccccccccccccccccccccccccccccc33"

func accountCenter(t *testing.T) (issuer string, revoked *sync.Map) {
	t.Helper()
	revoked = &sync.Map{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			AccessToken string   `json:"accessToken"`
			Usernames   []string `json:"usernames"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		id, ok := strings.CutPrefix(body.AccessToken, "tok:")
		admin := false
		if !ok {
			id, ok = strings.CutPrefix(body.AccessToken, "adm:")
			admin = ok
		}
		_, gone := revoked.Load(body.AccessToken)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/internal/v1/web/revoke":
			revoked.Store(body.AccessToken, true)
			w.Write([]byte(`{"ok":true}`))
		case r.URL.Path == "/internal/v1/users/lookup-by-username":
			// One account exists that has never been to the site: Newcomer1.
			users := []map[string]any{}
			if len(body.Usernames) == 1 && body.Usernames[0] == "Newcomer1" {
				users = append(users, map[string]any{"id": newcomer, "username": "Newcomer1", "nickname": "新来的", "avatarUrl": ""})
			}
			json.NewEncoder(w).Encode(map[string]any{"users": users})
		case r.URL.Path == "/internal/v1/web/introspect" && ok && !gone:
			json.NewEncoder(w).Encode(map[string]any{"user": map[string]any{"id": id, "username": "player", "nickname": "Player", "isAdmin": admin, "emailVerified": true}, "expiresAt": time.Now().Add(time.Hour)})
		default:
			w.WriteHeader(401)
			w.Write([]byte(`{"code":"UNAUTHORIZED","message":"no"}`))
		}
	}))
	t.Cleanup(server.Close)
	return server.URL, revoked
}

// One sign-in for the whole site: a session opened by the players' part is a session in both
// sources, signing out ends it everywhere, and the sources' own sign-in routes are not offered.
func TestOneSignInForTheWholeSite(t *testing.T) {
	pool, url := netdbtest.NewURL(t, "fanmade")
	ctx := context.Background()
	issuer, _ := accountCenter(t)
	key, secret := strings.Repeat("k", 64), strings.Repeat("b", 64)
	sso, err := account.New(account.Config{Issuer: issuer, ServiceID: "net", ServiceKey: key, ClientID: "net", ClientSecret: "secret", RedirectURL: "http://localhost/api/v1/auth/sso/callback", EncryptionKey: secret})
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(ctx, Config{DatabaseURL: url, Origin: "http://localhost", StorageDir: t.TempDir(), SSO: sso})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	h := a.Handler()
	player := strings.Repeat("7", 32)
	// Nobody had heard of this player: signing in makes them known and is their first sign-in.
	cookie, _, err := a.Players.OpenSession(ctx, player, "tok:"+player, "refresh", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	var first, last *time.Time
	if err = pool.QueryRow(ctx, `SELECT first_login_at,last_active_at FROM net.users WHERE id=$1`, player).Scan(&first, &last); err != nil || first == nil || last == nil {
		t.Fatal("the sign-in is not recorded for the player:", first, last, err)
	}
	me := func(path string, signedIn bool) (csrf string) {
		t.Helper()
		r := httptest.NewRequest("GET", path, nil)
		r.Host = "net.ourtaiko.org"
		if signedIn {
			r.AddCookie(&http.Cookie{Name: SessionCookie, Value: cookie})
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		var body struct {
			User *struct {
				ID string `json:"id"`
			} `json:"user"`
			CSRF      string `json:"csrf"`
			CSRFToken string `json:"csrfToken"`
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &body) != nil {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		if signedIn && (body.User == nil || body.User.ID != player) {
			t.Fatalf("%s does not know the signed-in player: %s", path, w.Body.String())
		}
		if !signedIn && body.User != nil {
			t.Fatalf("%s knows a player without a session: %s", path, w.Body.String())
		}
		return body.CSRF + body.CSRFToken
	}
	paths := []string{"/api/v1/me", "/api/v1/sources/fanmade/me", "/api/v1/sources/official/me"}
	tokens := map[string]bool{}
	for _, path := range paths {
		me(path, false)
		tokens[me(path, true)] = true
	}
	if len(tokens) != 1 {
		t.Fatalf("the parts give different CSRF tokens for one session: %v", tokens)
	}
	var csrf string
	for token := range tokens {
		csrf = token
	}
	if csrf == "" {
		t.Fatal("no CSRF token for the session")
	}

	// The sources' own sign-in is not offered at the new address.
	for _, path := range []string{"/api/v1/sources/fanmade/auth/sso/login", "/api/v1/sources/official/auth/sso/login"} {
		r := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 404 {
			t.Errorf("%s: %d, want 404", path, w.Code)
		}
	}

	// Signing out once signs out of every part.
	r := httptest.NewRequest("POST", "/api/v1/auth/logout", strings.NewReader("{}"))
	r.Header.Set("Origin", "http://localhost")
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-CSRF-Token", csrf)
	r.AddCookie(&http.Cookie{Name: SessionCookie, Value: cookie})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal("sign out:", w.Code, w.Body.String())
	}
	for _, path := range paths {
		r := httptest.NewRequest("GET", path, nil)
		r.AddCookie(&http.Cookie{Name: SessionCookie, Value: cookie})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code == 200 && strings.Contains(w.Body.String(), player) {
			t.Errorf("%s still knows the player after signing out: %s", path, w.Body.String())
		}
	}
}

// wholeSite is the whole program on a database of the test's own, with the stand-in account center.
func wholeSite(t *testing.T) (*App, http.Handler, *pgxpool.Pool) {
	t.Helper()
	pool, url := netdbtest.NewURL(t, "fanmade")
	issuer, _ := accountCenter(t)
	key, secret := strings.Repeat("k", 64), strings.Repeat("b", 64)
	const callback = "http://localhost/api/v1/auth/sso/callback"
	sso, err := account.New(account.Config{Issuer: issuer, ServiceID: "net", ServiceKey: key, ClientID: "net", ClientSecret: "secret", RedirectURL: callback, EncryptionKey: secret})
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(context.Background(), Config{DatabaseURL: url, Origin: "http://localhost", StorageDir: t.TempDir(), SSO: sso})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Close)
	return a, a.Handler(), pool
}

// The manage panel finds a player by what an administrator can remember: part of a nickname,
// or a login name in full. An id still works.
func TestTheAdministratorFindsPlayersByName(t *testing.T) {
	a, h, pool := wholeSite(t)
	ctx := context.Background()
	boss, drummer, master, percent := strings.Repeat("a", 31)+"1", strings.Repeat("b", 31)+"2", strings.Repeat("d", 31)+"4", strings.Repeat("e", 31)+"5"
	if _, err := pool.Exec(ctx, `INSERT INTO net.users(id,nickname,avatar_url,groups,last_active_at,profile_checked_at) VALUES
 ($1,'太鼓小将','https://cdn.example/a.webp',1,now(),now()),($2,'Taiko_Master','',0,now()-interval '1 day',now()),($3,'taiko 100%','',0,NULL,now())`, drummer, master, percent); err != nil {
		t.Fatal(err)
	}
	cookie, _, err := a.Players.OpenSession(ctx, boss, "adm:"+boss, "refresh", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	type found struct {
		ID        string   `json:"id"`
		Username  string   `json:"username"`
		Nickname  string   `json:"nickname"`
		AvatarURL string   `json:"avatarUrl"`
		Groups    []string `json:"groups"`
	}
	find := func(session, query string) (int, []found) {
		t.Helper()
		r := httptest.NewRequest("GET", "/api/v1/admin/users?"+query, nil)
		r.Host = "net.ourtaiko.org"
		if session != "" {
			r.AddCookie(&http.Cookie{Name: SessionCookie, Value: session})
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		var out struct {
			Users []found `json:"users"`
		}
		if rec.Code == 200 {
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.Users == nil {
				t.Fatalf("%s: %s", query, rec.Body.String())
			}
		}
		return rec.Code, out.Users
	}
	ids := func(users []found) string {
		out := []string{}
		for _, u := range users {
			out = append(out, u.ID)
		}
		return strings.Join(out, ",")
	}
	for _, c := range []struct{ about, text, want string }{
		{"part of a nickname", "小将", drummer},
		{"whatever its case", "TAIKO_m", master},
		{"an underscore is an underscore", "Taiko-Master", ""},
		{"and not any character", "Taiko_Maste_", ""},
		{"a percent sign is a percent sign", "%", percent},
		{"several, the one active last first", "taiko", master + "," + percent},
		{"an id", master, master},
		{"a login name in full, of someone who has never been here", "Newcomer1", newcomer},
		{"not part of a login name", "Newcomer", ""},
		{"nor in another case", "newcomer1", ""},
		{"nothing", "nobody-by-this-name", ""},
	} {
		status, users := find(cookie, "q="+url.QueryEscape(c.text))
		if status != 200 || ids(users) != c.want {
			t.Errorf("%s (%q): %d %+v", c.about, c.text, status, users)
		}
	}
	// What the panel shows of each: the name and avatar kept here, their groups, and the login
	// name only when that is what was typed.
	if _, users := find(cookie, "q="+url.QueryEscape("小将")); len(users) != 1 || users[0].Nickname != "太鼓小将" || users[0].AvatarURL != "https://cdn.example/a.webp" ||
		users[0].Username != "" || strings.Join(users[0].Groups, ",") != "OFFICIAL_MANAGER" {
		t.Errorf("found by nickname: %+v", users)
	}
	if _, users := find(cookie, "q=Newcomer1"); len(users) != 1 || users[0].Username != "Newcomer1" || users[0].Nickname != "新来的" || len(users[0].Groups) != 0 {
		t.Errorf("found by login name: %+v", users)
	}
	// The list of members names them too.
	if status, users := find(cookie, ""); status != 200 || len(users) != 1 || users[0].ID != drummer || users[0].Nickname != "太鼓小将" {
		t.Errorf("members: %d %+v", status, users)
	}
	if status, _ := find(cookie, "q="+strings.Repeat("x", 101)); status != 400 {
		t.Error("a search of 101 characters:", status)
	}
	// Only the administrator searches.
	other, _, err := a.Players.OpenSession(ctx, drummer, "tok:"+drummer, "refresh", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if status, _ := find(other, "q=taiko"); status != 403 {
		t.Error("a player searched:", status)
	}
	if status, _ := find("", "q=taiko"); status != 401 {
		t.Error("nobody searched:", status)
	}
}

// The account center's administrator manages the site and is in no group; what a part's code
// calls an administrator's work is for the members of that part's group, which only the
// site's administrator gives and takes.
func TestGroupsDecideWhoMayDoWhat(t *testing.T) {
	a, h, pool := wholeSite(t)
	ctx := context.Background()
	boss, player := strings.Repeat("a", 31)+"1", strings.Repeat("b", 31)+"2"
	type who struct{ cookie, csrf string }
	signIn := func(id, token string) *who {
		t.Helper()
		cookie, _, err := a.Players.OpenSession(ctx, id, token, "refresh", time.Now().Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		return &who{cookie: cookie}
	}
	call := func(w *who, method, path, body string, out any) int {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Host = "net.ourtaiko.org"
		r.Header.Set("Origin", "http://localhost")
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		if w != nil {
			r.AddCookie(&http.Cookie{Name: SessionCookie, Value: w.cookie})
			r.Header.Set("X-CSRF-Token", w.csrf)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if out != nil {
			if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
				t.Fatalf("%s %s: %d %s", method, path, rec.Code, rec.Body.String())
			}
		}
		return rec.Code
	}
	type me struct {
		User struct {
			IsAdmin   bool     `json:"isAdmin"`
			SiteAdmin bool     `json:"siteAdmin"`
			Groups    []string `json:"groups"`
		} `json:"user"`
		CSRFToken string `json:"csrfToken"`
	}
	look := func(w *who, path string) me {
		t.Helper()
		var v me
		if status := call(w, "GET", path, "", &v); status != 200 {
			t.Fatal(path, status)
		}
		return v
	}
	manages := func(w *who) string {
		t.Helper()
		out := []string{}
		for _, part := range []string{"/api/v1/sources/official/me", "/api/v1/sources/fanmade/me", "/api/v1/me"} {
			if look(w, part).User.IsAdmin {
				out = append(out, part)
			}
		}
		return strings.Join(out, ",")
	}
	admin, someone := signIn(boss, "adm:"+boss), signIn(player, "tok:"+player)
	admin.csrf, someone.csrf = look(admin, "/api/v1/me").CSRFToken, look(someone, "/api/v1/me").CSRFToken

	// The administrator is the site's administrator and nothing else.
	if v := look(admin, "/api/v1/me"); !v.User.SiteAdmin || len(v.User.Groups) != 0 || manages(admin) != "" {
		t.Fatalf("the administrator: %+v, manages %q", v.User, manages(admin))
	}
	if status := call(admin, "POST", "/api/v1/sources/official/charts", "{}", nil); status != 403 {
		t.Fatal("the administrator uploaded an official chart without the group:", status)
	}
	if status := call(admin, "PUT", "/api/v1/admin/titles/some-title", `{"name":"T","style":"gold","official":true}`, nil); status != 403 {
		t.Fatal("the administrator kept the title catalog without the group:", status)
	}
	// A player is nobody special and cannot use the manage panel.
	if v := look(someone, "/api/v1/me"); v.User.SiteAdmin || len(v.User.Groups) != 0 || manages(someone) != "" {
		t.Fatalf("a player: %+v", v.User)
	}
	for _, c := range []struct{ method, path, body string }{
		{"GET", "/api/v1/admin/groups", ""}, {"GET", "/api/v1/admin/users", ""},
		{"PUT", "/api/v1/admin/users/" + player + "/groups", `{"groups":["OFFICIAL_MANAGER"]}`},
	} {
		if status := call(someone, c.method, c.path, c.body, nil); status != 403 {
			t.Fatalf("a player used %s %s: %d", c.method, c.path, status)
		}
		if status := call(nil, c.method, c.path, c.body, nil); status != 401 {
			t.Fatalf("nobody used %s %s: %d", c.method, c.path, status)
		}
	}

	// The administrator gives the player OFFICIAL_MANAGER.
	var catalog struct {
		Groups []struct {
			Name string `json:"name"`
			Bit  int    `json:"bit"`
		} `json:"groups"`
	}
	if status := call(admin, "GET", "/api/v1/admin/groups", "", &catalog); status != 200 || len(catalog.Groups) != 3 || catalog.Groups[0].Name != "OFFICIAL_MANAGER" || catalog.Groups[0].Bit != 1 {
		t.Fatalf("group catalog: %d %+v", status, catalog)
	}
	give := "/api/v1/admin/users/" + player + "/groups"
	stale := &who{cookie: admin.cookie, csrf: "not-the-token"}
	if status := call(stale, "PUT", give, `{"groups":["OFFICIAL_MANAGER"]}`, nil); status != 403 {
		t.Fatal("groups given without the session's CSRF token:", status)
	}
	if status := call(admin, "PUT", give, `{"groups":["ADMIN"]}`, nil); status != 422 {
		t.Fatal("a group that does not exist:", status)
	}
	if status := call(admin, "PUT", give, `{"groups":["OFFICIAL_MANAGER"]}`, nil); status != 200 {
		t.Fatal("giving a group:", status)
	}
	if v := look(someone, "/api/v1/me"); len(v.User.Groups) != 1 || v.User.Groups[0] != "OFFICIAL_MANAGER" || v.User.SiteAdmin {
		t.Fatalf("the player after: %+v", v.User)
	}
	// It is the official source's group and no other part's.
	if got := manages(someone); got != "/api/v1/sources/official/me" {
		t.Fatal("OFFICIAL_MANAGER manages", got)
	}
	if status := call(someone, "POST", "/api/v1/sources/official/charts", "{}", nil); status == 403 || status == 401 {
		t.Fatal("an official manager may not upload:", status)
	}
	// Being in a group does not let anyone give groups.
	if status := call(someone, "PUT", "/api/v1/admin/users/"+boss+"/groups", `{"groups":[]}`, nil); status != 403 {
		t.Fatal("a group member gave groups:", status)
	}
	var listed struct {
		Users []struct {
			ID     string   `json:"id"`
			Groups []string `json:"groups"`
		} `json:"users"`
	}
	if status := call(admin, "GET", "/api/v1/admin/users", "", &listed); status != 200 || len(listed.Users) != 1 || listed.Users[0].ID != player {
		t.Fatalf("members: %d %+v", status, listed)
	}

	// The other two groups, then none.
	call(admin, "PUT", give, `{"groups":["FANMADE_MANAGER","TITLE_MANAGER"]}`, nil)
	if got := manages(someone); got != "/api/v1/sources/fanmade/me,/api/v1/me" {
		t.Fatal("FANMADE_MANAGER and TITLE_MANAGER manage", got)
	}
	if status := call(someone, "PUT", "/api/v1/admin/titles/some-title", `{"name":"T","style":"gold","official":true}`, nil); status == 403 {
		t.Fatal("a title manager may not keep the catalog")
	}
	call(admin, "PUT", give, `{"groups":[]}`, nil)
	if got := manages(someone); got != "" {
		t.Fatal("after the groups were taken the player still manages", got)
	}
	var changes int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM net.group_changes WHERE user_id=$1 AND changed_by=$2`, player, boss).Scan(&changes); err != nil || changes != 3 {
		t.Fatal("recorded changes:", changes, err)
	}
}
