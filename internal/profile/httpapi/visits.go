package httpapi

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"time"

	"ourtaiko.dev/net/internal/visits"
	"ourtaiko.dev/net/internal/web"
)

// How much the website is visited (package visits). The website tells the page it shows;
// everyone may read the totals, which the foot of every page shows, and the site's
// administrator the days and pages behind them.

const (
	// How long the totals are answered from memory.
	visitSummaryKept = 30 * time.Second
	// How many pages one visitor is counted for in a minute; more are dropped without a word.
	visitsAMinute = 60
	// How many pages a report lists.
	visitReportPages = 50
)

func (s *Server) visitStore() visits.Store { return visits.Store{DB: s.DB, Now: s.Config.Now} }

// RunVisitUpkeep closes the days that are over (their visitors are counted and what told
// them apart is deleted) now and every hour until ctx ends.
func (s *Server) RunVisitUpkeep(ctx context.Context) {
	s.visitStore().RunClose(ctx, func(err error) { log.Printf("closing the days of the visit count: %v", err) })
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

// countVisit is POST /api/v1/visits with {"path": "/fanmade/charts/…"}: the website shows that
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

// visitSummary is GET /api/v1/visits, public: {"total": {"views", "visitors"}, "today": {…},
// "since": "2026-11-16"}. Kept for visitSummaryKept and given to everyone who asks meanwhile.
func (s *Server) visitSummary(w http.ResponseWriter, r *http.Request) {
	s.visitMu.Lock()
	defer s.visitMu.Unlock()
	if s.visitAt.IsZero() || time.Since(s.visitAt) > visitSummaryKept {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		summary, err := s.visitStore().Summary(ctx)
		if err != nil {
			internal(w, err)
			return
		}
		s.visit, s.visitAt = summary, time.Now()
	}
	web.Respond(w, 200, s.visit)
}

// visitReport is GET /api/v1/admin/visits?days=30 (1 to visits.MaxDays), for the site's
// administrator: the summary, each of the last days (oldest first, today last) and the pages
// seen most in them.
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
	store := s.visitStore()
	summary, err := store.Summary(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	report, err := store.Report(r.Context(), days, visitReportPages)
	if err != nil {
		internal(w, err)
		return
	}
	web.Respond(w, 200, map[string]any{"total": summary.Total, "today": summary.Today, "since": summary.Since, "days": report.Days, "pages": report.Pages})
}
