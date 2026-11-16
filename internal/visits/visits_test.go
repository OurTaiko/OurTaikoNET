package visits

import (
	"context"
	"fmt"
	"testing"
	"time"

	"ourtaiko.dev/net/internal/netdb/netdbtest"
)

func TestAPageIsAnAddressWithoutItsIDs(t *testing.T) {
	for address, want := range map[string]string{
		"/":         "/",
		"/fanmade/": "/fanmade",
		"/fanmade/charts/0123456789abcdef0123456789abcdef":  "/fanmade/charts/:id",
		"/official/playlists/42?sort=new#top":               "/official/playlists/:id",
		"/players/abcdefabcdefabcdefabcdefabcdefab":         "/players/:id",
		"/fanmade/me/charts":                                "/fanmade/me/charts",
		"/fanmade//charts/%E5%A4%AA%E9%BC%93":               "/fanmade/charts/:id",
		"/Fanmade/Charts":                                   "/:id/:id",
		"/a/b/c/d/e/f/g/h":                                  "/a/b/c/d/e/f",
		"/fanmade/charts/0123456789abcdef0123456789abcdef/": "/fanmade/charts/:id",
	} {
		if got, ok := Page(address); !ok || got != want {
			t.Errorf("Page(%q) = %q, %v; want %q", address, got, ok, want)
		}
	}
	long := "/"
	for len(long) <= maxPath {
		long += "charts/"
	}
	for _, address := range []string{"", "fanmade", "https://example.com/", long} {
		if got, ok := Page(address); ok {
			t.Errorf("Page(%q) = %q; want none", address, got)
		}
	}
}

func TestProgramsAreNotVisitors(t *testing.T) {
	for agent, want := range map[string]bool{
		"": true,
		"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)": true,
		"curl/8.7.1":           true,
		"python-requests/2.32": true,
		"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) HeadlessChrome/141.0.0.0 Safari/537.36":         true,
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/19.0 Safari/605.1.15": false,
		"Mozilla/5.0 (Linux; Android 15; Pixel 9) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Mobile Safari/537.36": false,
	} {
		if got := Robot(agent); got != want {
			t.Errorf("Robot(%q) = %v", agent, got)
		}
	}
}

func TestViewsAndVisitorsADay(t *testing.T) {
	ctx := context.Background()
	pool := netdbtest.NewShared(t)
	// 23:00 UTC is already the next day where days are counted.
	now := time.Date(2026, 11, 15, 23, 0, 0, 0, time.UTC)
	store := Store{DB: pool, Now: func() time.Time { return now }}
	record := func(page, who string) {
		t.Helper()
		if err := store.Record(ctx, page, who); err != nil {
			t.Fatal(err)
		}
	}
	summary := func() Summary {
		t.Helper()
		s, err := store.Summary(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}

	if s := summary(); s != (Summary{}) {
		t.Fatalf("before anyone came %+v", s)
	}
	record("/", "1.1.1.1\nSafari")
	record("/fanmade/charts/:id", "1.1.1.1\nSafari")
	record("/fanmade/charts/:id", "2.2.2.2\nSafari")
	record("/", "1.1.1.1\nFirefox")
	if s := summary(); s.Total != (Count{4, 3}) || s.Today != (Count{4, 3}) || s.Since != "2026-11-16" {
		t.Fatalf("the first day %+v", s)
	}

	// Nothing that is kept says who came: only hashes, which the day's salt made.
	var plain int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM net.visit_visitors WHERE length(visitor) <> 32 OR position('1.1.1.1' in encode(visitor, 'escape')) > 0`).Scan(&plain); err != nil || plain != 0 {
		t.Fatal("visitors are not kept as hashes", plain, err)
	}

	// The next day the same people are visitors again, and the day before is closed: counted,
	// and what told its visitors apart is gone.
	now = now.Add(24 * time.Hour)
	if closed, err := store.Close(ctx); err != nil || closed != 1 {
		t.Fatal("closing the first day", closed, err)
	}
	if closed, err := store.Close(ctx); err != nil || closed != 0 {
		t.Fatal("closing again", closed, err)
	}
	var kept int
	var salted bool
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM net.visit_visitors), (SELECT salt IS NOT NULL FROM net.visit_days WHERE day = '2026-11-16')`).Scan(&kept, &salted); err != nil || kept != 0 || salted {
		t.Fatal("a closed day keeps hashes or its salt", kept, salted, err)
	}
	record("/", "1.1.1.1\nSafari")
	record("/players", "1.1.1.1\nSafari")
	if s := summary(); s.Total != (Count{6, 4}) || s.Today != (Count{2, 1}) || s.Since != "2026-11-16" {
		t.Fatalf("the second day %+v", s)
	}

	// A view that arrives for a closed day is a view, and no visitor.
	late := Store{DB: pool, Now: func() time.Time { return now.Add(-24 * time.Hour) }}
	if err := late.Record(ctx, "/", "9.9.9.9\nSafari"); err != nil {
		t.Fatal(err)
	}
	if s := summary(); s.Total != (Count{7, 4}) {
		t.Fatalf("after a late view %+v", s)
	}

	report, err := store.Report(ctx, 3, 2)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(report.Days) != "[{2026-11-15 {0 0}} {2026-11-16 {5 3}} {2026-11-17 {2 1}}]" {
		t.Fatalf("days %v", report.Days)
	}
	if fmt.Sprint(report.Pages) != "[{/ 4} {/fanmade/charts/:id 2}]" {
		t.Fatalf("pages %v", report.Pages)
	}
	if report, err = store.Report(ctx, 1, 10); err != nil || fmt.Sprint(report.Pages) != "[{/ 1} {/players 1}]" || len(report.Days) != 1 {
		t.Fatalf("today alone %v %v", report, err)
	}
}

func TestADayHasOnlySoManyPages(t *testing.T) {
	ctx := context.Background()
	pool := netdbtest.NewShared(t)
	store := Store{DB: pool}
	for i := range maxPages + 5 {
		if err := store.Record(ctx, fmt.Sprintf("/made-up-%d", i), "1.1.1.1\nSafari"); err != nil {
			t.Fatal(err)
		}
	}
	// A page already counted still is.
	if err := store.Record(ctx, "/made-up-0", "1.1.1.1\nSafari"); err != nil {
		t.Fatal(err)
	}
	var pages int
	var other, first int64
	if err := pool.QueryRow(ctx, `SELECT count(*), sum(views) FILTER (WHERE page = $1), sum(views) FILTER (WHERE page = '/made-up-0') FROM net.visit_pages`, Other).Scan(&pages, &other, &first); err != nil {
		t.Fatal(err)
	}
	if pages != maxPages+1 || other != 5 || first != 2 {
		t.Fatalf("%d pages, %d views of others, %d of the first", pages, other, first)
	}
}
