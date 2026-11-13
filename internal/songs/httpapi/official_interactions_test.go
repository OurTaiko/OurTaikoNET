package httpapi

import (
	"testing"

	"ourtaiko.dev/net/internal/web"
)

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
