package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	_ "image/jpeg"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const (
	playlistOwner = "d46774d30dd13b92d9e536808da468a4"
	playlistAlice = "9b893bc6d9422c93536ff0df503b81e9"
)

func playlistIDs(charts []Chart) string {
	ids := []string{}
	for _, c := range charts {
		ids = append(ids, c.ID)
	}
	return strings.Join(ids, ",")
}

// The fixture's requests share one address; start a new rate-limit window.
func resetRateLimit(s *Server) {
	s.mu.Lock()
	clear(s.limits)
	s.mu.Unlock()
}

func TestPlaylistLifecycle(t *testing.T) {
	world := interactionWorldOf(t, Fanmade)
	c, s := world.c, world.s
	ctx := context.Background()
	// The two charts by name, where a result lists several.
	named := strings.NewReplacer(world.song, "song", world.other, "other").Replace
	for _, body := range []string{`{}`, `{"name":"  "}`, `{"name":"` + strings.Repeat("歌", 51) + `"}`, `{"name":"a","categoryId":"none"}`,
		`{"name":"a","description":"` + strings.Repeat("字", 501) + `"}`, `{"name":"a\u0007"}`} {
		if code := c.do("owner", "POST", "/api/playlists", body, nil); code != 400 {
			t.Fatal("invalid playlist accepted", body, code)
		}
	}
	if code := c.do("", "POST", "/api/playlists", `{"name":"Mine"}`, nil); code != 401 {
		t.Fatal("guest created a playlist", code)
	}
	var p Playlist
	if code := c.do("owner", "POST", "/api/playlists", `{"name":" Mine ","description":"first\r\nline"}`, &p); code != 201 ||
		p.Name != "Mine" || p.Description != "first\nline" || p.Public || p.System || p.CategoryID != "variety" || p.Genre != "VARIETY" ||
		p.OwnerID != playlistOwner || p.Owner == nil || p.ChartCount != 0 || p.CreatedAt == nil {
		t.Fatalf("created %d %+v", code, p)
	}
	path := "/api/playlists/" + p.ID
	// A private playlist exists only for its owner.
	for user, want := range map[string]int{"": 404, "alice": 404, "admin": 404, "owner": 200} {
		if code := c.do(user, "GET", path, "", nil); code != want {
			t.Fatal("private visibility", user, code)
		}
	}
	for _, write := range [][2]string{{"PATCH", path}, {"DELETE", path}, {"PUT", path + "/items/" + world.song + ""}, {"DELETE", path + "/items/" + world.song + ""}, {"PUT", path + "/order"}, {"PUT", path + "/favorite"}} {
		if code := c.do("alice", write[0], write[1], `{}`, nil); code != 404 {
			t.Fatal("private playlist reachable", write, code)
		}
	}

	resetRateLimit(s)
	add := func(user, chart string, want int) {
		t.Helper()
		if code := c.do(user, "PUT", path+"/items/"+chart, `{}`, nil); code != want {
			t.Fatalf("add %s by %s: %d want %d", chart, user, code, want)
		}
	}
	add("owner", world.song, 201)
	add("owner", world.song, 200)
	add("owner", "missing", 404)
	add("owner", world.other, 201)
	detail := func(user string) Playlist {
		t.Helper()
		var v Playlist
		if code := c.do(user, "GET", path, "", &v); code != 200 {
			t.Fatal("detail", code)
		}
		return v
	}
	if v := detail("owner"); named(playlistIDs(v.Charts)) != "song,other" || v.ChartCount != 2 || v.Charts[0].Author == "" {
		t.Fatalf("order %s %+v", playlistIDs(v.Charts), v)
	}
	order := func(body string, want int) {
		t.Helper()
		if code := c.do("owner", "PUT", path+"/order", body, nil); code != want {
			t.Fatalf("order %s: %d want %d", body, code, want)
		}
	}
	order(`{"chartIds":["`+world.other+`","`+world.other+`"]}`, 400)
	order(`{"chartIds":["`+world.other+`","gone"]}`, 409)
	order(`{"chartIds":["`+world.other+`","`+world.song+`"]}`, 200)
	if v := detail("owner"); named(playlistIDs(v.Charts)) != "other,song" {
		t.Fatal("reordered", playlistIDs(v.Charts))
	}
	// Songs the request leaves out keep their place behind the listed ones.
	order(`{"chartIds":["`+world.song+`"]}`, 200)
	if v := detail("owner"); named(playlistIDs(v.Charts)) != "song,other" {
		t.Fatal("partial order", playlistIDs(v.Charts))
	}

	var mine MyPlaylists
	if code := c.do("owner", "GET", "/api/me/playlists?chart="+world.other, "", &mine); code != 200 || len(mine.Owned) != 1 || len(mine.Favorites) != 0 ||
		mine.Owned[0].ContainsChart == nil || !*mine.Owned[0].ContainsChart || mine.MaxItems != maxPlaylistItems {
		t.Fatalf("mine %d %+v", code, mine)
	}
	if code := c.do("", "GET", "/api/me/playlists", "", nil); code != 401 {
		t.Fatal("guest library", code)
	}

	resetRateLimit(s)
	// Publishing; an empty description clears it and absent fields are kept.
	if code := c.do("owner", "PATCH", path, `{"public":true,"description":"","categoryId":"game"}`, &p); code != 200 ||
		!p.Public || p.Description != "" || p.Name != "Mine" || p.Genre != "GAME" || p.ChartCount != 2 {
		t.Fatalf("published %d %+v", code, p)
	}
	if code := c.do("alice", "PATCH", path, `{"name":"Stolen"}`, nil); code != 403 {
		t.Fatal("foreign edit", code)
	}
	add("alice", world.song, 403)
	if code := c.do("", "GET", path, "", &p); code != 200 || len(p.Charts) != 2 {
		t.Fatal("public detail", code)
	}

	favorite := func(user, method string, want, total int) {
		t.Helper()
		var result struct {
			Favorited     bool
			FavoriteCount int
		}
		if code := c.do(user, method, path+"/favorite", `{}`, &result); code != want || (want == 200 && (result.FavoriteCount != total || result.Favorited != (method == "PUT"))) {
			t.Fatalf("favorite %s %s: %d %+v", user, method, code, result)
		}
	}
	favorite("", "PUT", 401, 0)
	favorite("owner", "PUT", 409, 0)
	favorite("alice", "PUT", 200, 1)
	favorite("alice", "PUT", 200, 1)
	favorite("bob", "PUT", 200, 2)
	favorite("bob", "DELETE", 200, 1)
	if v := detail("alice"); !v.Favorited || v.FavoriteCount != 1 {
		t.Fatalf("favorited detail %+v", v)
	}
	if code := c.do("alice", "GET", "/api/me/playlists", "", &mine); code != 200 || len(mine.Owned) != 0 || len(mine.Favorites) != 1 || mine.Favorites[0].ID != p.ID {
		t.Fatalf("alice library %d %+v", code, mine)
	}

	resetRateLimit(s)
	// Plaza: public playlists with songs; a user's page also lists empty ones.
	var empty Playlist
	if code := c.do("owner", "POST", "/api/playlists", `{"name":"Empty","public":true}`, &empty); code != 201 {
		t.Fatal("empty playlist", code)
	}
	var page PlaylistPage
	list := func(query string, want ...string) {
		t.Helper()
		if code := c.do("alice", "GET", "/api/playlists"+query, "", &page); code != 200 || page.Total != len(want) || len(page.Items) != len(want) {
			t.Fatalf("list %s: %d %+v", query, code, page)
		}
		for i, id := range want {
			if page.Items[i].ID != id {
				t.Fatalf("list %s item %d: %s", query, i, page.Items[i].ID)
			}
		}
	}
	list("", p.ID)
	if !page.Items[0].Favorited || page.Items[0].Owner == nil || page.Items[0].Charts != nil {
		t.Fatalf("plaza item %+v", page.Items[0])
	}
	list("?q=MIN", p.ID)
	list("?q=%25")
	list("?sort=favorites&owner="+playlistOwner, p.ID, empty.ID)
	list("?owner=" + playlistAlice)
	// By id: the public ones of those named, in the order named, empty ones too; one that
	// does not exist is left out.
	list("?ids="+empty.ID+","+strings.Repeat("0", 32)+","+p.ID, empty.ID, p.ID)
	if page.Items[1].Owner == nil || page.Items[1].Charts != nil || page.Items[1].ChartCount != 2 {
		t.Fatalf("named item %+v", page.Items[1])
	}
	for _, query := range []string{"?sort=random", "?page=0", "?owner=x", "?ids=liked", "?ids=" + p.ID + ",", "?ids=" + strings.Repeat(p.ID+",", playlistPageSize) + p.ID} {
		if code := c.do("", "GET", "/api/playlists"+query, "", nil); code != 400 {
			t.Fatal("invalid query", query, code)
		}
	}

	resetRateLimit(s)
	// A playlist that turns private hides from those who keep it, and returns with it.
	c.do("owner", "PATCH", path, `{"public":false}`, nil)
	if code := c.do("alice", "GET", "/api/me/playlists", "", &mine); code != 200 || len(mine.Favorites) != 0 {
		t.Fatal("private favorite shown", code)
	}
	if code := c.do("alice", "GET", path, "", nil); code != 404 {
		t.Fatal("private favorite readable", code)
	}
	list("?ids=" + p.ID)
	favorite("alice", "DELETE", 200, 0)
	c.do("owner", "PATCH", path, `{"public":true}`, nil)
	favorite("alice", "PUT", 200, 1)

	// The game receives own playlists (private too), favorites and system ones, without empty folders.
	game := func(user string) []GamePlaylist {
		t.Helper()
		tx, e := s.DB.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer tx.Rollback(ctx)
		var u *User
		if user != "" {
			u = &User{ID: user}
		}
		items, e := s.gamePlaylists(ctx, tx, u)
		if e != nil {
			t.Fatal(e)
		}
		if e = s.gamePlaylistDetails(ctx, items); e != nil {
			t.Fatal(e)
		}
		return items
	}
	kinds := func(items []GamePlaylist) string {
		out := []string{}
		for _, v := range items {
			out = append(out, v.Kind+":"+v.Title+":"+v.Genre+":"+named(strings.Join(v.ChartIDs, "+")))
		}
		return strings.Join(out, " ")
	}
	c.do("owner", "PATCH", "/api/playlists/"+empty.ID, `{"public":false}`, nil)
	c.do("owner", "PUT", "/api/playlists/"+empty.ID+"/items/"+world.other+"", `{}`, nil)
	c.do("alice", "PUT", "/api/charts/"+world.other+"/vote", `{"value":1}`, nil)
	if got := kinds(game(playlistOwner)); !strings.HasPrefix(got, "owned:Empty:VARIETY:other owned:Mine:GAME:song+other system:") || strings.Contains(got, "Liked") {
		t.Fatal("owner folders", got)
	}
	alice := game(playlistAlice)
	if got := kinds(alice); !strings.HasPrefix(got, "favorite:Mine:GAME:song+other system:Liked Songs:J-POP:other ") || alice[0].Owner == "" {
		t.Fatal("alice folders", got)
	}
	guest := kinds(game(""))
	if strings.Contains(guest, "Mine") || strings.Contains(guest, "Liked") || !strings.Contains(guest, "system:Daily Challenge:VARIETY:") || !strings.Contains(guest, "system:Recently Acclaimed:CLASSICAL:other") {
		t.Fatal("guest folders", guest)
	}
	// The account's own folders travel apart from the public catalog.
	if own := kinds(personalGamePlaylists(alice)); own != "favorite:Mine:GAME:song+other system:Liked Songs:J-POP:other" {
		t.Fatal("alice personal folders", own)
	}
	if own := personalGamePlaylists(game("")); len(own) != 0 {
		t.Fatal("guest personal folders", kinds(own))
	}
	r := httptest.NewRequest("GET", "/api/game/catalog", nil)
	w := httptest.NewRecorder()
	c.handler.ServeHTTP(w, r)
	var catalog struct {
		Playlists       []GamePlaylist
		PlaylistVersion int
	}
	if e := json.Unmarshal(w.Body.Bytes(), &catalog); e != nil || w.Code != 200 || catalog.PlaylistVersion != 1 || kinds(catalog.Playlists) != guest {
		t.Fatal("catalog", w.Code, e, kinds(catalog.Playlists))
	}

	resetRateLimit(s)
	// System playlists on the website.
	var systems struct{ Items []Playlist }
	if code := c.do("alice", "GET", "/api/playlists/system", "", &systems); code != 200 || len(systems.Items) != 3 || systems.Items[0].ID != "liked" || !systems.Items[0].System || systems.Items[0].Owner != nil {
		t.Fatalf("systems %d %+v", code, systems)
	}
	if code := c.do("", "GET", "/api/playlists/system", "", &systems); code != 200 || len(systems.Items) != 2 || systems.Items[0].ID != "acclaimed" {
		t.Fatalf("guest systems %d %+v", code, systems)
	}
	var liked Playlist
	if code := c.do("alice", "GET", "/api/playlists/liked", "", &liked); code != 200 || playlistIDs(liked.Charts) != world.other || !liked.System || liked.Public {
		t.Fatalf("liked %d %+v", code, liked)
	}
	if code := c.do("", "GET", "/api/playlists/liked", "", nil); code != 401 {
		t.Fatal("guest liked", code)
	}
	if code := c.do("", "GET", "/api/playlists/daily", "", &liked); code != 200 || len(liked.Charts) != 2 {
		t.Fatal("daily", code, len(liked.Charts))
	}
	for _, write := range [][2]string{{"PATCH", ""}, {"DELETE", ""}, {"PUT", "/items/" + world.song + ""}, {"PUT", "/favorite"}} {
		if code := c.do("alice", write[0], "/api/playlists/liked"+write[1], `{}`, nil); code != 404 {
			t.Fatal("system playlist written", write, code)
		}
	}

	resetRateLimit(s)
	// A deleted song leaves the playlist.
	if code := c.do("owner", "DELETE", "/api/charts/"+world.song+"", `{}`, nil); code != 200 {
		t.Fatal("delete song", code)
	}
	if v := detail("alice"); playlistIDs(v.Charts) != world.other || v.ChartCount != 1 {
		t.Fatal("deleted song listed", playlistIDs(v.Charts))
	}
	if code := c.do("owner", "DELETE", path+"/items/"+world.other+"", `{}`, nil); code != 200 {
		t.Fatal("remove", code)
	}
	if v := detail("alice"); len(v.Charts) != 0 {
		t.Fatal("removed song listed")
	}

	// Administrators may delete public playlists only; the owner is told.
	if code := c.do("alice", "DELETE", path, `{}`, nil); code != 403 {
		t.Fatal("foreign delete", code)
	}
	if code := c.do("admin", "DELETE", "/api/playlists/"+empty.ID, `{}`, nil); code != 404 {
		t.Fatal("admin deleted a private playlist", code)
	}
	if code := c.do("admin", "DELETE", path, `{}`, nil); code != 200 {
		t.Fatal("admin delete", code)
	}
	var notices NotificationPage
	if code := c.do("owner", "GET", "/api/me/notifications", "", &notices); code != 200 || len(notices.Items) == 0 ||
		notices.Items[0].Kind != "playlist_removed" || notices.Items[0].Data["name"] != "Mine" || notices.Items[0].Actor != nil {
		t.Fatalf("removal notice %d %+v", code, notices)
	}
	if code := c.do("owner", "DELETE", "/api/playlists/"+empty.ID, `{}`, nil); code != 200 {
		t.Fatal("own delete", code)
	}
	var left int
	if e := s.DB.QueryRow(ctx, `SELECT (SELECT count(*) FROM playlists)+(SELECT count(*) FROM playlist_items)+(SELECT count(*) FROM playlist_favorites)`).Scan(&left); e != nil || left != 0 {
		t.Fatal("rows left", left, e)
	}
}

