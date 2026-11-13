package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

// playableChart is a published chart with files: what the game can play.
const chartScoreSQL = `COALESCE((SELECT upvotes-downvotes FROM chart_stats WHERE chart_id=c.id),0)`

// gameSearch is the game's song picker: keyword, course and star level that
// hold for the same block (a course covers all its sides), optionally putting
// songs the player has not full-combo'd (unfc) or all-perfected (unperfect)
// yet first. A block's scores of replaced files count only while it has no
// current one. It returns every match, without pagination.
func (s *Server) gameSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	q := strings.TrimSpace(query.Get("q"))
	if len(q) > 200 {
		problem(w, 400, "QUERY_INVALID", "搜索内容过长")
		return
	}
	course := query.Get("course")
	switch course {
	case "", "Easy", "Normal", "Hard", "Oni", "Edit":
	default:
		problem(w, 400, "DIFFICULTY_INVALID", "难度必须为 Easy / Normal / Hard / Oni / Edit")
		return
	}
	level := -1
	if raw := query.Get("level"); raw != "" {
		var e error
		if level, e = strconv.Atoi(raw); e != nil || level < 1 || level > 10 {
			problem(w, 400, "QUERY_INVALID", "星数必须在 1 到 10 之间")
			return
		}
	}
	order := query.Get("order")
	switch order {
	case "", "default", "unfc", "unperfect":
	default:
		problem(w, 400, "QUERY_INVALID", "顺序必须为 default / unfc / unperfect")
		return
	}
	// Only a game session personalizes the order; guests keep the default.
	userID := ""
	if order == "unfc" || order == "unperfect" {
		if u, e := s.current(r); e == nil {
			userID = u.User.ID
		} else if !errors.Is(e, pgx.ErrNoRows) {
			internal(w, e)
			return
		}
	}
	const matching = `SELECT 1 FROM chart_difficulties d WHERE d.chart_id=c.id AND ($3='' OR d.course=$3) AND ($4::int=-1 OR d.level=$4)`
	rows, e := s.DB.Query(r.Context(), chartSelect+` WHERE `+playableChart+` AND c.category_flags<>0 AND `+keywordClause("$1", "$2")+` AND EXISTS(`+matching+`)
	 ORDER BY ($5::text<>'' AND EXISTS(`+matching+`
	 AND NOT EXISTS(SELECT 1 FROM scores sc WHERE sc.user_id=$5 AND sc.difficulty_id=d.id
	 AND (sc.is_latest OR NOT EXISTS(SELECT 1 FROM scores cur WHERE cur.user_id=$5 AND cur.difficulty_id=d.id AND cur.is_latest))
	 AND sc.bad=0 AND (sc.good>0 OR sc.ok>0) AND ($6::text='unfc' OR sc.ok=0)))) DESC, c.created_at DESC,c.id DESC`,
		q, keywordNumber(q), course, level, userID, order)
	if e != nil {
		internal(w, e)
		return
	}
	items, e := readCharts(rows)
	if e != nil {
		internal(w, e)
		return
	}
	s.chartNames(r.Context(), items)
	respond(w, 200, map[string]any{"items": items, "total": len(items)})
}
