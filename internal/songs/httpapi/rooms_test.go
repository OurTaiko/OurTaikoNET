package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
	"ourtaiko.dev/net/internal/account"
)

// A room is given a chart exactly as GET /charts/{id} describes it, and only a chart the
// game can play.
func TestRoomsReadPlayableCharts(t *testing.T) {
	w := interactionWorldOf(t, Fanmade)
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := w.s.DB.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	w.slot(t, w.song, "Oni")
	w.slot(t, w.song, "Easy")
	exec(`INSERT INTO chart_difficulties(chart_id,course,side,level) VALUES($1,'Edit','p1',9),($1,'Edit','p2',10)`, w.song)
	// A vote is the voter's own business: it is in the counts, not in what a room is given.
	if status := w.c.do("alice", "PUT", w.chart(w.song)+"/vote", `{"value":1}`, nil); status != 200 {
		t.Fatal("vote", status)
	}

	c, err := w.s.PlayableChart(ctx, w.song)
	if err != nil {
		t.Fatal(err)
	}
	if c.ID != w.song || c.Title != "Song" || c.Uploader != "owner" || len(c.Owners) != 1 || c.Upvotes != 1 || c.MyVote != 0 {
		t.Fatalf("chart: %+v", c)
	}
	var blocks []string
	for _, d := range c.Difficulties {
		blocks = append(blocks, d.Course+"/"+d.Side)
	}
	if got := strings.Join(blocks, " "); got != "Easy/single Oni/single Edit/p1 Edit/p2" {
		t.Fatalf("blocks: %s", got)
	}
	got, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	status, want := w.c.raw("", w.chart(w.song))
	if status != 200 || string(got)+"\n" != string(want) {
		t.Fatalf("a room is given\n%s\nand GET /charts/{id} answers %d\n%s", got, status, want)
	}

	for name, id := range map[string]string{"an unknown chart": strings.Repeat("0", 32), "a number": "1", "nothing": "", "not an ID": w.song + "'"} {
		if _, err = w.s.PlayableChart(ctx, id); !errors.Is(err, pgx.ErrNoRows) {
			t.Errorf("%s: %v", name, err)
		}
	}
	for _, status := range []string{"hidden", "deleted"} {
		exec(`UPDATE charts SET status=$2 WHERE id=$1`, w.other, status)
		if _, err = w.s.PlayableChart(ctx, w.other); !errors.Is(err, pgx.ErrNoRows) {
			t.Errorf("a %s chart: %v", status, err)
		}
	}
}

// A song registered without files (the official source has them) is in no room.
func TestRoomsDoNotReadSongsWithoutFiles(t *testing.T) {
	w := interactionWorldOf(t, Official)
	ctx := context.Background()
	if _, err := w.s.PlayableChart(ctx, w.song); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("a song without files: %v", err)
	}
	// Not even with a category, which is what lists a chart in the game.
	if _, err := w.s.DB.Exec(ctx, `UPDATE charts SET category_flags=1`); err != nil {
		t.Fatal(err)
	}
	if n, err := w.s.GameChartCount(ctx); err != nil || n != 0 {
		t.Fatalf("%d charts, %v", n, err)
	}
	if _, err := w.s.GameChartAt(ctx, 0); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("the first of none: %v", err)
	}
}

// おまかせ draws among the charts the game catalog lists: playable and in a category.
func TestRoomsDrawFromTheGameCatalog(t *testing.T) {
	w := interactionWorldOf(t, Fanmade)
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := w.s.DB.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	third := strings.Repeat("f", 32)
	w.addChart(t, third, "Third")
	// The same charts as GET /game/catalog lists.
	var catalog struct {
		Charts []struct {
			ID string `json:"id"`
		} `json:"charts"`
	}
	listed := func() map[string]bool {
		t.Helper()
		if status := w.c.do("", "GET", "/api/game/catalog", "", &catalog); status != 200 {
			t.Fatal("catalog", status)
		}
		ids := map[string]bool{}
		for _, c := range catalog.Charts {
			ids[c.ID] = true
		}
		return ids
	}
	drawn := func() map[string]bool {
		t.Helper()
		n, err := w.s.GameChartCount(ctx)
		if err != nil {
			t.Fatal(err)
		}
		ids, last := map[string]bool{}, ""
		for i := 0; i < n; i++ {
			c, err := w.s.GameChartAt(ctx, i)
			if err != nil {
				t.Fatal(i, err)
			}
			if c.ID <= last || c.Uploader != "owner" {
				t.Fatalf("chart %d is %s after %s, uploaded by %q", i, c.ID, last, c.Uploader)
			}
			ids[c.ID], last = true, c.ID
		}
		for _, i := range []int{n, n + 5, -1} {
			if _, err := w.s.GameChartAt(ctx, i); !errors.Is(err, pgx.ErrNoRows) {
				t.Fatalf("chart %d of %d: %v", i, n, err)
			}
		}
		return ids
	}
	same := func(a, b map[string]bool) bool {
		if len(a) != len(b) {
			return false
		}
		for id := range a {
			if !b[id] {
				return false
			}
		}
		return true
	}
	if got := drawn(); len(got) != 3 || !same(got, listed()) {
		t.Fatalf("drawn from %v, the catalog lists %v", got, listed())
	}
	// Out of every category, hidden: not drawn, as the catalog does not list them.
	exec(`UPDATE charts SET category_flags=0 WHERE id=$1`, w.other)
	exec(`UPDATE charts SET status='hidden' WHERE id=$1`, third)
	if got := drawn(); len(got) != 1 || !got[w.song] || !same(got, listed()) {
		t.Fatalf("drawn from %v, the catalog lists %v", got, listed())
	}
	exec(`UPDATE charts SET status='deleted' WHERE id=$1`, w.song)
	if got := drawn(); len(got) != 0 {
		t.Fatalf("drawn from %v", got)
	}
}