func TestPlaylistLimits(t *testing.T) {
	world := interactionWorldOf(t, Fanmade)
	c, s := world.c, world.s
	ctx := context.Background()
	// Stored playlists have ids of 32 hex digits; these are 1, 2, … padded with zeros.
	firstListed := strings.Repeat("0", 31) + "1"
	if _, e := s.DB.Exec(ctx, `INSERT INTO playlists(id,owner_id,name,is_public) SELECT lpad(n::text,32,'0'),$1,'List '||n,true FROM generate_series(1,$2) n`, playlistOwner, maxPlaylists-1); e != nil {
		t.Fatal(e)
	}
	// Concurrent creations cannot exceed the limit.
	var wg sync.WaitGroup
	codes := make(chan int, 4)
	for range 4 {
		wg.Add(1)
		go func() { defer wg.Done(); codes <- c.do("owner", "POST", "/api/playlists", `{"name":"Last"}`, nil) }()
	}
	wg.Wait()
	close(codes)
	created := 0
	for code := range codes {
		if code == 201 {
			created++
		} else if code != 409 {
			t.Fatal("create at limit", code)
		}
	}
	if created != 1 {
		t.Fatal("created over the limit", created)
	}
	if _, e := s.DB.Exec(ctx, `INSERT INTO charts(id,owner_ids,title,bpm,duration,tja_file,audio_file)
 SELECT 'c'||n,ARRAY[$1::text],'Song '||n,120,10,test_file('song/tja','a.tja',repeat('a',64),14),test_file('song/audio','a.ogg',repeat('a',64),14) FROM generate_series(1,$2) n`, playlistOwner, maxPlaylistItems); e != nil {
		t.Fatal(e)
	}
	if _, e := s.DB.Exec(ctx, `INSERT INTO playlist_items(playlist_id,chart_id,position) SELECT lpad('1',32,'0'),'c'||n,n FROM generate_series(1,$1) n`, maxPlaylistItems); e != nil {
		t.Fatal(e)
	}
	if code := c.do("owner", "PUT", "/api/playlists/"+firstListed+"/items/"+world.song+"", `{}`, nil); code != 409 {
		t.Fatal("add to a full playlist", code)
	}
	if code := c.do("owner", "PUT", "/api/playlists/"+firstListed+"/items/c7", `{}`, nil); code != 200 {
		t.Fatal("existing song of a full playlist", code)
	}
	var p Playlist
	if code := c.do("", "GET", "/api/playlists/"+firstListed, "", &p); code != 200 || len(p.Charts) != maxPlaylistItems || p.Charts[0].ID != "c1" || p.Charts[maxPlaylistItems-1].ID != fmt.Sprint("c", maxPlaylistItems) {
		t.Fatal("full playlist", code, len(p.Charts))
	}
	var page PlaylistPage
	if code := c.do("", "GET", "/api/playlists?page=2&owner="+playlistOwner, "", &page); code != 200 || page.Total != maxPlaylists-1 || len(page.Items) != playlistPageSize {
		t.Fatal("second page", code, page.Total, len(page.Items))
	}
}

