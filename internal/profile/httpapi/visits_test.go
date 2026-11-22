package httpapi

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"ourtaiko.dev/net/internal/visits"
	"ourtaiko.dev/net/internal/web"
)

const safari = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/19.0 Safari/605.1.15"

func TestTheWebsiteCountsItsVisits(t *testing.T) {
	now := time.Date(2026, 11, 16, 4, 0, 0, 0, time.UTC)
	// Every address the test asks from is in one country.
	app := New(testDB(t), Config{Origin: origin, DevUserID: player, Now: func() time.Time { return now },
		Country: func(netip.Addr) string { return "JP" }})
	c := &client{t: t, handler: app.Handler()}
	browser := map[string]string{"User-Agent": safari}
	var summary visits.Summary
	read := func() visits.Summary {
		t.Helper()
		clear(app.visits) // not from memory
		c.json("GET", "/api/visits?tz=Asia/Shanghai", "", 200, &summary)
		return summary
	}

	if s := read(); s != (visits.Summary{}) {
		t.Fatalf("before anyone came %+v", s)
	}
	// The website says each page it shows; nobody needs to be signed in.
	c.json("POST", "/api/visits", `{"path":"/"}`, 204, nil, browser)
	c.json("POST", "/api/visits", `{"path":"/fanmade/charts/0123456789abcdef0123456789abcdef?x=1"}`, 204, nil, browser)
	c.json("POST", "/api/visits", `{"path":"/fanmade/charts/fedcba9876543210fedcba9876543210"}`, 204, nil, map[string]string{"User-Agent": safari + " Edg/141"})
	// Not counted: a program, an address that is none, another site's page.
	c.json("POST", "/api/visits", `{"path":"/"}`, 204, nil, map[string]string{"User-Agent": "Googlebot/2.1"})
	c.json("POST", "/api/visits", `{"path":"/"}`, 204, nil)
	c.json("POST", "/api/visits", `{"path":"charts"}`, 204, nil, browser)
	c.json("POST", "/api/visits", `{"path":"/"}`, 403, nil, map[string]string{"User-Agent": safari, "Origin": "https://elsewhere.example"})
	c.json("POST", "/api/visits", `{"page":"/"}`, 400, nil, browser)
	if s := read(); s.Total != (visits.Count{Views: 3, Visitors: 2}) || s.Today != s.Total || s.Since != "2026-11-16" {
		t.Fatalf("after three views %+v", s)
	}

	// The totals are answered from memory for a while.
	c.json("POST", "/api/visits", `{"path":"/titles"}`, 204, nil, browser)
	c.json("GET", "/api/visits?tz=Asia/Shanghai", "", 200, &summary)
	if summary.Total.Views != 3 {
		t.Fatalf("not from memory %+v", summary)
	}
	// Each time zone is its own: at 04:00 UTC it is still the 15th in Los Angeles, where every
	// view so far was made on the 15th. A time zone that is none is refused.
	c.json("GET", "/api/visits?tz=America/Los_Angeles", "", 200, &summary)
	if summary.Total.Views != 4 || summary.Today != (visits.Count{Views: 4, Visitors: 2}) || summary.Since != "2026-11-15" {
		t.Fatalf("in Los Angeles %+v", summary)
	}
	c.json("GET", "/api/visits?tz=Mars/Olympus", "", 400, nil)

	// One visitor is counted for only so many pages a minute, and it costs them none of
	// their writes.
	for range visitsAMinute + 10 {
		c.json("POST", "/api/visits", `{"path":"/players"}`, 204, nil, browser)
	}
	if s := read(); s.Total.Views != visitsAMinute {
		t.Fatalf("%d views of a visitor in a minute", s.Total.Views)
	}

	// The days and pages are the administrator's to read.
	c.json("GET", "/api/admin/visits", "", 401, nil)
	c.signIn()
	c.json("GET", "/api/admin/visits?days=0", "", 400, nil)
	c.json("GET", "/api/admin/visits?days=367", "", 400, nil)
	c.json("GET", "/api/admin/visits?tz=Local", "", 400, nil)
	var report struct {
		Zone         string
		Total, Today visits.Count
		Since        string
		Days         []visits.DayCount
		Hours        []int64
		Pages        []visits.PageCount
		Countries    []visits.CountryCount
		Sites        []visits.SiteCount
	}
	c.json("GET", "/api/admin/visits?days=7&tz=Asia/Shanghai", "", 200, &report)
	if report.Zone != "Asia/Shanghai" || len(report.Hours) != 24 || report.Hours[12] != visitsAMinute {
		t.Fatalf("zone %q, hours %v", report.Zone, report.Hours)
	}
	if len(report.Days) != 7 || report.Days[0].Day != "2026-11-10" || report.Days[6] != (visits.DayCount{Day: "2026-11-16", Count: visits.Count{Views: visitsAMinute, Visitors: 2}}) {
		t.Fatalf("days %+v", report.Days)
	}
	if fmt.Sprint(report.Pages) != fmt.Sprintf("[{/players %d} {/fanmade/charts/:id 2} {/ 1} {/titles 1}]", visitsAMinute-4) || report.Today.Views != visitsAMinute || report.Since != "2026-11-16" {
		t.Fatalf("report %+v", report)
	}
	// Every view kept where it came from, and the report says the country only.
	if fmt.Sprint(report.Countries) != fmt.Sprintf("[{JP %d 1}]", visitsAMinute) {
		t.Fatalf("countries %+v", report.Countries)
	}
	// Every view was seen at the site's own address: the test's requests came to no other.
	if fmt.Sprint(report.Sites) != fmt.Sprintf("[{127.0.0.1:5175 %d 1}]", visitsAMinute) {
		t.Fatalf("sites %+v", report.Sites)
	}
	c.json("GET", "/api/admin/visits", "", 200, &report)
	if len(report.Days) != 30 || report.Zone != "UTC" || report.Days[29].Day != "2026-11-16" {
		t.Fatalf("%d days by default, in %q", len(report.Days), report.Zone)
	}
}

// A view is counted under the address of the site the request came to (web.Origins).
func TestAVisitKeepsTheSitesAddressItCameTo(t *testing.T) {
	s := &Server{Config: Config{Origin: "https://net.example"}}
	origins, err := web.NewOrigins("https://net.example", []string{"https://cn.example"})
	if err != nil {
		t.Fatal(err)
	}
	for host, want := range map[string]string{"cn.example": "cn.example", "net.example": "net.example", "fanmade.example": "net.example"} {
		var got string
		r := httptest.NewRequest("POST", "/api/visits", nil)
		r.Host = host
		origins.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got = s.visitSite(r) })).ServeHTTP(httptest.NewRecorder(), r)
		if got != want {
			t.Errorf("at %s: %q, want %q", host, got, want)
		}
	}
}
