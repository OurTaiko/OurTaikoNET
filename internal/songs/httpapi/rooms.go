package httpapi

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// What the game's online rooms (Multi-Enso, internal/multiplay) ask of a source. They run in
// this program, beside the HTTP API, and ask without HTTP.

// gameChart is a chart the game catalog lists: playable and in at least one category.
const gameChart = playableChart + ` AND c.category_flags<>0`

// GameIdentity is the player a game session (the token the game sends to /game/ routes)
// belongs to: the account center's answer, or, on a server without an account center, the
// development user for the development token. pgx.ErrNoRows: the session is refused. Any
// other error: the account center could not be asked.
func (s *Server) GameIdentity(ctx context.Context, token string) (User, error) {
	if len(token) != 64 {
		return User{}, pgx.ErrNoRows
	}
	if s.Config.SSO == nil {
		if s.Config.DevUserID == "" || token != s.devGameToken() {
			return User{}, pgx.ErrNoRows
		}
		return devUser(s.Config.DevUserID), nil
	}
	return s.Config.SSO.Identity(ctx, "game", token)
}

// chartThat reads the one chart the condition names, with its owners' names.
func (s *Server) chartThat(ctx context.Context, condition string, args ...any) (Chart, error) {
	c, e := s.readChart(s.DB.QueryRow(ctx, chartSelect+` WHERE `+condition, args...))
	if e == nil {
		items := []Chart{c}
		s.chartNames(ctx, items)
		c = items[0]
	}
	return c, e
}

// PlayableChart is the chart with that ID when the game can play it: published, with files.
// It is the chart as GET /charts/{id} describes it, without a viewer's vote and without the
// difficulty constants. pgx.ErrNoRows: there is none.
func (s *Server) PlayableChart(ctx context.Context, id string) (Chart, error) {
	if !validID(id) {
		return Chart{}, pgx.ErrNoRows
	}
	return s.chartThat(ctx, `c.id=$1 AND `+playableChart, id)
}

// GameChartCount is how many charts the game catalog lists (GET /game/catalog).
func (s *Server) GameChartCount(ctx context.Context) (int, error) {
	var n int
	e := s.DB.QueryRow(ctx, `SELECT count(*) FROM charts c WHERE `+gameChart).Scan(&n)
	return n, e
}

// GameChartAt is the n-th (from 0) of the charts the game catalog lists, in the order of
// their IDs: with a random n below GameChartCount, any of them with the same chance.
// pgx.ErrNoRows: there are not that many.
func (s *Server) GameChartAt(ctx context.Context, n int) (Chart, error) {
	if n < 0 {
		return Chart{}, pgx.ErrNoRows
	}
	return s.chartThat(ctx, `c.id=(SELECT c.id FROM charts c WHERE `+gameChart+` ORDER BY c.id LIMIT 1 OFFSET $1)`, n)
}
