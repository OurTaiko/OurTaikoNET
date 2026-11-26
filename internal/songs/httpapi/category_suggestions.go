package httpapi

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"ourtaiko.dev/net/internal/files"
	"ourtaiko.dev/net/internal/web"
)

// Category suggestions (Source.CategorySuggestions; migration 0027_fanmade_categories): a chart's
// owners and the source's managers set its categories (PATCH /charts/{id}); anyone else signed
// in suggests one, and a manager of the source accepts it, which puts the chart in the category,
// or rejects it, which is remembered. Either decision answers every suggestion of that category
// for that chart: those waiting, and those made later, which are filed already decided.

// The states of a suggestion.
const (
	suggestionPending  = "pending"
	suggestionAccepted = "accepted"
	suggestionRejected = "rejected"
)

// varietyFlag is the category a chart is in when it has nowhere else to be (categorySelection).
// Accepting a category for a chart that is in Variety alone takes it out of Variety.
const varietyFlag CategoryFlags = 16

// MySuggestion is one of the viewer's suggestions on a chart.
type MySuggestion struct {
	Category string `json:"category"`
	Status   string `json:"status"`
}

// suggestedCategory reads a category ID of this source as its bit, answering the request itself
// when it is none.
func (s *Server) suggestedCategory(w http.ResponseWriter, id string) (CategoryFlags, bool) {
	flag, ok := s.source().categoryFlag(id)
	if !ok {
		problem(w, 422, "CATEGORY_NOT_FOUND", "分类不存在")
	}
	return flag, ok
}

// categoryReviewer answers a request for a manager's work with who it is from, or answers it
// itself for anyone else.
func (s *Server) categoryReviewer(w http.ResponseWriter, r *http.Request, csrf bool) (session, bool) {
	v, ok := s.required(w, r, csrf)
	if ok && !v.User.IsAdmin {
		problem(w, 403, "FORBIDDEN", "只有曲库管理员可以处理分类建议")
		return v, false
	}
	return v, ok
}

// chartSuggestions answers GET /charts/{id}/category-suggestions: for a signed-in viewer, whether
// they set the chart's categories themselves (an owner or a manager) or decide suggestions (a
// manager), and what they have suggested for it.
func (s *Server) chartSuggestions(w http.ResponseWriter, r *http.Request) {
	v, ok := s.required(w, r, false)
	if !ok {
		return
	}
	ctx := r.Context()
	subject, ok := chartSubject(w, r, s.DB, "")
	if !ok {
		return
	}
	rows, e := s.DB.Query(ctx, `SELECT category_flag,status FROM category_suggestions WHERE chart_id=$1 AND user_id=$2 ORDER BY category_flag`, subject.chartID, v.User.ID)
	if e != nil {
		internal(w, e)
		return
	}
	mine := []MySuggestion{}
	for rows.Next() {
		var flag CategoryFlags
		var m MySuggestion
		if e = rows.Scan(&flag, &m.Status); e != nil {
			break
		}
		if m.Category = s.source().categoryByFlag(flag).ID; m.Category != "" {
			mine = append(mine, m)
		}
	}
	rows.Close()
	if e == nil {
		e = rows.Err()
	}
	if e != nil {
		internal(w, e)
		return
	}
	web.Respond(w, 200, map[string]any{
		"canEdit":     v.User.IsAdmin || slices.Contains(subject.owners, v.User.ID),
		"canReview":   v.User.IsAdmin,
		"suggestions": mine,
	})
}

