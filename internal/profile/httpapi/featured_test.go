package httpapi

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestTheHomePageRecommendsPlaylists(t *testing.T) {
	pool := testDB(t)
	ctx := context.Background()
	sources := []Source{{ID: "fanmade"}, {ID: "official"}}
	c := &client{t: t, handler: New(pool, Config{Origin: origin, Sources: sources, DevUserID: player}).Handler()}
	if _, err := pool.Exec(ctx, `INSERT INTO net.users(id) VALUES ($1)`, other); err != nil {
		t.Fatal(err)
	}
	// A playlist of a source, named by one hex digit.
	id := func(digit string) string { return strings.Repeat(digit, 32) }
	playlist := func(source, digit string, public bool) {
		t.Helper()
		if _, err := pool.Exec(ctx, fmt.Sprintf(`INSERT INTO %s.playlists(id,owner_id,name,is_public) VALUES ($1,$2,'List '||$3,$4)`, source), id(digit), other, digit, public); err != nil {
			t.Fatal(source, digit, err)
		}
	}
	playlist("fanmade", "a", true)
	playlist("fanmade", "b", true)
	playlist("fanmade", "c", false)
	playlist("official", "a", true)

	var answer struct {
		Items []FeaturedPlaylist
		Max   int
	}
	read := func(want ...FeaturedPlaylist) {
		t.Helper()
		c.json("GET", "/api/featured-playlists", "", 200, &answer)
		if fmt.Sprint(answer.Items) != fmt.Sprint(want) || answer.Items == nil || answer.Max != maxFeaturedPlaylists {
			t.Fatalf("recommended %+v, want %+v", answer, want)
		}
	}
	item := func(source, digit, note string) string {
		return fmt.Sprintf(`{"source":%q,"playlistId":%q,"note":%q}`, source, id(digit), note)
	}
	put := func(status int, items ...string) {
		t.Helper()
		c.json("PUT", "/api/admin/featured-playlists", `{"items":[`+strings.Join(items, ",")+`]}`, status, nil)
	}

	// Anyone reads the list; only the administrator writes it.
	read()
	put(401, item("fanmade", "a", ""))
	c.signIn()

	// The order sent is the order shown; a note is trimmed.
	put(200, item("official", "a", "  From the arcade  "), item("fanmade", "b", ""), item("fanmade", "a", "入门"))
	read(FeaturedPlaylist{"official", id("a"), "From the arcade"}, FeaturedPlaylist{"fanmade", id("b"), ""}, FeaturedPlaylist{"fanmade", id("a"), "入门"})

	// A list that cannot be kept changes nothing: a source that is none, an id that is none,
	// a playlist that does not exist or is private, one named twice, a note too long or of
	// several lines, too many, no list at all.
	kept := answer.Items
	for name, items := range map[string][]string{
		"source":   {`{"source":"elsewhere","playlistId":"` + id("a") + `","note":""}`},
		"id":       {`{"source":"fanmade","playlistId":"liked","note":""}`},
		"twice":    {item("fanmade", "a", ""), item("fanmade", "a", "again")},
		"long":     {item("fanmade", "a", strings.Repeat("长", maxFeaturedNote+1))},
		"lines":    {item("fanmade", "a", "one\ntwo")},
		"too many": {item("fanmade", "a", ""), item("fanmade", "b", ""), item("official", "a", ""), item("fanmade", "1", ""), item("fanmade", "2", ""), item("fanmade", "3", ""), item("fanmade", "4", "")},
	} {
		w := c.do("PUT", "/api/admin/featured-playlists", `{"items":[`+strings.Join(items, ",")+`]}`, nil)
		if w.Code != 422 || !strings.Contains(w.Body.String(), `"FEATURED_INVALID"`) {
			t.Fatalf("%s: %d %s", name, w.Code, w.Body.String())
		}
	}
	for _, missing := range []string{item("fanmade", "d", ""), item("fanmade", "c", ""), item("official", "b", "")} {
		w := c.do("PUT", "/api/admin/featured-playlists", `{"items":[`+missing+`]}`, nil)
		if w.Code != 422 || !strings.Contains(w.Body.String(), `"FEATURED_PLAYLIST_UNAVAILABLE"`) {
			t.Fatalf("%s: %d %s", missing, w.Code, w.Body.String())
		}
	}
	c.json("PUT", "/api/admin/featured-playlists", `{}`, 422, nil)
	read(kept...)
	// A note of exactly the limit is kept, counted in characters.
	put(200, item("fanmade", "a", strings.Repeat("长", maxFeaturedNote)), item("official", "a", ""), item("fanmade", "b", ""))

	// A playlist made private or deleted in its source is no longer recommended; the same id
	// in the other source is another playlist.
	if _, err := pool.Exec(ctx, `UPDATE fanmade.playlists SET name='Renamed' WHERE id=$1`, id("a")); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE fanmade.playlists SET is_public=false WHERE id=$1`, id("a")); err != nil {
		t.Fatal(err)
	}
	read(FeaturedPlaylist{"official", id("a"), ""}, FeaturedPlaylist{"fanmade", id("b"), ""})
	if _, err := pool.Exec(ctx, `DELETE FROM official.playlists WHERE id=$1`, id("a")); err != nil {
		t.Fatal(err)
	}
	read(FeaturedPlaylist{"fanmade", id("b"), ""})

	// An empty list recommends nothing.
	put(200)
	read()
}
