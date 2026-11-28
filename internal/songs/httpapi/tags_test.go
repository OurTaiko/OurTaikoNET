package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"ourtaiko.dev/net/internal/web"
)

const (
	tagChart2 = "fedcba9876543210fedcba9876543210"
	chartTags = "/api/charts/" + suggestChart + "/tags"
)

type tagCall = func(actor, method, path, body string) *httptest.ResponseRecorder

// tagServer is suggestionServer with a second chart, so a tag can be used on more than one.
func tagServer(t *testing.T) tagCall {
	t.Helper()
	return tagServerOn(t, testDB(t, Fanmade))
}

func tagServerOn(t *testing.T, pool *pgxpool.Pool) tagCall {
	t.Helper()
	call := suggestionServerOn(t, pool, Fanmade)
	if _, err := pool.Exec(context.Background(), `INSERT INTO charts(id,owner_ids,title,bpm,duration,tja_file,audio_file,category_flags) VALUES($1,ARRAY[$2],'Other',120,10,test_file('t2','t2.tja',repeat('c',64),1),test_file('a2','a2.ogg',repeat('d',64),1),16)`, tagChart2, suggestOwner); err != nil {
		t.Fatal(err)
	}
	return call
}

// shownTags reads the chart's tags as "name:count:status", mine marked with *.
func shownTags(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Tags []ChartTag `json:"tags"`
	}
	if w.Code != 200 {
		t.Fatalf("wanted 200: %d %s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(w.Body.String(), err)
	}
	parts := []string{}
	for _, tag := range body.Tags {
		mine := ""
		if tag.Mine {
			mine = "*"
		}
		parts = append(parts, fmt.Sprintf("%s:%d:%s%s", tag.Names["en"], tag.Count, tag.Status, mine))
	}
	return strings.Join(parts, ",")
}

func catalogTag(t *testing.T, call tagCall, name string) (Tag, bool) {
	t.Helper()
	var body struct {
		Items []Tag `json:"items"`
	}
	w := call("", "GET", "/api/tags", "")
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(w.Body.String(), err)
	}
	for _, tag := range body.Items {
		if tag.Names["en"] == name {
			return tag, true
		}
	}
	return Tag{}, false
}

