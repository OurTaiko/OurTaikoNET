package httpapi

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"ourtaiko.dev/net/internal/web"
)

// GET /charts/{id}: by=number finds the chart's number, by=id its ID, by=nijiiro its Nijiiro
// ID; without by the form decides between the first two (R051).
func TestOfficialChartDetailBy(t *testing.T) {
	w := officialInteractionFixture(t)
	var byID Chart
	if code := w.c.do("", "GET", w.chart(w.song)+"?by=id", "", &byID); code != 200 || byID.ID != w.song || byID.NumberID <= 0 {
		t.Fatal("by=id", code, byID.ID, byID.NumberID)
	}
	number := w.chart(strconv.FormatInt(byID.NumberID, 10))
	for _, path := range []string{number, number + "?by=number"} {
		var c Chart
		if code := w.c.do("", "GET", path, "", &c); code != 200 || c.ID != w.song || c.NumberID != byID.NumberID {
			t.Fatal("by number", path, code, c.ID)
		}
	}
	for path, want := range map[string]int{
		w.chart(w.song):                200, // without by, the form tells
		w.chart(w.song) + "?by=number": 404,
		number + "?by=id":              404, // a number is not an ID
		w.chart("0"):                   404,
		w.chart("01"):                  404,
		w.chart("99999999"):            404,
		w.chart(w.song) + "?by=title":  400,
		w.chart(w.song) + "?by=id&x=1": 200,
	} {
		if code := w.c.do("", "GET", path, "", nil); code != want {
			t.Fatal(path, code, "want", want)
		}
	}
	// A new chart takes the next number.
	var other Chart
	w.c.do("", "GET", w.chart(w.other)+"?by=id", "", &other)
	if other.NumberID != byID.NumberID+1 {
		t.Fatal("numbers", byID.NumberID, other.NumberID)
	}
	// by=nijiiro reads {id} as the Nijiiro ID, which is another number than the chart's own:
	// Other takes Song's number as its Nijiiro ID.
	if _, e := w.s.DB.Exec(context.Background(), `UPDATE charts SET nijiiro_id=$2 WHERE id=$1`, w.other, byID.NumberID); e != nil {
		t.Fatal(e)
	}
	var mapped Chart
	if code := w.c.do("", "GET", number+"?by=nijiiro", "", &mapped); code != 200 || mapped.ID != w.other {
		t.Fatal("by=nijiiro", code, mapped.ID)
	}
	for path, want := range map[string]int{
		w.chart("99999999") + "?by=nijiiro":    404,
		w.chart("0") + "?by=nijiiro":           404,
		w.chart("01") + "?by=nijiiro":          404,
		w.chart("99999999999") + "?by=nijiiro": 404, // past int4
		w.chart(w.other) + "?by=nijiiro":       404,
	} {
		if code := w.c.do("", "GET", path, "", nil); code != want {
			t.Fatal(path, code, "want", want)
		}
	}
	// A Nijiiro ID names one chart (027): the index refuses a second, and so does the edit.
	if _, e := w.s.DB.Exec(context.Background(), `UPDATE charts SET nijiiro_id=$2 WHERE id=$1`, w.song, byID.NumberID); !nijiiroConflict(e) {
		t.Fatal("shared Nijiiro ID", e)
	}
	taken := `{"nijiiroId":` + strconv.FormatInt(byID.NumberID, 10) + `}`
	if code := w.c.do("owner", "PATCH", w.chart(w.song), taken, nil); code != 409 {
		t.Fatal("taken Nijiiro ID", code)
	}
	// Its own chart keeps it, and a cleared one is free again.
	if code := w.c.do("owner", "PATCH", w.chart(w.other), taken, nil); code != 200 {
		t.Fatal("own Nijiiro ID", code)
	}
	if code := w.c.do("owner", "PATCH", w.chart(w.other), `{"nijiiroId":null}`, nil); code != 200 {
		t.Fatal("clear Nijiiro ID", code)
	}
	if code := w.c.do("owner", "PATCH", w.chart(w.song), taken, &mapped); code != 200 || mapped.NijiiroID == nil || int64(*mapped.NijiiroID) != byID.NumberID {
		t.Fatal("freed Nijiiro ID", code)
	}
}

