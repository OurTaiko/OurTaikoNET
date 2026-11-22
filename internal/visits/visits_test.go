package visits

import (
	"context"
	"fmt"
	"strings"
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

func TestATimeZoneIsNamed(t *testing.T) {
	for name, want := range map[string]string{"": "UTC", "UTC": "UTC", "Asia/Shanghai": "Asia/Shanghai", "Etc/GMT+8": "Etc/GMT+8", "America/Argentina/Buenos_Aires": "America/Argentina/Buenos_Aires"} {
		if loc, err := Zone(name); err != nil || loc.String() != want {
			t.Errorf("Zone(%q) = %v, %v", name, loc, err)
		}
	}
	for _, name := range []string{"Local", "Mars/Olympus", "../../etc/passwd", "Asia/Shanghai ", "/Asia", strings.Repeat("A", 65)} {
		if _, err := Zone(name); err != ErrZone {
			t.Errorf("Zone(%q): %v", name, err)
		}
	}
}

// Every reader sees the days of their own time zone, and a visitor is one visitor all of a day
// of any zone, though it spans two UTC days.
func TestViewsAndVisitorsInEveryTimeZone(t *testing.T) {
	ctx := context.Background()
	pool := netdbtest.NewShared(t)
	var now time.Time
	store := Store{DB: pool, Now: func() time.Time { return now }}
	at := func(when, page string, who Visitor) {
		t.Helper()
		var err error
		if now, err = time.Parse(time.RFC3339, when); err != nil {
			t.Fatal(err)
		}
		if err := store.Record(ctx, page, who); err != nil {
			t.Fatal(err)
		}
	}
	zone := func(name string) *time.Location {
		loc, err := Zone(name)
		if err != nil {
			t.Fatal(err)
		}
		return loc
	}
	shanghai, angeles, utc := zone("Asia/Shanghai"), zone("America/Los_Angeles"), zone("")
	summary := func(loc *time.Location) Summary {
		t.Helper()
		s, err := store.Summary(ctx, loc)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}

	if s := summary(shanghai); s != (Summary{}) {
		t.Fatalf("before anyone came %+v", s)
	}
	x, y := Visitor{Host: "1.1.1.1", Browser: "Safari", Country: "AU"}, Visitor{Host: "2.2.2.2", Browser: "Safari", Country: "FR"}
	//                          Shanghai     Los Angeles
	at("2026-11-15T15:00:00Z", "/", x)        // 15th 23:00  15th 07:00
	at("2026-11-15T17:00:00Z", "/players", x) // 16th 01:00  15th 09:00
	at("2026-11-16T01:00:00Z", "/", x)        // 16th 09:00  15th 17:00
	at("2026-11-16T03:00:00Z", "/", y)        // 16th 11:00  15th 19:00
	at("2026-11-16T10:00:00Z", "/titles", x)  // 16th 18:00  16th 02:00
	now = time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)

	for _, c := range []struct {
		loc  *time.Location
		want Summary
	}{
		{shanghai, Summary{Total: Count{5, 3}, Today: Count{4, 2}, Since: "2026-11-15"}},
		{angeles, Summary{Total: Count{5, 3}, Today: Count{1, 1}, Since: "2026-11-15"}},
		{utc, Summary{Total: Count{5, 3}, Today: Count{3, 2}, Since: "2026-11-15"}},
	} {
		if s := summary(c.loc); s != c.want {
			t.Errorf("in %v %+v; want %+v", c.loc, s, c.want)
		}
	}
	report := func(loc *time.Location, days, pages int) Report {
		t.Helper()
		r, err := store.Report(ctx, loc, days, pages)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	r := report(shanghai, 3, 10)
	if fmt.Sprint(r.Days) != "[{2026-11-14 {0 0}} {2026-11-15 {1 1}} {2026-11-16 {4 2}}]" {
		t.Errorf("days in Shanghai %v", r.Days)
	}
	if fmt.Sprint(r.Hours) != "[0 1 0 0 0 0 0 0 0 1 0 1 0 0 0 0 0 0 1 0 0 0 0 1]" {
		t.Errorf("hours in Shanghai %v", r.Hours)
	}
	if fmt.Sprint(r.Pages) != "[{/ 3} {/players 1} {/titles 1}]" {
		t.Errorf("pages %v", r.Pages)
	}
	r = report(angeles, 2, 1)
	if fmt.Sprint(r.Days) != "[{2026-11-15 {4 2}} {2026-11-16 {1 1}}]" || fmt.Sprint(r.Pages) != "[{/ 3}]" || r.Hours[2] != 1 || r.Hours[7] != 1 {
		t.Errorf("Los Angeles %+v", r)
	}
	// Today alone, in Los Angeles: only the last view.
	if r = report(angeles, 1, 10); fmt.Sprint(r.Days) != "[{2026-11-16 {1 1}}]" || fmt.Sprint(r.Pages) != "[{/titles 1}]" {
		t.Errorf("today in Los Angeles %+v", r)
	}

	// What tells one day's visitors apart is hashes, which the salts made, not the address.
	var plain int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM net.visit_views WHERE position('1.1.1.1' in encode(visitor_even || visitor_odd, 'escape')) > 0`).Scan(&plain); err != nil || plain != 0 {
		t.Fatal("visitors are not kept as hashes", plain, err)
	}

	// A span is over two days after it began, and its salt goes; the counts stay as they were.
	if closed, err := store.Close(ctx); err != nil || closed != 1 {
		t.Fatal("closing on the 16th", closed, err)
	}
	now = now.Add(48 * time.Hour)
	if closed, err := store.Close(ctx); err != nil || closed != 2 {
		t.Fatal("closing on the 18th", closed, err)
	}
	var salts int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM net.visit_salts`).Scan(&salts); err != nil || salts != 0 {
		t.Fatal("salts kept", salts, err)
	}
	if s := summary(shanghai); s.Total != (Count{5, 3}) || s.Today != (Count{}) {
		t.Fatalf("after the salts went %+v", s)
	}
	// The same people on another day are visitors of that day again.
	at("2026-11-18T13:00:00Z", "/", x)
	at("2026-11-18T13:30:00Z", "/", x)
	if s := summary(shanghai); s.Total != (Count{7, 4}) || s.Today != (Count{2, 1}) {
		t.Fatalf("on the 18th %+v", s)
	}
}

