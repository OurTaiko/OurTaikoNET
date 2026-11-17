package rewards

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ourtaiko.dev/net/internal/netdb/netdbtest"
	"ourtaiko.dev/net/internal/requester"
)

const player, other = "0000000000000000000000000000abcd", "0000000000000000000000000000beef"

// What can be given becomes a reward by itself; the header counts the ones not seen; the routes
// give them and mark them seen, for the player the given Auth says, and for nobody else.
func TestRewardsAreToldByAHeaderAndFetched(t *testing.T) {
	pool := netdbtest.NewShared(t)
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO net.users(id) VALUES ($1),($2)`, player, other)
	exec(`INSERT INTO net.titles(id,name,style,name_translations) VALUES ('hello','はじめまして','gold','{"en":"Hello"}'),('gone','x','wood','{}')`)
	exec(`INSERT INTO net.awards(service,request_key,user_id,course_title) VALUES ('fanmade','attempt-1',$1,'十段')`, player)
	// A title of a 段位 course, one an administrator gives and takes back before it was seen,
	// and a badge that gets better, then not.
	exec(`INSERT INTO net.user_titles(user_id,title_id,award_id) SELECT $1,'hello',id FROM net.awards WHERE request_key='attempt-1'`, player)
	exec(`INSERT INTO net.user_titles(user_id,title_id) VALUES ($1,'gone')`, player)
	exec(`DELETE FROM net.user_titles WHERE user_id=$1 AND title_id='gone'`, player)
	exec(`INSERT INTO net.user_dans(user_id,dan,verdict) VALUES ($1,20,2)`, player)
	exec(`UPDATE net.user_dans SET verdict=6 WHERE user_id=$1 AND dan=20`, player)
	exec(`UPDATE net.user_dans SET verdict=6 WHERE user_id=$1 AND dan=20`, player)
	exec(`INSERT INTO net.user_titles(user_id,title_id) VALUES ($1,'hello')`, other)

	store := Store{DB: pool}
	if n, err := store.Pending(ctx, player, true); err != nil || n != 3 {
		t.Fatal("pending", n, err)
	}

	// Auth is the site's: here, a header names the player.
	auth := func(w http.ResponseWriter, r *http.Request, write bool) (string, bool) {
		user := r.Header.Get("Player")
		if user == "" || (write && r.Header.Get("Csrf") != "ok") {
			w.WriteHeader(401)
			return "", false
		}
		requester.Note(r.Context(), user)
		return user, true
	}
	mux := http.NewServeMux()
	store.Routes(mux, "/me/rewards", auth)
	// Any route of the site: it only says who asked, as signing in does.
	mux.HandleFunc("/anything", func(w http.ResponseWriter, r *http.Request) {
		if user := r.Header.Get("Player"); user != "" {
			requester.Note(r.Context(), user)
		}
		w.Write([]byte("ok"))
	})
	h := store.Announce(mux)
	send := func(method, path, user, body string, headers ...string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		if user != "" {
			r.Header.Set("Player", user)
		}
		for i := 0; i+1 < len(headers); i += 2 {
			r.Header.Set(headers[i], headers[i+1])
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}

	// Nobody signed in: no header.
	if w := send("GET", "/anything", "", ""); w.Header().Get(Header) != "" {
		t.Fatal("a guest was told", w.Header())
	}
	if w := send("GET", "/anything", player, ""); w.Header().Get(Header) != "3" {
		t.Fatal(w.Header())
	}

	var got struct{ Rewards []Reward }
	w := send("GET", "/me/rewards", player, "")
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &got) != nil || len(got.Rewards) != 3 {
		t.Fatal(w.Code, w.Body.String())
	}
	var title struct {
		Title struct {
			ID, Name, Style  string
			NameTranslations map[string]string
		}
		Service, CourseTitle string
	}
	json.Unmarshal(got.Rewards[0].Data, &title)
	if got.Rewards[0].Kind != "title" || title.Title.ID != "hello" || title.Title.Style != "gold" || title.Title.NameTranslations["en"] != "Hello" || title.Service != "fanmade" || title.CourseTitle != "十段" {
		t.Fatal(string(got.Rewards[0].Data))
	}
	var dan struct {
		Index, Verdict, Combo int
		Gold                  bool
	}
	json.Unmarshal(got.Rewards[2].Data, &dan)
	if got.Rewards[1].Kind != "dan" || got.Rewards[2].Kind != "dan" || dan.Index != 20 || dan.Verdict != 6 || !dan.Gold || dan.Combo != 2 {
		t.Fatal(string(got.Rewards[1].Data), string(got.Rewards[2].Data))
	}

	// Marking: only one's own, only with the website's proof; the answer already counts it.
	other1 := `{"ids":[` + jsonID(got.Rewards[0].ID) + `]}`
	if w := send("POST", "/me/rewards/seen", other, other1, "Csrf", "ok"); w.Code != 200 || !strings.Contains(w.Body.String(), `"marked":0`) {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := send("POST", "/me/rewards/seen", player, other1); w.Code != 401 {
		t.Fatal(w.Code)
	}
	for _, body := range []string{`{"ids":[]}`, `{"ids":"x"}`} {
		if w := send("POST", "/me/rewards/seen", player, body, "Csrf", "ok"); w.Code < 400 {
			t.Fatal(body, w.Code)
		}
	}
	w = send("POST", "/me/rewards/seen", player, other1, "Csrf", "ok")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"marked":1`) || w.Header().Get(Header) != "2" {
		t.Fatal(w.Code, w.Body.String(), w.Header())
	}
	if w := send("GET", "/anything", other, ""); w.Header().Get(Header) != "1" {
		t.Fatal("the other player's reward was touched", w.Header())
	}
	// A failing count leaves the header out and the answer alone.
	broken := Store{DB: pool}
	pool.Close()
	r := httptest.NewRequest("GET", "/anything", nil)
	r.Header.Set("Player", player)
	rec := httptest.NewRecorder()
	broken.Announce(mux).ServeHTTP(rec, r)
	if rec.Code != 200 || rec.Body.String() != "ok" || rec.Header().Get(Header) != "" {
		t.Fatal(rec.Code, rec.Body.String(), rec.Header())
	}
}

