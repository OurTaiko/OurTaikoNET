package httpapi

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestChartVotes(t *testing.T) { bothSources(t, testChartVotes) }

func testChartVotes(t *testing.T, w interactionWorld) {
	c, song := w.c, w.chart(w.song)
	var v VoteResult
	if code := c.do("", "PUT", song+"/vote", `{"value":1}`, nil); code != 401 {
		t.Fatal("guest vote", code)
	}
	for _, body := range []string{`{"value":2}`, `{}`, `{"value":"1"}`} {
		if code := c.do("alice", "PUT", song+"/vote", body, nil); code != 400 {
			t.Fatal("invalid vote accepted", body, code)
		}
	}
	for _, path := range []string{"/api/v1/charts/999999/vote", "/api/v1/charts/song/vote"} {
		if code := c.do("alice", "PUT", path, `{"value":1}`, nil); code != 404 {
			t.Fatal("missing chart", path, code)
		}
	}
	c.do("alice", "PUT", song+"/vote", `{"value":1}`, &v)
	c.do("alice", "PUT", song+"/vote", `{"value":1}`, &v)
	c.do("bob", "PUT", song+"/vote", `{"value":1}`, &v)
	if v.Score != 2 || *v.Upvotes != 2 || v.MyVote != 1 {
		t.Fatalf("repeated vote counted twice: %+v", v)
	}
	c.do("bob", "PUT", song+"/vote", `{"value":-1}`, &v)
	if v.Score != 0 || *v.Upvotes != 1 || *v.Downvotes != 1 || v.MyVote != -1 {
		t.Fatalf("changed vote: %+v", v)
	}
	c.do("bob", "PUT", song+"/vote", `{"value":0}`, &v)
	if v.Score != 1 || *v.Downvotes != 0 || v.MyVote != 0 {
		t.Fatalf("withdrawn vote: %+v", v)
	}
	var chart Chart
	c.do("alice", "GET", song+"?by=id", "", &chart)
	if chart.Score != 1 || chart.MyVote != 1 || chart.Upvotes != 1 {
		t.Fatalf("detail: %+v %+v", chart.Score, chart.MyVote)
	}
	c.do("", "GET", song+"?by=id", "", &chart)
	if chart.MyVote != 0 || chart.Score != 1 {
		t.Fatal("guest detail", chart.MyVote)
	}
	// "top" puts the voted older chart first; the default order is newest first.
	for order, first := range map[string]string{"": w.other, "top": w.song, "hot": w.other, "comments": w.other} {
		var list struct{ Items []Chart }
		if code := c.do("", "GET", "/api/v1/charts?order="+order, "", &list); code != 200 || len(list.Items) != 2 || list.Items[0].ID != first {
			t.Fatal("order", order, code, list.Items)
		}
	}
}

func TestCommentThreads(t *testing.T) { bothSources(t, testCommentThreads) }

