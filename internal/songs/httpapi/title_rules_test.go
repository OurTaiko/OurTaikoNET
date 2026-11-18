package httpapi

import (
	"context"
	"fmt"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"ourtaiko.dev/net/internal/titlerules"
	"ourtaiko.dev/net/internal/web"
)

// Titles given by rule (package titlerules): for a chart, a category, the number of uploads,
// and for being seen on the website and in the game.
func TestRulesGiveTitles(t *testing.T) {
	pool := testDB(t, Fanmade)
	ctx := context.Background()
	s := testServer(t, pool, Config{Origin: "http://localhost"})
	c := newTestClient(t, s)
	alice, bob, owner := testUsers["alice"].ID, testUsers["bob"].ID, testUsers["owner"].ID

	// Three charts of the owner's: a and b in "game" (1), c in "pop" (4); b has no Oni.
	const a, b, cc = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa1", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa2", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa3"
	if _, err := pool.Exec(ctx, `INSERT INTO charts(id,owner_ids,title,bpm,duration,category_flags,tja_file,audio_file)
 SELECT id,ARRAY[$1::text],'Song '||id,120,10,flags,test_file('tja'||id,'t.tja',repeat('a',64),1),test_file('ogg'||id,'t.ogg',repeat('b',64),1)
 FROM (VALUES($2,1),($3,1),($4,4)) v(id,flags)`, owner, a, b, cc); err != nil {
		t.Fatal(err)
	}
	var aOni, aEasy, bHard, cOni int64
	if err := pool.QueryRow(ctx, `SELECT test_slot($1,'Oni'),test_slot($1,'Easy'),test_slot($2,'Hard'),test_slot($3,'Oni')`, a, b, cc).Scan(&aOni, &aEasy, &bHard, &cOni); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO net.titles(id,name,style) SELECT id,id,'wood' FROM unnest(ARRAY['song','genre','genre-oni','site','game','uploader','mine']) id;
 UPDATE net.titles SET owner_id='`+owner+`',official=false WHERE id='mine';
 INSERT INTO net.title_rules(title_id,kind) VALUES('site','site_login'),('game','game_login');
 INSERT INTO net.title_rules(title_id,kind,source,chart_id,course,clear) VALUES('song','chart_clear','fanmade','`+a+`','Oni',2);
 INSERT INTO net.title_rules(title_id,kind,source,category_flag,clear,count) VALUES('genre','category_clear','fanmade',1,1,NULL),('genre-oni','category_clear','fanmade',1,1,1);
 UPDATE net.title_rules SET course='Oni' WHERE title_id='genre-oni'`); err != nil {
		t.Fatal(err)
	}
	holds := func(user string) []string {
		t.Helper()
		rows, err := pool.Query(ctx, `SELECT title_id FROM net.user_titles WHERE user_id=$1 ORDER BY title_id`, user)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		ids := []string{}
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				t.Fatal(err)
			}
			ids = append(ids, id)
		}
		return ids
	}
	expect := func(user string, titles ...string) {
		t.Helper()
		if got := holds(user); !slices.Equal(got, titles) {
			t.Fatalf("holds %v, want %v", got, titles)
		}
	}

	// Being seen on the website gives its title; it says nothing of the game.
	if code := c.do("alice", "GET", "/api/me", "", nil); code != 200 {
		t.Fatal(code)
	}
	expect(alice, "site")

	// Bob plays: the game's token is seen, and the play is judged.
	token := strings.Repeat("c", 64)
	if _, err := pool.Exec(ctx, `INSERT INTO test_sessions(token_hash,user_id,csrf_token,expires_at) VALUES($1,$2,'',now()+interval '1 day')`, web.Hash("game:"+token), bob); err != nil {
		t.Fatal(err)
	}
	play := func(slot int64, clear int) {
		t.Helper()
		r := httptest.NewRequest("POST", "/api/game/scores", strings.NewReader(fmt.Sprintf(`{"difficultyId":%d,"good":5,"ok":1,"bad":0,"score":6000,"drumroll":2,"max_combo":6,"ClearStatus":%d}`, slot, clear)))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		// The answer is the score alone: rewards are not told in it.
		if w.Code != 201 || strings.Contains(w.Body.String(), "granted") {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	// Failed: nothing. Easy cleared: not the course either rule of the chart asks for, and
	// one of two charts of the category.
	play(aOni, 0)
	expect(bob, "game")
	play(aEasy, 3)
	expect(bob, "game")
	// Oni cleared: one Oni chart of the category. Not the full combo the chart's title wants.
	play(aOni, 1)
	expect(bob, "game", "genre-oni")
	play(aOni, 2)
	expect(bob, "game", "genre-oni", "song")
	// Another category's chart does not count; the second chart of this one completes it.
	play(cOni, 3)
	expect(bob, "game", "genre-oni", "song")
	play(bHard, 1)
	expect(bob, "game", "genre", "genre-oni", "song")

	// A title taken back is not given again by the same rule.
	if _, err := pool.Exec(ctx, `DELETE FROM net.user_titles WHERE user_id=$1 AND title_id='song'`, bob); err != nil {
		t.Fatal(err)
	}
	play(aOni, 3)
	expect(bob, "game", "genre", "genre-oni")

	// A new rule gives its title at once to those who meet it already.
	var rule titlerules.Rule
	insert := func(sql string) titlerules.Rule {
		t.Helper()
		r, err := titlerules.Scan(pool.QueryRow(ctx, sql+` RETURNING id,title_id,don_part,don_item,kind,source,chart_id,category_flag,course,clear,count`))
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	rule = insert(`INSERT INTO net.title_rules(title_id,kind,source,count) VALUES('uploader','charts_uploaded','fanmade',3)`)
	if n, err := titlerules.Backfill(ctx, pool, rule); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	if n, err := titlerules.Backfill(ctx, pool, rule); err != nil || n != 0 {
		t.Fatal("given twice", n, err)
	}
	expect(owner, "uploader")
	// And an upload is judged when it happens.
	if _, err := pool.Exec(ctx, `DELETE FROM net.user_titles WHERE title_id='uploader'; DELETE FROM net.awards WHERE service='rule' AND user_id='`+owner+`'`); err != nil {
		t.Fatal(err)
	}
	s.titlesAfterUpload(ctx, a)
	expect(owner, "uploader")
	// A rule for the game, made later, finds those who have played.
	rule = insert(`INSERT INTO net.title_rules(title_id,kind) VALUES('uploader','game_login')`)
	if n, err := titlerules.Backfill(ctx, pool, rule); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	expect(bob, "game", "genre", "genre-oni", "uploader")
}
