package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	fanmade "ourtaiko.dev/net/internal/fanmade/httpapi"
	"ourtaiko.dev/net/internal/netdb"
	"ourtaiko.dev/net/internal/netdb/netdbtest"
	official "ourtaiko.dev/net/internal/official/httpapi"
)

// One program, one database: each source answers under its name at the new address and as
// its old site at the old one, and neither sees the other's charts.
func TestBothSourcesFromOneDatabase(t *testing.T) {
	pool, url := netdbtest.NewURL(t, "fanmade")
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
INSERT INTO net.users(id) VALUES (repeat('a',32));
INSERT INTO fanmade.chart_resources(storage_key,original_filename,sha256,byte_size,extension) VALUES
 (repeat('1',64)||'.tja','a.tja',repeat('1',64),10,'tja'), (repeat('2',64)||'.ogg','a.ogg',repeat('2',64),10,'ogg');
INSERT INTO official.chart_resources(storage_key,original_filename,sha256,byte_size,extension) VALUES
 (repeat('3',64)||'.tja','b.tja',repeat('3',64),10,'tja'), (repeat('4',64)||'.ogg','b.ogg',repeat('4',64),10,'ogg');
INSERT INTO fanmade.charts(id,owner_ids,title,bpm,duration,demo_end,tja_file,audio_file)
 SELECT repeat('f',32), ARRAY[repeat('a',32)], 'Fan song', 120, 60, 10, min(id), max(id) FROM fanmade.chart_resources;
-- The official source lists a chart in the game only once it has a category.
INSERT INTO official.charts(id,owner_ids,title,bpm,duration,demo_end,tja_file,audio_file,category_flags)
 SELECT repeat('d',32), ARRAY[repeat('a',32)], 'Official song', 120, 60, 10, min(id), max(id), 1 FROM official.chart_resources;
INSERT INTO fanmade.chart_difficulties(chart_id,course,side,level,content_hash) VALUES (repeat('f',32),'Oni','single',8,'v2:'||repeat('0',64));
INSERT INTO official.chart_difficulties(chart_id,course,side,level,content_hash) VALUES (repeat('d',32),'Oni','single',9,'v2:'||repeat('1',64));
`); err != nil {
		t.Fatal(err)
	}
	a, err := New(ctx, Config{DatabaseURL: url, Origin: "http://localhost", StorageDir: t.TempDir(),
		LegacyFanmadeHost: "fanmade.ourtaiko.org", LegacyOfficialHost: "database.ourtaiko.org"})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	h := a.Handler()
	titles := func(host, path string) (int, []string) {
		t.Helper()
		r := httptest.NewRequest("GET", path, nil)
		r.Host = host
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		var body struct {
			Charts []struct {
				Title string `json:"title"`
			} `json:"charts"`
		}
		if w.Code == http.StatusOK {
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("%s%s: %v: %s", host, path, err, w.Body.String())
			}
		}
		out := []string{}
		for _, c := range body.Charts {
			out = append(out, c.Title)
		}
		return w.Code, out
	}
	for _, c := range []struct {
		host, path string
		status     int
		want       string
	}{
		{"net.ourtaiko.org", "/api/v1/sources/fanmade/game/catalog", 200, "Fan song"},
		{"net.ourtaiko.org", "/api/v1/sources/official/game/catalog", 200, "Official song"},
		{"fanmade.ourtaiko.org", "/api/v1/game/catalog", 200, "Fan song"},
		{"database.ourtaiko.org", "/api/v1/game/catalog", 200, "Official song"},
		// The new address has no catalog of its own outside a source, and the old addresses
		// do not know the new routes.
		{"net.ourtaiko.org", "/api/v1/game/catalog", 404, ""},
		{"fanmade.ourtaiko.org", "/api/v1/sources/official/game/catalog", 404, ""},
	} {
		status, got := titles(c.host, c.path)
		if status != c.status || (c.want != "" && (len(got) != 1 || got[0] != c.want)) {
			t.Errorf("%s%s: status %d charts %v, want %d [%s]", c.host, c.path, status, got, c.status, c.want)
		}
	}
}

func TestInitRefusesADatabaseThatIsNotEmpty(t *testing.T) {
	_, url := netdbtest.NewURL(t, "fanmade")
	if err := Init(context.Background(), url); err == nil {
		t.Fatal("Init filled a database that already had tables")
	}
}

func TestNewRefusesADatabaseOfAnotherVersion(t *testing.T) {
	pool, url := netdbtest.NewURL(t, "fanmade")
	if _, err := pool.Exec(context.Background(), `INSERT INTO net.schema_migrations(version) VALUES (99)`); err != nil {
		t.Fatal(err)
	}
	if _, err := New(context.Background(), Config{DatabaseURL: url, Origin: "http://localhost", StorageDir: t.TempDir()}); err == nil {
		t.Fatal("New accepted a newer schema")
	}
}

// The players' part reads both sources' scores and the sources ask it for awards, all inside
// one program; from outside, the award route does not exist.
func TestPlayersAcrossSources(t *testing.T) {
	pool, url := netdbtest.NewURL(t, "fanmade")
	ctx := context.Background()
	player := strings.Repeat("a", 32)
	if _, err := pool.Exec(ctx, `
INSERT INTO net.users(id) VALUES (repeat('a',32));
INSERT INTO fanmade.chart_resources(storage_key,original_filename,sha256,byte_size,extension) VALUES
 (repeat('1',64)||'.tja','a.tja',repeat('1',64),10,'tja'), (repeat('2',64)||'.ogg','a.ogg',repeat('2',64),10,'ogg');
INSERT INTO official.chart_resources(storage_key,original_filename,sha256,byte_size,extension) VALUES
 (repeat('3',64)||'.tja','b.tja',repeat('3',64),10,'tja'), (repeat('4',64)||'.ogg','b.ogg',repeat('4',64),10,'ogg');
INSERT INTO fanmade.charts(id,owner_ids,title,bpm,duration,demo_end,tja_file,audio_file)
 SELECT repeat('f',32), ARRAY[repeat('a',32)], 'Fan song', 120, 60, 10, min(id), max(id) FROM fanmade.chart_resources;
INSERT INTO official.charts(id,owner_ids,title,bpm,duration,demo_end,tja_file,audio_file,category_flags)
 SELECT repeat('d',32), ARRAY[repeat('a',32)], 'Official song', 120, 60, 10, min(id), max(id), 1 FROM official.chart_resources;
INSERT INTO fanmade.chart_difficulties(id,chart_id,course,side,level,content_hash) OVERRIDING SYSTEM VALUE VALUES (5,repeat('f',32),'Oni','single',8,'v2:'||repeat('0',64));
INSERT INTO official.chart_difficulties(id,chart_id,course,side,level,content_hash) OVERRIDING SYSTEM VALUE VALUES (5,repeat('d',32),'Oni','single',9,'v2:'||repeat('1',64));
INSERT INTO net.scores(source,id,user_id,good,ok,bad,score,drumroll,max_combo,payload_digest,difficulty_id) VALUES
 ('fanmade',repeat('1',32),repeat('a',32),10,0,0,1000,0,10,repeat('d',64),5),
 ('official',repeat('2',32),repeat('a',32),20,0,0,2000,0,20,repeat('d',64),5);
`); err != nil {
		t.Fatal(err)
	}
	a, err := New(ctx, Config{DatabaseURL: url, Origin: "http://localhost", StorageDir: t.TempDir(), LegacyProfileHost: "profile.ourtaiko.org"})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	h := a.Handler()
	get := func(method, host, path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, nil)
		r.Host = host
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}

	// One list of a player's plays, from both sources, at the new address and the old one.
	for _, host := range []string{"net.ourtaiko.org", "profile.ourtaiko.org"} {
		w := get("GET", host, "/api/v1/users/"+player+"/scores?view=all")
		var list struct {
			Items []struct {
				Source   string `json:"source"`
				Title    string `json:"title"`
				Score    int    `json:"score"`
				ChartURL string `json:"chartUrl"`
			} `json:"items"`
			Sources []struct {
				ID    string `json:"id"`
				Error string `json:"error"`
			} `json:"sources"`
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &list) != nil {
			t.Fatalf("%s: scores %d %s", host, w.Code, w.Body.String())
		}
		for _, s := range list.Sources {
			if s.Error != "" {
				t.Errorf("%s: source %s could not be read: %s", host, s.ID, s.Error)
			}
		}
		got := map[string]string{}
		for _, item := range list.Items {
			got[item.Source] = item.Title + " " + item.ChartURL
		}
		if len(list.Items) != 2 || got["fanmade"] != "Fan song http://localhost/fanmade/charts/"+strings.Repeat("f", 32) ||
			got["official"] != "Official song http://localhost/official/charts/"+strings.Repeat("d", 32) {
			t.Errorf("%s: scores %v from %s", host, got, w.Body.String())
		}
	}

	// What a dan attempt earns is recorded in the transaction the source saves the attempt in,
	// under the source's name: kept when that transaction commits, gone when it does not.
	official1, err := netdb.Open(ctx, url, "official")
	if err != nil {
		t.Fatal(err)
	}
	defer official1.Close()
	grant := func(pool *pgxpool.Pool, commit bool, give func(tx pgx.Tx) (json.RawMessage, error)) {
		t.Helper()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		answer, err := give(tx)
		if err != nil || !strings.Contains(string(answer), `"granted"`) || !strings.Contains(string(answer), `"nameplate"`) {
			t.Fatalf("award: %s %v", answer, err)
		}
		if commit {
			if err = tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
		}
	}
	grant(pool, true, func(tx pgx.Tx) (json.RawMessage, error) {
		return a.Fanmade.Config.Awards.Grant(ctx, tx, fanmade.AwardRequest{Key: "attempt-f", UserID: player, CourseID: strings.Repeat("c", 32), CourseTitle: "Course", Verdict: 4, Titles: []string{}})
	})
	grant(official1, true, func(tx pgx.Tx) (json.RawMessage, error) {
		return a.Official.Config.Awards.Grant(ctx, tx, official.AwardRequest{Key: "attempt-o", UserID: strings.Repeat("b", 32), CourseID: strings.Repeat("c", 32), CourseTitle: "Course", Verdict: 4, Titles: []string{}})
	})
	grant(pool, false, func(tx pgx.Tx) (json.RawMessage, error) {
		return a.Fanmade.Config.Awards.Grant(ctx, tx, fanmade.AwardRequest{Key: "attempt-undone", UserID: player, CourseID: strings.Repeat("c", 32), CourseTitle: "Course", Verdict: 4, Titles: []string{}})
	})
	var recorded string
	if err = pool.QueryRow(ctx, `SELECT string_agg(service||':'||request_key, ',' ORDER BY service) FROM net.awards`).Scan(&recorded); err != nil || recorded != "fanmade:attempt-f,official:attempt-o" {
		t.Fatalf("awards recorded: %q %v", recorded, err)
	}
	// An attempt already used for another player is refused for good, as the source's own kind
	// of refusal, and the transaction goes on.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.Fanmade.Config.Awards.Grant(ctx, tx, fanmade.AwardRequest{Key: "attempt-f", UserID: strings.Repeat("b", 32), CourseID: strings.Repeat("c", 32), Verdict: 4, Titles: []string{}})
	var refused *fanmade.ProfileRejection
	if !errors.As(err, &refused) || refused.Code != "AWARD_CONFLICT" {
		t.Fatal("an attempt of another player:", err)
	}
	if _, err = tx.Exec(ctx, `SELECT 1`); err != nil {
		t.Fatal("the transaction is not usable after a refusal:", err)
	}
	tx.Rollback(ctx)
	// The second award was for someone no source had seen: they are a player now.
	var known bool
	if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM net.users WHERE id=repeat('b',32))`).Scan(&known); err != nil || !known {
		t.Fatal("the awarded player is not in net.users", err)
	}
	// The sources still read the title catalog from the players' part.
	if titles, err := a.Fanmade.Config.Profile.Titles(ctx, true); err != nil || titles == nil {
		t.Fatal("title catalog:", titles, err)
	}

	// Nameplates are public; asking for awards is not possible from outside.
	if w := get("GET", "profile.ourtaiko.org", "/api/v1/users/"+player+"/nameplate"); w.Code != 200 {
		t.Errorf("nameplate at the old address: %d %s", w.Code, w.Body.String())
	}
	if w := get("GET", "net.ourtaiko.org", "/api/v1/users/"+player+"/nameplate"); w.Code != 200 {
		t.Errorf("nameplate at the new address: %d %s", w.Code, w.Body.String())
	}
	for _, host := range []string{"net.ourtaiko.org", "profile.ourtaiko.org", "fanmade.ourtaiko.org"} {
		if w := get("POST", host, "/internal/v1/awards"); w.Code != 404 {
			t.Errorf("POST /internal/v1/awards on %s: %d, want 404", host, w.Code)
		}
	}
}