// A playlist names the play backdrop the game uses for songs opened from it: its owner chooses
// it, an administrator may choose it for any public playlist and change nothing else, and the
// game's folders and the categories carry it.
func TestPlaylistScenePreset(t *testing.T) {
	world := interactionWorldOf(t, Fanmade)
	c, s := world.c, world.s
	ctx := context.Background()
	for _, preset := range []string{"no such-one", "间", strings.Repeat("A", 33), "A B"} {
		if code := c.do("owner", "POST", "/api/playlists", `{"name":"a","scenePreset":"`+preset+`"}`, nil); code != 400 {
			t.Fatal("invalid scene preset accepted", preset, code)
		}
	}
	var p Playlist
	if code := c.do("owner", "POST", "/api/playlists", `{"name":"Plain"}`, &p); code != 201 || p.ScenePreset != "" {
		t.Fatalf("created without one %d %+v", code, p)
	}
	if code := c.do("owner", "POST", "/api/playlists", `{"name":"Stage","scenePreset":" TOHO "}`, &p); code != 201 || p.ScenePreset != "TOHO" {
		t.Fatalf("created %d %+v", code, p)
	}
	path := "/api/playlists/" + p.ID
	// Another field leaves it; an empty one clears it.
	if code := c.do("owner", "PATCH", path, `{"description":"x"}`, &p); code != 200 || p.ScenePreset != "TOHO" || p.Description != "x" {
		t.Fatalf("kept %d %+v", code, p)
	}
	if code := c.do("owner", "PATCH", path, `{"scenePreset":""}`, &p); code != 200 || p.ScenePreset != "" {
		t.Fatalf("cleared %d %+v", code, p)
	}
	if code := c.do("owner", "PATCH", path, `{"scenePreset":"not ok"}`, nil); code != 400 {
		t.Fatal("invalid edit", code)
	}
	if code := c.do("owner", "PATCH", path, `{"scenePreset":"Miku_2"}`, &p); code != 200 || p.ScenePreset != "Miku_2" {
		t.Fatalf("set %d %+v", code, p)
	}

	// Private, it is nobody else's to see, an administrator's neither.
	for _, user := range []string{"alice", "admin"} {
		if code := c.do(user, "PATCH", path, `{"scenePreset":"GUMI"}`, nil); code != 404 {
			t.Fatal("private playlist reachable", user, code)
		}
	}
	if code := c.do("owner", "PATCH", path, `{"public":true}`, &p); code != 200 || !p.Public {
		t.Fatalf("published %d %+v", code, p)
	}
	updated := *p.UpdatedAt
	if code := c.do("alice", "PATCH", path, `{"scenePreset":"GUMI"}`, nil); code != 403 {
		t.Fatal("another player chose the backdrop", code)
	}
	// An administrator chooses the backdrop, and only that.
	for _, body := range []string{`{"scenePreset":"GUMI","name":"Theirs"}`, `{"name":"Theirs"}`, `{"scenePreset":"GUMI","public":false}`, `{}`} {
		if code := c.do("admin", "PATCH", path, body, nil); code != 403 {
			t.Fatal("administrator edited a playlist", body, code)
		}
	}
	var chosen Playlist
	if code := c.do("admin", "PATCH", path, `{"scenePreset":"GUMI"}`, &chosen); code != 200 || chosen.ScenePreset != "GUMI" || chosen.Name != "Stage" ||
		chosen.OwnerID != playlistOwner || !chosen.UpdatedAt.Equal(updated) {
		t.Fatalf("administrator's choice %d %+v (was updated %v)", code, chosen, updated)
	}
	var mine MyPlaylists
	if code := c.do("owner", "GET", "/api/me/playlists", "", &mine); code != 200 || len(mine.Owned) != 2 || mine.Owned[0].ScenePreset != "GUMI" || mine.Owned[1].ScenePreset != "" {
		t.Fatalf("listed %d %+v", code, mine.Owned)
	}

	// The game's folder of the playlist carries it; a system playlist names none.
	resetRateLimit(s)
	if code := c.do("owner", "PUT", path+"/items/"+world.song, `{}`, nil); code != 201 {
		t.Fatal("add", code)
	}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	folders, e := s.gamePlaylists(ctx, tx, &User{ID: playlistOwner})
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, folder := range folders {
		if folder.ID == p.ID {
			found = folder.ScenePreset == "GUMI"
		} else if folder.Kind == "system" && folder.ScenePreset != "" {
			t.Fatalf("system playlist names a backdrop %+v", folder)
		}
	}
	if !found {
		t.Fatalf("the game's folder has no scene preset: %+v", folders)
	}
	encoded, _ := json.Marshal(folders)
	if !strings.Contains(string(encoded), `"scenePreset":"GUMI"`) {
		t.Fatal("not in the game's answer", string(encoded))
	}

	// Every category carries the field, empty until its source names one.
	var categories struct {
		Items []map[string]any `json:"items"`
	}
	if code := c.do("", "GET", "/api/categories", "", &categories); code != 200 || len(categories.Items) == 0 {
		t.Fatal("categories", code)
	}
	for _, category := range categories.Items {
		if preset, ok := category["scenePreset"]; !ok || preset != "" {
			t.Fatalf("category %+v", category)
		}
	}
}