func jsonID(id int64) string { b, _ := json.Marshal(id); return string(b) }

// A handler that writes nothing still answers with the header.
func TestAnAnswerWithNothingWrittenIsToldToo(t *testing.T) {
	pool := netdbtest.NewShared(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO net.users(id) VALUES ($1)`, player); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO net.titles(id,name,style) VALUES ('t','t','wood')`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO net.user_titles(user_id,title_id) VALUES ($1,'t')`, player); err != nil {
		t.Fatal(err)
	}
	h := Store{DB: pool}.Announce(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requester.Note(r.Context(), player) }))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/quiet", nil))
	if w.Header().Get(Header) != "1" {
		t.Fatal(w.Header())
	}
}

// While Run listens, a player's count is kept between answers and dropped as soon as the
// database says it changed, by whatever wrote it; a request that may give something counts afresh.
// Seen rewards are pruned after Retention.
func TestCountsAreKeptWhileListeningAndSeenRewardsArePruned(t *testing.T) {
	pool := netdbtest.NewShared(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO net.users(id) VALUES ($1)`, player)
	exec(`INSERT INTO net.titles(id,name,style) SELECT id,id,'wood' FROM unnest(ARRAY['a','b','c']) id`)
	store := NewStore(pool)
	go store.Run(ctx, func(ctx context.Context) (Listener, func(), error) {
		conn, err := pool.Acquire(ctx)
		if err != nil {
			return nil, nil, err
		}
		return conn.Conn(), conn.Release, nil
	})
	waitFor := func(what string, ok func() bool) {
		t.Helper()
		for deadline := time.Now().Add(5 * time.Second); !ok(); time.Sleep(10 * time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatal(what)
			}
		}
	}
	pending := func(fresh bool) int {
		t.Helper()
		n, err := store.Pending(ctx, player, fresh)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	waitFor("not listening", func() bool { store.counts.mu.Lock(); defer store.counts.mu.Unlock(); return store.counts.live })
	if pending(false) != 0 {
		t.Fatal("nothing given yet")
	}
	_, _, kept := store.counts.get(player)
	if !kept {
		t.Fatal("the count was not kept")
	}

	// Another connection gives a title: the kept count is dropped and the next answer counts it.
	exec(`INSERT INTO net.user_titles(user_id,title_id) VALUES ($1,'a')`, player)
	waitFor("the change was not heard", func() bool { _, _, ok := store.counts.get(player); return !ok })
	if pending(false) != 1 {
		t.Fatal("not counted after the change")
	}
	// A request that may have given something counts afresh, before any word arrives.
	if _, err := pool.Exec(ctx, `INSERT INTO net.user_titles(user_id,title_id) VALUES ($1,'b')`, player); err != nil {
		t.Fatal(err)
	}
	if pending(true) != 2 {
		t.Fatal("a fresh count read the kept one")
	}

	// Marked seen: dropped at once.
	var ids []int64
	rows, _ := pool.Query(ctx, `SELECT id FROM net.rewards WHERE user_id=$1`, player)
	for rows.Next() {
		var id int64
		rows.Scan(&id)
		ids = append(ids, id)
	}
	if n, err := store.Seen(ctx, player, ids); err != nil || n != 2 {
		t.Fatal(n, err)
	}
	if pending(false) != 0 {
		t.Fatal("still counted after being seen")
	}

	// Pruned: seen long ago only.
	exec(`INSERT INTO net.user_titles(user_id,title_id) VALUES ($1,'c')`, player)
	exec(`UPDATE net.rewards SET seen_at=now()-interval '91 days' WHERE id=$1`, ids[0])
	if n, err := store.Prune(ctx); err != nil || n != 1 {
		t.Fatal("pruned", n, err)
	}
	var left int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM net.rewards`).Scan(&left); err != nil || left != 2 {
		t.Fatal("left", left, err)
	}
}