// Each view keeps the address it came from and its country; the countries of a period are
// counted with their views and their different addresses, and what has no country is unknown.
func TestViewsAreCountedByCountry(t *testing.T) {
	ctx := context.Background()
	pool := netdbtest.NewShared(t)
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	store := Store{DB: pool, Now: func() time.Time { return now }}
	// A view made before addresses were kept.
	if _, err := pool.Exec(ctx, `INSERT INTO net.visit_views (at, page, visitor_even, visitor_odd) VALUES ($1, '/', sha256('a'), sha256('b'))`, now); err != nil {
		t.Fatal(err)
	}
	for _, v := range []Visitor{
		{Host: "1.1.1.1", Browser: "Safari", Country: "AU"},
		{Host: "1.1.1.1", Browser: "Firefox", Country: "AU"},
		{Host: "1.1.1.2", Browser: "Safari", Country: "AU"},
		{Host: "::ffff:2.2.2.2", Browser: "Safari", Country: "FR"},
		{Host: "2001:db8::1", Browser: "Safari", Country: "JP"},
		{Host: "2001:db8::1", Browser: "Safari", Country: "JP"},
		// No country known, a country that is none, an address that is none.
		{Host: "3.3.3.3", Browser: "Safari"},
		{Host: "3.3.3.4", Browser: "Safari", Country: "jp"},
		{Host: "somewhere", Browser: "Safari", Country: "Japan"},
	} {
		if err := store.Record(ctx, "/", v); err != nil {
			t.Fatal(v, err)
		}
	}
	r, err := store.Report(ctx, time.UTC, 1, 10)
	if err != nil || fmt.Sprint(r.Countries) != "[{ 4 2} {AU 3 2} {JP 2 1} {FR 1 1}]" {
		t.Fatalf("countries %v %v", r.Countries, err)
	}
	// In the order of addresses, which is the same in every database (that of their text is the
	// database's collation's).
	var kept string
	if err := pool.QueryRow(ctx, `SELECT string_agg(host(address), ' ' ORDER BY address) FROM (SELECT DISTINCT address FROM net.visit_views WHERE address IS NOT NULL) a`).Scan(&kept); err != nil || kept != "1.1.1.1 1.1.1.2 2.2.2.2 3.3.3.3 3.3.3.4 2001:db8::1" {
		t.Fatalf("addresses kept %q %v", kept, err)
	}
	// The views of another period are not of this one's countries.
	now = now.Add(48 * time.Hour)
	if r, err = store.Report(ctx, time.UTC, 1, 10); err != nil || len(r.Countries) != 0 {
		t.Fatalf("two days later %v %v", r.Countries, err)
	}
}

func TestViewsAreCountedBySite(t *testing.T) {
	ctx := context.Background()
	pool := netdbtest.NewShared(t)
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	store := Store{DB: pool, Now: func() time.Time { return now }}
	// A view made before the site was kept.
	if _, err := pool.Exec(ctx, `INSERT INTO net.visit_views (at, page, visitor_even, visitor_odd) VALUES ($1, '/', sha256('a'), sha256('b'))`, now); err != nil {
		t.Fatal(err)
	}
	for _, v := range []Visitor{
		{Host: "1.1.1.1", Browser: "Safari", Site: "net.ourtaiko.org"},
		{Host: "1.1.1.2", Browser: "Safari", Site: "net.ourtaiko.org"},
		{Host: "2.2.2.2", Browser: "Safari", Site: "otnet.sakura-bot.cn"},
		{Host: "2.2.2.2", Browser: "Firefox", Site: "otnet.sakura-bot.cn"},
		{Host: "3.3.3.3", Browser: "Safari", Site: "127.0.0.1:5173"},
		// No site known, and sites that are none.
		{Host: "4.4.4.4", Browser: "Safari"},
		{Host: "4.4.4.5", Browser: "Safari", Site: "Net.OurTaiko.org"},
		{Host: "4.4.4.6", Browser: "Safari", Site: "https://net.ourtaiko.org"},
	} {
		if err := store.Record(ctx, "/", v); err != nil {
			t.Fatal(v, err)
		}
	}
	r, err := store.Report(ctx, time.UTC, 1, 10)
	if err != nil || fmt.Sprint(r.Sites) != "[{ 4 3} {net.ourtaiko.org 2 2} {otnet.sakura-bot.cn 2 1} {127.0.0.1:5173 1 1}]" {
		t.Fatalf("sites %v %v", r.Sites, err)
	}
	// The views of another period are not of this one's sites.
	now = now.Add(48 * time.Hour)
	if r, err = store.Report(ctx, time.UTC, 1, 10); err != nil || len(r.Sites) != 0 {
		t.Fatalf("two days later %v %v", r.Sites, err)
	}
}