// A 段位 course takes votes and comments like a chart: its own tallies, the
// maker's notifications, and the comment showing the course it belongs to.
func TestOfficialDanInteractions(t *testing.T) {
	w := officialInteractionFixture(t)
	c, s := w.c, w.s
	ctx := context.Background()
	danID := web.NewID()
	if _, err := s.DB.Exec(ctx, `INSERT INTO dan_courses(id,owner_id,title,code,exams) VALUES($2,$1,'{"ja":"初段","en":"1st Dan"}','SHODAN','[]')`, testUsers["owner"].ID, danID); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/dans/" + danID
	var v VoteResult
	if code := c.do("", "PUT", path+"/vote", `{"value":1}`, nil); code != 401 {
		t.Fatal("guest vote", code)
	}
	for _, missing := range []string{"999999", "abc"} {
		if code := c.do("alice", "PUT", "/api/v1/dans/"+missing+"/vote", `{"value":1}`, nil); code != 404 {
			t.Fatal("missing course", missing, code)
		}
	}
	c.do("alice", "PUT", path+"/vote", `{"value":1}`, &v)
	c.do("bob", "PUT", path+"/vote", `{"value":-1}`, &v)
	if v.Score != 0 || *v.Upvotes != 1 || *v.Downvotes != 1 || v.MyVote != -1 {
		t.Fatalf("tally: %+v", v)
	}
	c.do("bob", "PUT", path+"/vote", `{"value":0}`, &v)
	var course DanCourse
	c.do("alice", "GET", path, "", &course)
	if course.Score != 1 || course.Upvotes != 1 || course.MyVote != 1 || course.CommentCount != 0 {
		t.Fatalf("detail: %+v", course)
	}
	var top, reply Comment
	if code := c.do("alice", "POST", path+"/comments", `{"body":"良い段位"}`, &top); code != 201 || top.DanID != danID || top.ChartID != "" {
		t.Fatalf("top comment %d %+v", code, top)
	}
	if code := c.do("bob", "POST", path+"/comments", `{"body":"同感","parentId":"`+top.ID+`"}`, &reply); code != 201 || reply.Depth != 1 {
		t.Fatalf("reply %d %+v", code, reply)
	}
	// A chart's comment cannot be replied to from a course, nor the reverse.
	var chartTop Comment
	c.do("alice", "POST", w.chart(w.song)+"/comments", `{"body":"song"}`, &chartTop)
	if code := c.do("bob", "POST", path+"/comments", `{"body":"x","parentId":"`+chartTop.ID+`"}`, nil); code != 404 {
		t.Fatal("cross-subject reply", code)
	}
	if code := c.do("bob", "POST", w.chart(w.song)+"/comments", `{"body":"x","parentId":"`+top.ID+`"}`, nil); code != 404 {
		t.Fatal("cross-subject reply to chart", code)
	}
	var page CommentPage
	if code := c.do("", "GET", path+"/comments", "", &page); code != 200 || page.Total != 1 || page.CommentCount != 2 || len(page.Items[0].Replies) != 1 {
		t.Fatalf("page %d %+v", code, page)
	}
	var thread struct{ Item Comment }
	if code := c.do("", "GET", "/api/v1/comments/"+reply.ID, "", &thread); code != 200 || thread.Item.DanTitle["ja"] != "初段" || thread.Item.DanID != danID {
		t.Fatalf("thread %d %+v", code, thread.Item)
	}
	var inbox NotificationPage
	c.do("owner", "GET", "/api/v1/me/notifications", "", &inbox)
	if len(inbox.Items) != 2 || inbox.Items[1].Kind != "dan_comment" || inbox.Items[1].Dan == nil || inbox.Items[1].Dan.Title["ja"] != "初段" || inbox.Items[1].Chart != nil {
		t.Fatalf("owner inbox %+v", inbox.Items)
	}
	var space UserSpace
	c.do("", "GET", "/api/v1/users/"+testUsers["owner"].ID, "", &space)
	if space.User.Karma != 1 {
		t.Fatal("karma", space.User.Karma)
	}
	// A deleted course hides its comments and notices.
	if _, err := s.DB.Exec(ctx, `UPDATE dan_courses SET status='deleted' WHERE id=$1`, danID); err != nil {
		t.Fatal(err)
	}
	if code := c.do("", "GET", path+"/comments", "", nil); code != 404 {
		t.Fatal("deleted course comments", code)
	}
	c.do("owner", "GET", "/api/v1/me/notifications", "", &inbox)
	if len(inbox.Items) != 1 || inbox.Items[0].Kind != "chart_comment" {
		t.Fatalf("notices after deletion %+v", inbox.Items)
	}
}

