package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ourtaiko.dev/net/internal/account"
	"ourtaiko.dev/net/internal/netdb/netdbtest"
	"ourtaiko.dev/net/internal/rewards"
)

// What a player is given is told on every answer they get, the website's and the game's, from
// every part of the site; a play that earns a title by rule is told the same way.
func TestEveryAnswerSaysWhatWasGiven(t *testing.T) {
	pool, url := netdbtest.NewURL(t, "fanmade")
	ctx := context.Background()
	issuer, _ := accountCenter(t)
	sso, err := account.New(account.Config{Issuer: issuer, ServiceID: "net", ServiceKey: strings.Repeat("k", 64), ClientID: "net", ClientSecret: "secret", RedirectURL: "http://localhost/api/v1/auth/sso/callback", EncryptionKey: strings.Repeat("b", 64)})
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(ctx, Config{DatabaseURL: url, Origin: "http://localhost", StorageDir: t.TempDir(), SSO: sso, LegacyFanmadeHost: "fanmade.ourtaiko.org"})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	h := a.Handler()
	player := strings.Repeat("7", 32)
	cookie, _, err := a.Players.OpenSession(ctx, player, "tok:"+player, "refresh", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	game := gameTokenPrefix + player

	// A chart whose clear gives a title by rule. Its block has a scoring, so the site takes its
	// scores.
	if _, err = pool.Exec(ctx, `
INSERT INTO fanmade.chart_resources(storage_key,original_filename,sha256,byte_size,extension) VALUES
 (repeat('1',64)||'.tja','a.tja',repeat('1',64),10,'tja'), (repeat('2',64)||'.ogg','a.ogg',repeat('2',64),10,'ogg');
INSERT INTO fanmade.charts(id,owner_ids,title,bpm,duration,demo_end,tja_file,audio_file)
 SELECT repeat('f',32), ARRAY['`+player+`'], 'Fan song', 120, 60, 10, min(id), max(id) FROM fanmade.chart_resources;
INSERT INTO fanmade.chart_difficulties(chart_id,course,side,level,content_hash,good_points,ok_points,long_hit_points,kiwami_threshold,scoring_version)
 VALUES (repeat('f',32),'Oni','single',8,'v2:'||repeat('0',64),1000,500,100,1000000,1);
INSERT INTO net.titles(id,name,style) VALUES ('cleared','クリア','wood');
INSERT INTO net.title_rules(title_id,kind,source,chart_id,clear) VALUES ('cleared','chart_clear','fanmade',repeat('f',32),1)`); err != nil {
		t.Fatal(err)
	}
	var slot int64
	if err = pool.QueryRow(ctx, `SELECT id FROM fanmade.chart_difficulties`).Scan(&slot); err != nil {
		t.Fatal(err)
	}

	send := func(method, path, body string, how string, csrf string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Host = "net.ourtaiko.org"
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		switch how {
		case "web":
			r.AddCookie(&http.Cookie{Name: SessionCookie, Value: cookie})
			r.Header.Set("X-CSRF-Token", csrf)
			r.Header.Set("Origin", "http://localhost")
		case "game":
			r.Header.Set("Authorization", "Bearer "+game)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	pending := func(w *httptest.ResponseRecorder) string { return w.Header().Get(rewards.Header) }

	// Nothing given yet: signed in, every part says 0; a guest is told nothing.
	var me struct {
		CSRFToken string `json:"csrfToken"`
	}
	w := send("GET", "/api/v1/me", "", "web", "")
	if json.Unmarshal(w.Body.Bytes(), &me) != nil || pending(w) != "0" {
		t.Fatal(w.Code, w.Header(), w.Body.String())
	}
	for _, path := range []string{"/api/v1/sources/fanmade/me", "/api/v1/sources/official/me"} {
		if w := send("GET", path, "", "web", ""); pending(w) != "0" {
			t.Fatal(path, w.Header())
		}
	}
	if w := send("GET", "/api/v1/me", "", "", ""); pending(w) != "" {
		t.Fatal("a guest was told", w.Header())
	}

	// The game clears the chart. The score's answer is the score, and already counts the title.
	w = send("POST", "/api/v1/sources/fanmade/game/scores", `{"difficultyId":`+jsonInt(slot)+`,"good":5,"ok":0,"bad":0,"score":1000,"drumroll":0,"max_combo":5,"ClearStatus":1}`, "game", "")
	if w.Code != 201 || strings.Contains(w.Body.String(), "granted") || pending(w) != "1" {
		t.Fatal(w.Code, w.Header(), w.Body.String())
	}
	// The website hears of it on any answer, too.
	if w := send("GET", "/api/v1/sources/official/me", "", "web", ""); pending(w) != "1" {
		t.Fatal(w.Header())
	}

	// The game fetches it with its token, the website with its session; it is the same reward.
	var fromGame, fromWeb struct{ Rewards []rewards.Reward }
	w = send("GET", "/api/v1/game/rewards", "", "game", "")
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &fromGame) != nil || len(fromGame.Rewards) != 1 || fromGame.Rewards[0].Kind != "title" || !strings.Contains(string(fromGame.Rewards[0].Data), `"service":"rule"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	w = send("GET", "/api/v1/me/rewards", "", "web", "")
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &fromWeb) != nil || len(fromWeb.Rewards) != 1 || fromWeb.Rewards[0].ID != fromGame.Rewards[0].ID {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := send("GET", "/api/v1/me/rewards", "", "", ""); w.Code != 401 {
		t.Fatal("a guest's rewards", w.Code)
	}
	// A game that still uses an old domain is told and fetches there too.
	old := httptest.NewRequest("GET", "/api/v1/game/rewards", nil)
	old.Host = "fanmade.ourtaiko.org"
	old.Header.Set("Authorization", "Bearer "+game)
	ow := httptest.NewRecorder()
	h.ServeHTTP(ow, old)
	if ow.Code != 200 || !strings.Contains(ow.Body.String(), `"title"`) || ow.Header().Get(rewards.Header) != "1" {
		t.Fatal("old domain", ow.Code, ow.Header(), ow.Body.String())
	}
	seen := `{"ids":[` + jsonInt(fromWeb.Rewards[0].ID) + `]}`
	// The website marks with its CSRF token, the game without one.
	if w := send("POST", "/api/v1/me/rewards/seen", seen, "web", "wrong"); w.Code != 403 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := send("POST", "/api/v1/game/rewards/seen", seen, "game", ""); w.Code != 200 || pending(w) != "0" {
		t.Fatal(w.Code, w.Header(), w.Body.String())
	}
	if w := send("POST", "/api/v1/me/rewards/seen", seen, "web", me.CSRFToken); w.Code != 200 || !strings.Contains(w.Body.String(), `"marked":0`) || pending(w) != "0" {
		t.Fatal(w.Code, w.Header(), w.Body.String())
	}
}

func jsonInt(n int64) string { b, _ := json.Marshal(n); return string(b) }
