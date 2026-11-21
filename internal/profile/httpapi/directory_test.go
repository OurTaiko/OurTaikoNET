package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"ourtaiko.dev/net/internal/account"
)

// directorySite is the players' part with an account center that finds "鼓手" as the player.
func directorySite(t *testing.T) (*client, func(string, ...any)) {
	t.Helper()
	c, exec, _ := directorySiteKeeping(t, true)
	return c, exec
}

// directorySiteKeeping is directorySite; keep is whether the site keeps a copy of the players' names.
func directorySiteKeeping(t *testing.T, keep bool) (*client, func(string, ...any), *account.Client) {
	t.Helper()
	pool := testDB(t)
	ctx := context.Background()
	center := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Query string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		ids := []string{}
		if body.Query == "鼓手" {
			ids = append(ids, player)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"userIds": ids})
	}))
	t.Cleanup(center.Close)
	sso, err := account.New(account.Config{Issuer: center.URL, ServiceID: "net", ServiceKey: strings.Repeat("k", 64), ClientID: "net", ClientSecret: "secret", RedirectURL: center.URL + "/callback", EncryptionKey: strings.Repeat("b", 64)})
	if err != nil {
		t.Fatal(err)
	}
	if keep {
		sso.KeepProfiles(pool)
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(sql, err)
		}
	}
	sources := []Source{
		{ID: "fanmade", Name: "Fanmade", Site: "https://fanmade.test"},
		{ID: "official", Name: "Database", Site: "https://database.test"},
	}
	return &client{t: t, handler: New(pool, Config{Origin: origin, Sources: sources, SSO: sso}).Handler()}, exec, sso
}

// noAnswer is an account center that cannot be reached.
type noAnswer struct{}

func (noAnswer) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("no answer")
}

