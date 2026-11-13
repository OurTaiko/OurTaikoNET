package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"ourtaiko.dev/net/internal/account"
	"ourtaiko.dev/net/internal/web"
)

// Source is a song source whose plays are shown. ID is its name in ourtaiko_net (the schema
// its charts are in, and the source of its rows in net.scores); Site is where its chart pages
// are, for links.
type Source struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Site string `json:"site"`
}

// SourceScore is one row of a player's list: a play (or, in the best view, the best play of a
// chart block), the song's title, the block's level, and the source it was played on.
type SourceScore struct {
	ID                string          `json:"id"`
	SongID            json.RawMessage `json:"songId"`
	DifficultyID      int64           `json:"difficultyId"`
	Course            string          `json:"course"`
	Side              string          `json:"side"`
	Good              int64           `json:"good"`
	OK                int64           `json:"ok"`
	Bad               int64           `json:"bad"`
	Score             int64           `json:"score"`
	Drumroll          int64           `json:"drumroll"`
	MaxCombo          int64           `json:"max_combo"`
	ClearStatus       int             `json:"ClearStatus"`
	IsLatest          bool            `json:"isLatest"`
	SubmittedAt       time.Time       `json:"submittedAt"`
	Title             string          `json:"title"`
	TitleTranslations json.RawMessage `json:"titleTranslations"`
	Level             int             `json:"level"`
	BestClearStatus   *int            `json:"bestClearStatus,omitempty"`
	PlayCount         *int            `json:"playCount,omitempty"`
	Source            string          `json:"source"`
	ChartURL          string          `json:"chartUrl"`
	// The service kept the play's recording; ReplayURL opens it on the chart's page there. A
	// recording of files replaced since does not fit the chart any more and has no address.
	HasReplay bool   `json:"hasReplay"`
	ReplayURL string `json:"replayUrl,omitempty"`
}

const scorePageSize = 50

type sourceList struct {
	rows []SourceScore
	err  error
}

// plays reads a player's whole list on one source. view=all is every play on a published
// chart; view=best is one row per chart block: the highest score among the plays of the
// song's current files, or of replaced files while the block has none of the current ones,
// with the best clear status of those plays and their count. These are the rules of each
// source's own list (its GET /api/v1/users/{id}/scores).
func (s *Server) plays(ctx context.Context, source Source, user, view, course string) *sourceList {
	list := &sourceList{rows: []SourceScore{}}
	schema := pgx.Identifier{source.ID}.Sanitize()
	args := []any{user, source.ID}
	plays := `SELECT sc.id,d.chart_id,sc.difficulty_id,d.course,d.side,sc.good,sc.ok,sc.bad,sc.score,sc.drumroll,sc.max_combo,sc.clear_status,sc.is_latest,sc.submitted_at,sc.replay_data IS NOT NULL AS has_replay,
 c.title,c.title_translations,d.level
 FROM net.scores sc JOIN ` + schema + `.chart_difficulties d ON d.id=sc.difficulty_id JOIN ` + schema + `.charts c ON c.id=d.chart_id
 WHERE sc.source=$2 AND sc.user_id=$1 AND c.status='published'`
	if course != "" {
		args = append(args, course)
		plays += ` AND d.course=$3`
	}
	columns := `id,chart_id,difficulty_id,course,side,good,ok,bad,score,drumroll,max_combo,clear_status,is_latest,submitted_at,has_replay,title,title_translations,level`
	var sql string
	if view == "all" {
		sql = `SELECT ` + columns + `,NULL::int,NULL::int FROM (` + plays + `) p`
	} else {
		sql = `WITH plays AS (SELECT p.*,bool_or(p.is_latest) OVER (PARTITION BY p.difficulty_id) AS has_latest FROM (` + plays + `) p),
 considered AS (SELECT * FROM plays WHERE is_latest OR NOT has_latest),
 best AS (SELECT DISTINCT ON (difficulty_id) *,max(clear_status) OVER (PARTITION BY difficulty_id) AS best_clear,count(*) OVER (PARTITION BY difficulty_id) AS play_count
  FROM considered ORDER BY difficulty_id,score DESC,submitted_at,id)
 SELECT ` + columns + `,best_clear::int,play_count::int FROM best`
	}
	rows, err := s.DB.Query(ctx, sql, args...)
	if err != nil {
		list.err = errors.New("unreadable")
		log.Printf("scores of %s on %s: %v", user, source.ID, err)
		return list
	}
	defer rows.Close()
	for rows.Next() {
		var row SourceScore
		var chart string
		var translations []byte
		if err = rows.Scan(&row.ID, &chart, &row.DifficultyID, &row.Course, &row.Side, &row.Good, &row.OK, &row.Bad, &row.Score, &row.Drumroll, &row.MaxCombo, &row.ClearStatus, &row.IsLatest, &row.SubmittedAt, &row.HasReplay,
			&row.Title, &translations, &row.Level, &row.BestClearStatus, &row.PlayCount); err != nil {
			break
		}
		row.SongID = json.RawMessage(strconv.Quote(chart))
		row.TitleTranslations = translations
		row.Source = source.ID
		row.ChartURL = source.Site + "/charts/" + chart
		if row.HasReplay && row.IsLatest {
			row.ReplayURL = row.ChartURL + "?replay=" + url.QueryEscape(row.ID)
		}
		list.rows = append(list.rows, row)
	}
	if err == nil {
		err = rows.Err()
	}
	if err != nil {
		log.Printf("scores of %s on %s: %v", user, source.ID, err)
		return &sourceList{rows: []SourceScore{}, err: errors.New("unreadable")}
	}
	return list
}

