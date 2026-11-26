package httpapi

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"ourtaiko.dev/net/internal/files"
	"ourtaiko.dev/net/internal/web"
)

// Tags say what a song is beside its categories (migration 0026_chart_tags): the site's
// catalog (net.tags) is the same for both sources, and each source keeps which of its charts
// has which tag (chart_tags). A tag manager, whoever manages this source and a chart's owners set
// its tags; anyone else signed in suggests one, and a decision on a suggestion (a tag manager's
// or the source manager's) answers every suggestion of that tag for that chart, those made later
// included.

// Tag is one tag of the catalog with its name in each language, and how many published charts
// of this source have it.
type Tag struct {
	ID         string            `json:"id"`
	Names      map[string]string `json:"names"`
	ChartCount int               `json:"chartCount"`
}

// The states of a suggestion.
const (
	suggestionPending  = "pending"
	suggestionAccepted = "accepted"
	suggestionRejected = "rejected"
)

// tagQuerier is a pool, a connection or a transaction.
type tagQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func tagExists(ctx context.Context, db tagQuerier, id string) (bool, error) {
	var known bool
	e := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM tags WHERE id=$1)`, id).Scan(&known)
	return known, e
}

// tags answers GET /tags: the catalog in its order.
func (s *Server) tags(w http.ResponseWriter, r *http.Request) {
	rows, e := s.DB.Query(r.Context(), `SELECT g.id,g.names,
 (SELECT count(*) FROM chart_tags ct JOIN charts c ON c.id=ct.chart_id WHERE ct.tag_id=g.id AND `+publishedChart+`)
 FROM tags g ORDER BY g.position`)
	if e != nil {
		internal(w, e)
		return
	}
	items, e := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Tag, error) {
		var t Tag
		return t, row.Scan(&t.ID, &t.Names, &t.ChartCount)
	})
	if e != nil {
		internal(w, e)
		return
	}
	web.Respond(w, 200, map[string]any{"items": items})
}

// reviewsTags says whether u decides the tags players suggest and sets any chart's tags: a tag
// manager, or whoever manages this source.
func (s *Server) reviewsTags(ctx context.Context, u User) (bool, error) {
	switch {
	case s.Config.SSO == nil, u.IsAdmin:
		// The one account of a server without an account center may do everything.
		return true, nil
	case s.Config.TagManager != nil:
		return s.Config.TagManager(ctx, u.ID)
	}
	return false, nil
}

// tagRoles says what u may do with the tags of a chart owned by owners: edit sets them (whoever
// reviews tags, and the chart's owners), review decides suggestions (reviewsTags).
func (s *Server) tagRoles(ctx context.Context, u User, owners []string) (edit, review bool, e error) {
	review, e = s.reviewsTags(ctx, u)
	return review || slices.Contains(owners, u.ID), review, e
}

// tagReviewer answers a request for tag managers' work with who it is from, or answers it itself
// for anyone else.
func (s *Server) tagReviewer(w http.ResponseWriter, r *http.Request, csrf bool) (session, bool) {
	v, ok := s.required(w, r, csrf)
	if !ok {
		return v, false
	}
	review, e := s.reviewsTags(r.Context(), v.User)
	if e != nil {
		internal(w, e)
		return v, false
	}
	if !review {
		problem(w, 403, "FORBIDDEN", "只有标签管理员或曲库管理员可以处理标签建议")
		return v, false
	}
	return v, true
}

// MySuggestion is one of the viewer's suggestions on a chart.
type MySuggestion struct {
	Tag    string `json:"tag"`
	Status string `json:"status"`
}

// chartTags answers GET /charts/{id}/tags: the chart's tags, and for a signed-in viewer what
// they may do and what they have suggested.
func (s *Server) chartTags(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	subject, ok := chartSubject(w, r, s.DB, "")
	if !ok {
		return
	}
	answer := map[string]any{"canEdit": false, "canReview": false, "suggestions": []MySuggestion{}}
	tags, e := s.tagsOf(ctx, s.DB, subject.chartID)
	if e != nil {
		internal(w, e)
		return
	}
	answer["tags"] = tags
	v, e := s.current(r)
	if errors.Is(e, pgx.ErrNoRows) {
		web.Respond(w, 200, answer)
		return
	}
	if e != nil {
		internal(w, e)
		return
	}
	edit, review, e := s.tagRoles(ctx, v.User, subject.owners)
	if e != nil {
		internal(w, e)
		return
	}
	answer["canEdit"], answer["canReview"] = edit, review
	rows, e := s.DB.Query(ctx, `SELECT s.tag_id,s.status FROM tag_suggestions s JOIN tags g ON g.id=s.tag_id
 WHERE s.chart_id=$1 AND s.user_id=$2 ORDER BY g.position`, subject.chartID, v.User.ID)
	if e == nil {
		answer["suggestions"], e = pgx.CollectRows(rows, func(row pgx.CollectableRow) (MySuggestion, error) {
			var m MySuggestion
			return m, row.Scan(&m.Tag, &m.Status)
		})
	}
	if e != nil {
		internal(w, e)
		return
	}
	web.Respond(w, 200, answer)
}

func (s *Server) tagsOf(ctx context.Context, db interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}, chart string) ([]string, error) {
	rows, e := db.Query(ctx, `SELECT ct.tag_id FROM chart_tags ct JOIN tags g ON g.id=ct.tag_id WHERE ct.chart_id=$1 ORDER BY g.position`, chart)
	if e != nil {
		return nil, e
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// setChartTags answers PUT /charts/{id}/tags {"tags": [...]}: the chart has exactly these tags
// from now on. A tag put on answers its waiting suggestions as accepted and forgets that it was
// rejected; a tag taken off leaves the suggestions as they were decided.
func (s *Server) setChartTags(w http.ResponseWriter, r *http.Request) {
	v, ok := s.required(w, r, true)
	if !ok {
		return
	}
	var body struct {
		Tags *[]string `json:"tags"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Tags == nil {
		problem(w, 400, "TAGS_INVALID", "缺少标签列表")
		return
	}
	ctx := r.Context()
	want := slices.Clone(*body.Tags)
	slices.Sort(want)
	want = slices.Compact(want)
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		internal(w, e)
		return
	}
	defer tx.Rollback(ctx)
	subject, ok := chartSubject(w, r, tx, subjectWriteLock)
	if !ok {
		return
	}
	edit, _, e := s.tagRoles(ctx, v.User, subject.owners)
	if e != nil {
		internal(w, e)
		return
	}
	if !edit {
		problem(w, 403, "FORBIDDEN", "只有谱面所有者、标签管理员或曲库管理员可以直接修改标签，其他人请提交建议")
		return
	}
	var known int
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM tags WHERE id=ANY($1)`, want).Scan(&known); e != nil {
		internal(w, e)
		return
	}
	if known != len(want) {
		problem(w, 422, "TAG_NOT_FOUND", "标签不存在")
		return
	}
	have, e := s.tagsOf(ctx, tx, subject.chartID)
	if e != nil {
		internal(w, e)
		return
	}
	changed := false
	for _, tag := range want {
		if !slices.Contains(have, tag) {
			if e = addChartTag(ctx, tx, subject.chartID, tag, v.User.ID); e != nil {
				internal(w, e)
				return
			}
			changed = true
		}
	}
	for _, tag := range have {
		if !slices.Contains(want, tag) {
			if _, e = tx.Exec(ctx, `DELETE FROM chart_tags WHERE chart_id=$1 AND tag_id=$2`, subject.chartID, tag); e != nil {
				internal(w, e)
				return
			}
			changed = true
		}
	}
	if changed {
		e = touchChart(ctx, tx, subject.chartID)
	}
	if e == nil {
		e = tx.Commit(ctx)
	}
	if e != nil {
		internal(w, e)
		return
	}
	tags, e := s.tagsOf(ctx, s.DB, subject.chartID)
	if e != nil {
		internal(w, e)
		return
	}
	web.Respond(w, 200, map[string]any{"tags": tags})
}

// addChartTag puts a tag on a chart for by: it is no longer rejected, and every waiting
// suggestion of it is accepted.
func addChartTag(ctx context.Context, tx pgx.Tx, chart, tag, by string) error {
	if _, e := tx.Exec(ctx, `INSERT INTO chart_tags(chart_id,tag_id,added_by) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, chart, tag, by); e != nil {
		return e
	}
	if _, e := tx.Exec(ctx, `DELETE FROM tag_rejections WHERE chart_id=$1 AND tag_id=$2`, chart, tag); e != nil {
		return e
	}
	return decideWaiting(ctx, tx, chart, tag, suggestionAccepted, by)
}