// Whose a game session is: the account center's answer, apart from an account center that
// cannot be asked.
func TestGameIdentity(t *testing.T) {
	pool := testDB(t, Fanmade)
	ctx := context.Background()
	hash, _ := bcrypt.GenerateFromPassword([]byte("test-password"), bcrypt.MinCost)
	if _, err := pool.Exec(ctx, `INSERT INTO test_sso_users(id,username,nickname,password_hash) VALUES('abd722b06b5255372acb7511472ff700','gameuser','Don',$1)`, string(hash)); err != nil {
		t.Fatal(err)
	}
	s := testServer(t, pool, Config{Origin: "http://localhost"})
	// The game signs in without an Origin, as no browser does.
	var login struct{ AccessToken string }
	signIn := func(server *Server, credentials string) int {
		r := httptest.NewRequest("POST", "/api/game/login", strings.NewReader(credentials))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		server.Handler().ServeHTTP(w, r)
		json.Unmarshal(w.Body.Bytes(), &login)
		return w.Code
	}
	if status := signIn(s, `{"username":"gameuser","password":"test-password"}`); status != 200 || len(login.AccessToken) != 64 {
		t.Fatal("login", status)
	}
	u, err := s.GameIdentity(ctx, login.AccessToken)
	if err != nil || u.ID != "abd722b06b5255372acb7511472ff700" || u.Nickname != "Don" {
		t.Fatalf("%+v, %v", u, err)
	}
	for name, token := range map[string]string{"an unknown session": strings.Repeat("0", 64), "a short one": "abc", "none": ""} {
		if _, err = s.GameIdentity(ctx, token); !errors.Is(err, pgx.ErrNoRows) {
			t.Errorf("%s: %v", name, err)
		}
	}

	// An account center that does not answer is not a refusal.
	unreachable, err := account.New(account.Config{Issuer: "http://127.0.0.1:1", ServiceID: "fanmade", ServiceKey: strings.Repeat("a", 64), ClientID: "web", ClientSecret: "secret", RedirectURL: "http://127.0.0.1/callback", EncryptionKey: strings.Repeat("b", 64)})
	if err != nil {
		t.Fatal(err)
	}
	down := New(pool, Config{Origin: "http://localhost", SSO: unreachable})
	if _, err = down.GameIdentity(ctx, login.AccessToken); err == nil || errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("with the account center down: %v", err)
	}

	// Without an account center, the development session is the development user's, and
	// only on a server that has one.
	dev := New(pool, Config{Origin: "http://localhost", DevUserID: strings.Repeat("d", 32)})
	if u, err = dev.GameIdentity(ctx, dev.devGameToken()); err != nil || u.ID != strings.Repeat("d", 32) || u.Nickname == "" {
		t.Fatalf("%+v, %v", u, err)
	}
	if _, err = dev.GameIdentity(ctx, strings.Repeat("0", 64)); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("another session on a development server: %v", err)
	}
	if _, err = New(pool, Config{Origin: "http://localhost"}).GameIdentity(ctx, dev.devGameToken()); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("the development session on a server without a development user: %v", err)
	}
	// It is the session the game is given when it signs in there.
	if status := signIn(dev, `{"username":"anyone","password":"anything"}`); status != 200 || login.AccessToken != dev.devGameToken() {
		t.Fatal("development login", status)
	}
}