func testCommentThreads(t *testing.T, w interactionWorld) {
	c, s, song := w.c, w.s, w.chart(w.song)
	ctx := context.Background()
	post := func(user, chart, body string, parent string) Comment {
		t.Helper()
		payload := map[string]any{"body": body}
		if parent != "" {
			payload["parentId"] = parent
		}
		raw, _ := json.Marshal(payload)
		var out Comment
		if code := c.do(user, "POST", chart+"/comments", string(raw), &out); code != 201 {
			t.Fatal("post", body, code)
		}
		return out
	}
	if code := c.do("", "POST", song+"/comments", `{"body":"hi"}`, nil); code != 401 {
		t.Fatal("guest comment", code)
	}
	for _, body := range []string{`{"body":"   "}`, `{"body":"a\u0000b"}`, `{"body":"` + strings.Repeat("长", 10001) + `"}`} {
		if code := c.do("alice", "POST", song+"/comments", body, nil); code != 400 {
			t.Fatal("invalid body accepted", len(body), code)
		}
	}
	long := post("alice", song, strings.Repeat("长", 10000), "")
	if long.Score != 1 || long.MyVote != 1 || long.Author != "alice" || long.ChartID != w.song {
		t.Fatalf("own comment not self-upvoted: %+v", long)
	}
	top := post("alice", song, " First\r\nline ", "")
	if top.Body != "First\nline" || top.Depth != 0 {
		t.Fatalf("body not normalized: %q", top.Body)
	}
	reply := post("bob", song, "reply", top.ID)
	deep := post("owner", song, "deeper", reply.ID)
	if deep.Depth != 2 || *deep.ParentID != reply.ID {
		t.Fatalf("nested reply: %+v", deep)
	}
	if code := c.do("bob", "POST", w.chart(w.other)+"/comments", `{"body":"x","parentId":"`+top.ID+`"}`, nil); code != 404 {
		t.Fatal("reply across charts", code)
	}
	// Bob downvotes the long comment so "best" ranks the first comment ahead.
	var v VoteResult
	c.do("bob", "PUT", "/api/v1/comments/"+long.ID+"/vote", `{"value":-1}`, &v)
	if v.Upvotes != nil {
		t.Fatal("comment vote published separate counts")
	}
	c.do("owner", "PUT", "/api/v1/comments/"+top.ID+"/vote", `{"value":1}`, &v)
	if v.Score != 2 || v.MyVote != 1 {
		t.Fatalf("comment vote: %+v", v)
	}
	var page CommentPage
	c.do("bob", "GET", song+"/comments", "", &page)
	if page.Total != 2 || page.CommentCount != 4 || len(page.Items) != 2 || page.Items[0].ID != top.ID {
		t.Fatalf("best page: %+v", page)
	}
	if got := page.Items[0]; len(got.Replies) != 1 || got.Replies[0].ID != reply.ID || len(got.Replies[0].Replies) != 1 || got.Replies[0].Replies[0].Author != "owner" || got.Replies[0].MyVote != 1 {
		t.Fatalf("tree: %+v", got.Replies)
	}
	if page.Items[1].MyVote != -1 {
		t.Fatal("viewer's downvote not reported")
	}
	for sort, first := range map[string]string{"new": top.ID, "old": long.ID, "top": top.ID, "controversial": long.ID} {
		c.do("", "GET", song+"/comments?sort="+sort, "", &page)
		if page.Items[0].ID != first {
			t.Fatal("sort", sort)
		}
	}
	if code := c.do("", "GET", song+"/comments?sort=hot", "", nil); code != 400 {
		t.Fatal("unknown sort", code)
	}
	var chart Chart
	c.do("", "GET", song+"?by=id", "", &chart)
	if chart.CommentCount != 4 {
		t.Fatal("chart comment count", chart.CommentCount)
	}

	// Permalink of the middle reply contains its subtree only.
	var thread struct{ Item Comment }
	c.do("", "GET", "/api/v1/comments/"+reply.ID, "", &thread)
	if thread.Item.ID != reply.ID || thread.Item.ChartTitle != "Song" || len(thread.Item.Replies) != 1 || thread.Item.Replies[0].ID != deep.ID {
		t.Fatalf("permalink: %+v", thread.Item)
	}

	// Editing: only the author, and the edit is marked.
	if code := c.do("alice", "PATCH", "/api/v1/comments/"+reply.ID, `{"body":"hijack"}`, nil); code != 403 {
		t.Fatal("edit by another user", code)
	}
	var edited Comment
	c.do("bob", "PATCH", "/api/v1/comments/"+reply.ID, `{"body":"reply, edited"}`, &edited)
	if edited.Body != "reply, edited" || edited.EditedAt == nil {
		t.Fatalf("edit: %+v", edited)
	}

	// Notifications: alice got bob's reply; owner got alice's top-level comments.
	var unread map[string]int
	c.do("alice", "GET", "/api/v1/me/notifications/unread", "", &unread)
	if unread["count"] != 1 {
		t.Fatal("alice unread", unread)
	}
	var notes NotificationPage
	c.do("owner", "GET", "/api/v1/me/notifications", "", &notes)
	if notes.Total != 2 || notes.Unread != 2 || notes.Items[0].Kind != "chart_comment" || notes.Items[0].Read ||
		notes.Items[0].Actor == nil || notes.Items[0].Actor.Nickname != "alice" || notes.Items[0].Chart.Title != "Song" || notes.Items[0].Comment.ID != top.ID {
		t.Fatalf("owner notifications: %+v", notes)
	}
	if code := c.do("owner", "POST", "/api/v1/me/notifications/read", `{}`, nil); code != 200 {
		t.Fatal("mark all read", code)
	}
	c.do("owner", "GET", "/api/v1/me/notifications/unread", "", &unread)
	if unread["count"] != 0 {
		t.Fatal("still unread", unread)
	}

	// Deleting a comment with replies keeps an anonymous placeholder.
	if code := c.do("alice", "DELETE", "/api/v1/comments/"+reply.ID, "", nil); code != 403 {
		t.Fatal("delete by another user", code)
	}
	c.do("bob", "DELETE", "/api/v1/comments/"+reply.ID, "", nil)
	c.do("", "GET", song+"/comments?sort=new", "", &page)
	placeholder := page.Items[0].Replies[0]
	if !placeholder.Deleted || placeholder.Body != "" || placeholder.AuthorID != "" || placeholder.Author != "" || len(placeholder.Replies) != 1 {
		t.Fatalf("placeholder: %+v", placeholder)
	}
	if code := c.do("alice", "POST", song+"/comments", `{"body":"x","parentId":"`+reply.ID+`"}`, nil); code != 409 {
		t.Fatal("reply to deleted comment", code)
	}
	if code := c.do("alice", "PUT", "/api/v1/comments/"+reply.ID+"/vote", `{"value":1}`, nil); code != 409 {
		t.Fatal("vote on deleted comment", code)
	}
	c.do("alice", "GET", "/api/v1/me/notifications/unread", "", &unread)
	if unread["count"] != 0 {
		t.Fatal("notice of a deleted reply", unread)
	}
	// An administrator removes the last reply; the empty placeholder above goes too.
	var removed map[string]bool
	c.do("admin", "DELETE", "/api/v1/comments/"+deep.ID, "", &removed)
	if !removed["purged"] {
		t.Fatal("leaf not purged")
	}
	var n int
	if err := s.DB.QueryRow(ctx, `SELECT count(*) FROM comments WHERE id=ANY($1)`, []string{reply.ID, deep.ID}).Scan(&n); err != nil || n != 0 {
		t.Fatal("placeholder chain kept", n, err)
	}
	c.do("", "GET", song+"/comments", "", &page)
	if page.CommentCount != 2 || page.Items[0].ReplyCount != 0 {
		t.Fatalf("counters after purge: %+v", page)
	}
	// Removal by an administrator keeps replies under a "removed" placeholder.
	post("bob", song, "under", long.ID)
	c.do("admin", "DELETE", "/api/v1/comments/"+long.ID, "", nil)
	c.do("alice", "GET", "/api/v1/me/notifications?filter=unread", "", &notes)
	if notes.Total != 2 || notes.Items[0].Kind != "comment_removed" || notes.Items[0].Actor != nil || notes.Items[0].Chart.ID != w.song || notes.Items[1].Kind != "comment_reply" {
		t.Fatalf("removal notice: %+v", notes)
	}
	c.do("", "GET", song+"/comments?sort=old", "", &page)
	if !page.Items[0].Removed || page.Items[0].Body != "" {
		t.Fatalf("removed placeholder: %+v", page.Items[0])
	}

	// History lists live comments only. Karma keeps every vote from others: the
	// owner's upvote on the live comment and bob's downvote on the removed one.
	alice := testUsers["alice"].ID
	var history CommentList
	c.do("", "GET", "/api/v1/users/"+alice+"/comments", "", &history)
	if history.Total != 1 || history.Items[0].ID != top.ID || history.Items[0].ChartTitle != "Song" {
		t.Fatalf("history: %+v", history)
	}
	var space UserSpace
	c.do("", "GET", "/api/v1/users/"+alice, "", &space)
	if space.User.CommentCount != 1 || space.User.Karma != 0 || space.User.Nickname == nil || *space.User.Nickname != "alice" {
		t.Fatalf("profile: %+v", space.User)
	}

	// Deleting the chart cascades through comments and votes.
	if code := c.do("owner", "DELETE", song, "", nil); code != 200 {
		t.Fatal("delete chart", code)
	}
	for _, table := range []string{"comments", "comment_votes", "chart_votes", "chart_stats"} {
		if err := s.DB.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&n); err != nil || n != 0 {
			t.Fatal(table, "left after chart deletion", n, err)
		}
	}
}