func TestTagCatalogDropsWhatCategoriesSay(t *testing.T) {
	call := tagServer(t)
	var body struct {
		Items []Tag `json:"items"`
	}
	if err := json.Unmarshal(call("", "GET", "/api/tags", "").Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, tag := range body.Items {
		if tag.Status != tagOfficial {
			t.Fatal("the site's own tags are official:", tag)
		}
		ids = append(ids, tag.ID)
	}
	if got := strings.Join(ids, ","); got != "gekichumai,touhou,bms,bemani,album,internet,project-sekai" {
		t.Fatal(got)
	}
}

func TestPlayersTagAChart(t *testing.T) {
	call := tagServer(t)
	wantStatus(t, call("", "PUT", chartTags+"/touhou", `{"vote":1}`), 401)
	wantStatus(t, call(suggestPlayer, "PUT", chartTags+"/nothing", `{"vote":1}`), 404)
	wantStatus(t, call(suggestPlayer, "PUT", chartTags+"/touhou", `{"vote":2}`), 400)

	if got := shownTags(t, call(suggestPlayer, "PUT", chartTags+"/touhou", `{"vote":1}`)); got != "TOUHOU PROJECT:1:official*" {
		t.Fatal(got)
	}
	// Voting again changes nothing; another player's vote counts beside it.
	call(suggestPlayer, "PUT", chartTags+"/touhou", `{"vote":1}`)
	if got := shownTags(t, call(suggestPlayer2, "PUT", chartTags+"/touhou", `{"vote":1}`)); got != "TOUHOU PROJECT:2:official*" {
		t.Fatal(got)
	}
	// A name in any of the tag's languages, whatever the case, is that tag.
	if got := shownTags(t, call(suggestOwner, "POST", chartTags, `{"name":" 东方 project "}`)); got != "TOUHOU PROJECT:3:official*" {
		t.Fatal(got)
	}
	// A guest sees the votes, none of them theirs.
	if got := shownTags(t, call("", "GET", chartTags, "")); got != "TOUHOU PROJECT:3:official" {
		t.Fatal(got)
	}
	// Taking a vote back is the player's own alone.
	if got := shownTags(t, call(suggestPlayer, "PUT", chartTags+"/touhou", `{"vote":0}`)); got != "TOUHOU PROJECT:2:official" {
		t.Fatal(got)
	}
	var c Chart
	if err := json.Unmarshal(call("", "GET", "/api/charts/"+suggestChart, "").Body.Bytes(), &c); err != nil || strings.Join(c.Tags, ",") != "touhou" {
		t.Fatal("the chart names its official tags:", c.Tags, err)
	}
	if tag, _ := catalogTag(t, call, "TOUHOU PROJECT"); tag.ChartCount != 1 {
		t.Fatal("the catalog counts the charts with the tag:", tag)
	}
	list := wantStatus(t, call("", "GET", "/api/charts?tag=touhou", ""), 200)
	if list["total"].(float64) != 1 {
		t.Fatal(list)
	}
	if list = wantStatus(t, call("", "GET", "/api/charts?tag=bms", ""), 200); list["total"].(float64) != 0 {
		t.Fatal(list)
	}
	wantStatus(t, call("", "GET", "/api/charts?tag=nothing", ""), 400)
}

func TestPendingTagBecomesOfficialAtThreePlayers(t *testing.T) {
	call := tagServer(t)
	wantStatus(t, call(suggestPlayer, "POST", chartTags, `{"name":""}`), 422)
	wantStatus(t, call(suggestPlayer, "POST", chartTags, `{"name":"`+strings.Repeat("字", 25)+`"}`), 422)

	if got := shownTags(t, call(suggestPlayer, "POST", chartTags, `{"name":"Denpa"}`)); got != "Denpa:1:pending*" {
		t.Fatal(got)
	}
	tag, ok := catalogTag(t, call, "Denpa")
	if !ok || tag.Status != tagPending {
		t.Fatal("a pending tag is offered with the others:", tag, ok)
	}
	// The chart names only its official tags, and the list does not filter by a pending one.
	var c Chart
	if err := json.Unmarshal(call("", "GET", "/api/charts/"+suggestChart, "").Body.Bytes(), &c); err != nil || len(c.Tags) != 0 {
		t.Fatal(c.Tags, err)
	}
	wantStatus(t, call("", "GET", "/api/charts?tag="+tag.ID, ""), 400)

	// The same name again, whatever the case, is the same tag.
	if got := shownTags(t, call(suggestPlayer2, "POST", chartTags, `{"name":"denpa"}`)); got != "Denpa:2:pending*" {
		t.Fatal(got)
	}
	// The third player uses it on another chart: it is counted over the site, not per chart.
	if got := shownTags(t, call(suggestOwner, "PUT", "/api/charts/"+tagChart2+"/tags/"+tag.ID, `{"vote":1}`)); got != "Denpa:1:official*" {
		t.Fatal(got)
	}
	// Official stays official whoever leaves.
	call(suggestOwner, "PUT", "/api/charts/"+tagChart2+"/tags/"+tag.ID, `{"vote":0}`)
	call(suggestPlayer2, "PUT", chartTags+"/"+tag.ID, `{"vote":0}`)
	if got := shownTags(t, call(suggestPlayer, "PUT", chartTags+"/"+tag.ID, `{"vote":0}`)); got != "" {
		t.Fatal(got)
	}
	if tag, ok = catalogTag(t, call, "Denpa"); !ok || tag.Status != tagOfficial || tag.ChartCount != 0 {
		t.Fatal(tag, ok)
	}
}

func TestPendingTagNobodyUsesIsDeleted(t *testing.T) {
	call := tagServer(t)
	call(suggestPlayer, "POST", chartTags, `{"name":"Lonely"}`)
	tag, ok := catalogTag(t, call, "Lonely")
	if !ok {
		t.Fatal("made")
	}
	call(suggestPlayer, "PUT", chartTags+"/"+tag.ID, `{"vote":0}`)
	if _, ok = catalogTag(t, call, "Lonely"); ok {
		t.Fatal("a pending tag without a vote is gone")
	}
}

func TestPendingTagLimit(t *testing.T) {
	pool := testDB(t, Fanmade)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO net.users(id) VALUES($1) ON CONFLICT DO NOTHING`, suggestPlayer); err != nil {
		t.Fatal(err)
	}
	for i := range maxPendingTags {
		if _, err := pool.Exec(ctx, `INSERT INTO net.tags(id,names,position,status,created_by) VALUES($1,jsonb_build_object('en',$2::text),$3,'pending',$4)`,
			web.NewID(), fmt.Sprint("Filler ", i), 1000+i, suggestPlayer); err != nil {
			t.Fatal(err)
		}
	}
	call := tagServerOn(t, pool)
	wantStatus(t, call(suggestPlayer, "POST", chartTags, `{"name":"One more"}`), 409)
	// A tag that is there is no new one; a manager's are official and not counted.
	wantStatus(t, call(suggestPlayer, "POST", chartTags, `{"name":"filler 3"}`), 200)
	if got := shownTags(t, call(suggestAdmin, "POST", chartTags, `{"name":"Staff pick"}`)); got != "Staff pick:1:official*,Filler 3:1:pending" {
		t.Fatal(got)
	}
}

func TestTagReports(t *testing.T) {
	call := tagServer(t)
	wantStatus(t, call(suggestPlayer2, "POST", chartTags+"/touhou/reports", `{"reason":"not there"}`), 404)
	call(suggestPlayer, "PUT", chartTags+"/touhou", `{"vote":1}`)
	call(suggestOwner, "PUT", chartTags+"/touhou", `{"vote":1}`)
	wantStatus(t, call(suggestPlayer2, "POST", chartTags+"/touhou/reports", `{"reason":"first words"}`), 204)
	wantStatus(t, call(suggestPlayer2, "POST", chartTags+"/touhou/reports", `{"reason":"This is not Touhou"}`), 204)
	state := wantStatus(t, call(suggestPlayer2, "GET", chartTags, ""), 200)
	if fmt.Sprint(state["reported"]) != "[touhou]" || state["canReview"] != false {
		t.Fatal(state)
	}
	wantStatus(t, call(suggestPlayer, "GET", "/api/tag-reports", ""), 403)
	wantStatus(t, call(suggestPlayer, "POST", "/api/tag-reports/decision", `{"chartId":"`+suggestChart+`","tag":"touhou","uphold":true}`), 403)

	queue := wantStatus(t, call(suggestAdmin, "GET", "/api/tag-reports", ""), 200)
	items := queue["items"].([]any)
	if len(items) != 1 {
		t.Fatal(queue)
	}
	group := items[0].(map[string]any)
	reporter := group["reporters"].([]any)[0].(map[string]any)
	if group["count"].(float64) != 1 || group["votes"].(float64) != 2 || reporter["reason"] != "This is not Touhou" || group["tag"].(map[string]any)["id"] != "touhou" {
		t.Fatal(group)
	}

	// Dismissed: the chart keeps the tag, and the same player's report stays answered.
	wantStatus(t, call(suggestAdmin, "POST", "/api/tag-reports/decision", `{"chartId":"`+suggestChart+`","tag":"touhou","uphold":false}`), 200)
	if got := shownTags(t, call("", "GET", chartTags, "")); got != "TOUHOU PROJECT:2:official" {
		t.Fatal(got)
	}
	wantStatus(t, call(suggestPlayer2, "POST", chartTags+"/touhou/reports", `{"reason":"again"}`), 204)
	if queue = wantStatus(t, call(suggestAdmin, "GET", "/api/tag-reports", ""), 200); queue["total"].(float64) != 0 {
		t.Fatal("a dismissed report is not reopened by its player:", queue)
	}

	// Upheld: every vote is withdrawn, and no player's vote puts the tag back.
	wantStatus(t, call(suggestAdmin, "POST", "/api/tag-reports/decision", `{"chartId":"`+suggestChart+`","tag":"touhou","uphold":true}`), 200)
	if got := shownTags(t, call("", "GET", chartTags, "")); got != "" {
		t.Fatal(got)
	}
	wantStatus(t, call(suggestPlayer, "PUT", chartTags+"/touhou", `{"vote":1}`), 409)
	wantStatus(t, call(suggestPlayer, "POST", chartTags, `{"name":"touhou project"}`), 409)
	// A manager's vote does, and lifts the decision for everyone.
	call(suggestAdmin, "PUT", chartTags+"/touhou", `{"vote":1}`)
	if got := shownTags(t, call(suggestPlayer, "PUT", chartTags+"/touhou", `{"vote":1}`)); got != "TOUHOU PROJECT:2:official*" {
		t.Fatal(got)
	}
}

func TestUpheldReportDeletesAPendingTagNobodyElseUses(t *testing.T) {
	call := tagServer(t)
	call(suggestPlayer, "POST", chartTags, `{"name":"Rude word"}`)
	tag, _ := catalogTag(t, call, "Rude word")
	wantStatus(t, call(suggestPlayer2, "POST", chartTags+"/"+tag.ID+"/reports", `{"reason":""}`), 204)
	wantStatus(t, call(suggestAdmin, "POST", "/api/tag-reports/decision", `{"chartId":"`+suggestChart+`","tag":"`+tag.ID+`","uphold":true}`), 200)
	if _, ok := catalogTag(t, call, "Rude word"); ok {
		t.Fatal("gone with its last vote")
	}
}

func TestManagersKeepTheCatalog(t *testing.T) {
	call := tagServer(t)
	wantStatus(t, call(suggestPlayer, "POST", "/api/tags", `{"names":{"en":"Mine"}}`), 403)
	wantStatus(t, call(suggestAdmin, "POST", "/api/tags", `{"names":{"zh-Hans":"没有英文名"}}`), 422)
	wantStatus(t, call(suggestAdmin, "POST", "/api/tags", `{"names":{"en":"X","fr":"Y"}}`), 422)
	// A name another tag has in any language is taken.
	wantStatus(t, call(suggestAdmin, "POST", "/api/tags", `{"names":{"en":"Shrine","ja":"東方project"}}`), 409)
	made := wantStatus(t, call(suggestAdmin, "POST", "/api/tags", `{"names":{"en":"Eurobeat","zh-Hans":"欧陆节拍"}}`), 201)
	eurobeat := made["id"].(string)
	if made["status"] != tagOfficial {
		t.Fatal(made)
	}

	wantStatus(t, call(suggestAdmin, "PATCH", "/api/tags/"+eurobeat, `{"names":{"en":"bms"}}`), 409)
	edited := wantStatus(t, call(suggestAdmin, "PATCH", "/api/tags/"+eurobeat, `{"names":{"en":"EUROBEAT","ja":"ユーロビート"}}`), 200)
	if names := edited["names"].(map[string]any); names["en"] != "EUROBEAT" || names["ja"] != "ユーロビート" || names["zh-Hans"] != nil {
		t.Fatal("the names given replace the tag's:", edited)
	}

	// A pending tag made official without its three players.
	call(suggestPlayer, "POST", chartTags, `{"name":"Speedcore"}`)
	speedcore, _ := catalogTag(t, call, "Speedcore")
	wantStatus(t, call(suggestAdmin, "PATCH", "/api/tags/"+speedcore.ID, `{"official":false}`), 422)
	if edited = wantStatus(t, call(suggestAdmin, "PATCH", "/api/tags/"+speedcore.ID, `{"official":true}`), 200); edited["status"] != tagOfficial {
		t.Fatal(edited)
	}

	// Merged: a player with a vote for both keeps one, the others come along.
	call(suggestPlayer, "PUT", chartTags+"/"+eurobeat, `{"vote":1}`)
	call(suggestPlayer2, "PUT", chartTags+"/"+speedcore.ID, `{"vote":1}`)
	wantStatus(t, call(suggestAdmin, "POST", "/api/tags/"+speedcore.ID+"/merge", `{"into":"`+speedcore.ID+`"}`), 422)
	wantStatus(t, call(suggestAdmin, "POST", "/api/tags/"+speedcore.ID+"/merge", `{"into":"nothing"}`), 404)
	merged := wantStatus(t, call(suggestAdmin, "POST", "/api/tags/"+speedcore.ID+"/merge", `{"into":"`+eurobeat+`"}`), 200)
	if merged["id"] != eurobeat || merged["chartCount"].(float64) != 1 {
		t.Fatal(merged)
	}
	if got := shownTags(t, call(suggestPlayer, "GET", chartTags, "")); got != "EUROBEAT:2:official*" {
		t.Fatal(got)
	}
	if _, ok := catalogTag(t, call, "Speedcore"); ok {
		t.Fatal("the merged tag is gone")
	}

	wantStatus(t, call(suggestPlayer, "DELETE", "/api/tags/"+eurobeat, ""), 403)
	wantStatus(t, call(suggestAdmin, "DELETE", "/api/tags/"+eurobeat, ""), 204)
	wantStatus(t, call(suggestAdmin, "DELETE", "/api/tags/"+eurobeat, ""), 404)
	if got := shownTags(t, call("", "GET", chartTags, "")); got != "" {
		t.Fatal("a deleted tag leaves its charts:", got)
	}
}

func TestGameCatalogNamesTheOfficialTags(t *testing.T) {
	call := tagServer(t)
	call(suggestPlayer, "POST", chartTags, `{"name":"Not yet"}`)
	var body struct {
		Tags []GameTag `json:"tags"`
	}
	if err := json.Unmarshal(call("", "GET", "/api/game/catalog", "").Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Tags) != 7 || body.Tags[0].ID != "gekichumai" || body.Tags[0].Names["ja"] != "ゲキチュウマイ" {
		t.Fatal(body.Tags)
	}
}
