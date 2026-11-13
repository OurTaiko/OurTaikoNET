package httpapi

import (
	"context"
	"testing"
)

// Deleting a song is the same code as on the Fanmade source, whose test covers the scores
// and shared files; here: who may delete on this source, and that its own way of releasing
// files removes them.
func TestDeleteChart(t *testing.T) {
	w := officialInteractionFixture(t)
	c, s, ctx := w.c, w.s, context.Background()
	// Each song has its own TJA; both share one audio file.
	file := func(extension string) (id int64) {
		digest := ID() + ID()
		if e := s.DB.QueryRow(ctx, `INSERT INTO chart_resources(storage_key,original_filename,sha256,byte_size,extension) VALUES($1||'.'||$2,'a.'||$2,$1,1,$2) RETURNING id`, digest, extension).Scan(&id); e != nil {
			t.Fatal(e)
		}
		return id
	}
	shared := file("ogg")
	for _, chart := range []string{w.song, w.other} {
		if _, e := s.DB.Exec(ctx, `UPDATE charts SET tja_file=$1,audio_file=$2,duration=10 WHERE id=$3`, file("tja"), shared, chart); e != nil {
			t.Fatal(e)
		}
	}
	files := func() int {
		var n int
		if e := s.DB.QueryRow(ctx, `SELECT count(*) FROM chart_resources`).Scan(&n); e != nil {
			t.Fatal(e)
		}
		return n
	}
	for user, want := range map[string]int{"": 401, "alice": 403} {
		if code := c.do(user, "DELETE", w.chart(w.song), `{}`, nil); code != want {
			t.Fatal("delete by", user, code)
		}
	}
	// A manager of the source deletes a song that is not theirs; the audio the other song
	// shares stays.
	if code := c.do("admin", "DELETE", w.chart(w.song), `{}`, nil); code != 200 || files() != 2 {
		t.Fatal("manager delete", code, files())
	}
	if code := c.do("", "GET", w.chart(w.song)+"?by=id", "", nil); code != 404 {
		t.Fatal("deleted song still answered", code)
	}
	if code := c.do("admin", "DELETE", w.chart(w.song), `{}`, nil); code != 404 {
		t.Fatal("repeated delete", code)
	}
	if code := c.do("owner", "DELETE", w.chart(w.other), `{}`, nil); code != 200 || files() != 0 {
		t.Fatal("owner delete", code, files())
	}
}
