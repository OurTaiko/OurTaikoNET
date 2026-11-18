package httpapi

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"ourtaiko.dev/net/internal/visits"
	"ourtaiko.dev/net/internal/web"
)

// How much the website is visited (package visits). The website tells the page it shows;
// everyone may read the totals, which the foot of every page shows, and the site's
// administrator the days, hours and pages behind them. Whoever reads names their time zone
// (?tz=Asia/Shanghai, as their browser gives it; UTC without one), and its days are the days
// they are given.

const (
	// How long the totals of a time zone are answered from memory, and of how many zones at
	// most; the memory is emptied when more are asked for.
	visitSummaryKept, visitSummaryZones = 30 * time.Second, 64
	// How many pages one visitor is counted for in a minute; more are dropped without a word.
	visitsAMinute = 60
	// How many pages a report lists.
	visitReportPages = 50
)

// keptVisits is the summary of a time zone as read at at.
type keptVisits struct {
	summary visits.Summary
	at      time.Time
}

func (s *Server) visitStore() visits.Store { return visits.Store{DB: s.DB, Now: s.Config.Now} }

// RunVisitUpkeep deletes the salts that are over (after which what told visitors apart tells
// nobody who they were) now and every hour until ctx ends.
func (s *Server) RunVisitUpkeep(ctx context.Context) {
	s.visitStore().RunClose(ctx, func(err error) { log.Printf("closing the spans of the visit count: %v", err) })
}

// visitAllowed counts a view against its visitor's visitsAMinute.
func (s *Server) visitAllowed(host string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, v := range s.visitLimits {
		if now.Sub(v.since) > time.Minute {
			delete(s.visitLimits, k)
		}
	}
	limit := s.visitLimits[host]
	if limit.since.IsZero() {
		limit.since = now
	}
	limit.count++
	s.visitLimits[host] = limit
	return limit.count <= visitsAMinute
}

// countVisit is POST /api/visits with {"path": "/fanmade/charts/…"}: the website shows that
// page. No session is needed and nothing is answered (204), also when the view is not counted:
// a program's, one too many, or an address that is none of the site's.
func (s *Server) countVisit(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if !answers.DecodeLimit(w, r, &body, 1024) {
		return
	}
	page, ok := visits.Page(body.Path)
	host := web.ClientIP(r, s.Config.TrustedProxies)
	agent := r.UserAgent()
	if ok && !visits.Robot(agent) && s.visitAllowed(host) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		// A count that could not be written is the site's trouble, not the visitor's.
		if err := s.visitStore().Record(ctx, page, host+"\n"+agent); err != nil {
			log.Printf("request=%s counting a visit: %v", w.Header().Get("X-Request-ID"), err)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// visitZone is the time zone the request names (?tz=), or false once it has answered 400.
func visitZone(w http.ResponseWriter, r *http.Request) (*time.Location, bool) {
	loc, err := visits.Zone(r.URL.Query().Get("tz"))
	if err != nil {
		problem(w, 400, "QUERY_INVALID", "tz 需要是时区名，例如 Asia/Shanghai")
		return nil, false
	}
	return loc, true
}

// unknownZone reports whether the database did not know a time zone the program knew.
func unknownZone(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "22023"
}

// visitSummary is GET /api/visits?tz=Asia/Shanghai, public: {"total": {"views",
// "visitors"}, "today": {…}, "since": "2026-11-16"}, today and since being of that time zone.
// Kept for visitSummaryKept and given to everyone who asks for that zone meanwhile.
func (s *Server) visitSummary(w http.ResponseWriter, r *http.Request) {
	loc, ok := visitZone(w, r)
	if !ok {
		return
	}
	s.visitMu.Lock()
	defer s.visitMu.Unlock()
	kept, ok := s.visits[loc.String()]
	if !ok || time.Since(kept.at) > visitSummaryKept {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		summary, err := s.visitStore().Summary(ctx, loc)
		if unknownZone(err) {
			problem(w, 400, "QUERY_INVALID", "tz 需要是时区名，例如 Asia/Shanghai")
			return
		}
		if err != nil {
			internal(w, err)
			return
		}
		if len(s.visits) >= visitSummaryZones {
			clear(s.visits)
		}
		kept = keptVisits{summary, time.Now()}
		s.visits[loc.String()] = kept
	}
	web.Respond(w, 200, kept.summary)
}

// visitReport is GET /api/admin/visits?days=30&tz=Asia/Shanghai (days 1 to visits.MaxDays),
// for the site's administrator: the summary, each of the last days of that time zone (oldest
// first, today last), the views in each of its hours over them, and the pages seen most.
func (s *Server) visitReport(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.siteAdmin(w, r); !ok {
		return
	}
	days := 30
	if text := r.URL.Query().Get("days"); text != "" {
		n, err := strconv.Atoi(text)
		if err != nil || n < 1 || n > visits.MaxDays {
			problem(w, 400, "QUERY_INVALID", "days 需要是 1 到 366 的整数")
			return
		}
		days = n
	}
	loc, ok := visitZone(w, r)
	if !ok {
		return
	}
	store := s.visitStore()
	summary, err := store.Summary(r.Context(), loc)
	if err == nil {
		var report visits.Report
		if report, err = store.Report(r.Context(), loc, days, visitReportPages); err == nil {
			web.Respond(w, 200, map[string]any{"zone": loc.String(), "total": summary.Total, "today": summary.Today, "since": summary.Since, "days": report.Days, "hours": report.Hours, "pages": report.Pages})
			return
		}
	}
	if unknownZone(err) {
		problem(w, 400, "QUERY_INVALID", "tz 需要是时区名，例如 Asia/Shanghai")
		return
	}
	internal(w, err)
}
