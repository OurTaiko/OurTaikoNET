package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ourtaiko.dev/net/internal/web"
)

// A suggestion test's players: an owner of the chart, two other players and a manager of the
// source (IsAdmin by the account center, as Config.Manager is not set).
const (
	suggestOwner   = "d46774d30dd13b92d9e536808da468a4"
	suggestPlayer  = "9b893bc6d9422c93536ff0df503b81e9"
	suggestPlayer2 = "1c3b2f8e9a7d4c6b5e0f1a2b3c4d5e6f"
	suggestAdmin   = "5f63f79d876d201a13f8710453636837"
	suggestChart   = "0123456789abcdef0123456789abcdef"
)

func suggestionServer(t *testing.T, src *Source) func(actor, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	pool := testDB(t, src)
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
		// The official source takes no suggestions: its routes are not there to find a chart.
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

const chartSuggestions = "/api/charts/" + suggestChart + "/category-suggestions"

func decision(category string, accept bool) string {
	b, _ := json.Marshal(map[string]any{"chartId": suggestChart, "category": category, "accept": accept})
	return string(b)
}

func TestFanmadeCategories(t *testing.T) {
	call := suggestionServer(t, Fanmade)
	body := wantStatus(t, call("", "GET", "/api/categories", ""), 200)
	var ids []string
	for _, item := range body["items"].([]any) {
		ids = append(ids, item.(map[string]any)["id"].(string))
	}
	if got := strings.Join(ids, ","); got != "game,virtual-singer,pop,classic,variety,anime,touhou,gekichumai,bemani,project-sekai,other-rhythm-games,bms,album,internet" {
		t.Fatal("Fanmade's fourteen categories in the order of their bits:", got)
	}
	// The widest bit fits the database.
	wantStatus(t, call(suggestOwner, "PATCH", "/api/charts/"+suggestChart, `{"categoryIds":["internet","touhou"]}`), 200)
	if got := categoriesOf(t, call); got != "internet,touhou" {
		t.Fatal(got)
	}
}

func TestOfficialTakesNoSuggestions(t *testing.T) {
	call := suggestionServer(t, Official)
	for _, w := range []*httptest.ResponseRecorder{
		call(suggestPlayer, "POST", chartSuggestions, `{"category":"variety"}`),
		call(suggestAdmin, "GET", "/api/category-suggestions", ""),
	} {
		if w.Code != 404 {
			t.Fatal("the official source has no suggestions", w.Code, w.Body.String())
		}
	}
}

func TestCategorySuggestions(t *testing.T) {
	call := suggestionServer(t, Fanmade)
	wantStatus(t, call("", "POST", chartSuggestions, `{"category":"touhou"}`), 401)
	wantStatus(t, call(suggestPlayer, "POST", chartSuggestions, `{"category":"nope"}`), 422)
	for _, who := range []string{suggestPlayer, suggestPlayer2} {
		if body := wantStatus(t, call(who, "POST", chartSuggestions, `{"category":"touhou"}`), 200); body["status"] != "pending" {
			t.Fatal("a suggestion waits", body)
		}
	}
	wantStatus(t, call(suggestPlayer, "POST", chartSuggestions, `{"category":"bms"}`), 200)
	wantStatus(t, call(suggestPlayer2, "POST", chartSuggestions, `{"category":"album"}`), 200)
	// Suggesting again changes nothing.
	wantStatus(t, call(suggestPlayer, "POST", chartSuggestions, `{"category":"touhou"}`), 200)
	mine := wantStatus(t, call(suggestPlayer, "GET", chartSuggestions, ""), 200)
	if mine["canEdit"] != false || mine["canReview"] != false || len(mine["suggestions"].([]any)) != 2 {
		t.Fatal("a player sees their own suggestions", mine)
	}
	if owner := wantStatus(t, call(suggestOwner, "GET", chartSuggestions, ""), 200); owner["canEdit"] != true || owner["canReview"] != false {
		t.Fatal("an owner sets the categories, and does not review", owner)
	}
	if admin := wantStatus(t, call(suggestAdmin, "GET", chartSuggestions, ""), 200); admin["canEdit"] != true || admin["canReview"] != true {
		t.Fatal("a manager sets and reviews", admin)
	}

	// Only the source's managers see and decide the queue.
	wantStatus(t, call(suggestPlayer, "GET", "/api/category-suggestions", ""), 403)
	wantStatus(t, call(suggestOwner, "POST", "/api/category-suggestions/decision", decision("touhou", true)), 403)
	queue := wantStatus(t, call(suggestAdmin, "GET", "/api/category-suggestions", ""), 200)
	items := queue["items"].([]any)
	if queue["total"].(float64) != 3 || len(items) != 3 {
		t.Fatal("suggestions are grouped by chart and category", queue)
	}
	first := items[0].(map[string]any)
	if first["category"] != "touhou" || first["count"].(float64) != 2 || len(first["suggesters"].([]any)) != 2 || first["chart"].(map[string]any)["title"] != "Song" {
		t.Fatal("the oldest group comes first, with its players and chart", first)
	}

	// Accepting puts the chart in the category, out of Variety that was its only one, and answers
	// every suggestion of it, and those made later.
	wantStatus(t, call(suggestAdmin, "POST", "/api/category-suggestions/decision", decision("touhou", true)), 200)
	if got := categoriesOf(t, call); got != "touhou" {
		t.Fatal("the chart left Variety for the accepted category", got)
	}
	wantStatus(t, call(suggestAdmin, "POST", "/api/category-suggestions/decision", decision("bms", true)), 200)
	if got := categoriesOf(t, call); got != "bms,touhou" {
		t.Fatal("a second category is added beside the first", got)
	}
	if late := wantStatus(t, call(suggestOwner, "POST", chartSuggestions, `{"category":"touhou"}`), 200); late["status"] != "accepted" {
		t.Fatal("a later suggestion of an accepted category is filed accepted", late)
	}
	accepted := wantStatus(t, call(suggestAdmin, "GET", "/api/category-suggestions?status=accepted", ""), 200)
	if accepted["total"].(float64) != 2 {
		t.Fatal("two groups were accepted", accepted)
	}
	var group map[string]any
	for _, item := range accepted["items"].([]any) {
		if item.(map[string]any)["category"] == "touhou" {
			group = item.(map[string]any)
		}
	}
	if group == nil || group["count"].(float64) != 3 || group["decidedBy"].(map[string]any)["id"] != suggestAdmin {
		t.Fatal("the accepted group holds every suggestion and who decided", group)
	}
	wantStatus(t, call(suggestAdmin, "POST", "/api/category-suggestions/decision", decision("touhou", false)), 409)

	// Rejecting answers every suggestion, and those made later.
	wantStatus(t, call(suggestAdmin, "POST", "/api/category-suggestions/decision", decision("album", false)), 200)
	if late := wantStatus(t, call(suggestPlayer, "POST", chartSuggestions, `{"category":"album"}`), 200); late["status"] != "rejected" {
		t.Fatal("a later suggestion of a rejected category is filed rejected", late)
	}
	if pending := wantStatus(t, call(suggestAdmin, "GET", "/api/category-suggestions", ""), 200); pending["total"].(float64) != 0 {
		t.Fatal("nothing waits", pending)
	}

	// The owner putting the chart in a category answers its waiting suggestions and forgets a
	// rejection.
	wantStatus(t, call(suggestPlayer, "POST", chartSuggestions, `{"category":"internet"}`), 200)
	wantStatus(t, call(suggestOwner, "PATCH", "/api/charts/"+suggestChart, `{"categoryIds":["album","bms","internet","touhou"]}`), 200)
	mine = wantStatus(t, call(suggestPlayer, "GET", chartSuggestions, ""), 200)
	for _, s := range mine["suggestions"].([]any) {
		if suggestion := s.(map[string]any); suggestion["category"] == "internet" && suggestion["status"] != "accepted" {
			t.Fatal("an edit that adds the category accepts its suggestion", suggestion)
		}
	}
	wantStatus(t, call(suggestOwner, "PATCH", "/api/charts/"+suggestChart, `{"categoryIds":["bms","touhou"]}`), 200)
	if again := wantStatus(t, call(suggestPlayer2, "POST", chartSuggestions, `{"category":"anime"}`), 200); again["status"] != "pending" {
		t.Fatal(again)
	}
	if again := wantStatus(t, call(suggestOwner, "POST", chartSuggestions, `{"category":"album"}`), 200); again["status"] != "pending" {
		t.Fatal("a category set and taken off again waits for a decision", again)
	}

	// A player takes back a suggestion while it waits, not after.
	wantStatus(t, call(suggestPlayer2, "DELETE", chartSuggestions+"/anime", ""), 204)
	wantStatus(t, call(suggestPlayer2, "DELETE", chartSuggestions+"/touhou", ""), 404)
}