func TestCommentBurstLimit(t *testing.T) { bothSources(t, testCommentBurstLimit) }

func testCommentBurstLimit(t *testing.T, w interactionWorld) {
	for i := 0; i < commentBurst; i++ {
		if code := w.c.do("alice", "POST", w.chart(w.song)+"/comments", `{"body":"spam"}`, nil); code != 201 {
			t.Fatal("comment", i, code)
		}
	}
	if code := w.c.do("alice", "POST", w.chart(w.song)+"/comments", `{"body":"spam"}`, nil); code != 429 {
		t.Fatal("burst not limited", code)
	}
}

// Concurrent votes on the same chart and comment neither deadlock nor drift.
func TestConcurrentVotes(t *testing.T) { bothSources(t, testConcurrentVotes) }

func testConcurrentVotes(t *testing.T, w interactionWorld) {
	c := w.c
	var comment Comment
	if code := c.do("owner", "POST", w.chart(w.song)+"/comments", `{"body":"vote here"}`, &comment); code != 201 {
		t.Fatal("comment", code)
	}
	var wg sync.WaitGroup
	codes := make(chan int, 64)
	for _, user := range []string{"owner", "alice", "bob", "admin"} {
		for _, path := range []string{w.chart(w.song) + "/vote", "/api/v1/comments/" + comment.ID + "/vote"} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for _, value := range []string{"1", "-1", "0", "-1"} {
					codes <- c.do(user, "PUT", path, `{"value":`+value+`}`, nil)
				}
			}()
		}
	}
	wg.Wait()
	close(codes)
	for code := range codes {
		if code != 200 {
			t.Fatal("concurrent vote", code)
		}
	}
	var up, down, cup, cdown int
	ctx := context.Background()
	if err := w.s.DB.QueryRow(ctx, `SELECT upvotes,downvotes FROM chart_stats WHERE chart_id=$1`, w.song).Scan(&up, &down); err != nil || up != 0 || down != 4 {
		t.Fatal("chart tally", up, down, err)
	}
	if err := w.s.DB.QueryRow(ctx, `SELECT upvotes,downvotes FROM comments WHERE id=$1`, comment.ID).Scan(&cup, &cdown); err != nil || cup != 0 || cdown != 4 {
		t.Fatal("comment tally", cup, cdown, err)
	}
}

