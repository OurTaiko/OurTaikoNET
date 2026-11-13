package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// The leaderboard of a block: each player's best play of the current files, ties sharing a
// rank, twenty to a page.
func TestLeaderboard(t *testing.T) { bothSources(t, testLeaderboard) }

func testLeaderboard(t *testing.T, w interactionWorld) {
	c, s := w.c, w.s
	ctx := context.Background()
	oni, hard, easy, otherOni := w.slot(t, w.song, "Oni"), w.slot(t, w.song, "Hard"), w.slot(t, w.song, "Easy"), w.slot(t, w.other, "Oni")
	// The first player is alice, whom the account center knows; of the rest it has never heard.
	players := []string{testUsers["alice"].ID}
	for i := 1; i < 23; i++ {
		id := fmt.Sprintf("%032x", i+1000)
		if _, err := s.DB.Exec(ctx, `INSERT INTO users(id) VALUES($1)`, id); err != nil {
			t.Fatal(err)
		}
		players = append(players, id)
	}
	add := func(id, user string, difficulty, points, good int64, date string) {
		t.Helper()
		if _, err := s.DB.Exec(ctx, `INSERT INTO scores(id,user_id,difficulty_id,good,ok,bad,score,drumroll,max_combo,payload_digest,submitted_at) VALUES($1,$2,$3,$4,2,1,$5,55,$4,repeat('a',64),$6::timestamptz)`, id, user, difficulty, good, points, date); err != nil {
			t.Fatal(err)
		}
	}
	for i, player := range players {
		points := int64(800000 - i)
		if i < 2 {
			points = 900000
		}
		add(fmt.Sprintf("score%02d", i), player, oni, points, 300, "2026-01-01T00:00:00Z")
	}
	// A player's lower and later plays do not rank twice, and a play of another chart not here.
	add("lower", players[0], oni, 840000, 100, "2025-12-01T00:00:00Z")
	add("later", players[0], oni, 900000, 999, "2026-02-01T00:00:00Z")
	add("elsewhere", players[0], otherOni, 1300000, 600, "2025-12-01T00:00:00Z")
	get := func(path string, status int, code string) leaderboardResponse {
		t.Helper()
		got, body := c.raw("", path)
		if got != status || (code != "" && !strings.Contains(string(body), code)) {
			t.Fatalf("%s: %d %s", path, got, body)
		}
		var v leaderboardResponse
		if status == 200 {
			if err := json.Unmarshal(body, &v); err != nil {
				t.Fatal(err)
			}
			for _, private := range []string{"versionId", "password", "email"} {
				if strings.Contains(string(body), private) {
					t.Fatal("field that is not for the public:", private)
				}
			}
		}
		return v
	}
	path := w.chart(w.song) + "/leaderboard"
	block := func(id int64) string { return fmt.Sprintf("?difficultyId=%d", id) }
	v := get(path, 200, "")
	if v.Course != "Oni" || v.Side != "single" || v.Total != 23 || len(v.Items) != 20 {
		t.Fatalf("unexpected summary: %+v", v)
	}
	if v.Items[0].ID != "score00" || v.Items[0].Good != 300 || v.Items[0].MaxCombo != 300 || v.Items[0].Score.Score != 900000 || v.Items[0].Rank != 1 || v.Items[1].Rank != 1 || v.Items[2].Rank != 3 {
		t.Fatalf("best score/ties incorrect: %+v", v.Items[:3])
	}
	if v.Items[0].Nickname != "alice" || v.Items[1].Nickname != "未知用户" {
		t.Fatalf("names: %q %q", v.Items[0].Nickname, v.Items[1].Nickname)
	}
	second := get(path+block(oni)+"&page=2", 200, "")
	if len(second.Items) != 3 || second.Items[0].Rank != 21 || second.Total != 23 {
		t.Fatalf("pagination incorrect: %+v", second)
	}
	if v := get(path+"?page=3", 200, ""); v.Total != 23 || len(v.Items) != 0 {
		t.Fatal(v)
	}
	if v := get(path+block(hard), 200, ""); v.Course != "Hard" || v.Total != 0 || v.Items == nil {
		t.Fatal(v)
	}
	get(path+block(easy), 200, "")
	// A block of another chart, or none at all, is not this chart's.
	get(path+block(otherOni), 404, "DIFFICULTY_NOT_FOUND")
	get(path+"?difficultyId=987654321", 404, "DIFFICULTY_NOT_FOUND")
	get(path+"?difficultyId=Oni", 400, "DIFFICULTY_INVALID")
	// Scores of replaced files stay stored but leave the ranking.
	if _, err := s.DB.Exec(ctx, `UPDATE scores SET is_latest=false WHERE id IN ('score00','later')`); err != nil {
		t.Fatal(err)
	}
	if v := get(path, 200, ""); v.Total != 23 || v.Items[0].ID != "score01" || v.Items[0].Rank != 1 || v.Items[1].Rank != 2 {
		t.Fatalf("retired scores ranked: %+v", v.Items[:2])
	}
	if _, err := s.DB.Exec(ctx, `UPDATE scores SET is_latest=true`); err != nil {
		t.Fatal(err)
	}
	for _, page := range []string{"0", "-1", "10001", "invalid", "1.5"} {
		get(path+"?page="+page, 400, "QUERY_INVALID")
	}
	for _, missing := range []string{"missing", "999999"} {
		get(w.chart(missing)+"/leaderboard", 404, "CHART_NOT_FOUND")
	}
	// Renaming the chart leaves its ranking alone; removing it takes the ranking with it.
	if _, err := s.DB.Exec(ctx, `UPDATE charts SET title_translations=jsonb_build_object('en','Renamed') WHERE id=$1`, w.song); err != nil {
		t.Fatal(err)
	}
	if v := get(path, 200, ""); v.Items[0].ID != "score00" || v.Total != 23 {
		t.Fatal("metadata edit changed leaderboard")
	}
	if _, err := s.DB.Exec(ctx, `UPDATE charts SET status='deleted' WHERE id=$1`, w.song); err != nil {
		t.Fatal(err)
	}
	get(path, 404, "CHART_NOT_FOUND")
}
