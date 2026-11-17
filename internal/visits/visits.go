// Package visits counts how much the website is visited: each page shown, with the moment it
// was shown, and how many different visitors saw them (net.visit_views and visit_salts,
// migration 0008). Whoever reads the count sees days and hours of their own time zone.
//
// The website says which page it shows; nothing is counted from the requests themselves, so
// programs that read the API or fetch a page without running it are not visitors. No cookie
// is set and no address is kept. A visitor is told from another by a hash of who they are
// (their address and browser) with a salt that lives for two UTC days; when they are over, the
// salt is deleted, after which nobody can tell who visited. Since a day of any time zone lies
// within one such span, the visitors of a day are told apart by that span's hashes (the
// migration explains how).
//
// Before 0008, views were counted by days of UTC+8 without their times (net.visit_days,
// visit_pages); those count in the totals only.
package visits

import (
	"context"
	"crypto/rand"
	"errors"
	"regexp"
	"strings"
	"time"
	// Time zones are named by the visitor's browser; the program knows them all, wherever it runs.
	_ "time/tzdata"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// The longest address a page is counted under, and how many of its parts are kept.
	maxPath, maxParts = 200, 6
	// MaxDays is the longest period a report covers.
	MaxDays = 366
	// The longest name of a time zone taken (the longest there is has 32 letters).
	maxZone = 64
)

var (
	// A part of an address that names a kind of page (charts, playlists, me).
	word = regexp.MustCompile(`^[a-z][a-z0-9-]{0,23}$`)
	// A browser that says it is a program, not a person.
	robot = regexp.MustCompile(`(?i)bot|crawl|spider|slurp|headless|lighthouse|preview|monitor|curl|wget|python|scrapy|http-?client`)
	// The name of a time zone as browsers give it (Asia/Shanghai, UTC, Etc/GMT+8).
	zoneName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_+\-]*(/[A-Za-z0-9_+\-]+)*$`)
)

// ErrZone is a time zone that is not one.
var ErrZone = errors.New("unknown time zone")

// Zone is the time zone a reader named (an IANA name such as Asia/Shanghai); the empty name is
// UTC. Local, and names that are not a zone's, are refused with ErrZone.
func Zone(name string) (*time.Location, error) {
	if name == "" {
		return time.UTC, nil
	}
	if len(name) > maxZone || !zoneName.MatchString(name) || name == "Local" {
		return nil, ErrZone
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, ErrZone
	}
	return loc, nil
}

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

func (s Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// utcDay is the UTC day of t, counted in days from 1970-01-01.
func utcDay(t time.Time) int64 {
	u := t.Unix()
	if u < 0 {
		return (u+1)/86400 - 1
	}
	return u / 86400
}

func dayText(n int64) string {
	return time.Unix(n*86400, 0).UTC().Format(time.DateOnly)
}

// Record counts one view of page (as Page gives it) by who, now. who is what tells the visitor
// from others (address and browser); it is only ever stored hashed with the salts of the two
// spans the moment lies in.
func (s Store) Record(ctx context.Context, page, who string) error {
	now := s.now()
	day := utcDay(now)
	// The two spans that hold today: the one that began yesterday and the one beginning today.
	even, odd := day, day-1
	if day%2 != 0 {
		even, odd = day-1, day
	}
	salts := make([]byte, 32)
	if _, err := rand.Read(salts); err != nil {
		return err
	}
	// A salt given is kept only by the first view of its span; every view hashes with the one
	// the span has.
	_, err := s.DB.Exec(ctx, `
WITH salts AS (
    INSERT INTO net.visit_salts AS s (span, salt) VALUES ($1::date, $3::bytea), ($2::date, $4::bytea)
    ON CONFLICT (span) DO UPDATE SET salt = s.salt
    RETURNING span, salt
)
INSERT INTO net.visit_views (at, page, visitor_even, visitor_odd)
SELECT $5, $6,
       sha256((SELECT salt FROM salts WHERE span = $1::date) || convert_to($7, 'UTF8')),
       sha256((SELECT salt FROM salts WHERE span = $2::date) || convert_to($7, 'UTF8'))`,
		dayText(even), dayText(odd), salts[:16], salts[16:], now, page, who)
	return err
}

// Close deletes the salts of the spans that are over, after which the hashes they made tell
// nobody who visited. It also finishes what was counted before views had their times: a day
// still open there is counted and its hashes deleted. It reports how many salts it deleted.
func (s Store) Close(ctx context.Context) (int, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	// A span that began two days ago or earlier is over.
	tag, err := tx.Exec(ctx, `DELETE FROM net.visit_salts WHERE span <= $1::date`, dayText(utcDay(s.now())-2))
	if err != nil {
		return 0, err
	}
	if _, err = tx.Exec(ctx, `