func TestNotificationCenter(t *testing.T) { bothSources(t, testNotificationCenter) }

func testNotificationCenter(t *testing.T, w interactionWorld) {
	c, s, song := w.c, w.s, w.chart(w.song)
	ctx := context.Background()
	var comment Comment
	c.do("alice", "POST", song+"/comments", `{"body":"hello"}`, &comment)
	var notes NotificationPage
	c.do("owner", "GET", "/api/v1/me/notifications", "", &notes)
	if len(notes.Items) != 1 {
		t.Fatal("no notice", notes)
	}
	path := "/api/v1/me/notifications/" + notes.Items[0].ID
	// Only the recipient can see or change a notice.
	if code := c.do("bob", "PATCH", path, `{"read":true}`, nil); code != 404 {
		t.Fatal("foreign mark", code)
	}
	if code := c.do("bob", "DELETE", path, "", nil); code != 404 {
		t.Fatal("foreign dismiss", code)
	}
	c.do("owner", "PATCH", path, `{"read":true}`, nil)
	c.do("owner", "GET", "/api/v1/me/notifications?filter=unread", "", &notes)
	if notes.Total != 0 || notes.Unread != 0 {
		t.Fatal("read notice listed as unread", notes)
	}
	c.do("owner", "PATCH", path, `{"read":false}`, nil)
	c.do("owner", "GET", "/api/v1/me/notifications?filter=unread", "", &notes)
	if notes.Total != 1 {
		t.Fatal("marked unread again", notes)
	}
	if code := c.do("owner", "DELETE", path, "", nil); code != 200 {
		t.Fatal("dismiss", code)
	}
	c.do("owner", "GET", "/api/v1/me/notifications", "", &notes)
	if notes.Total != 0 {
		t.Fatal("dismissed notice listed", notes)
	}

	// Muting a kind stops new notices of that kind only.
	if code := c.do("owner", "PUT", "/api/v1/me/notification-settings", `{"nope":false}`, nil); code != 400 {
		t.Fatal("unknown kind accepted", code)
	}
	var settings struct{ Items []notificationSetting }
	c.do("owner", "PUT", "/api/v1/me/notification-settings", `{"chart_comment":false}`, &settings)
	if len(settings.Items) != len(notificationKinds) || settings.Items[1].Kind != "chart_comment" || settings.Items[1].Enabled || !settings.Items[0].Enabled {
		t.Fatalf("settings: %+v", settings)
	}
	c.do("bob", "POST", song+"/comments", `{"body":"muted"}`, nil)
	c.do("owner", "GET", "/api/v1/me/notifications", "", &notes)
	if notes.Total != 0 {
		t.Fatal("muted kind delivered", notes)
	}
	c.do("owner", "PUT", "/api/v1/me/notification-settings", `{"chart_comment":true}`, &settings)
	if !settings.Items[1].Enabled {
		t.Fatal("not re-enabled")
	}

	// The fifth upvote (the author's own counts as on Reddit) notifies once,
	// even when a vote is withdrawn and cast again; it names no voter.
	if _, err := s.DB.Exec(ctx, `INSERT INTO users(id) VALUES('77777777777777777777777777777777')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(ctx, `INSERT INTO comment_votes(comment_id,user_id,value) VALUES($1,'77777777777777777777777777777777',1)`, comment.ID); err != nil {
		t.Fatal(err)
	}
	for _, vote := range []struct{ user, value string }{{"owner", "1"}, {"bob", "1"}, {"admin", "1"}, {"bob", "0"}, {"bob", "1"}} {
		if code := c.do(vote.user, "PUT", "/api/v1/comments/"+comment.ID+"/vote", `{"value":`+vote.value+`}`, nil); code != 200 {
			t.Fatal("vote", code)
		}
	}
	c.do("alice", "GET", "/api/v1/me/notifications", "", &notes)
	if notes.Total != 1 || notes.Items[0].Kind != "comment_upvotes" || notes.Items[0].Data["upvotes"] != float64(5) || notes.Items[0].Comment.Excerpt != "hello" || notes.Items[0].Actor != nil {
		t.Fatalf("milestone: %+v", notes)
	}

	// Hiding the chart hides its notices; deleting the chart removes them.
	if _, err := s.DB.Exec(ctx, `UPDATE charts SET status='hidden' WHERE id=$1`, w.song); err != nil {
		t.Fatal(err)
	}
	var unread map[string]int
	c.do("alice", "GET", "/api/v1/me/notifications/unread", "", &unread)
	if unread["count"] != 0 {
		t.Fatal("notice of a hidden chart", unread)
	}
	if _, err := s.DB.Exec(ctx, `UPDATE charts SET status='published' WHERE id=$1`, w.song); err != nil {
		t.Fatal(err)
	}
	c.do("owner", "DELETE", song, "", nil)
	var n int
	if err := s.DB.QueryRow(ctx, `SELECT count(*) FROM notifications`).Scan(&n); err != nil || n != 0 {
		t.Fatal("notices left after chart deletion", n, err)
	}
}

func TestNotificationPrune(t *testing.T) { bothSources(t, testNotificationPrune) }

func testNotificationPrune(t *testing.T, w interactionWorld) {
	ctx := context.Background()
	alice := testUsers["alice"].ID
	_, err := w.s.DB.Exec(ctx, `INSERT INTO notifications(id,recipient_id,kind,created_at,read_at) VALUES
	 ('old-read',$1,'chart_upvotes',now()-interval '100 days',now()-interval '91 days'),
	 ('recent-read',$1,'chart_upvotes',now()-interval '100 days',now()-interval '10 days'),
	 ('old-unread',$1,'chart_upvotes',now()-interval '400 days',NULL),
	 ('unread',$1,'chart_upvotes',now()-interval '200 days',NULL)`, alice)
	if err != nil {
		t.Fatal(err)
	}
	if err = w.s.PruneNotifications(ctx); err != nil {
		t.Fatal(err)
	}
	rows, _ := w.s.DB.Query(ctx, `SELECT id FROM notifications ORDER BY id`)
	kept, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil || strings.Join(kept, ",") != "recent-read,unread" {
		t.Fatal("pruned", kept, err)
	}
}

// GET /charts/{id}: by=number finds the chart's number, by=id its ID, by=nijiiro its Nijiiro
// ID; without by the form decides between the first two.
func TestChartDetailBy(t *testing.T) { bothSources(t, testChartDetailBy) }

func testChartDetailBy(t *testing.T, w interactionWorld) {
	c, s := w.c, w.s
	ctx := context.Background()
	var song, other Chart
	if code := c.do("", "GET", w.chart(w.song)+"?by=id", "", &song); code != 200 || song.ID != w.song || song.NumberID <= 0 {
		t.Fatal("by=id", code, song.ID, song.NumberID)
	}
	// A new chart takes the next number.
	c.do("", "GET", w.chart(w.other)+"?by=id", "", &other)
	if other.NumberID != song.NumberID+1 {
		t.Fatal("numbers", song.NumberID, other.NumberID)
	}
	number := w.chart(strconv.FormatInt(song.NumberID, 10))
	for _, path := range []string{number, number + "?by=number"} {
		var found Chart
		if code := c.do("", "GET", path, "", &found); code != 200 || found.ID != w.song || found.NumberID != song.NumberID {
			t.Fatal("by number", path, code, found.ID)
		}
	}
	// Without by, a value in the form of an ID (32 hex digits, even all digits) is an ID,
	// anything else a number.
	for _, id := range []string{"0123456789abcdef0123456789abcdef", "12345678901234567890123456789012"} {
		w.addChart(t, id, "Hex")
		var found Chart
		if code := c.do("", "GET", w.chart(id), "", &found); code != 200 || found.ID != id {
			t.Fatal("ID without by", id, code, found.ID)
		}
		if code := c.do("", "GET", w.chart(id)+"?by=number", "", nil); code != 404 {
			t.Fatal("ID by number", id, code)
		}
	}
	for path, want := range map[string]int{
		w.chart(w.song):                200, // without by, the form tells
		w.chart(w.song) + "?by=number": 404,
		number + "?by=id":              404, // a number is not an ID
		w.chart("song"):                404, // neither an ID's form nor a number
		w.chart("0"):                   404,
		w.chart("01"):                  404,
		w.chart("99999999"):            404,
		w.chart(w.song) + "?by=title":  400,
		w.chart(w.song) + "?by=id&x=1": 200,
	} {
		if code := c.do("", "GET", path, "", nil); code != want {
			t.Fatal(path, code, "want", want)
		}
	}
	// by=nijiiro reads {id} as the Nijiiro ID, which is another number than the chart's own:
	// Other takes Song's number as its Nijiiro ID.
	setNijiiro := func(id string) error {
		_, err := s.DB.Exec(ctx, `UPDATE charts SET nijiiro_id=$2 WHERE id=$1`, id, song.NumberID)
		return err
	}
	if err := setNijiiro(w.other); err != nil {
		t.Fatal(err)
	}
	var mapped Chart
	if code := c.do("", "GET", number+"?by=nijiiro", "", &mapped); code != 200 || mapped.ID != w.other || mapped.NijiiroID == nil || int64(*mapped.NijiiroID) != song.NumberID {
		t.Fatal("by=nijiiro", code, mapped.ID)
	}
	if code := c.do("", "GET", number, "", &mapped); code != 200 || mapped.ID != w.song || mapped.NijiiroID != nil {
		t.Fatal("number beside a Nijiiro ID", code, mapped.ID)
	}
	for path, want := range map[string]int{
		w.chart("99999999") + "?by=nijiiro":    404,
		w.chart("0") + "?by=nijiiro":           404,
		w.chart("01") + "?by=nijiiro":          404,
		w.chart("99999999999") + "?by=nijiiro": 404, // past int4
		w.chart(w.other) + "?by=nijiiro":       404,
	} {
		if code := c.do("", "GET", path, "", nil); code != want {
			t.Fatal(path, code, "want", want)
		}
	}
	// A Nijiiro ID names one chart: the index refuses a second.
	if err := setNijiiro(w.song); !nijiiroConflict(err) {
		t.Fatal("shared Nijiiro ID", err)
	}
	taken := `{"nijiiroId":` + strconv.FormatInt(song.NumberID, 10) + `}`
	if !s.source().Official {
		// Only the official source's songs are given one: no request sets or clears it here.
		for _, body := range []string{taken, `{"nijiiroId":7}`, `{"nijiiroId":null}`} {
			if code := c.do("owner", "PATCH", w.chart(w.other), body, nil); code != 422 {
				t.Fatal("PATCH", body, code)
			}
		}
		if code := c.do("", "GET", number+"?by=nijiiro", "", &mapped); code != 200 || mapped.ID != w.other {
			t.Fatal("Nijiiro ID after refused edits", code, mapped.ID)
		}
		return
	}
	// On the official source an edit sets it, and refuses one another chart has.
	if code := c.do("owner", "PATCH", w.chart(w.song), taken, nil); code != 409 {
		t.Fatal("taken Nijiiro ID", code)
	}
	// Its own chart keeps it, and a cleared one is free again.
	if code := c.do("owner", "PATCH", w.chart(w.other), taken, nil); code != 200 {
		t.Fatal("own Nijiiro ID", code)
	}
	if code := c.do("owner", "PATCH", w.chart(w.other), `{"nijiiroId":null}`, nil); code != 200 {
		t.Fatal("clear Nijiiro ID", code)
	}
	if code := c.do("owner", "PATCH", w.chart(w.song), taken, &mapped); code != 200 || mapped.NijiiroID == nil || int64(*mapped.NijiiroID) != song.NumberID {
		t.Fatal("freed Nijiiro ID", code)
	}
}
