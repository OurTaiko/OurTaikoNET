package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"ourtaiko.dev/net/internal/web"
)

// The players of a test of what players do to a chart: an owner of the chart, two other players
// and a manager of the source (IsAdmin by the account center, as Config.Manager is not set).
const (
	suggestOwner   = "d46774d30dd13b92d9e536808da468a4"
	suggestPlayer  = "9b893bc6d9422c93536ff0df503b81e9"
	suggestPlayer2 = "1c3b2f8e9a7d4c6b5e0f1a2b3c4d5e6f"
	suggestAdmin   = "5f63f79d876d201a13f8710453636837"
	suggestChart   = "0123456789abcdef0123456789abcdef"
)

func suggestionServer(t *testing.T, src *Source) func(actor, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	return suggestionServerOn(t, testDB(t, src), src)
}

// suggestionServerOn is suggestionServer on a database the test has already put rows in.
func suggestionServerOn(t *testing.T, pool *pgxpool.Pool, src *Source) func(actor, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `INSERT INTO test_sso_users(id,username,password_hash,is_admin) VALUES
 ($1,'owner','unused',false),($2,'player','unused',false),($3,'player2','unused',false),($4,'admin','unused',true)`,
		suggestOwner, suggestPlayer, suggestPlayer2, suggestAdmin)
	if err != nil {
		t.Fatal(err)
	}
	tokens := map[string]string{}
	for i, id := range []string{suggestOwner, suggestPlayer, suggestPlayer2, suggestAdmin} {
		tokens[id] = strings.Repeat(string(rune('1'+i)), 64)
		if _, err = pool.Exec(ctx, `INSERT INTO test_sessions(token_hash,user_id,csrf_token,expires_at) VALUES($1,$2,'csrf',now()+interval '1 day')`, web.Hash(tokens[id]), id); err != nil {
			t.Fatal(err)
		}
	}
	if src.Official {
		// A fixture file's key is its digest there; no test of the official source needs the chart.
	} else if _, err = pool.Exec(ctx, `INSERT INTO charts(id,owner_ids,title,bpm,duration,tja_file,audio_file,category_flags) VALUES($1,ARRAY[$2],'Song',120,10,test_file('t','t.tja',repeat('a',64),1),test_file('a','a.ogg',repeat('b',64),1),16)`, suggestChart, suggestOwner); err != nil {
		t.Fatal(err)
	}
	handler := testServer(t, pool, Config{Source: src, Storage: t.TempDir(), Origin: "http://127.0.0.1:5173"}).Handler()
	return func(actor, method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		r.Header.Set("Origin", "http://127.0.0.1:5173")
		r.Header.Set("X-CSRF-Token", "csrf")
		if actor != "" {
			r.AddCookie(&http.Cookie{Name: "ourtaiko_session", Value: tokens[actor]})
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
}

func wantStatus(t *testing.T, w *httptest.ResponseRecorder, status int) map[string]any {
	t.Helper()
	if w.Code != status {
		t.Fatalf("wanted %d: %d %s", status, w.Code, w.Body.String())
	}
	var body map[string]any
	if w.Body.Len() > 0 {
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
	}
	return body
}

func categoriesOf(t *testing.T, call func(actor, method, path, body string) *httptest.ResponseRecorder) string {
	t.Helper()
	var c Chart
	w := call("", "GET", "/api/charts/"+suggestChart, "")
	if err := json.Unmarshal(w.Body.Bytes(), &c); err != nil {
		t.Fatal(w.Body.String(), err)
	}
	return strings.Join(c.CategoryIDs, ",")
}

func TestFanmadeCategories(t *testing.T) {
	call := suggestionServer(t, Fanmade)
	body := wantStatus(t, call("", "GET", "/api/categories", ""), 200)
	var ids []string
	for _, item := range body["items"].([]any) {
		ids = append(ids, item.(map[string]any)["id"].(string))
	}
	if got := strings.Join(ids, ","); got != "game,vocaloid,pop,classics,variety,anime,rhythm-game,kids" {
		t.Fatal("Fanmade's eight categories in the order of their bits:", got)
	}
	// The widest bit fits the database.
	wantStatus(t, call(suggestOwner, "PATCH", "/api/charts/"+suggestChart, `{"categoryIds":["kids","rhythm-game"]}`), 200)
	if got := categoriesOf(t, call); got != "kids,rhythm-game" {
		t.Fatal(got)
	}
}
