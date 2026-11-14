package httpapi

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestTheSitesLatestActivity(t *testing.T) {
	pool := testDB(t)
	ctx := context.Background()
	for _, id := range []string{player, other} {
		if _, err := pool.Exec(ctx, `INSERT INTO net.users(id) VALUES ($1)`, id); err != nil {
			t.Fatal(err)
		}
	}
	// A chart with one block, published by owner at a given time; the block's ID.
	chart := func(source, id, owner, at, status string) int64 {
		t.Helper()
		var block int64
		err := pool.QueryRow(ctx, fmt.Sprintf(`
WITH tja AS (INSERT INTO %[1]s.chart_resources(storage_key,original_filename,sha256,byte_size,extension) VALUES ($1||$1||'.tja','a.tja',$1||$1,1,'tja') RETURNING id),
 audio AS (INSERT INTO %[1]s.chart_resources(storage_key,original_filename,sha256,byte_size,extension) VALUES (repeat('a',32)||$1||'.ogg','a.ogg',repeat('a',32)||$1,1,'ogg') RETURNING id),
 chart AS (INSERT INTO %[1]s.charts(id,owner_ids,title,bpm,duration,demo_end,tja_file,audio_file,created_at,status) SELECT $1,ARRAY[$2],'Song '||left($1,1),120,60,10,tja.id,audio.id,$3::timestamptz,$4 FROM tja,audio RETURNING id)
INSERT INTO %[1]s.chart_difficulties(chart_id,course,side,level,content_hash) SELECT id,'Oni','single',9,'v2:'||repeat('0',64) FROM chart RETURNING id`, source),
			id, owner, at, status).Scan(&block)
		if err != nil {
			t.Fatal(source, id, err)
		}
		return block
	}
	play := func(source, id, user string, block int64, clear int, at string) {
		t.Helper()
		if _, err := pool.Exec(ctx, `INSERT INTO net.scores(source,id,user_id,good,ok,bad,score,drumroll,max_combo,payload_digest,difficulty_id,clear_status,submitted_at)
 VALUES ($1,$2,$3,1,0,0,900000,0,1,repeat('d',64),$4,$5,$6::timestamptz)`, source, id, user, block, clear, at); err != nil {
			t.Fatal(source, id, err)
		}
	}
	f1 := chart("fanmade", strings.Repeat("1", 32), player, "2026-01-01T00:00:00Z", "published")
	o1 := chart("official", strings.Repeat("2", 32), other, "2026-01-02T00:00:00Z", "published")
	hidden := chart("fanmade", strings.Repeat("3", 32), player, "2026-01-09T00:00:00Z", "hidden")
	// The other player clears f1 twice and fails it once; the player full-combos o1.
	play("fanmade", "a1", other, f1, 1, "2026-01-03T00:00:00Z")
	play("fanmade", "a2", other, f1, 2, "2026-01-05T00:00:00Z")
	play("fanmade", "a3", other, f1, 0, "2026-01-06T00:00:00Z")
	play("official", "b1", player, o1, 2, "2026-01-04T00:00:00Z")
	play("fanmade", "c1", player, hidden, 3, "2026-01-08T00:00:00Z")

	sources := []Source{
		{ID: "fanmade", Name: "Fanmade", Site: "https://fanmade.test"},
		{ID: "official", Name: "Database", Site: "https://database.test"},
		// A source this database does not have: left out, and the others still show.
		{ID: "down", Name: "Down", Site: "https://down.test"},
	}
	c := &client{t: t, handler: New(pool, Config{Origin: origin, Sources: sources, DevUserID: player}).Handler()}
	var feed struct{ Items []Activity }
	c.json("GET", "/api/v1/activity", "", 200, &feed)
	got := []string{}
	for _, item := range feed.Items {
		row := item.Kind + ":" + item.Source + ":" + item.Chart.Title + ":" + item.User.ID[28:]
		if item.Play != nil {
			row += fmt.Sprintf(":%s:%d", item.Play.ID, item.Play.ClearStatus)
		}
		got = append(got, row)
	}
	// Newest first; one row for the two clears of f1, the failed play and the hidden chart not at all.
	want := "score:fanmade:Song 1:beef:a2:2,score:official:Song 2:abcd:b1:2,upload:official:Song 2:beef,upload:fanmade:Song 1:abcd"
	if strings.Join(got, ",") != want {
		t.Fatalf("activity:\n got %s\nwant %s", strings.Join(got, ","), want)
	}
	if play := feed.Items[0].Play; play.Course != "Oni" || play.Level != 9 || play.Score != 900000 {
		t.Fatalf("play: %+v", play)
	}
	// Without an account center only the development user has a name.
	if feed.Items[1].User.Nickname == "" || feed.Items[0].User.Nickname != "" {
		t.Fatalf("names: %+v, %+v", feed.Items[0].User, feed.Items[1].User)
	}

	// A player who clears chart after chart shows with the latest four. (The answer above is
	// kept for a while, so this asks a server that has not answered yet.)
	for i := range 6 {
		block := chart("official", strings.Repeat(string(rune('a'+i)), 32), other, "2025-12-01T00:00:00Z", "published")
		play("official", fmt.Sprint("m", i), player, block, 1, fmt.Sprintf("2026-02-0%dT00:00:00Z", i+1))
	}
	c = &client{t: t, handler: New(pool, Config{Origin: origin, Sources: sources}).Handler()}
	c.json("GET", "/api/v1/activity", "", 200, &feed)
	got = got[:0]
	for _, item := range feed.Items {
		if item.Play != nil && item.User.ID == player {
			got = append(got, item.Play.ID)
		}
	}
	if strings.Join(got, ",") != "m5,m4,m3,m2" {
		t.Fatalf("one player's plays: %v", got)
	}
}