// A player's numbers are the two sources' added together, in the list and on their page alike.
func TestThePlayersDirectoryAddsUpBothSources(t *testing.T) {
	c, exec := directorySite(t)
	exec(`INSERT INTO net.users(id,nickname,first_login_at,last_active_at,profile_checked_at) VALUES
 ($1,'鼓手','2026-01-01Z','2026-03-01Z',now()),($2,'旁观者','2026-02-01Z','2026-02-02Z',now())`, player, other)
	exec(`UPDATE net.users SET last_active_at=now(),first_login_at=now() WHERE id=$1`, account.SystemUserID)
	chart := func(source, id, owner, status string) {
		t.Helper()
		exec(fmt.Sprintf(`WITH tja AS (INSERT INTO %[1]s.chart_resources(storage_key,original_filename,sha256,byte_size,extension) VALUES ($1||$1||'.tja','a.tja',$1||$1,1,'tja') RETURNING id),
 audio AS (INSERT INTO %[1]s.chart_resources(storage_key,original_filename,sha256,byte_size,extension) VALUES (repeat('a',32)||$1||'.ogg','a.ogg',repeat('a',32)||$1,1,'ogg') RETURNING id),
 chart AS (INSERT INTO %[1]s.charts(id,owner_ids,title,bpm,duration,demo_end,tja_file,audio_file,status) SELECT $1,ARRAY[$2],'Song',120,60,10,tja.id,audio.id,$3 FROM tja,audio RETURNING id)
INSERT INTO %[1]s.chart_difficulties(chart_id,course,side,level,content_hash) SELECT id,'Oni','single',9,'v2:'||repeat('0',64) FROM chart`, source), id, owner, status)
	}
	play := func(source, id, chartID string) {
		t.Helper()
		exec(fmt.Sprintf(`INSERT INTO net.scores(source,id,user_id,good,ok,bad,score,drumroll,max_combo,payload_digest,difficulty_id,clear_status)
 SELECT $1,$2,$3,1,0,0,900000,0,1,repeat('d',64),d.id,1 FROM %s.chart_difficulties d WHERE d.chart_id=$4`, source), source, id, player, chartID)
	}
	mine, hidden, official := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32)
	chart("fanmade", mine, player, "published")
	chart("fanmade", hidden, player, "hidden")
	// Official charts belong to the built-in account: playing one makes it nobody's work.
	chart("official", official, account.SystemUserID, "published")
	play("fanmade", "f1", mine)
	play("fanmade", "f2", hidden)
	play("official", "o1", official)
	play("official", "o2", official)
	comment := func(source, id, chartID string) {
		t.Helper()
		exec(fmt.Sprintf(`INSERT INTO %s.comments(id,chart_id,root_id,depth,author_id,body) VALUES ($1,$2,$1,0,$3,'hello')`, source), id, chartID, player)
	}
	comment("fanmade", "c1", mine)
	comment("fanmade", "c2", hidden)
	comment("official", "c3", official)
	// What was deleted or taken down is not counted.
	comment("fanmade", "c4", mine)
	comment("fanmade", "c5", mine)
	exec(`UPDATE fanmade.comments SET deleted_at=now(),body='' WHERE id='c4'`)
	exec(`UPDATE fanmade.comments SET removed_at=now(),body='' WHERE id='c5'`)
	// Others' votes count, the player's own do not.
	exec(`INSERT INTO fanmade.chart_votes(chart_id,user_id,value) VALUES ($1,$2,1),($1,$3,1)`, mine, other, player)
	exec(`INSERT INTO official.comment_votes(comment_id,user_id,value) VALUES ('c3',$1,1)`, other)
	exec(`INSERT INTO official.dan_courses(id,owner_id,title,exams,code) VALUES ($1,$2,'{"ja":"初段"}','[]','ABC123')`, strings.Repeat("4", 32), player)
	exec(`INSERT INTO official.dan_votes(dan_id,user_id,value) VALUES ($1,$2,-1)`, strings.Repeat("4", 32), other)
	for i, public := range []bool{true, false} {
		exec(`INSERT INTO fanmade.playlists(id,owner_id,name,is_public) VALUES ($1,$2,'List',$3)`, fmt.Sprintf("%032d", i+5), player, public)
	}
	exec(`INSERT INTO official.playlists(id,owner_id,name,is_public) VALUES ($1,$2,'List',true)`, strings.Repeat("7", 32), player)

	// Votes on what is no longer on show do not count either.
	exec(`INSERT INTO fanmade.chart_votes(chart_id,user_id,value) VALUES ($1,$2,1)`, hidden, other)
	exec(`INSERT INTO fanmade.comment_votes(comment_id,user_id,value) VALUES ('c2',$1,1),('c4',$1,1),('c5',$1,1)`, other)
	exec(`INSERT INTO official.dan_courses(id,owner_id,title,exams,code,status) VALUES ($1,$2,'{"ja":"二段"}','[]','ABC124','deleted')`, strings.Repeat("8", 32), player)
	exec(`INSERT INTO official.dan_votes(dan_id,user_id,value) VALUES ($1,$2,1)`, strings.Repeat("8", 32), other)

	want := PlayerTotals{ChartCount: 1, ScoreCount: 3, CommentCount: 2, Karma: 1, PlaylistCount: 2}
	check := func(about string, got PlayerTotals) {
		t.Helper()
		if got.FirstLoginAt == nil || got.LastActiveAt == nil {
			t.Fatalf("%s: no times: %+v", about, got)
		}
		got.FirstLoginAt, got.LastActiveAt = nil, nil
		if got != want {
			t.Fatalf("%s: %+v, want %+v", about, got, want)
		}
	}
	var list UserDirectory
	c.json("GET", "/api/users", "", 200, &list)
	if list.Total != 2 || len(list.Items) != 2 || list.Items[0].ID != player || !list.ProfilesAvailable {
		t.Fatalf("%+v", list)
	}
	if list.Items[0].Nickname == nil || *list.Items[0].Nickname != "鼓手" {
		t.Fatalf("%+v", list.Items[0])
	}
	check("the list", list.Items[0].PlayerTotals)
	var page struct {
		User struct {
			ID       string
			Nickname string
			PlayerTotals
		}
		TotalsAvailable bool
	}
	c.json("GET", "/api/users/"+player, "", 200, &page)
	if page.User.ID != player || page.User.Nickname != "鼓手" || !page.TotalsAvailable {
		t.Fatalf("%+v", page.User)
	}
	check("their page", page.User.PlayerTotals)

	c.json("GET", "/api/users?sort=newest", "", 200, &list)
	if list.Items[0].ID != other {
		t.Fatalf("newest first: %+v", list.Items)
	}
	c.json("GET", "/api/users?q="+url.QueryEscape("鼓手"), "", 200, &list)
	if list.Total != 1 || list.Items[0].ID != player {
		t.Fatalf("search: %+v", list)
	}
	c.json("GET", "/api/users?q=nobody", "", 200, &list)
	if list.Total != 0 || list.Items == nil {
		t.Fatalf("no match: %+v", list)
	}
	// The built-in account is nobody's page, and it is in no list (Total above).
	c.json("GET", "/api/users/"+account.SystemUserID, "", 404, nil)
}

// Without the account center the list still shows the players and their numbers, and a search
// says it could not be made instead of finding nobody.
func TestThePlayersDirectoryWithoutTheAccountCenter(t *testing.T) {
	c, exec, sso := directorySiteKeeping(t, false)
	exec(`INSERT INTO net.users(id,nickname,profile_checked_at) VALUES ($1,'鼓手',now())`, player)
	sso.HTTP = &http.Client{Transport: noAnswer{}}
	var list UserDirectory
	c.json("GET", "/api/users", "", 200, &list)
	if list.Total != 1 || len(list.Items) != 1 || list.ProfilesAvailable || list.Items[0].Nickname != nil {
		t.Fatalf("%+v", list)
	}
	c.json("GET", "/api/users?q=hello", "", 503, nil)
}

