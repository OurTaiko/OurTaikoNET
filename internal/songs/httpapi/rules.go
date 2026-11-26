package httpapi

import (
	"context"
	"log"

	"github.com/jackc/pgx/v5"
	"ourtaiko.dev/net/internal/rewards/rules"
)

// What the source's players do and what happens to its charts can meet the rules that give
// items by themselves (package rules). The answers never say what was given: a client learns of
// its rewards as of any others (docs/TITLES_PLAN.md, 1.1). A failure to judge is only logged and
// fails nothing: the rules are judged again with the player's next play of the chart, or the
// next change of the source's charts.

// rulesAfterScore judges, inside the score's transaction, the rules a play that cleared its chart
// can have met.
func (s *Server) rulesAfterScore(ctx context.Context, tx pgx.Tx, user string, result Score) {
	if result.ClearStatus < 1 {
		return
	}
	inner, err := tx.Begin(ctx)
	if err != nil {
		log.Printf("rules after a score: %v", err)
		return
	}
	defer inner.Rollback(ctx)
	if _, err = rules.Played(ctx, inner, s.source().schema(), user, result.SongID); err == nil {
		err = inner.Commit(ctx)
	}
	if err != nil {
		log.Printf("rules after a score: %v", err)
	}
}

// rulesAfterDanAttempt judges, inside the attempt's transaction, the rules of the course a new
// attempt that passed it can have met.
func (s *Server) rulesAfterDanAttempt(ctx context.Context, tx pgx.Tx, user string, a DanAttempt) {
	if a.Verdict < 1 {
		return
	}
	inner, err := tx.Begin(ctx)
	if err != nil {
		log.Printf("rules after a dan attempt: %v", err)
		return
	}
	defer inner.Rollback(ctx)
	if _, err = rules.DanAttempted(ctx, inner, s.source().schema(), user, a.CourseID); err == nil {
		err = inner.Commit(ctx)
	}
	if err != nil {
		log.Printf("rules after a dan attempt: %v", err)
	}
}

// rulesAfterChartChange judges, once it is saved, the rules that go by the source's charts as a
// whole (rules.ChartsChanged): after a chart is published or uploaded again, deleted, put in
// other categories, or joined by a collaborator.
func (s *Server) rulesAfterChartChange(ctx context.Context) {
	if _, err := rules.ChartsChanged(context.WithoutCancel(ctx), s.DB, s.source().schema()); err != nil {
		log.Printf("rules after a change of charts: %v", err)
	}
}
