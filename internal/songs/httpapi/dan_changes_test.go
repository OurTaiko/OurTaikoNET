package httpapi

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"ourtaiko.dev/net/internal/rewards/rules"
	"ourtaiko.dev/net/internal/web"
)

// A course's maker fixes it; its songs and conditions then change only by a request a manager
// approves, and only a fixed course's attempts made since it was fixed meet a dan_clear rule.
func TestFixedDanCourses(t *testing.T) {
	pool := testDB(t, Fanmade)
	ctx := context.Background()
	s := testServer(t, pool, Config{Origin: "http://localhost"})
	c := newTestClient(t, s)
	alice, bob := testUsers["alice"].ID, testUsers["bob"].ID
	const a, b = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa1", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa2"
	if _, err := pool.Exec(ctx, `INSERT INTO charts(id,owner_ids,title,bpm,duration,tja_file,audio_file)
 SELECT id,ARRAY[$1::text],'Song '||id,120,10,test_file('tja'||id,'t.tja',repeat('a',64),1),test_file('ogg'||id,'t.ogg',repeat('b',64),1)
 FROM unnest(ARRAY[$2,$3]) id`, testUsers["owner"].ID, a, b); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `SELECT test_slot($1,'Oni',9),test_slot($2,'Oni',10),test_slot($2,'Hard',7)`, a, b); err != nil {
		t.Fatal(err)
	}
	course := func(songs string) string {
		return `{"title":{"ja":"十段"},"color":3,"songs":[` + songs + `],"exams":[{"type":"gauge","range":"more","value":[100]}],"rewards":{}}`
	}
	two := fmt.Sprintf(`{"chartId":%q,"course":"Oni","side":"single"},{"chartId":%q,"course":"Oni","side":"single"}`, a, b)
	other := fmt.Sprintf(`{"chartId":%q,"course":"Oni","side":"single"},{"chartId":%q,"course":"Hard","side":"single"}`, a, b)
	var dan DanCourse
	if code := c.do("owner", "POST", "/api/dans", course(two), &dan); code != 201 || dan.FixedAt != nil {
		t.Fatal(code, dan)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO net.titles(id,name,style,visibility) VALUES('passed','Passed','wood','public')`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO net.reward_rules(item,title_id,kind,source,dan_id,clear) VALUES('title','passed','dan_clear','fanmade',$1,4)`, dan.ID); err != nil {
		t.Fatal(err)
	}
	holds := func(user string) bool {
		t.Helper()
		var held bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM net.user_titles WHERE user_id=$1 AND title_id='passed')`, user).Scan(&held); err != nil {
			t.Fatal(err)
		}
		return held
	}
	tokens := map[string]string{}
	for _, user := range []string{alice, bob} {
		tokens[user] = web.NewID() + web.NewID()
		if _, err := pool.Exec(ctx, `INSERT INTO test_sessions(token_hash,user_id,csrf_token,expires_at) VALUES($1,$2,'',now()+interval '1 day')`, web.Hash("game:"+tokens[user]), user); err != nil {
			t.Fatal(err)
		}
	}
	attempts := 0
	attempt := func(user string, verdict int) {
		t.Helper()
		attempts++
		r := httptest.NewRequest("POST", "/api/game/dan-attempts", strings.NewReader(fmt.Sprintf(`{"courseId":%q,"verdict":%d,"songsPlayed":0}`, dan.ID, verdict)))
		r.Header.Set("Authorization", "Bearer "+tokens[user])
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Idempotency-Key", fmt.Sprintf("attempt-%04d", attempts))
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}

	// Not fixed: a gold pass meets no rule, and the maker changes the songs freely.
	attempt(alice, 6)
	if holds(alice) {
		t.Fatal("a course that is not fixed gave its title")
	}
	if code := c.do("owner", "PUT", "/api/dans/"+dan.ID, course(other), nil); code != 200 {
		t.Fatal(code)
	}
	if code := c.do("owner", "POST", "/api/dans/"+dan.ID+"/changes", `{"songs":[`+two+`],"exams":[{"type":"gauge","range":"more","value":[100]}],"request":"x"}`, nil); code != 409 {
		t.Fatal("a request for a course that is not fixed", code)
	}
	if code := c.do("owner", "PUT", "/api/dans/"+dan.ID, course(two), nil); code != 200 {
		t.Fatal(code)
	}

	// Only its maker or a manager fixes it.
	if code := c.do("alice", "POST", "/api/dans/"+dan.ID+"/fix", "", nil); code != 403 {
		t.Fatal(code)
	}
	if code := c.do("owner", "POST", "/api/dans/"+dan.ID+"/fix", "", &dan); code != 200 || dan.FixedAt == nil {
		t.Fatal(code, dan)
	}
	before := *dan.FixedAt
	if code := c.do("owner", "POST", "/api/dans/"+dan.ID+"/fix", "", nil); code != 409 {
		t.Fatal("fixed twice", code)
	}
	// The attempt before it was fixed does not count, the ones since do: a red pass is not the gold
	// the rule asks for.
	if n, err := rules.Backfill(ctx, pool, ruleOf(t, pool, dan.ID)); err != nil || n != 0 {
		t.Fatal("an attempt before the course was fixed", n, err)
	}
	attempt(bob, 3)
	if holds(bob) {
		t.Fatal("a red pass met a gold rule")
	}
	attempt(bob, 4)
	if !holds(bob) {
		t.Fatal("a gold pass of a fixed course gave nothing")
	}

	// Its name, colour and hidden songs change; its songs and conditions do not.
	if code := c.do("owner", "PUT", "/api/dans/"+dan.ID, course(other), nil); code != 409 {
		t.Fatal("songs of a fixed course changed", code)
	}
	if code := c.do("owner", "PUT", "/api/dans/"+dan.ID, strings.Replace(course(two), `"value":[100]`, `"value":[90]`, 1), nil); code != 409 {
		t.Fatal("conditions of a fixed course changed", code)
	}
	hidden := strings.Replace(strings.Replace(course(two), `"十段"`, `"十段・改"`, 1), `"course":"Oni","side":"single"}]`, `"course":"Oni","side":"single","hidden":true}]`, 1)
	if code := c.do("owner", "PUT", "/api/dans/"+dan.ID, hidden, &dan); code != 200 || dan.Title["ja"] != "十段・改" || !dan.Songs[1].Hidden || dan.FixedAt == nil {
		t.Fatal(code, dan)
	}

	// A change is asked for, with a reason; one waits at a time, and it must change something.
	ask := func(user, songs, request string) (int, DanChange) {
		t.Helper()
		var change DanChange
		code := c.do(user, "POST", "/api/dans/"+dan.ID+"/changes", `{"songs":[`+songs+`],"exams":[{"type":"gauge","range":"more","value":[100]}],"request":`+request+`}`, &change)
		return code, change
	}
	if code, _ := ask("alice", other, `"x"`); code != 403 {
		t.Fatal(code)
	}
	if code, _ := ask("owner", other, `" "`); code != 422 {
		t.Fatal("a request without a reason", code)
	}
	if code, _ := ask("owner", two, `"same"`); code != 422 {
		t.Fatal("a request that changes nothing", code)
	}
	code, change := ask("owner", other, `"太难了"`)
	if code != 201 || change.Status != danChangePending || len(change.Songs) != 2 || change.Songs[1].Chart.Course != "Hard" || change.Songs[1].Level != 7 || !change.Songs[1].Valid {
		t.Fatal(code, change)
	}
	if code, _ := ask("owner", other, `"again"`); code != 409 {
		t.Fatal("two requests waiting", code)
	}
	var mine struct{ Changes []DanChange }
	if code := c.do("alice", "GET", "/api/dans/"+dan.ID+"/changes", "", nil); code != 403 {
		t.Fatal(code)
	}
	if code := c.do("owner", "GET", "/api/dans/"+dan.ID+"/changes", "", &mine); code != 200 || len(mine.Changes) != 1 {
		t.Fatal(code, mine)
	}

	// Managers see the queue and decide; a returned request needs a reply, and the maker is told.
	var queue struct{ Changes []DanChangeReview }
	if code := c.do("owner", "GET", "/api/dan-changes", "", nil); code != 403 {
		t.Fatal(code)
	}
	if code := c.do("admin", "GET", "/api/dan-changes", "", &queue); code != 200 || len(queue.Changes) != 1 || queue.Changes[0].Course.ID != dan.ID || queue.Changes[0].Course.Songs[1].Chart.Course != "Oni" {
		t.Fatal(code, queue)
	}
	if code := c.do("admin", "POST", "/api/dan-changes/"+change.ID+"/decision", `{"approve":false}`, nil); code != 422 {
		t.Fatal("returned without a reply", code)
	}
	if code := c.do("admin", "POST", "/api/dan-changes/"+change.ID+"/decision", `{"approve":false,"reply":"再想想"}`, &change); code != 200 || change.Status != danChangeReturned || change.Reply != "再想想" {
		t.Fatal(code, change)
	}
	var told int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE recipient_id=$1 AND kind='dan_change_result' AND dan_id=$2 AND data->>'approved'='false'`, testUsers["owner"].ID, dan.ID).Scan(&told); err != nil || told != 1 {
		t.Fatal("the maker is told", told, err)
	}
	if code := c.do("admin", "POST", "/api/dan-changes/"+change.ID+"/decision", `{"approve":true}`, nil); code != 404 {
		t.Fatal("decided twice", code)
	}
	// Withdrawn by its maker.
	_, change = ask("owner", other, `"again"`)
	if code := c.do("owner", "DELETE", "/api/dans/"+dan.ID+"/changes/"+change.ID, "", &change); code != 200 || change.Status != danChangeWithdrawn {
		t.Fatal(code, change)
	}
	// Approved: the course takes the songs and keeps which are hidden as asked. The change is the
	// managers' own: the course stays fixed from when it was, and the attempts before still count.
	_, change = ask("owner", other, `"again"`)
	if code := c.do("admin", "POST", "/api/dan-changes/"+change.ID+"/decision", `{"approve":true}`, &change); code != 200 || change.Status != danChangeApproved {
		t.Fatal(code, change)
	}
	if code := c.do("owner", "GET", "/api/dans/"+dan.ID, "", &dan); code != 200 || dan.Songs[1].Chart.Course != "Hard" || dan.Songs[1].Hidden || dan.FixedAt == nil || !dan.FixedAt.Equal(before) {
		t.Fatal(code, dan)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM net.user_titles WHERE user_id=$1`, bob); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM net.awards WHERE service='rule'`); err != nil {
		t.Fatal(err)
	}
	if n, err := rules.Backfill(ctx, pool, ruleOf(t, pool, dan.ID)); err != nil || n != 1 || !holds(bob) {
		t.Fatal("an attempt at the songs before an approved change counts", n, err)
	}

	// A manager undoes the fixing: the rule gives nothing, and the maker changes it freely.
	if code := c.do("owner", "DELETE", "/api/dans/"+dan.ID+"/fix", "", nil); code != 403 {
		t.Fatal(code)
	}
	if code := c.do("admin", "DELETE", "/api/dans/"+dan.ID+"/fix", "", &dan); code != 200 || dan.FixedAt != nil {
		t.Fatal(code, dan)
	}
	attempt(alice, 6)
	if holds(alice) {
		t.Fatal("a course no longer fixed gave its title")
	}
	if code := c.do("owner", "PUT", "/api/dans/"+dan.ID, course(two), nil); code != 200 {
		t.Fatal(code)
	}
}

func ruleOf(t *testing.T, pool interface {
	rules.DB
}, dan string) rules.Rule {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT `+rules.Columns+` FROM net.reward_rules r WHERE r.dan_id=$1`, dan)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatal("no rule", rows.Err())
	}
	r, err := rules.Scan(rows)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
