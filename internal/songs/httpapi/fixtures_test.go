package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
	"ourtaiko.dev/net/internal/account"
	"ourtaiko.dev/net/internal/netdb/netdbtest"
	"ourtaiko.dev/net/internal/web"
)

// What every test of this package is built from: a database of its own for one source
// (testDB), a server on it whose account center is a fake (testServer), and a client with the
// usual four players signed in (newTestClient).

// testDB gives a test its own ourtaiko_net, opened for the given source and dropped afterwards,
// and beside it what the fake account center of testServer knows: its accounts (test_sso_users; adding one
// also makes the player known to the site) and the tokens it has given out (test_sessions: a
// website session by the hash of its cookie, a game token by the hash of "game:" and the
// token). testServer opens a real website session for each of the former.
func testDB(t *testing.T, src *Source) *pgxpool.Pool {
	t.Helper()
	schema := "fanmade"
	if src.Official {
		schema = "official"
	}
	pool := netdbtest.New(t, schema)
	ctx := context.Background()
	var err error
	if _, err = pool.Exec(ctx, `
CREATE TABLE test_sso_users (id text PRIMARY KEY, username text NOT NULL UNIQUE, password_hash text NOT NULL, nickname text,
 email_verified_at timestamptz, is_admin boolean NOT NULL DEFAULT false);
CREATE FUNCTION test_sso_user() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.nickname IS NULL THEN NEW.nickname := NEW.username; END IF;
 INSERT INTO users(id) VALUES (NEW.id) ON CONFLICT DO NOTHING;
 RETURN NEW;
END $$;
CREATE TRIGGER test_sso_user BEFORE INSERT ON test_sso_users FOR EACH ROW EXECUTE FUNCTION test_sso_user();
CREATE TABLE test_sessions (token_hash text PRIMARY KEY, user_id text NOT NULL, csrf_token text NOT NULL, expires_at timestamptz NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	// Fixtures register a file under a key and reference its ID. (On the official source a
	// key has to be the file's digest and extension.)
	if _, err = pool.Exec(ctx, `CREATE FUNCTION test_file(key text, name text, digest text, size bigint) RETURNS bigint LANGUAGE sql AS $$
 INSERT INTO chart_resources(storage_key,original_filename,sha256,byte_size,extension)
 VALUES(key,name,digest,size,COALESCE(lower(substring(name from '\.([A-Za-z0-9]+)$')),'bin'))
 ON CONFLICT(storage_key) DO UPDATE SET storage_key=EXCLUDED.storage_key RETURNING id $$`); err != nil {
		t.Fatal(err)
	}
	// test_slot adds a chart's single block of a course, or finds it again. Its content hash
	// is made up from the chart and course: what a real upload gets from TJAParser.
	if _, err = pool.Exec(ctx, `CREATE FUNCTION test_slot(chart text, slot_course text, slot_level integer DEFAULT 5, slot_maker text DEFAULT '') RETURNS bigint LANGUAGE sql AS $$
 INSERT INTO chart_difficulties(chart_id,course,side,level,maker,content_hash) VALUES(chart,slot_course,'single',slot_level,slot_maker,'v1:'||encode(sha256((chart||'/'||slot_course)::bytea),'hex'))
 ON CONFLICT(chart_id,course,side) DO UPDATE SET level=chart_difficulties.level RETURNING id $$`); err != nil {
		t.Fatal(err)
	}
	return pool
}

// testServer makes a server, of the source cfg names, whose account center is a fake one over
// real HTTP. The fake knows the accounts and tokens the fixtures put in test_sso_users and
// test_sessions (testDB); every website session among the latter becomes a real session of
// the site.
func testServer(t *testing.T, pool *pgxpool.Pool, cfg Config) *Server {
	t.Helper()
	ctx := context.Background()
	var e error
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Service-ID") != "fanmade" || r.Header.Get("X-Service-Key") != strings.Repeat("a", 64) || r.Header.Get("Origin") != "" {
			w.WriteHeader(401)
			return
		}
		var body map[string]json.RawMessage
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			w.WriteHeader(400)
			return
		}
		str := func(k string) string { var v string; _ = json.Unmarshal(body[k], &v); return v }
		user := func(where string, args ...any) (User, error) {
			var u User
			e := pool.QueryRow(r.Context(), `SELECT id,username,nickname,email_verified_at IS NOT NULL,is_admin FROM test_sso_users WHERE `+where, args...).Scan(&u.ID, &u.Username, &u.Nickname, &u.EmailVerified, &u.IsAdmin)
			return u, e
		}
		switch r.URL.Path {
		case "/internal/v1/game/login":
			u, e := user("username=$1", str("username"))
			var password string
			_ = pool.QueryRow(r.Context(), `SELECT password_hash FROM test_sso_users WHERE id=$1`, u.ID).Scan(&password)
			if e != nil || bcrypt.CompareHashAndPassword([]byte(password), []byte(str("password"))) != nil {
				w.WriteHeader(401)
				return
			}
			token := web.NewID() + web.NewID()
			_, e = pool.Exec(r.Context(), `INSERT INTO test_sessions(token_hash,user_id,csrf_token,expires_at) VALUES($1,$2,'',now()+interval '1 day')`, web.Hash("game:"+token), u.ID)
			if e != nil {
				w.WriteHeader(503)
				return
			}
			web.Respond(w, 200, map[string]any{"user": u, "accessToken": token, "expiresIn": 86400})
		case "/internal/v1/game/introspect", "/internal/v1/web/introspect":
			var id string
			if strings.Contains(r.URL.Path, "/game/") {
				_ = pool.QueryRow(r.Context(), `SELECT user_id FROM test_sessions WHERE token_hash=$1 AND expires_at>now()`, web.Hash("game:"+str("accessToken"))).Scan(&id)
			} else {
				v, ok := strings.CutPrefix(str("accessToken"), "web:")
				if ok {
					id = v
				}
			}
			u, e := user("id=$1", id)
			if e != nil {
				w.WriteHeader(401)
				return
			}
			web.Respond(w, 200, map[string]any{"user": u})
		case "/internal/v1/web/revoke":
			web.Respond(w, 200, map[string]bool{"ok": true})
		case "/internal/v1/users/lookup":
			var ids []string
			_ = json.Unmarshal(body["userIds"], &ids)
			users := []map[string]string{}
			for _, id := range ids {
				u, e := user("id=$1", id)
				if e == nil {
					users = append(users, map[string]string{"id": u.ID, "nickname": u.Nickname})
				}
			}
			web.Respond(w, 200, map[string]any{"users": users})
		case "/internal/v1/users/search":
			ids := []string{}
			rows, e := pool.Query(r.Context(), `SELECT id FROM test_sso_users WHERE nickname ILIKE '%'||$1||'%'`, str("query"))
			if e != nil {
				w.WriteHeader(503)
				return
			}
			defer rows.Close()
			for rows.Next() {
				var id string
				_ = rows.Scan(&id)
				ids = append(ids, id)
			}
			web.Respond(w, 200, map[string]any{"userIds": ids, "next": ""})
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(remote.Close)
	cfg.SSO, e = account.New(account.Config{Issuer: remote.URL, ServiceID: "fanmade", ServiceKey: strings.Repeat("a", 64), ClientID: "web", ClientSecret: "secret", RedirectURL: "http://127.0.0.1/callback", EncryptionKey: strings.Repeat("b", 64)})
	if e != nil {
		t.Fatal(e)
	}
	rows, e := pool.Query(ctx, `SELECT token_hash,user_id,csrf_token,expires_at FROM test_sessions`)
	if e != nil {
		t.Fatal(e)
	}
	type old struct {
		hash, id, csrf string
		expires        any
	}
	items := []old{}
	for rows.Next() {
		values, e := rows.Values()
		if e != nil {
			t.Fatal(e)
		}
		items = append(items, old{values[0].(string), values[1].(string), values[2].(string), values[3]})
	}
	rows.Close()
	for _, v := range items {
		_, e = pool.Exec(ctx, `INSERT INTO sessions(token_hash,user_id,csrf_token,expires_at,upstream_token) VALUES($1,$2,$3,$4,$5)`, v.hash, v.id, v.csrf, v.expires, cfg.SSO.Seal("web:"+v.id, v.hash))
		if e != nil {
			t.Fatal(e)
		}
	}
	return New(pool, cfg)
}

func seedWebSession(t *testing.T, s *Server, id string) string {
	t.Helper()
	token := web.NewID() + web.NewID()
	digest := web.Hash(token)
	_, e := s.DB.Exec(context.Background(), `INSERT INTO sessions(token_hash,user_id,csrf_token,expires_at,upstream_token) VALUES($1,$2,'csrf',now()+interval '1 day',$3)`, digest, id, s.Config.SSO.Seal("web:"+id, digest))
	if e != nil {
		t.Fatal(e)
	}
	return token
}

type testUser struct {
	ID, Nickname string
	Admin        bool
}

// testUsers are the four players of a test client; each one's name is its key. admin is the
// account center's administrator.
var testUsers = map[string]testUser{
	"owner": {"d46774d30dd13b92d9e536808da468a4", "owner", false},
	"alice": {"9b893bc6d9422c93536ff0df503b81e9", "alice", false},
	"bob":   {"5f0c4a0b0e7b4d3c9a6f1e2d3c4b5a69", "bob", false},
	"admin": {"0a1b2c3d4e5f60718293a4b5c6d7e8f9", "admin", true},
}

type testClient struct {
	t       *testing.T
	handler http.Handler
	tokens  map[string]string
	n       *atomic.Int64
	// cookie names the server's session cookie.
	cookie string
}

// newTestClient gives the fake account center the four testUsers and signs each of them in
// with a website session.
func newTestClient(t *testing.T, s *Server) testClient {
	t.Helper()
	c := testClient{t, s.Handler(), map[string]string{}, &atomic.Int64{}, s.sessionCookie()}
	for name, u := range testUsers {
		if _, err := s.DB.Exec(context.Background(), `INSERT INTO test_sso_users(id,username,password_hash,is_admin) VALUES($1,$2,'unused',$3)`, u.ID, u.Nickname, u.Admin); err != nil {
			t.Fatal(err)
		}
		c.tokens[name] = seedWebSession(t, s, u.ID)
	}
	return c
}

// do sends a request as the given user ("" for a guest) and decodes the JSON reply.
func (c testClient) do(user, method, path, body string, out any) int {
	c.t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	// A fresh client address per request keeps the per-address write limit out of the way.
	r.RemoteAddr = fmt.Sprintf("10.%d.%d.%d:1234", c.n.Load()>>16&255, c.n.Load()>>8&255, c.n.Add(1)&255)
	if method != "GET" {
		r.Header.Set("Origin", "http://localhost")
		r.Header.Set("X-CSRF-Token", "csrf")
	}
	if user != "" {
		r.AddCookie(&http.Cookie{Name: c.cookie, Value: c.tokens[user]})
	}
	w := httptest.NewRecorder()
	c.handler.ServeHTTP(w, r)
	if out != nil && w.Code < 300 {
		if e := json.Unmarshal(w.Body.Bytes(), out); e != nil {
			c.t.Fatal(path, e, w.Body.String())
		}
	}
	return w.Code
}

// raw sends a GET as the given user and returns the status and body unparsed.
func (c testClient) raw(user, path string) (int, []byte) {
	c.t.Helper()
	r := httptest.NewRequest("GET", path, nil)
	if user != "" {
		r.AddCookie(&http.Cookie{Name: c.cookie, Value: c.tokens[user]})
	}
	w := httptest.NewRecorder()
	c.handler.ServeHTTP(w, r)
	return w.Code, w.Body.Bytes()
}

// storedCover is what the cover file of a chart or playlist (table "charts" or "playlists")
// holds in the store, read there because no API route serves covers; nil when it has none.
func storedCover(t *testing.T, s *Server, table, id string) []byte {
	t.Helper()
	ctx := context.Background()
	var key string
	err := s.DB.QueryRow(ctx, `SELECT r.storage_key FROM `+table+` x JOIN chart_resources r ON r.id=x.cover_file WHERE x.id=$1`, id).Scan(&key)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	f, err := s.Config.Objects.Open(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