// suggestCategory answers POST /charts/{id}/category-suggestions {"category": "…"}: the viewer
// suggests the chart is in the category. A suggestion of what was already decided is filed with
// that decision; suggesting the same again answers with the suggestion as it stands.
func (s *Server) suggestCategory(w http.ResponseWriter, r *http.Request) {
	v, ok := s.required(w, r, true)
	if !ok {
		return
	}
	var body struct {
		Category string `json:"category"`
	}
	if !decode(w, r, &body) {
		return
	}
	flag, ok := s.suggestedCategory(w, body.Category)
	if !ok {
		return
	}
	ctx := r.Context()
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		internal(w, e)
		return
	}
	defer tx.Rollback(ctx)
	// The chart's row lock orders the suggestion with a decision on it.
	subject, ok := chartSubject(w, r, tx, subjectWriteLock)
	if !ok {
		return
	}
	var status string
	e = tx.QueryRow(ctx, `INSERT INTO category_suggestions(chart_id,category_flag,user_id,status,decided_at)
 SELECT $1,$2,$3,d.status,CASE WHEN d.status='pending' THEN NULL ELSE now() END
 FROM (SELECT CASE WHEN (SELECT category_flags & $2 <> 0 FROM charts WHERE id=$1) THEN 'accepted'
   WHEN EXISTS(SELECT 1 FROM category_rejections WHERE chart_id=$1 AND category_flag=$2) THEN 'rejected' ELSE 'pending' END AS status) d
 ON CONFLICT (chart_id,category_flag,user_id) DO UPDATE SET status=category_suggestions.status
 RETURNING status`, subject.chartID, int(flag), v.User.ID).Scan(&status)
	if e == nil {
		e = tx.Commit(ctx)
	}
	if e != nil {
		internal(w, e)
		return
	}
	web.Respond(w, 200, MySuggestion{Category: body.Category, Status: status})
}

