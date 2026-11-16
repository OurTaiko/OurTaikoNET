package httpapi

import (
	"fmt"
	"testing"
	"time"

	"ourtaiko.dev/net/internal/visits"
)

const safari = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/19.0 Safari/605.1.15"

func TestTheWebsiteCountsItsVisits(t *testing.T) {
	now := time.Date(2026, 11, 16, 4, 0, 0, 0, time.UTC)
	app := New(testDB(t), Config{Origin: origin, DevUserID: player, Now: func() time.Time { return now }})
	c := &client{t: t, handler: app.Handler()}
	browser := map[string]string{"User-Agent": safari}
	var summary visits.Summary
	read := func() visits.Summary {
		t.Helper()
		app.visitAt = time.Time{} // not from memory
		c.json("GET", "/api/v1/visits", "", 200, &summary)
		return summary
	}

	if s := read(); s != (visits.Summary{}) {
		t.Fatalf("before anyone came %+v", s)
	}
	// The website says each page it shows; nobody needs to be signed in.
	c.json("POST", "/api/v1/visits", `{"path":"/"}`, 204, nil, browser)
	c.json("POST", "/api/v1/visits", `{"path":"/fanmade/charts/0123456789abcdef0123456789abcdef?x=1"}`, 204, nil, browser)
	c.json("POST", "/api/v1/visits", `{"path":"/fanmade/charts/fedcba9876543210fedcba9876543210"}`, 204, nil, map[string]string{"User-Agent": safari + " Edg/141"})
	// Not counted: a program, an address that is none, another site's page.
	c.json("POST", "/api/v1/visits", `{"path":"/"}`, 204, nil, map[string]string{"User-Agent": "Googlebot/2.1"})
	c.json("POST", "/api/v1/visits", `{"path":"/"}`, 204, nil)
	c.json("POST", "/api/v1/visits", `{"path":"charts"}`, 204, nil, browser)
	c.json("POST", "/api/v1/visits", `{"path":"/"}`, 403, nil, map[string]string{"User-Agent": safari, "Origin": "https://elsewhere.example"})
	c.json("POST", "/api/v1/visits", `{"page":"/"}`, 400, nil, browser)
	if s := read(); s.Total != (visits.Count{Views: 3, Visitors: 2}) || s.Today != s.Total || s.Since != "2026-11-16" {
		t.Fatalf("after three views %+v", s)
	}

	// The totals are answered from memory for a while.
	c.json("POST", "/api/v1/visits", `{"path":"/titles"}`, 204, nil, browser)
	c.json("GET", "/api/v1/visits", "", 200, &summary)
	if summary.Total.Views != 3 {
		t.Fatalf("not from memory %+v", summary)
	}

	// One visitor is counted for only so many pages a minute, and it costs them none of
	// their writes.
	for range visitsAMinute + 10 {
		c.json("POST", "/api/v1/visits", `{"path":"/players"}`, 204, nil, browser)
	}
	if s := read(); s.Total.Views != visitsAMinute {
		t.Fatalf("%d views of a visitor in a minute", s.Total.Views)
	}

	// The days and pages are the administrator's to read.
	c.json("GET", "/api/v1/admin/visits", "", 401, nil)
	c.signIn()
	c.json("GET", "/api/v1/admin/visits?days=0", "", 400, nil)
	c.json("GET", "/api/v1/admin/visits?days=367", "", 400, nil)
	var report struct {
		Total, Today visits.Count
		Since        string
		Days         []visits.DayCount
		Pages        []visits.PageCount
	}
	c.json("GET", "/api/v1/admin/visits?days=7", "", 200, &report)
	if len(report.Days) != 7 || report.Days[0].Day != "2026-11-10" || report.Days[6] != (visits.DayCount{Day: "2026-11-16", Count: visits.Count{Views: visitsAMinute, Visitors: 2}}) {
		t.Fatalf("days %+v", report.Days)
	}
	if fmt.Sprint(report.Pages) != fmt.Sprintf("[{/players %d} {/fanmade/charts/:id 2} {/ 1} {/titles 1}]", visitsAMinute-4) || report.Today.Views != visitsAMinute || report.Since != "2026-11-16" {
		t.Fatalf("report %+v", report)
	}
	c.json("GET", "/api/v1/admin/visits", "", 200, &report)
	if len(report.Days) != 30 {
		t.Fatalf("%d days by default", len(report.Days))
	}
}
