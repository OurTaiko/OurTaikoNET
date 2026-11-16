// Package visits counts how much the website is visited: the pages shown each day and how
// many different visitors saw them (net.visit_days, visit_pages, visit_visitors; migration
// 0005).
//
// The website says which page it shows; nothing is counted from the requests themselves, so
// programs that read the API or fetch a page without running it are not visitors. No cookie
// is set and no address is kept. A visitor is told from another within one day only, by a
// hash of who they are (their address and browser) with a salt made for that day; when the
// day is over, Close counts the hashes, writes the number down and deletes them and the
// salt, after which nobody can tell who visited.
package visits

import (
	"context"
	"crypto/rand"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Zone is where a day begins and ends: UTC+8, where most of the site's players are.
var Zone = time.FixedZone("UTC+8", 8*60*60)

// Day is the day of t, as the tables name it.
func Day(t time.Time) string { return t.In(Zone).Format(time.DateOnly) }

const (
	// The longest address a page is counted under, and how many of its parts are kept.
	maxPath, maxParts = 200, 6
	// How many different pages a day is counted under; the views of any more go to Other.
	maxPages = 300
	// Other is the page of views that could not have a page of their own.
	Other = "(other)"
	// MaxDays is the longest period a report covers.
	MaxDays = 366
)

var (
	// A part of an address that names a kind of page (charts, playlists, me).
	word = regexp.MustCompile(`^[a-z][a-z0-9-]{0,23}$`)
	// A browser that says it is a program, not a person.
	robot = regexp.MustCompile(`(?i)bot|crawl|spider|slurp|headless|lighthouse|preview|monitor|curl|wget|python|scrapy|http-?client`)
)

// Page is the page an address of the website is counted under: the address without what
// follows ? or #, with every part that is not a plain word (an id, a number, a name) replaced
// by ":id", so that all charts are one page, /fanmade/charts/:id. It reports false for what is
// no address of the site.
func Page(path string) (string, bool) {
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}
	if !strings.HasPrefix(path, "/") || len(path) > maxPath {
		return "", false
	}
	parts := []string{}
	for _, part := range strings.Split(path, "/") {
		if part == "" {
			continue
		}
		if len(parts) == maxParts {
			break
		}
		if !word.MatchString(part) {
			part = ":id"
		}
		parts = append(parts, part)
	}
	return "/" + strings.Join(parts, "/"), true
}

// Robot reports whether a browser's name (User-Agent) is a program's, or missing.
func Robot(userAgent string) bool {
	return strings.TrimSpace(userAgent) == "" || robot.MatchString(userAgent)
}

// Store counts in ourtaiko_net. Now is the clock (nil: time.Now).
type Store struct {
	DB  *pgxpool.Pool
	Now func() time.Time
}

func (s Store) today() string {
	if s.Now != nil {
		return Day(s.Now())
	}
	return Day(time.Now())
}

// Record counts one view of page (as Page gives it) by who, today. who is what tells the
// visitor from others (address and browser); it is only ever stored hashed with the day's salt.
func (s Store) Record(ctx context.Context, page, who string) error {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	// The salt given is kept only by the first view of the day; every view hashes with the
	// one the day has.
	_, err := s.DB.Exec(ctx, `
WITH day AS (
    INSERT INTO net.visit_days AS d (day, views, salt) VALUES ($1::date, 1, $4)
    ON CONFLICT (day) DO UPDATE SET views = d.views + 1
    RETURNING salt
), seen AS (
    -- A day already closed (a slow request at midnight) has no salt: no visitor is added.
    INSERT INTO net.visit_visitors (day, visitor)
    SELECT $1::date, sha256(day.salt || convert_to($3, 'UTF8')) FROM day WHERE day.salt IS NOT NULL
    ON CONFLICT DO NOTHING
)
INSERT INTO net.visit_pages AS p (day, page, views)
SELECT $1::date,
       CASE WHEN EXISTS (SELECT 1 FROM net.visit_pages WHERE day = $1::date AND page = $2)
                 OR (SELECT count(*) FROM net.visit_pages WHERE day = $1::date) < $5 THEN $2 ELSE $6 END,
       1
ON CONFLICT (day, page) DO UPDATE SET views = p.views + 1`,
		s.today(), page, who, salt, maxPages, Other)
	return err
}