// withdrawSuggestion answers DELETE /charts/{id}/category-suggestions/{category}: the viewer
// takes back a suggestion nobody has decided yet.
func (s *Server) withdrawSuggestion(w http.ResponseWriter, r *http.Request) {
	v, ok := s.required(w, r, true)
	if !ok {
		return
	}
	flag, ok := s.source().categoryFlag(r.PathValue("category"))
	if !ok {
		problem(w, 404, "SUGGESTION_NOT_FOUND", "没有待处理的这条建议")
		return
	}
	tag, err := s.DB.Exec(r.Context(), `DELETE FROM category_suggestions WHERE chart_id=$1 AND category_flag=$2 AND user_id=$3 AND status='pending'`, r.PathValue("id"), int(flag), v.User.ID)
	if err != nil {
		internal(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		problem(w, 404, "SUGGESTION_NOT_FOUND", "没有待处理的这条建议")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// settleSuggestions answers what the chart now being in the categories added says: their waiting
// suggestions are accepted, by by, and they are no longer rejected. Whatever puts a chart in a
// category calls it in the same transaction.
func settleSuggestions(ctx context.Context, tx pgx.Tx, chart string, added CategoryFlags, by string) error {
	if added == 0 {
		return nil
	}
	if _, e := tx.Exec(ctx, `DELETE FROM category_rejections WHERE chart_id=$1 AND category_flag & $2 <> 0`, chart, int(added)); e != nil {
		return e
	}
	_, e := tx.Exec(ctx, `UPDATE category_suggestions SET status='accepted',decided_by=$3,decided_at=now()
 WHERE chart_id=$1 AND category_flag & $2 <> 0 AND status='pending'`, chart, int(added), by)
	return e
}

// SuggestionGroup is every suggestion of one category for one chart, as the managers see them.
type SuggestionGroup struct {
	Chart      SuggestedChart      `json:"chart"`
	Category   string              `json:"category"`
	Status     string              `json:"status"`
	Count      int                 `json:"count"`
	Suggesters []NotificationActor `json:"suggesters"`
	FirstAt    time.Time           `json:"firstAt"`
	LastAt     time.Time           `json:"lastAt"`
	DecidedAt  *time.Time          `json:"decidedAt"`
	DecidedBy  *NotificationActor  `json:"decidedBy"`
}

// SuggestedChart is what the queue shows of a chart.
type SuggestedChart struct {
	ID                   string            `json:"id"`
	NumberID             int64             `json:"numberId"`
	Title                string            `json:"title"`
	Subtitle             string            `json:"subtitle"`
	TitleTranslations    map[string]string `json:"titleTranslations"`
	SubtitleTranslations map[string]string `json:"subtitleTranslations"`
	SongCoverPath        string            `json:"songCoverPath,omitempty"`
	CategoryIDs          []string          `json:"categoryIds"`
}

const (
	suggestionPageSize = 30
	// maxSuggesters is how many of a group's players the queue names.
	maxSuggesters = 5
)

// categorySuggestions answers GET /category-suggestions?status=pending|accepted|rejected&page=n,
// for the source's managers: the suggestions grouped by chart and category, waiting ones oldest
// first, decided ones most recently decided first.
func (s *Server) categorySuggestions(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.categoryReviewer(w, r, false); !ok {
		return
	}
	ctx := r.Context()
	query := r.URL.Query()
	status := query.Get("status")
	if status == "" {
		status = suggestionPending
	}
	ordering := `min(s.created_at),s.chart_id,s.category_flag`
	switch status {
	case suggestionPending:
	case suggestionAccepted, suggestionRejected:
		ordering = `max(s.decided_at) DESC,s.chart_id,s.category_flag`
	default:
		problem(w, 400, "QUERY_INVALID", "status 必须为 pending / accepted / rejected")
		return
	}
	page, _ := strconv.Atoi(query.Get("page"))
	if page < 1 {
		page = 1
	}
	if page > 10000 {
		problem(w, 400, "QUERY_INVALID", "页码超出范围")
		return
	}
	from := ` FROM category_suggestions s JOIN charts c ON c.id=s.chart_id WHERE s.status=$1 AND ` + publishedChart
	var total int
	if e := s.DB.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1`+from+` GROUP BY s.chart_id,s.category_flag) g`, status).Scan(&total); e != nil {
		internal(w, e)
		return
	}
	// The decider of a group is whoever decided its latest decided suggestion that names one.
	rows, e := s.DB.Query(ctx, `SELECT s.chart_id,s.category_flag,count(*),min(s.created_at),max(s.created_at),
 (array_agg(s.user_id ORDER BY s.created_at))[1:`+strconv.Itoa(maxSuggesters)+`],max(s.decided_at),
 (array_agg(s.decided_by ORDER BY s.decided_at DESC) FILTER (WHERE s.decided_by IS NOT NULL))[1]`+
		from+` GROUP BY s.chart_id,s.category_flag ORDER BY `+ordering+` LIMIT $2 OFFSET $3`, status, suggestionPageSize, (page-1)*suggestionPageSize)
	if e != nil {
		internal(w, e)
		return
	}
	type row struct {
		group      SuggestionGroup
		chart      string
		suggesters []string
		decider    *string
	}
	found, e := pgx.CollectRows(rows, func(rw pgx.CollectableRow) (row, error) {
		var x row
		var flag CategoryFlags
		x.group.Status = status
		e := rw.Scan(&x.chart, &flag, &x.group.Count, &x.group.FirstAt, &x.group.LastAt, &x.suggesters, &x.group.DecidedAt, &x.decider)
		x.group.Category = s.source().categoryByFlag(flag).ID
		return x, e
	})
	if e != nil {
		internal(w, e)
		return
	}
	ids, people := []string{}, []string{}
	for _, x := range found {
		if !slices.Contains(ids, x.chart) {
			ids = append(ids, x.chart)
		}
		people = append(people, x.suggesters...)
		if x.decider != nil {
			people = append(people, *x.decider)
		}
	}
	charts, e := s.playlistCharts(ctx, ids)
	if e != nil {
		internal(w, e)
		return
	}
	byID := map[string]SuggestedChart{}
	for _, c := range charts {
		byID[c.ID] = SuggestedChart{ID: c.ID, NumberID: c.NumberID, Title: c.Title, Subtitle: c.Subtitle,
			TitleTranslations: c.TitleTranslations, SubtitleTranslations: c.SubtitleTranslations, SongCoverPath: c.SongCoverPath, CategoryIDs: c.CategoryIDs}
	}
	profiles := s.profiles(ctx, people)
	actor := func(id string) NotificationActor {
		p := ownerProfile(profiles, id)
		return NotificationActor{ID: id, Nickname: p.Nickname, AvatarPath: files.Path(p.AvatarURL)}
	}
	items := make([]SuggestionGroup, 0, len(found))
	for _, x := range found {
		x.group.Chart = byID[x.chart]
		x.group.Suggesters = make([]NotificationActor, 0, len(x.suggesters))
		for _, id := range x.suggesters {
			x.group.Suggesters = append(x.group.Suggesters, actor(id))
		}
		if x.decider != nil {
			a := actor(*x.decider)
			x.group.DecidedBy = &a
		}
		items = append(items, x.group)
	}
	web.Respond(w, 200, map[string]any{"items": items, "total": total, "page": page, "pageSize": suggestionPageSize})
}

// decideSuggestion answers POST /category-suggestions/decision {"chartId","category","accept"},
// for the source's managers. Accepting puts the chart in the category (and out of Variety, when
// that was its only one); rejecting remembers that it is not in it. Either answers every
// suggestion of that category for that chart, and those made later.
func (s *Server) decideSuggestion(w http.ResponseWriter, r *http.Request) {
	v, ok := s.categoryReviewer(w, r, true)
	if !ok {
		return
	}
	var body struct {
		ChartID  string `json:"chartId"`
		Category string `json:"category"`
		Accept   *bool  `json:"accept"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Accept == nil || !validID(body.ChartID) {
		problem(w, 400, "DECISION_INVALID", "缺少谱面或决定")
		return
	}
	flag, ok := s.suggestedCategory(w, body.Category)
	if !ok {
		return
	}
	ctx := r.Context()
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		internal(w, e)
		return
	}
	defer tx.Rollback(ctx)
	r.SetPathValue("id", body.ChartID)
	subject, ok := chartSubject(w, r, tx, subjectWriteLock)
	if !ok {
		return
	}
	var flags CategoryFlags
	if e = tx.QueryRow(ctx, `SELECT category_flags FROM charts WHERE id=$1`, subject.chartID).Scan(&flags); e != nil {
		internal(w, e)
		return
	}
	status, changed := suggestionAccepted, false
	if *body.Accept {
		if flags&flag == 0 {
			next := flags | flag
			if flags == varietyFlag {
				next = flag
			}
			if _, e = tx.Exec(ctx, `UPDATE charts SET category_flags=$2 WHERE id=$1`, subject.chartID, int(next)); e != nil {
				internal(w, e)
				return
			}
			changed = true
		}
		e = settleSuggestions(ctx, tx, subject.chartID, flag, v.User.ID)
	} else {
		status = suggestionRejected
		if flags&flag != 0 {
			problem(w, 409, "CATEGORY_ALREADY_SET", "谱面已经在这个分类里，要移出请编辑谱面")
			return
		}
		if _, e = tx.Exec(ctx, `INSERT INTO category_rejections(chart_id,category_flag,rejected_by) VALUES($1,$2,$3)
 ON CONFLICT (chart_id,category_flag) DO UPDATE SET rejected_by=excluded.rejected_by,rejected_at=now()`, subject.chartID, int(flag), v.User.ID); e == nil {
			_, e = tx.Exec(ctx, `UPDATE category_suggestions SET status='rejected',decided_by=$3,decided_at=now() WHERE chart_id=$1 AND category_flag=$2 AND status='pending'`, subject.chartID, int(flag), v.User.ID)
		}
	}
	if e == nil {
		e = tx.Commit(ctx)
	}
	if e != nil {
		internal(w, e)
		return
	}
	if changed {
		s.rulesAfterChartChange(ctx)
	}
	web.Respond(w, 200, map[string]string{"status": status})
}