var scoreOrders = map[string]func(a, b *SourceScore) bool{
	"recent": func(a, b *SourceScore) bool { return a.SubmittedAt.After(b.SubmittedAt) },
	"score": func(a, b *SourceScore) bool {
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		return a.SubmittedAt.After(b.SubmittedAt)
	},
	"level": func(a, b *SourceScore) bool {
		if a.Level != b.Level {
			return a.Level > b.Level
		}
		return a.Score > b.Score
	},
}

var scoreCourses = map[string]bool{"Easy": true, "Normal": true, "Hard": true, "Oni": true, "Edit": true}

// userScores is GET /api/v1/users/{id}/scores: the player's rows of every source (or one,
// source=<id>) in one list, public like the sources' own. view=best (default) or all, sort
// level/score/recent (default level for best, recent for all), course, 50 rows a page. A
// source that cannot be read is reported in sources[].error while the others still show.
func (s *Server) userScores(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !account.ValidSubject(id) {
		problem(w, 404, "USER_NOT_FOUND", "用户不存在")
		return
	}
	query := r.URL.Query()
	view := query.Get("view")
	if view == "" {
		view = "best"
	}
	order := query.Get("sort")
	if order == "" {
		order = map[string]string{"best": "level", "all": "recent"}[view]
	}
	less, sorted := scoreOrders[order]
	course := query.Get("course")
	source := query.Get("source")
	if source == "" {
		source = "all"
	}
	page := 1
	if raw := query.Get("page"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 10000 {
			problem(w, 400, "QUERY_INVALID", "页码无效")
			return
		}
		page = n
	}
	selected := []Source{}
	for _, src := range s.Config.Sources {
		if source == "all" || source == src.ID {
			selected = append(selected, src)
		}
	}
	if (view != "best" && view != "all") || !sorted || (course != "" && !scoreCourses[course]) || len(selected) == 0 {
		problem(w, 400, "QUERY_INVALID", "查询参数无效")
		return
	}
	lists := make([]*sourceList, len(selected))
	var wait sync.WaitGroup
	for i, src := range selected {
		wait.Add(1)
		go func() {
			defer wait.Done()
			lists[i] = s.plays(r.Context(), src, id, view, course)
		}()
	}
	wait.Wait()
	rows := []SourceScore{}
	type status struct {
		Source
		Count     int    `json:"count"`
		Truncated bool   `json:"truncated"`
		Error     string `json:"error,omitempty"`
	}
	statuses := []status{}
	for i, list := range lists {
		// Truncated is from when a source's list was fetched and capped; it is always false now.
		st := status{Source: selected[i], Count: len(list.rows)}
		if list.err != nil {
			st.Error = list.err.Error()
		}
		statuses = append(statuses, st)
		rows = append(rows, list.rows...)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := &rows[i], &rows[j]
		if less(a, b) {
			return true
		}
		if less(b, a) {
			return false
		}
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		return a.ID < b.ID
	})
	total := len(rows)
	start := min((page-1)*scorePageSize, total)
	end := min(start+scorePageSize, total)
	web.Respond(w, 200, map[string]any{"items": rows[start:end], "total": total, "page": page, "pageSize": scorePageSize, "view": view, "sort": order, "sources": statuses})
}

func (s *Server) sources(w http.ResponseWriter, r *http.Request) {
	web.Respond(w, 200, map[string]any{"sources": s.Config.Sources})
}
