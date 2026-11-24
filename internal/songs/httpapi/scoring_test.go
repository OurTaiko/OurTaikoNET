package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"ourtaiko.dev/net/internal/kit/tja"
	"ourtaiko.dev/net/internal/web"
)

// A block's 真打 scoring is what TJAParser reads from the stored file; a score is the site's,
// worked out from the counts the game sends, with its rank against the block's kiwami line. A
// block stored before the scoring was kept gets it at startup, which rescores its current
// scores without touching the chart; scores of replaced content keep their values.
func TestChartScoring(t *testing.T) {
	bothSources(t, testChartScoring)
}

func testChartScoring(t *testing.T, w interactionWorld) {
	if !tja.Available() {
		t.Skip("tjaparser not found: set TJAPARSER_BIN or put it on PATH")
	}
	s := w.s
	ctx := context.Background()
	audio, err := os.ReadFile("../../kit/audio/testdata/vorbis.ogg")
	if err != nil {
		t.Fatal(err)
	}
	send := func(method, path, notes string) Chart {
		t.Helper()
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		out, _ := form.CreateFormFile("tja", "chart.tja")
		_, _ = out.Write([]byte("TITLE:Scored\r\nBPM:120\r\nWAVE:music.ogg\r\nCOURSE:Oni\r\nLEVEL:5\r\n#START\r\n" + notes + "#END\r\n"))
		if method == "POST" {
			out, _ = form.CreateFormFile("audio", "music.ogg")
			_, _ = out.Write(audio)
		} else {
			_ = form.WriteField("confirmReset", "true")
		}
		_ = form.Close()
		r := httptest.NewRequest(method, path, &body)
		r.Header.Set("Content-Type", form.FormDataContentType())
		r.Header.Set("Origin", "http://localhost")
		r.Header.Set("X-CSRF-Token", "csrf")
		r.Header.Set("Idempotency-Key", web.NewID())
		r.RemoteAddr = fmt.Sprintf("10.9.%d.1:1234", w.c.n.Add(1)&255)
		r.AddCookie(&http.Cookie{Name: w.c.cookie, Value: w.c.tokens["admin"]})
		rec := httptest.NewRecorder()
		w.c.handler.ServeHTTP(rec, r)
		var c Chart
		if rec.Code >= 300 || json.Unmarshal(rec.Body.Bytes(), &c) != nil {
			t.Fatalf("%s %s: %d %s", method, path, rec.Code, rec.Body.String())
		}
		return c
	}
	type scoring struct{ good, ok, long, kiwami, version *int }
	read := func(difficulty int64) scoring {
		t.Helper()
		var v scoring
		if err := s.DB.QueryRow(ctx, `SELECT good_points,ok_points,long_hit_points,kiwami_threshold,scoring_version FROM chart_difficulties WHERE id=$1`, difficulty).
			Scan(&v.good, &v.ok, &v.long, &v.kiwami, &v.version); err != nil {
			t.Fatal(err)
		}
		return v
	}
	// The game signs in with a bearer token of its own session.
	game := strings.Repeat("c", 64)
	if _, err = s.DB.Exec(ctx, `INSERT INTO test_sessions(token_hash,user_id,csrf_token,expires_at) VALUES($1,$2,'',now()+interval '1 day')`,
		web.Hash("game:"+game), testUsers["alice"].ID); err != nil {
		t.Fatal(err)
	}
	upload := func(body, key string, want int) []byte {
		t.Helper()
		r := httptest.NewRequest("POST", "/api/game/scores", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+game)
		r.Header.Set("Content-Type", "application/json")
		if key != "" {
			r.Header.Set("Idempotency-Key", key)
		}
		rec := httptest.NewRecorder()
		w.c.handler.ServeHTTP(rec, r)
		if rec.Code != want {
			t.Fatalf("POST %s: %d, want %d: %s", body, rec.Code, want, rec.Body.String())
		}
		return rec.Body.Bytes()
	}
	post := func(body, key string, want int) Score {
		t.Helper()
		var v Score
		if data := upload(body, key, want); json.Unmarshal(data, &v) != nil {
			t.Fatalf("POST %s: %s", body, data)
		}
		return v
	}
	submit := func(body string, want int) Score { t.Helper(); return post(body, "", want) }
	rank := func(v *int) int {
		if v == nil {
			return -1
		}
		return *v
	}

	// Three notes and a three-second roll on Oni: k = 45, H = 51, 良 331640, 可 165820, line 1000020.
	c := send("POST", "/api/charts", "1110,\r\n5000,\r\n0080,\r\n")
	block := c.Difficulties[0].ID
	if v := read(block); v.good == nil || *v.good != 331640 || *v.ok != 165820 || *v.long != 100 || *v.kiwami != 1000020 || *v.version != tja.ScoringVersion {
		t.Fatalf("scoring from the file: %+v", v)
	}

	// The site's score, whatever the game sent: 2 × 331640 + 165820 + 30 × 100 = 832100, 金粋
	// (≥ 80 % of 1000020, < 90 %); Ad-Libs add as a 良 and a 可 do.
	played := fmt.Sprintf(`{"difficultyId":%d,"good":2,"ok":1,"bad":0,"score":1,"drumroll":30,"max_combo":3,"ClearStatus":1}`, block)
	if v := submit(played, 201); v.Score != 832100 || rank(v.ScoreRank) != 4 {
		t.Fatalf("score %d rank %d", v.Score, rank(v.ScoreRank))
	}
	adlib := fmt.Sprintf(`{"difficultyId":%d,"good":3,"ok":0,"bad":0,"score":1,"drumroll":40,"max_combo":3,"ClearStatus":2,"adlib":[1,1]}`, block)
	if v := submit(adlib, 201); v.Score != 4*331640+165820+4000 || rank(v.ScoreRank) != 7 {
		t.Fatalf("with Ad-Libs: score %d rank %d", v.Score, rank(v.ScoreRank))
	}

	// A retry under the same key answers the stored score, even with the game's score changed.
	retry := fmt.Sprintf(`{"difficultyId":%d,"good":1,"ok":0,"bad":2,"score":%%d,"drumroll":0,"max_combo":1}`, block)
	key := web.NewID()
	first := post(fmt.Sprintf(retry, 5), key, 201)
	if again := post(fmt.Sprintf(retry, 6), key, 200); first.Score != 331640 || again.ID != first.ID || again.Score != first.Score || rank(again.ScoreRank) != 0 {
		t.Fatalf("retry answered %+v after %+v", again, first)
	}

	// A block from before the scoring takes no new scores: the site could not work them out.
	if _, err = s.DB.Exec(ctx, `UPDATE chart_difficulties SET good_points=NULL,ok_points=NULL,long_hit_points=NULL,kiwami_threshold=NULL,scoring_version=NULL WHERE id=$1`, block); err != nil {
		t.Fatal(err)
	}
	if refused := upload(played, "", 422); !strings.Contains(string(refused), `"code":"DIFFICULTY_NOT_SCORABLE"`) {
		t.Fatalf("an unscored block took a score: %s", refused)
	}
	// Its stored scores keep their values, as one from before the scoring written here with the
	// game's value does, until the startup backfill reads the file: that rescores them, with
	// their ranks, and leaves the chart's update_at.
	var stale int64
	var staleRank *int
	if err = s.DB.QueryRow(ctx, `INSERT INTO scores(id,user_id,difficulty_id,good,ok,bad,score,drumroll,max_combo,payload_digest)
 VALUES($1,$2,$3,2,1,0,1,30,3,repeat('a',64)) RETURNING score,score_rank`, web.NewID(), testUsers["alice"].ID, block).Scan(&stale, &staleRank); err != nil {
		t.Fatal(err)
	}
	if stale != 1 || staleRank != nil {
		t.Fatalf("an unscored block's score kept %d rank %d", stale, rank(staleRank))
	}
	var updated time.Time
	if err = s.DB.QueryRow(ctx, `SELECT update_at FROM charts WHERE id=$1`, c.ID).Scan(&updated); err != nil {
		t.Fatal(err)
	}
	// Song and Other on Fanmade point at files the store does not hold; they fail and stay.
	_ = s.backfillScoring(ctx)
	var wrong, unranked int
	var after time.Time
	if err = s.DB.QueryRow(ctx, `SELECT count(*) FILTER (WHERE score<>net.shinuchi_score(sc.good,sc.ok,sc.drumroll,sc.adlib,d.good_points,d.ok_points,d.long_hit_points)),
 count(*) FILTER (WHERE score_rank IS NULL),(SELECT update_at FROM charts WHERE id=$2)
 FROM scores sc JOIN chart_difficulties d ON d.id=sc.difficulty_id WHERE d.id=$1`, block, c.ID).Scan(&wrong, &unranked, &after); err != nil {
		t.Fatal(err)
	}
	if v := read(block); v.good == nil || *v.good != 331640 || wrong != 0 || unranked != 0 || !after.Equal(updated) {
		t.Fatalf("backfill: %+v, %d wrong, %d unranked, update_at %v → %v", v, wrong, unranked, updated, after)
	}

	// Replaced content: the old scores leave the latest with their values; new ones score by the
	// new file (1111: 良 250000, line 1000000).
	var before []int64
	if err = s.DB.QueryRow(ctx, `SELECT array_agg(score ORDER BY id) FROM scores WHERE difficulty_id=$1`, block).Scan(&before); err != nil {
		t.Fatal(err)
	}
	replaced := send("PUT", w.chart(c.ID)+"/files", "1111,\r\n")
	if replaced.Difficulties[0].ID != block {
		t.Fatal("the block's row was not kept")
	}
	var kept []int64
	var latest int
	if err = s.DB.QueryRow(ctx, `SELECT array_agg(score ORDER BY id),count(*) FILTER (WHERE is_latest) FROM scores WHERE difficulty_id=$1`, block).Scan(&kept, &latest); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(kept) != fmt.Sprint(before) || latest != 0 {
		t.Fatalf("replaced content rescored: %v → %v, %d latest", before, kept, latest)
	}
	if v := read(block); *v.good != 250000 || *v.kiwami != 1000000 {
		t.Fatalf("scoring of the new file: %+v", v)
	}
	if v := submit(played, 201); v.Score != 2*250000+125000+3000 || rank(v.ScoreRank) != 2 {
		t.Fatalf("after the replacement: score %d rank %d", v.Score, rank(v.ScoreRank))
	}
}