func TestOfficialLeaderboard(t *testing.T) {
	w := officialInteractionFixture(t)
	c, s := w.c, w.s
	ctx := context.Background()
	slot := func(chart string, course string) int64 {
		t.Helper()
		var id int64
		if err := s.DB.QueryRow(ctx, `INSERT INTO chart_difficulties(chart_id,course,side,level) VALUES($1,$2,'single',5)
		 ON CONFLICT(chart_id,course,side) DO UPDATE SET level=EXCLUDED.level RETURNING id`, chart, course).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	oni, hard, otherOni := slot(w.song, "Oni"), slot(w.song, "Hard"), slot(w.other, "Oni")
	for i := 0; i < 23; i++ {
		if _, err := s.DB.Exec(ctx, `INSERT INTO users(id) VALUES($1)`, fmt.Sprintf("%032x", i+1000)); err != nil {
			t.Fatal(err)
		}
	}
	add := func(id, user string, difficulty, points int64, date string) {
		t.Helper()
		if _, err := s.DB.Exec(ctx, `INSERT INTO scores(id,user_id,difficulty_id,good,ok,bad,score,drumroll,max_combo,payload_digest,submitted_at) VALUES($1,$2,$3,300,2,1,$4,55,300,repeat('a',64),$5::timestamptz)`, id, user, difficulty, points, date); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 23; i++ {
		points := int64(800000 - i)
		if i < 2 {
			points = 900000
		}
		add(fmt.Sprintf("score%02d", i), fmt.Sprintf("%032x", i+1000), oni, points, "2026-01-01T00:00:00Z")
	}
	// A player's lower and later plays do not rank twice.
	add("lower", fmt.Sprintf("%032x", 1000), oni, 840000, "2025-12-01T00:00:00Z")
	add("elsewhere", fmt.Sprintf("%032x", 1000), otherOni, 1300000, "2025-12-01T00:00:00Z")
	path := w.chart(w.song) + "/leaderboard"
	var v leaderboardResponse
	if code := c.do("", "GET", path, "", &v); code != 200 || v.Course != "Oni" || v.Side != "single" || v.Total != 23 || len(v.Items) != 20 {
		t.Fatalf("summary %d %+v", code, v)
	}
	if v.Items[0].ID != "score00" || v.Items[0].Rank != 1 || v.Items[1].Rank != 1 || v.Items[2].Rank != 3 || v.Items[0].Nickname != "未知用户" {
		t.Fatalf("ties: %+v", v.Items[:3])
	}
	c.do("", "GET", path+"?page=2&difficultyId="+strconv.FormatInt(oni, 10), "", &v)
	if len(v.Items) != 3 || v.Items[0].Rank != 21 {
		t.Fatalf("page 2: %+v", v)
	}
	if code := c.do("", "GET", path+"?difficultyId="+strconv.FormatInt(hard, 10), "", &v); code != 200 || v.Course != "Hard" || v.Total != 0 || v.Items == nil {
		t.Fatalf("empty block %d %+v", code, v)
	}
	for query, want := range map[string]int{"?difficultyId=" + strconv.FormatInt(otherOni, 10): 404, "?difficultyId=Oni": 400, "?page=0": 400} {
		if code := c.do("", "GET", path+query, "", nil); code != want {
			t.Fatal(query, code)
		}
	}
	// Scores of replaced files stay stored but leave the ranking.
	if _, err := s.DB.Exec(ctx, `UPDATE scores SET is_latest=false WHERE id='score00'`); err != nil {
		t.Fatal(err)
	}
	c.do("", "GET", path, "", &v)
	if v.Items[0].ID != "score01" || v.Items[1].Rank != 2 {
		t.Fatalf("retired score ranked: %+v", v.Items[:2])
	}
	if code := c.do("", "GET", "/api/v1/charts/999999/leaderboard", "", nil); code != 404 {
		t.Fatal("missing chart", code)
	}
}

func TestUserDirectory(t *testing.T) {
	w := officialInteractionFixture(t)
	c := w.c
	var dir UserDirectory
	if code := c.do("", "GET", "/api/v1/users", "", &dir); code != 200 || dir.Total != 4 || !dir.ProfilesAvailable {
		t.Fatalf("directory %d %+v", code, dir)
	}
	for _, u := range dir.Items {
		if u.Nickname == nil {
			t.Fatal("nickname missing", u.ID)
		}
		if u.ID == testUsers["owner"].ID && u.ChartCount != 2 {
			t.Fatal("chart count", u.ChartCount)
		}
	}
	if c.do("", "GET", "/api/v1/users?q=ali", "", &dir); dir.Total != 1 || *dir.Items[0].Nickname != "alice" {
		t.Fatalf("search %+v", dir)
	}
	for path, want := range map[string]int{"/api/v1/users/ffffffffffffffffffffffffffffffff": 404, "/api/v1/users/nope": 404, "/api/v1/users?sort=karma": 400} {
		if code := c.do("", "GET", path, "", nil); code != want {
			t.Fatal(path, code)
		}
	}
}

// An administrator deleting someone's public playlist tells its owner.
func TestPlaylistRemovedNotice(t *testing.T) {
	w := officialInteractionFixture(t)
	c := w.c
	var p struct{ ID string }
	if code := c.do("alice", "POST", "/api/v1/playlists", `{"name":"Mix","public":true}`, &p); code != 201 {
		t.Fatal("create playlist", code)
	}
	if code := c.do("admin", "DELETE", "/api/v1/playlists/"+p.ID, "", nil); code != 200 {
		t.Fatal("admin delete", code)
	}
	var notes NotificationPage
	c.do("alice", "GET", "/api/v1/me/notifications", "", &notes)
	if notes.Total != 1 || notes.Items[0].Kind != "playlist_removed" || notes.Items[0].Data["name"] != "Mix" || notes.Items[0].Actor != nil {
		t.Fatalf("notice %+v", notes)
	}
}

// Only administrators upload; an owner who is not one may still manage their charts.
func TestUploadPermission(t *testing.T) {
	w := officialInteractionFixture(t)
	for name, want := range map[string]bool{"owner": false, "admin": true, "alice": false} {
		if code := w.c.do(name, "POST", "/api/v1/charts", "", nil); (code == 403) == want {
			t.Fatalf("%s upload status %d", name, code)
		}
	}
}

// Playlists have 32-hex IDs (R052) through creation, items, order, favorites and detail.
func TestPlaylistIDs(t *testing.T) {
	w := officialInteractionFixture(t)
	c := w.c
	var p Playlist
	if code := c.do("owner", "POST", "/api/v1/playlists", `{"name":"P","public":true}`, &p); code != 201 || !validID(p.ID) {
		t.Fatal("create", code, p.ID)
	}
	path := "/api/v1/playlists/" + p.ID
	for _, chart := range []string{w.song, w.other} {
		if code := c.do("owner", "PUT", path+"/items/"+chart, "", nil); code != 201 {
			t.Fatal("add", chart, code)
		}
	}
	if code := c.do("owner", "PUT", path+"/order", `{"chartIds":["`+w.other+`","`+w.song+`"]}`, nil); code != 200 {
		t.Fatal("order", code)
	}
	var detail Playlist
	if code := c.do("", "GET", path, "", &detail); code != 200 || len(detail.Charts) != 2 || detail.Charts[0].ID != w.other {
		t.Fatal("detail", code, detail.Charts)
	}
	if code := c.do("alice", "PUT", path+"/favorite", "", nil); code != 200 {
		t.Fatal("favorite", code)
	}
	var mine MyPlaylists
	if code := c.do("alice", "GET", "/api/v1/me/playlists", "", &mine); code != 200 || len(mine.Favorites) != 1 || mine.Favorites[0].ID != p.ID {
		t.Fatal("favorites", code, mine.Favorites)
	}
	if code := c.do("owner", "PATCH", path, `{"name":"Q"}`, &detail); code != 200 || detail.ID != p.ID || detail.Name != "Q" {
		t.Fatal("edit", code, detail.ID)
	}
	for _, other := range []string{"1", "0123456789ABCDEF0123456789ABCDEF", web.NewID()} {
		if code := c.do("", "GET", "/api/v1/playlists/"+other, "", nil); code != 404 {
			t.Fatal("missing playlist", other, code)
		}
	}
	if code := c.do("owner", "DELETE", path+"/items/"+w.song, "", nil); code != 200 {
		t.Fatal("remove", code)
	}
	if code := c.do("owner", "DELETE", path, "", nil); code != 200 {
		t.Fatal("delete", code)
	}
}