UPDATE net.visit_days d SET salt = NULL,
       visitors = (SELECT count(*) FROM net.visit_visitors v WHERE v.day = d.day)
 WHERE d.visitors IS NULL`); err != nil {
		return 0, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM net.visit_visitors`); err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), tx.Commit(ctx)
}

// RunClose closes what is over now and every hour until ctx ends.
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

// Summary is what everyone may know, in the reader's time zone: all there has been, today's,
// and the first day counted (empty when nothing has been).
type Summary struct {
	Total Count  `json:"total"`
	Today Count  `json:"today"`
	Since string `json:"since"`
}

// localViews is each view (v) with the day of the time zone $1 it was made on (day), and the
// hash that tells its visitor apart within that day (visitor): the one of the span beginning
// on the UTC day the local day begins on.
const localViews = `
SELECT v.at, l.day, CASE WHEN ((l.start AT TIME ZONE 'UTC')::date - date '1970-01-01') % 2 = 0
                        THEN v.visitor_even ELSE v.visitor_odd END AS visitor
  FROM net.visit_views v
 CROSS JOIN LATERAL (SELECT (v.at AT TIME ZONE $1)::date AS day) d
 CROSS JOIN LATERAL (SELECT d.day, d.day::timestamp AT TIME ZONE $1 AS start) l`

// Summary reads the summary in loc.
func (s Store) Summary(ctx context.Context, loc *time.Location) (Summary, error) {
	var out Summary
	today := s.now().In(loc).Format(time.DateOnly)
	err := s.DB.QueryRow(ctx, `
WITH days AS (
    SELECT day, count(*) AS views, count(DISTINCT visitor) AS visitors
      FROM (`+localViews+`) l
     GROUP BY day
), before AS (
    SELECT COALESCE(sum(views), 0) AS views, COALESCE(sum(visitors), 0) AS visitors, min(day) AS day
      FROM net.visit_days
)
SELECT ((SELECT COALESCE(sum(views), 0) FROM days) + before.views)::bigint,
       ((SELECT COALESCE(sum(visitors), 0) FROM days) + before.visitors)::bigint,
       COALESCE((SELECT views FROM days WHERE day = $2::date), 0)::bigint,
       COALESCE((SELECT visitors FROM days WHERE day = $2::date), 0)::bigint,
       COALESCE(least(before.day, (SELECT min(day) FROM days))::text, '')
  FROM before`, loc.String(), today).
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

// Report is the last days in full, in the reader's time zone: each day, oldest first and with
// the days nobody came; the views made in each hour of the day (0 to 23) over those days; and
// the pages seen most.
type Report struct {
	Days  []DayCount  `json:"days"`
	Hours []int64     `json:"hours"`
	Pages []PageCount `json:"pages"`
}

// Report reads the last days days of loc, today included, and the pages most seen pages of them.
func (s Store) Report(ctx context.Context, loc *time.Location, days, pages int) (Report, error) {
	out := Report{Days: []DayCount{}, Hours: make([]int64, 24), Pages: []PageCount{}}
	now := s.now().In(loc)
	last := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	first := last.AddDate(0, 0, 1-days)
	start, stop := first, last.AddDate(0, 0, 1)
	zone := loc.String()

	rows, err := s.DB.Query(ctx, `
WITH counted AS (
    SELECT day, count(*) AS views, count(DISTINCT visitor) AS visitors
      FROM (`+localViews+`
             WHERE v.at >= $3 AND v.at < $4) l
     GROUP BY day
)
SELECT g.day::text, COALESCE(c.views, 0)::bigint, COALESCE(c.visitors, 0)::bigint
  FROM (SELECT $2::date + n AS day FROM generate_series(0, $5::int - 1) AS n) g
  LEFT JOIN counted c ON c.day = g.day
 ORDER BY g.day`, zone, first.Format(time.DateOnly), start, stop, days)
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
SELECT extract(hour FROM at AT TIME ZONE $1)::int, count(*)
  FROM net.visit_views WHERE at >= $2 AND at < $3
 GROUP BY 1`, zone, start, stop)
	if err != nil {
		return out, err
	}
	var hour int
	var views int64
	if _, err = pgx.ForEachRow(rows, []any{&hour, &views}, func() error {
		if hour >= 0 && hour < 24 {
			out.Hours[hour] += views
		}
		return nil
	}); err != nil {
		return out, err
	}

	rows, err = s.DB.Query(ctx, `
SELECT page, count(*) AS views FROM net.visit_views
 WHERE at >= $1 AND at < $2
 GROUP BY page ORDER BY views DESC, page LIMIT $3`, start, stop, pages)
	if err != nil {
		return out, err
	}
	out.Pages, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (PageCount, error) {
		var p PageCount
		return p, row.Scan(&p.Page, &p.Views)
	})
	return out, err
}