// A player's page stays up when their numbers cannot be added up, and says they are missing.
func TestThePlayersPageOutlivesTheirTotals(t *testing.T) {
	c, exec := directorySite(t)
	exec(`INSERT INTO net.users(id,nickname,profile_checked_at) VALUES ($1,'鼓手',now())`, player)
	exec(`DROP TABLE official.playlists CASCADE`)
	var page struct {
		User struct {
			Nickname string
			PlayerTotals
		}
		TotalsAvailable bool
	}
	c.json("GET", "/api/users/"+player, "", 200, &page)
	if page.User.Nickname != "鼓手" || page.TotalsAvailable || page.User.ChartCount != 0 {
		t.Fatalf("%+v", page)
	}
}

// Karma ranks the players from high to low, below zero last; equals go by their last visit.
func TestThePlayersDirectoryByKarma(t *testing.T) {
	c, exec := directorySite(t)
	third, fourth := strings.Repeat("c", 32), strings.Repeat("d", 32)
	exec(`INSERT INTO net.users(id,nickname,last_active_at,profile_checked_at) VALUES
 ($1,'高',  '2026-01-01Z',now()),($2,'低',  '2026-04-01Z',now()),($3,'零新','2026-03-01Z',now()),($4,'零旧','2026-02-01Z',now())`, player, other, third, fourth)
	course := func(id, owner, code string) {
		t.Helper()
		exec(`INSERT INTO official.dan_courses(id,owner_id,title,exams,code) VALUES ($1,$2,'{"ja":"初段"}','[]',$3)`, id, owner, code)
	}
	high, low := strings.Repeat("4", 32), strings.Repeat("5", 32)
	course(high, player, "ABC123")
	course(low, other, "ABC124")
	exec(`INSERT INTO official.dan_votes(dan_id,user_id,value) VALUES ($1,$2,1),($1,$3,1),($4,$2,-1)`, high, third, fourth, low)
	var list UserDirectory
	c.json("GET", "/api/users?sort=karma", "", 200, &list)
	want := []string{player, third, fourth, other}
	if len(list.Items) != len(want) {
		t.Fatalf("%+v", list.Items)
	}
	for i, id := range want {
		if list.Items[i].ID != id {
			t.Fatalf("place %d: %+v", i, list.Items)
		}
	}
	if list.Items[0].Karma != 2 || list.Items[3].Karma != -1 {
		t.Fatalf("%+v", list.Items)
	}
}

func TestThePlayersDirectoryPages(t *testing.T) {
	c, exec := directorySite(t)
	for i := range defaultDirectoryPageSize + 1 {
		exec(`INSERT INTO net.users(id,nickname,profile_checked_at) VALUES ($1,$2,now())`, fmt.Sprintf("%032x", i+1), fmt.Sprintf("P%d", i))
	}
	var list UserDirectory
	c.json("GET", "/api/users", "", 200, &list)
	if list.Total != defaultDirectoryPageSize+1 || len(list.Items) != defaultDirectoryPageSize || list.PageSize != defaultDirectoryPageSize || list.Page != 1 {
		t.Fatalf("%+v", list)
	}
	c.json("GET", "/api/users?page=2", "", 200, &list)
	if list.Total != defaultDirectoryPageSize+1 || len(list.Items) != 1 || list.Page != 2 {
		t.Fatalf("%+v", list)
	}
	// The client may say how many a page has.
	c.json("GET", "/api/users?pageSize=5&page=2", "", 200, &list)
	if list.Total != defaultDirectoryPageSize+1 || len(list.Items) != 5 || list.PageSize != 5 || list.Page != 2 {
		t.Fatalf("%+v", list)
	}
	c.json("GET", fmt.Sprintf("/api/users?pageSize=%d", maxDirectoryPageSize), "", 200, &list)
	if len(list.Items) != defaultDirectoryPageSize+1 || list.PageSize != maxDirectoryPageSize {
		t.Fatalf("%+v", list)
	}
	for _, query := range []string{"page=0", "page=bad", "page=1.5", "page=10001", "sort=invalid", "q=" + strings.Repeat("x", 201), "pageSize=0", "pageSize=bad", "pageSize=101"} {
		c.json("GET", "/api/users?"+query, "", 400, nil)
	}
	// Only what is public leaves the server.
	w := c.do("GET", "/api/users", "", nil)
	var raw struct{ Items []map[string]any }
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	public := []string{"id", "nickname", "avatarPath", "firstLoginAt", "lastActiveAt", "chartCount", "scoreCount", "commentCount", "karma", "playlistCount"}
	if len(raw.Items[0]) != len(public) {
		t.Fatalf("fields: %v", raw.Items[0])
	}
	for _, field := range public {
		if _, ok := raw.Items[0][field]; !ok {
			t.Fatalf("no %s: %v", field, raw.Items[0])
		}
	}
}