func decideWaiting(ctx context.Context, tx pgx.Tx, chart, tag, status, by string) error {
	_, e := tx.Exec(ctx, `UPDATE tag_suggestions SET status=$3,decided_by=$4,decided_at=now() WHERE chart_id=$1 AND tag_id=$2 AND status='pending'`, chart, tag, status, by)
	return e
}

// touchChart records that something about the chart changed (charts.update_at), as a vote or
// a comment does.
func touchChart(ctx context.Context, tx pgx.Tx, chart string) error {
	_, e := tx.Exec(ctx, `UPDATE charts SET update_at=clock_timestamp() WHERE id=$1`, chart)
	return e
}

// suggestTag answers POST /charts/{id}/tag-suggestions {"tag": "…"}: the viewer suggests the
// chart has the tag. A suggestion of what was already decided is filed with that decision;
// suggesting the same tag again answers with the suggestion as it stands.
func (s *Server) suggestTag(w http.ResponseWriter, r *http.Request) {
	v, ok := s.required(w, r, true)
	if !ok {
		return
	}
	var body struct {
		Tag string `json:"tag"`
	}
	if !decode(w, r, &body) {
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
	known, e := tagExists(ctx, tx, body.Tag)
	if e != nil {
		internal(w, e)
		return
	}
	if !known {
		problem(w, 422, "TAG_NOT_FOUND", "标签不存在")
		return
	}
	var status string
	e = tx.QueryRow(ctx, `INSERT INTO tag_suggestions(chart_id,tag_id,user_id,status,decided_at)
 SELECT $1,$2,$3,d.status,CASE WHEN d.status='pending' THEN NULL ELSE now() END
 FROM (SELECT CASE WHEN EXISTS(SELECT 1 FROM chart_tags WHERE chart_id=$1 AND tag_id=$2) THEN 'accepted'
   WHEN EXISTS(SELECT 1 FROM tag_rejections WHERE chart_id=$1 AND tag_id=$2) THEN 'rejected' ELSE 'pending' END AS status) d
 ON CONFLICT (chart_id,tag_id,user_id) DO UPDATE SET status=tag_suggestions.status
 RETURNING status`, subject.chartID, body.Tag, v.User.ID).Scan(&status)
	if e == nil {
		e = tx.Commit(ctx)
	}
	if e != nil {
		internal(w, e)
		return
	}
	web.Respond(w, 200, MySuggestion{Tag: body.Tag, Status: status})
}

// withdrawSuggestion answers DELETE /charts/{id}/tag-suggestions/{tag}: the viewer takes back
// a suggestion nobody has decided yet.
func (s *Server) withdrawSuggestion(w http.ResponseWriter, r *http.Request) {
	v, ok := s.required(w, r, true)
	if !ok {
		return
	}
	tag, err := s.DB.Exec(r.Context(), `DELETE FROM tag_suggestions WHERE chart_id=$1 AND tag_id=$2 AND user_id=$3 AND status='pending'`, r.PathValue("id"), r.PathValue("tag"), v.User.ID)
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

// SuggestionGroup is every suggestion of one tag for one chart, as the tag managers see them.
type SuggestionGroup struct {
	Chart      SuggestedChart      `json:"chart"`
	Tag        string              `json:"tag"`
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
	Tags                 []string          `json:"tags"`
}

const (
	suggestionPageSize = 30
	// maxSuggesters is how many of a group's players the queue names.
	maxSuggesters = 5
)

// tagSuggestions answers GET /tag-suggestions?status=pending|accepted|rejected&page=n, for tag
// managers and the source's managers: the suggestions grouped by chart and tag, waiting ones oldest first, decided ones
// most recently decided first.
func (s *Server) tagSuggestions(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.tagReviewer(w, r, false); !ok {
		return
	}
	ctx := r.Context()
	query := r.URL.Query()
	status := query.Get("status")
	if status == "" {
		status = suggestionPending
	}
	ordering := `min(s.created_at),s.chart_id,s.tag_id`
	switch status {
	case suggestionPending:
	case suggestionAccepted, suggestionRejected:
		ordering = `max(s.decided_at) DESC,s.chart_id,s.tag_id`
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
	from := ` FROM tag_suggestions s JOIN charts c ON c.id=s.chart_id WHERE s.status=$1 AND ` + publishedChart
	var total int
	if e := s.DB.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1`+from+` GROUP BY s.chart_id,s.tag_id) g`, status).Scan(&total); e != nil {
		internal(w, e)
		return
	}
	// The decider of a group is whoever decided its latest decided suggestion that names one.
	rows, e := s.DB.Query(ctx, `SELECT s.chart_id,s.tag_id,count(*),min(s.created_at),max(s.created_at),
 (array_agg(s.user_id ORDER BY s.created_at))[1:`+strconv.Itoa(maxSuggesters)+`],max(s.decided_at),
 (array_agg(s.decided_by ORDER BY s.decided_at DESC) FILTER (WHERE s.decided_by IS NOT NULL))[1]`+
		from+` GROUP BY s.chart_id,s.tag_id ORDER BY `+ordering+` LIMIT $2 OFFSET $3`, status, suggestionPageSize, (page-1)*suggestionPageSize)
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
		x.group.Status = status
		return x, rw.Scan(&x.chart, &x.group.Tag, &x.group.Count, &x.group.FirstAt, &x.group.LastAt, &x.suggesters, &x.group.DecidedAt, &x.decider)
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
			TitleTranslations: c.TitleTranslations, SubtitleTranslations: c.SubtitleTranslations, SongCoverPath: c.SongCoverPath, Tags: c.Tags}
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

// decideSuggestion answers POST /tag-suggestions/decision {"chartId","tag","accept"}, for tag
// managers and the source's managers. Accepting puts the tag on the chart; rejecting remembers that the chart does not
// have it. Either answers every suggestion of that tag for that chart, and those made later.
func (s *Server) decideSuggestion(w http.ResponseWriter, r *http.Request) {
	v, ok := s.tagReviewer(w, r, true)
	if !ok {
		return
	}
	var body struct {
		ChartID string `json:"chartId"`
		Tag     string `json:"tag"`
		Accept  *bool  `json:"accept"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Accept == nil || !validID(body.ChartID) {
		problem(w, 400, "DECISION_INVALID", "缺少谱面或决定")
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
	known, e := tagExists(ctx, tx, body.Tag)
	if e != nil {
		internal(w, e)
		return
	}
	if !known {
		problem(w, 422, "TAG_NOT_FOUND", "标签不存在")
		return
	}
	status := suggestionAccepted
	if *body.Accept {
		e = addChartTag(ctx, tx, subject.chartID, body.Tag, v.User.ID)
		if e == nil {
			e = touchChart(ctx, tx, subject.chartID)
		}
	} else {
		status = suggestionRejected
		var tagged bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chart_tags WHERE chart_id=$1 AND tag_id=$2)`, subject.chartID, body.Tag).Scan(&tagged); e != nil {
			internal(w, e)
			return
		}
		if tagged {
			problem(w, 409, "TAG_ALREADY_SET", "谱面已经有这个标签，要去掉请直接修改谱面的标签")
			return
		}
		if _, e = tx.Exec(ctx, `INSERT INTO tag_rejections(chart_id,tag_id,rejected_by) VALUES($1,$2,$3)
 ON CONFLICT (chart_id,tag_id) DO UPDATE SET rejected_by=excluded.rejected_by,rejected_at=now()`, subject.chartID, body.Tag, v.User.ID); e == nil {
			e = decideWaiting(ctx, tx, subject.chartID, body.Tag, suggestionRejected, v.User.ID)
		}
	}
	if e == nil {
		e = tx.Commit(ctx)
	}
	if e != nil {
		internal(w, e)
		return
	}
	web.Respond(w, 200, map[string]string{"status": status})
}
