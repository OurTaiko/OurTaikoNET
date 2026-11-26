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

// A tag test's players: an owner of the chart, two other players, a tag manager and a manager of
// the source (IsAdmin by the account center, as Config.Manager is not set).
const (
	tagOwner   = "d46774d30dd13b92d9e536808da468a4"
	tagPlayer  = "9b893bc6d9422c93536ff0df503b81e9"
	tagPlayer2 = "1c3b2f8e9a7d4c6b5e0f1a2b3c4d5e6f"
	tagKeeper  = "7a0e6d1c2b3f4a5e6d7c8b9a0f1e2d3c"
	tagAdmin   = "5f63f79d876d201a13f8710453636837"
)

func tagServer(t *testing.T) (http.Handler, func(actor, method, path, body string) *httptest.ResponseRecorder) {
	t.Helper()
	pool := testDB(t, Fanmade)
	ctx := context.Background()
	_, err := pool.Exec(ctx, `INSERT INTO test_sso_users(id,username,password_hash,is_admin) VALUES
 ($1,'owner','unused',false),($2,'player','unused',false),($3,'player2','unused',false),($4,'keeper','unused',false),($5,'admin','unused',true)`,
		tagOwner, tagPlayer, tagPlayer2, tagKeeper, tagAdmin)
	if err != nil {
		t.Fatal(err)
	}
	tokens := map[string]string{}
	for i, id := range []string{tagOwner, tagPlayer, tagPlayer2, tagKeeper, tagAdmin} {
		tokens[id] = strings.Repeat(string(rune('1'+i)), 64)
		if _, err = pool.Exec(ctx, `INSERT INTO test_sessions(token_hash,user_id,csrf_token,expires_at) VALUES($1,$2,'csrf',now()+interval '1 day')`, web.Hash(tokens[id]), id); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"0123456789abcdef0123456789abcdef", "fedcba9876543210fedcba9876543210"} {
		if _, err = pool.Exec(ctx, `INSERT INTO charts(id,owner_ids,title,bpm,duration,tja_file,audio_file) VALUES($1,ARRAY[$2],'Song',120,10,test_file($1||'t','t.tja',repeat('a',64),1),test_file($1||'a','a.ogg',repeat('b',64),1))`, id, tagOwner); err != nil {
			t.Fatal(err)
		}
	}
	app := testServer(t, pool, Config{Storage: t.TempDir(), Origin: "http://127.0.0.1:5173",
		TagManager: func(_ context.Context, user string) (bool, error) { return user == tagKeeper, nil }})
	handler := app.Handler()
	return handler, func(actor, method, path, body string) *httptest.ResponseRecorder {
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

const tagChart = "/api/charts/0123456789abcdef0123456789abcdef"

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

func tagsOfChart(t *testing.T, call func(actor, method, path, body string) *httptest.ResponseRecorder) string {
	t.Helper()
	var c Chart
	w := call("", "GET", tagChart, "")
	if err := json.Unmarshal(w.Body.Bytes(), &c); err != nil {
		t.Fatal(w.Body.String(), err)
	}
	return strings.Join(c.Tags, ",")
}

func TestTagCatalog(t *testing.T) {
	_, call := tagServer(t)
	body := wantStatus(t, call("", "GET", "/api/tags", ""), 200)
	items := body["items"].([]any)
	if len(items) != 12 {
		t.Fatalf("12 tags come with the site: %v", items)
	}
	first := items[0].(map[string]any)
	if first["id"] != "gekichumai" || first["names"].(map[string]any)["zh-Hans"] != "击中萌" || first["chartCount"].(float64) != 0 {
		t.Fatal("the catalog is in its order, with names and counts", first)
	}
}

func TestSetChartTags(t *testing.T) {
	_, call := tagServer(t)
	// A tag manager, the source's manager and the chart's owners set tags; anyone else suggests.
	wantStatus(t, call("", "PUT", tagChart+"/tags", `{"tags":["touhou"]}`), 401)
	wantStatus(t, call(tagPlayer, "PUT", tagChart+"/tags", `{"tags":["touhou"]}`), 403)
	wantStatus(t, call(tagOwner, "PUT", "/api/charts/fedcba9876543210fedcba9876543210/tags", `{"tags":["touhou"]}`), 200)
	wantStatus(t, call(tagKeeper, "PUT", tagChart+"/tags", `{"tags":["touhou","nope"]}`), 422)
	body := wantStatus(t, call(tagKeeper, "PUT", tagChart+"/tags", `{"tags":["bms","touhou","touhou"]}`), 200)
	if tags := body["tags"].([]any); len(tags) != 2 || tags[0] != "touhou" || tags[1] != "bms" {
		t.Fatal("tags are answered in the catalog's order, once each", tags)
	}
	if got := tagsOfChart(t, call); got != "touhou,bms" {
		t.Fatal("the chart carries its tags", got)
	}
	wantStatus(t, call(tagAdmin, "PUT", tagChart+"/tags", `{"tags":["anime"]}`), 200)
	if got := tagsOfChart(t, call); got != "anime" {
		t.Fatal("the source's manager sets exactly the tags sent", got)
	}
	// The list filters by tag; an unknown one is a mistake.
	list := wantStatus(t, call("", "GET", "/api/charts?tag=anime", ""), 200)
	if list["total"].(float64) != 1 {
		t.Fatal("one chart has the tag", list)
	}
	list = wantStatus(t, call("", "GET", "/api/charts?tag=touhou", ""), 200)
	if items := list["items"].([]any); len(items) != 1 || items[0].(map[string]any)["id"] != "fedcba9876543210fedcba9876543210" {
		t.Fatal("only the owner's other chart has the tag", list)
	}
	wantStatus(t, call("", "GET", "/api/charts?tag=bms", ""), 200)
	wantStatus(t, call("", "GET", "/api/charts?tag=nope", ""), 400)
	catalog := wantStatus(t, call("", "GET", "/api/tags", ""), 200)
	for _, item := range catalog["items"].([]any) {
		tag := item.(map[string]any)
		if want := map[bool]float64{true: 1, false: 0}[tag["id"] == "anime" || tag["id"] == "touhou"]; tag["chartCount"].(float64) != want {
			t.Fatal("the catalog counts the charts of each tag", tag)
		}
	}
}

func TestTagSuggestions(t *testing.T) {
	_, call := tagServer(t)
	wantStatus(t, call("", "POST", tagChart+"/tag-suggestions", `{"tag":"touhou"}`), 401)
	wantStatus(t, call(tagPlayer, "POST", tagChart+"/tag-suggestions", `{"tag":"nope"}`), 422)
	for _, who := range []string{tagPlayer, tagPlayer2, tagOwner} { // an owner may suggest too
		if body := wantStatus(t, call(who, "POST", tagChart+"/tag-suggestions", `{"tag":"touhou"}`), 200); body["status"] != "pending" {
			t.Fatal("a suggestion waits", body)
		}
	}
	wantStatus(t, call(tagPlayer, "POST", tagChart+"/tag-suggestions", `{"tag":"bms"}`), 200)
	// Suggesting again changes nothing.
	wantStatus(t, call(tagPlayer, "POST", tagChart+"/tag-suggestions", `{"tag":"touhou"}`), 200)
	mine := wantStatus(t, call(tagPlayer, "GET", tagChart+"/tags", ""), 200)
	if mine["canEdit"] != false || mine["canReview"] != false || len(mine["suggestions"].([]any)) != 2 {
		t.Fatal("a player sees their own suggestions", mine)
	}
	if keeper := wantStatus(t, call(tagKeeper, "GET", tagChart+"/tags", ""), 200); keeper["canEdit"] != true || keeper["canReview"] != true {
		t.Fatal("a tag manager may edit and review", keeper)
	}
	if admin := wantStatus(t, call(tagAdmin, "GET", tagChart+"/tags", ""), 200); admin["canEdit"] != true || admin["canReview"] != true {
		t.Fatal("the source's manager may edit and review", admin)
	}
	if owner := wantStatus(t, call(tagOwner, "GET", tagChart+"/tags", ""), 200); owner["canEdit"] != true || owner["canReview"] != false {
		t.Fatal("an owner may edit their chart's tags, not review", owner)
	}

	// Tag managers and the source's managers see and decide the queue; owners and players do not.
	wantStatus(t, call(tagPlayer, "GET", "/api/tag-suggestions", ""), 403)
	wantStatus(t, call(tagOwner, "POST", "/api/tag-suggestions/decision", `{"chartId":"0123456789abcdef0123456789abcdef","tag":"touhou","accept":true}`), 403)
	wantStatus(t, call(tagAdmin, "GET", "/api/tag-suggestions", ""), 200)
	queue := wantStatus(t, call(tagKeeper, "GET", "/api/tag-suggestions", ""), 200)
	items := queue["items"].([]any)
	if queue["total"].(float64) != 2 || len(items) != 2 {
		t.Fatal("suggestions are grouped by chart and tag", queue)
	}
	touhou := items[0].(map[string]any)
	if touhou["tag"] != "touhou" || touhou["count"].(float64) != 3 || len(touhou["suggesters"].([]any)) != 3 || touhou["chart"].(map[string]any)["title"] != "Song" {
		t.Fatal("the oldest group comes first, with its players and chart", touhou)
	}

	// Accepting puts the tag on and answers every suggestion of it, and those made later.
	wantStatus(t, call(tagKeeper, "POST", "/api/tag-suggestions/decision", `{"chartId":"0123456789abcdef0123456789abcdef","tag":"touhou","accept":true}`), 200)
	if got := tagsOfChart(t, call); got != "touhou" {
		t.Fatal("an accepted tag is on the chart", got)
	}
	if late := wantStatus(t, call(tagKeeper, "POST", tagChart+"/tag-suggestions", `{"tag":"touhou"}`), 200); late["status"] != "accepted" {
		t.Fatal("a later suggestion of an accepted tag is filed accepted", late)
	}
	accepted := wantStatus(t, call(tagKeeper, "GET", "/api/tag-suggestions?status=accepted", ""), 200)
	group := accepted["items"].([]any)[0].(map[string]any)
	if group["count"].(float64) != 4 || group["decidedBy"].(map[string]any)["id"] != tagKeeper {
		t.Fatal("the accepted group holds every suggestion and who decided", group)
	}
	// A rejected tag cannot be the chart's while it has it.
	wantStatus(t, call(tagKeeper, "POST", "/api/tag-suggestions/decision", `{"chartId":"0123456789abcdef0123456789abcdef","tag":"touhou","accept":false}`), 409)

	// Rejecting answers every suggestion, and those made later.
	wantStatus(t, call(tagAdmin, "POST", "/api/tag-suggestions/decision", `{"chartId":"0123456789abcdef0123456789abcdef","tag":"bms","accept":false}`), 200)
	if late := wantStatus(t, call(tagPlayer2, "POST", tagChart+"/tag-suggestions", `{"tag":"bms"}`), 200); late["status"] != "rejected" {
		t.Fatal("a later suggestion of a rejected tag is filed rejected", late)
	}
	if pending := wantStatus(t, call(tagKeeper, "GET", "/api/tag-suggestions", ""), 200); pending["total"].(float64) != 0 {
		t.Fatal("nothing waits", pending)
	}
	// Setting a rejected tag directly forgets the rejection.
	wantStatus(t, call(tagKeeper, "PUT", tagChart+"/tags", `{"tags":["touhou","bms"]}`), 200)
	wantStatus(t, call(tagKeeper, "PUT", tagChart+"/tags", `{"tags":["touhou"]}`), 200)
	if again := wantStatus(t, call(tagOwner, "POST", tagChart+"/tag-suggestions", `{"tag":"bms"}`), 200); again["status"] != "pending" {
		t.Fatal("a tag set and taken off again waits for a decision", again)
	}

	// A player takes back a suggestion while it waits, not after.
	wantStatus(t, call(tagOwner, "DELETE", tagChart+"/tag-suggestions/bms", ""), 204)
	wantStatus(t, call(tagOwner, "DELETE", tagChart+"/tag-suggestions/touhou", ""), 404)
}