// Close ends every day before today that is still open: its visitors are counted, and the
// hashes that told them apart are deleted with the day's salt. It reports how many days it
// closed.
func (s Store) Close(ctx context.Context) (int, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	today := s.today()
	tag, err := tx.Exec(ctx, `
UPDATE net.visit_days d SET salt = NULL,
       visitors = (SELECT count(*) FROM net.visit_visitors v WHERE v.day = d.day)
 WHERE d.day < $1::date AND d.visitors IS NULL`, today)
	if err != nil {
		return 0, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM net.visit_visitors WHERE day < $1::date`, today); err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), tx.Commit(ctx)
}

// RunClose closes finished days now and every hour until ctx ends.
func (s Store) RunClose(ctx context.Context, failed func(error)) {
	tick := time.NewTicker(time.Hour)
	defer tick.Stop()
	for {
		if _, err := s.Close(ctx); err != nil && ctx.Err() == nil {
			failed(err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// Count is a number of page views and of visitors. Over more than one day, Visitors is the
// sum of each day's: someone who came on three days is three.
type Count struct {
	Views    int64 `json:"views"`
	Visitors int64 `json:"visitors"`
}

// Summary is what everyone may know: all there has been, today's, and the first day counted
// (empty when nothing has been).
type Summary struct {
	Total Count  `json:"total"`
	Today Count  `json:"today"`
	Since string `json:"since"`
}

// The visitors of a day, whether it is closed or still open.
const visitorsOf = `COALESCE(d.visitors, (SELECT count(*) FROM net.visit_visitors v WHERE v.day = d.day))`

// Summary reads the summary.
func (s Store) Summary(ctx context.Context) (Summary, error) {
	var out Summary
	err := s.DB.QueryRow(ctx, `
SELECT COALESCE(sum(views), 0)::bigint, COALESCE(sum(visitors), 0)::bigint,
       COALESCE(sum(views) FILTER (WHERE day = $1::date), 0)::bigint,
       COALESCE(sum(visitors) FILTER (WHERE day = $1::date), 0)::bigint,
       COALESCE(min(day)::text, '')
  FROM (SELECT d.day, d.views, `+visitorsOf+` AS visitors FROM net.visit_days d) days`, s.today()).
		Scan(&out.Total.Views, &out.Total.Visitors, &out.Today.Views, &out.Today.Visitors, &out.Since)
	return out, err
}

// DayCount is one day of a report.
type DayCount struct {
	Day string `json:"day"`
	Count
}

// PageCount is one page of a report.
type PageCount struct {
	Page  string `json:"page"`
	Views int64  `json:"views"`
}

// Report is the last days in full: each day, oldest first and with the days nobody came, and
// the pages seen most over the period.
type Report struct {
	Days  []DayCount  `json:"days"`
	Pages []PageCount `json:"pages"`
}

// Report reads the last days days, today included, and the pages most seen pages of them.
func (s Store) Report(ctx context.Context, days, pages int) (Report, error) {
	out := Report{Days: []DayCount{}, Pages: []PageCount{}}
	today := s.today()
	rows, err := s.DB.Query(ctx, `
SELECT g.day::text, COALESCE(d.views, 0), CASE WHEN d.day IS NULL THEN 0 ELSE `+visitorsOf+` END
  FROM (SELECT $1::date - n AS day FROM generate_series(0, $2::int - 1) AS n) g
  LEFT JOIN net.visit_days d ON d.day = g.day
 ORDER BY g.day`, today, days)
	if err != nil {
		return out, err
	}
	if out.Days, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (DayCount, error) {
		var d DayCount
		return d, row.Scan(&d.Day, &d.Views, &d.Visitors)
	}); err != nil {
		return out, err
	}
	rows, err = s.DB.Query(ctx, `
SELECT page, sum(views)::bigint AS views FROM net.visit_pages
 WHERE day > $1::date - $2::int AND day <= $1::date
 GROUP BY page ORDER BY views DESC, page LIMIT $3`, today, days, pages)
	if err != nil {
		return out, err
	}
	out.Pages, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (PageCount, error) {
		var p PageCount
		return p, row.Scan(&p.Page, &p.Views)
	})
	return out, err
}
