package httpapi

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"ourtaiko.dev/net/internal/files"
	"ourtaiko.dev/net/internal/netdb"
	"ourtaiko.dev/net/internal/web"
)

// Tags say what a song is beside its categories (migration 0031_tag_votes). The catalog
// (net.tags) is the site's, the same for both sources; each source keeps its players' votes
// (chart_tag_votes). A chart has a tag while at least one vote for it stands. A signed-in player
// adds a tag, takes their own vote back and reports a tag that does not belong; only a manager of
// the source takes a tag off a chart against its voters, by upholding a report, and that is
// remembered (tag_rejections).
//
// A manager's new tag is official at once. A player naming a tag the catalog does not have makes
// a pending one, offered to everyone like the others, which becomes official when
// tagPromotionUsers different players have used it, on whatever charts of either source. A
// pending tag nobody votes for any more is deleted.

const (
	tagPending  = "pending"
	tagOfficial = "official"
	// tagPromotionUsers is how many different players must have used a pending tag for it to
	// become official.
	tagPromotionUsers = 3
	// maxPendingTags is how many pending tags a player may have named at a time.
	maxPendingTags = 20
	// maxTagName is a tag name's length in characters; the table checks the same.
	maxTagName = 24
	// maxTagReason is a report's reason in characters; the table checks the same.
	maxTagReason = 500
)

// tagLanguages are the languages a tag may be named in: the site's. en is the name shown where
// a language has none, and the only one a player's tag has.
var tagLanguages = []string{"en", "zh-Hans", "ja", "ko"}

// Tag is one tag of the catalog with its name in each language it has, and how many published
// charts of this source have it.
type Tag struct {
	ID         string            `json:"id"`
	Names      map[string]string `json:"names"`
	Status     string            `json:"status"`
	ChartCount int               `json:"chartCount"`
}

// ChartTag is a tag on a chart: how many players voted for it, and whether the viewer did.
type ChartTag struct {
	ID     string            `json:"id"`
	Names  map[string]string `json:"names"`
	Status string            `json:"status"`
	Count  int               `json:"count"`
	Mine   bool              `json:"mine"`
}

// tagTables locks the catalog for whoever adds to it or reshapes it, so that names stay apart and
// positions in step. Votes on tags that are there do not take it.
const tagTables = `SELECT pg_advisory_xact_lock(hashtext('net.tags'))`

// votesEverywhere is a subquery of the players whose vote for tag $1 stands, in every source:
// what makes a pending tag official is counted over the whole site.
var votesEverywhere = func() string {
	parts := make([]string, 0, len(netdb.Sources))
	for _, src := range netdb.Sources {
		parts = append(parts, `SELECT user_id FROM `+pgx.Identifier{src, "chart_tag_votes"}.Sanitize()+` WHERE tag_id=$1 AND vote=1`)
	}
	return strings.Join(parts, " UNION ")
}()

// tagList answers GET /tags: the official tags in the catalog's order, then the pending ones,
// oldest first.
func (s *Server) tagList(w http.ResponseWriter, r *http.Request) {
	rows, e := s.DB.Query(r.Context(), `SELECT g.id,g.names,g.status,
 (SELECT count(DISTINCT v.chart_id) FROM chart_tag_votes v JOIN charts c ON c.id=v.chart_id WHERE v.tag_id=g.id AND v.vote=1 AND `+publishedChart+`)
 FROM tags g ORDER BY (g.status='official') DESC,g.position`)
	if e != nil {
		internal(w, e)
		return
	}
	items, e := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Tag, error) {
		var t Tag
		return t, row.Scan(&t.ID, &t.Names, &t.Status, &t.ChartCount)
	})
	if e != nil {
		internal(w, e)
		return
	}
	web.Respond(w, 200, map[string]any{"items": items})
}

// tagQuerier is a pool or a transaction.
type tagQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func officialTag(ctx context.Context, db tagQuerier, id string) (bool, error) {
	var known bool
	e := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM tags WHERE id=$1 AND status='official')`, id).Scan(&known)
	return known, e
}

// tagsOf lists the tags on a chart, the most voted first; viewer is who Mine is of, "" for
// nobody.
func tagsOf(ctx context.Context, db tagQuerier, chart, viewer string) ([]ChartTag, error) {
	rows, e := db.Query(ctx, `SELECT g.id,g.names,g.status,count(*),COALESCE(bool_or(v.user_id=$2),false)
 FROM chart_tag_votes v JOIN tags g ON g.id=v.tag_id WHERE v.chart_id=$1 AND v.vote=1
 GROUP BY g.id ORDER BY count(*) DESC,(g.status='official') DESC,g.position`, chart, viewer)
	if e != nil {
		return nil, e
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (ChartTag, error) {
		var t ChartTag
		return t, row.Scan(&t.ID, &t.Names, &t.Status, &t.Count, &t.Mine)
	})
}

// chartTags answers GET /charts/{id}/tags: the chart's tags with their votes, and for a signed-in
// viewer which are theirs, which they have reported, and whether they decide reports.
func (s *Server) chartTags(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	subject, ok := chartSubject(w, r, s.DB, "")
	if !ok {
		return
	}
	v, e := s.current(r)
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		internal(w, e)
		return
	}
	signedIn := e == nil
	viewer := ""
	if signedIn {
		viewer = v.User.ID
	}
	tags, e := tagsOf(ctx, s.DB, subject.chartID, viewer)
	if e != nil {
		internal(w, e)
		return
	}
	reported := []string{}
	if signedIn {
		rows, e := s.DB.Query(ctx, `SELECT tag_id FROM chart_tag_reports WHERE chart_id=$1 AND user_id=$2 AND status='pending' ORDER BY tag_id`, subject.chartID, viewer)
		if e == nil {
			reported, e = pgx.CollectRows(rows, pgx.RowTo[string])
		}
		if e != nil {
			internal(w, e)
			return
		}
	}
	web.Respond(w, 200, map[string]any{"tags": tags, "reported": reported, "canReview": signedIn && v.User.IsAdmin})
}

// tagName cleans a name a tag is to have: no space at either end, 1 to maxTagName characters,
// none of them a control character.
func tagName(raw string) (string, bool) {
	name := strings.TrimSpace(raw)
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > maxTagName || strings.ContainsFunc(name, unicode.IsControl) {
		return "", false
	}
	return name, true
}

// tagNamed finds the tag with the name in any of its languages, whatever the case, or the ID;
// official ones first. "" is none.
func tagNamed(ctx context.Context, tx pgx.Tx, name string) (string, error) {
	var id string
	e := tx.QueryRow(ctx, `SELECT g.id FROM tags g WHERE g.id=lower($1) OR EXISTS(SELECT 1 FROM jsonb_each_text(g.names) n WHERE lower(n.value)=lower($1))
 ORDER BY (g.status='official') DESC,g.position LIMIT 1`, name).Scan(&id)
	if errors.Is(e, pgx.ErrNoRows) {
		return "", nil
	}
	return id, e
}

// newTag adds a tag to the catalog's end. The caller holds tagTables.
func newTag(ctx context.Context, tx pgx.Tx, names map[string]string, status, by string) (string, error) {
	id := web.NewID()
	_, e := tx.Exec(ctx, `INSERT INTO tags(id,names,position,status,created_by) SELECT $1,$2,COALESCE(max(position),0)+1,$3,$4 FROM tags`, id, names, status, by)
	return id, e
}

// castVote records that by says the chart has the tag, and makes a pending tag official when
// that is its tagPromotionUsers-th player. A tag a manager took off the chart is refused (the
// answer is false), unless by is a manager, whose vote puts it back. The caller holds the chart's
// row lock.
func castVote(ctx context.Context, tx pgx.Tx, chart, tag string, by User) (bool, error) {
	// The tag's row lock orders this with the other votes that might make it official.
	var status string
	if e := tx.QueryRow(ctx, `SELECT status FROM tags WHERE id=$1 FOR UPDATE`, tag).Scan(&status); e != nil {
		return false, e
	}
	if by.IsAdmin {
		if _, e := tx.Exec(ctx, `DELETE FROM tag_rejections WHERE chart_id=$1 AND tag_id=$2`, chart, tag); e != nil {
			return false, e
		}
	} else {
		var rejected bool
		if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM tag_rejections WHERE chart_id=$1 AND tag_id=$2)`, chart, tag).Scan(&rejected); e != nil || rejected {
			return false, e
		}
	}
	if _, e := tx.Exec(ctx, `INSERT INTO chart_tag_votes(chart_id,tag_id,user_id,vote) VALUES($1,$2,$3,1)
 ON CONFLICT (chart_id,tag_id,user_id) DO UPDATE SET vote=1,updated_at=now() WHERE chart_tag_votes.vote<>1`, chart, tag, by.ID); e != nil {
		return false, e
	}
	if status == tagPending {
		if _, e := tx.Exec(ctx, `UPDATE tags SET status='official' WHERE id=$1 AND (SELECT count(*) FROM (`+votesEverywhere+`) u)>=$2`, tag, tagPromotionUsers); e != nil {
			return false, e
		}
	}
	return true, nil
}

// dropUnusedTag deletes a pending tag no vote stands for any more, anywhere.
func dropUnusedTag(ctx context.Context, tx pgx.Tx, tag string) error {
	_, e := tx.Exec(ctx, `DELETE FROM tags WHERE id=$1 AND status='pending' AND NOT EXISTS(`+votesEverywhere+`)`, tag)
	return e
}

// answerTags commits and answers with the chart's tags as they now are.
func (s *Server) answerTags(w http.ResponseWriter, ctx context.Context, tx pgx.Tx, chart, viewer string) {
	tags, e := tagsOf(ctx, tx, chart, viewer)
	if e == nil {
		e = tx.Commit(ctx)
	}
	if e != nil {
		internal(w, e)
		return
	}
	web.Respond(w, 200, map[string]any{"tags": tags})
}

// voteTag answers PUT /charts/{id}/tags/{tag} {"vote": 1|0}: the viewer says the chart has the
// tag, or takes that back. Nobody's vote but the viewer's own changes.
func (s *Server) voteTag(w http.ResponseWriter, r *http.Request) {
	v, ok := s.required(w, r, true)
	if !ok {
		return
	}
	var body struct {
		Vote *int `json:"vote"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Vote == nil || (*body.Vote != 0 && *body.Vote != 1) {
		problem(w, 400, "VOTE_INVALID", "vote 必须为 1 或 0")
		return
	}
	ctx := r.Context()
	tag := r.PathValue("tag")
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
	if *body.Vote == 1 {
		var cast bool
		cast, e = castVote(ctx, tx, subject.chartID, tag, v.User)
		if errors.Is(e, pgx.ErrNoRows) {
			problem(w, 404, "TAG_NOT_FOUND", "标签不存在")
			return
		}
		if e == nil && !cast {
			problem(w, 409, "TAG_REJECTED", "管理员已认定这首歌不属于这个标签")
			return
		}
	} else {
		if _, e = tx.Exec(ctx, `UPDATE chart_tag_votes SET vote=0,updated_at=now() WHERE chart_id=$1 AND tag_id=$2 AND user_id=$3 AND vote=1`, subject.chartID, tag, v.User.ID); e == nil {
			e = dropUnusedTag(ctx, tx, tag)
		}
	}
	if e != nil {
		internal(w, e)
		return
	}
	s.answerTags(w, ctx, tx, subject.chartID, v.User.ID)
}

// addTag answers POST /charts/{id}/tags {"name": "…"}: the viewer says the chart has the tag of
// that name. A name no tag has makes one: official from a manager, pending from anyone else.
func (s *Server) addTag(w http.ResponseWriter, r *http.Request) {
	v, ok := s.required(w, r, true)
	if !ok {
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &body) {
		return
	}
	name, ok := tagName(body.Name)
	if !ok {
		problem(w, 422, "TAG_NAME_INVALID", "标签名须为 1–"+strconv.Itoa(maxTagName)+" 个字符")
		return
	}
	ctx := r.Context()
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
	if _, e = tx.Exec(ctx, tagTables); e != nil {
		internal(w, e)
		return
	}
	tag, e := tagNamed(ctx, tx, name)
	if e != nil {
		internal(w, e)
		return
	}
	if tag == "" {
		status := tagOfficial
		if !v.User.IsAdmin {
			status = tagPending
			var pending int
			if e = tx.QueryRow(ctx, `SELECT count(*) FROM tags WHERE status='pending' AND created_by=$1`, v.User.ID).Scan(&pending); e != nil {
				internal(w, e)
				return
			}
			if pending >= maxPendingTags {
				problem(w, 409, "TAG_LIMIT", "你创建的待定标签已达上限，等它们转正或撤回后再创建")
				return
			}
		}
		if tag, e = newTag(ctx, tx, map[string]string{"en": name}, status, v.User.ID); e != nil {
			internal(w, e)
			return
		}
	}
	cast, e := castVote(ctx, tx, subject.chartID, tag, v.User)
	if e != nil {
		internal(w, e)
		return
	}
	if !cast {
		problem(w, 409, "TAG_REJECTED", "管理员已认定这首歌不属于这个标签")
		return
	}
	s.answerTags(w, ctx, tx, subject.chartID, v.User.ID)
}

// reportTag answers POST /charts/{id}/tags/{tag}/reports {"reason": "…"}: the viewer says the tag
// does not belong on the chart. Reporting again while the first waits replaces its reason; a
// report already decided stays as decided.
func (s *Server) reportTag(w http.ResponseWriter, r *http.Request) {
	v, ok := s.required(w, r, true)
	if !ok {
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	if !decode(w, r, &body) {
		return
	}
	reason := strings.TrimSpace(body.Reason)
	if utf8.RuneCountInString(reason) > maxTagReason {
		problem(w, 422, "REASON_TOO_LONG", "理由不能超过 "+strconv.Itoa(maxTagReason)+" 个字符")
		return
	}
	ctx := r.Context()
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		internal(w, e)
		return
	}
	defer tx.Rollback(ctx)
	// The chart's row lock orders the report with a decision on the tag.
	subject, ok := chartSubject(w, r, tx, subjectWriteLock)
	if !ok {
		return
	}
	tag := r.PathValue("tag")
	var there bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chart_tag_votes WHERE chart_id=$1 AND tag_id=$2 AND vote=1)`, subject.chartID, tag).Scan(&there); e != nil {
		internal(w, e)
		return
	}
	if !there {
		problem(w, 404, "TAG_NOT_ON_CHART", "这首歌没有这个标签")
		return
	}
	if _, e = tx.Exec(ctx, `INSERT INTO chart_tag_reports(chart_id,tag_id,user_id,reason) VALUES($1,$2,$3,$4)
 ON CONFLICT (chart_id,tag_id,user_id) DO UPDATE SET reason=excluded.reason WHERE chart_tag_reports.status='pending'`, subject.chartID, tag, v.User.ID, reason); e == nil {
		e = tx.Commit(ctx)
	}
	if e != nil {
		internal(w, e)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// tagManager answers a request for a manager's work on tags with who it is from, or answers it
// itself for anyone else.
func (s *Server) tagManager(w http.ResponseWriter, r *http.Request, csrf bool) (session, bool) {
	v, ok := s.required(w, r, csrf)
	if ok && !v.User.IsAdmin {
		problem(w, 403, "FORBIDDEN", "只有曲库管理员可以管理标签")
		return v, false
	}
	return v, ok
}

// The states of a report.
const (
	reportPending   = "pending"
	reportUpheld    = "upheld"
	reportDismissed = "dismissed"
)

// TagReporter is a player who reported a tag, with what they said.
type TagReporter struct {
	NotificationActor
	Reason string `json:"reason"`
}

// TagReportGroup is every report of one tag on one chart, as the managers see them.
type TagReportGroup struct {
	Chart     SuggestedChart `json:"chart"`
	Tag       Tag            `json:"tag"`
	Status    string         `json:"status"`
	Count     int            `json:"count"`
	Reporters []TagReporter  `json:"reporters"`
	// Votes is how many players' votes for the tag stand on the chart.
	Votes     int                `json:"votes"`
	FirstAt   time.Time          `json:"firstAt"`
	LastAt    time.Time          `json:"lastAt"`
	DecidedAt *time.Time         `json:"decidedAt"`
	DecidedBy *NotificationActor `json:"decidedBy"`
}

// tagReports answers GET /tag-reports?status=pending|upheld|dismissed&page=n, for the source's
// managers: the reports grouped by chart and tag, waiting ones oldest first, decided ones most
// recently decided first.
func (s *Server) tagReports(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.tagManager(w, r, false); !ok {
		return
	}
	ctx := r.Context()
	query := r.URL.Query()
	status := query.Get("status")
	if status == "" {
		status = reportPending
	}
	ordering := `min(p.created_at),p.chart_id,g.id`
	switch status {
	case reportPending:
	case reportUpheld, reportDismissed:
		ordering = `max(p.decided_at) DESC,p.chart_id,g.id`
	default:
		problem(w, 400, "QUERY_INVALID", "status 必须为 pending / upheld / dismissed")
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
	from := ` FROM chart_tag_reports p JOIN charts c ON c.id=p.chart_id JOIN tags g ON g.id=p.tag_id WHERE p.status=$1 AND ` + publishedChart
	var total int
	if e := s.DB.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1`+from+` GROUP BY p.chart_id,p.tag_id) x`, status).Scan(&total); e != nil {
		internal(w, e)
		return
	}
	// The decider of a group is whoever decided its latest decided report that names one.
	rows, e := s.DB.Query(ctx, `SELECT p.chart_id,g.id,g.names,g.status,count(*),min(p.created_at),max(p.created_at),
 (array_agg(p.user_id ORDER BY p.created_at))[1:`+strconv.Itoa(maxSuggesters)+`],(array_agg(p.reason ORDER BY p.created_at))[1:`+strconv.Itoa(maxSuggesters)+`],
 max(p.decided_at),(array_agg(p.decided_by ORDER BY p.decided_at DESC) FILTER (WHERE p.decided_by IS NOT NULL))[1],
 (SELECT count(*) FROM chart_tag_votes v WHERE v.chart_id=p.chart_id AND v.tag_id=g.id AND v.vote=1)`+
		from+` GROUP BY p.chart_id,g.id ORDER BY `+ordering+` LIMIT $2 OFFSET $3`, status, suggestionPageSize, (page-1)*suggestionPageSize)
	if e != nil {
		internal(w, e)
		return
	}
	type row struct {
		group     TagReportGroup
		chart     string
		reporters []string
		reasons   []string
		decider   *string
	}
	found, e := pgx.CollectRows(rows, func(rw pgx.CollectableRow) (row, error) {
		var x row
		x.group.Status = status
		return x, rw.Scan(&x.chart, &x.group.Tag.ID, &x.group.Tag.Names, &x.group.Tag.Status, &x.group.Count, &x.group.FirstAt, &x.group.LastAt,
			&x.reporters, &x.reasons, &x.group.DecidedAt, &x.decider, &x.group.Votes)
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
		people = append(people, x.reporters...)
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
	items := make([]TagReportGroup, 0, len(found))
	for _, x := range found {
		x.group.Chart = byID[x.chart]
		x.group.Reporters = make([]TagReporter, 0, len(x.reporters))
		for i, id := range x.reporters {
			x.group.Reporters = append(x.group.Reporters, TagReporter{NotificationActor: actor(id), Reason: x.reasons[i]})
		}
		if x.decider != nil {
			a := actor(*x.decider)
			x.group.DecidedBy = &a
		}
		items = append(items, x.group)
	}
	web.Respond(w, 200, map[string]any{"items": items, "total": total, "page": page, "pageSize": suggestionPageSize})
}

// decideTagReport answers POST /tag-reports/decision {"chartId","tag","uphold"}, for the source's
// managers. Upholding takes the tag off the chart, every vote for it withdrawn, and remembers it
// so no vote puts it back; dismissing leaves the chart as it is. Either answers every waiting
// report of that tag on that chart.
func (s *Server) decideTagReport(w http.ResponseWriter, r *http.Request) {
	v, ok := s.tagManager(w, r, true)
	if !ok {
		return
	}
	var body struct {
		ChartID string `json:"chartId"`
		Tag     string `json:"tag"`
		Uphold  *bool  `json:"uphold"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Uphold == nil || !validID(body.ChartID) || body.Tag == "" {
		problem(w, 400, "DECISION_INVALID", "缺少谱面、标签或决定")
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
	status := reportDismissed
	if *body.Uphold {
		status = reportUpheld
		// The tag's row lock comes before its votes change, as in castVote.
		var known bool
		if e = tx.QueryRow(ctx, `SELECT true FROM tags WHERE id=$1 FOR UPDATE`, body.Tag).Scan(&known); errors.Is(e, pgx.ErrNoRows) {
			problem(w, 404, "TAG_NOT_FOUND", "标签不存在")
			return
		}
		if e == nil {
			_, e = tx.Exec(ctx, `INSERT INTO tag_rejections(chart_id,tag_id,rejected_by) VALUES($1,$2,$3)
 ON CONFLICT (chart_id,tag_id) DO UPDATE SET rejected_by=excluded.rejected_by,rejected_at=now()`, subject.chartID, body.Tag, v.User.ID)
		}
		if e == nil {
			_, e = tx.Exec(ctx, `UPDATE chart_tag_votes SET vote=0,updated_at=now() WHERE chart_id=$1 AND tag_id=$2 AND vote=1`, subject.chartID, body.Tag)
		}
	}
	if e == nil {
		_, e = tx.Exec(ctx, `UPDATE chart_tag_reports SET status=$4,decided_by=$3,decided_at=now() WHERE chart_id=$1 AND tag_id=$2 AND status='pending'`, subject.chartID, body.Tag, v.User.ID, status)
	}
	// Last: it may delete the tag, and the reports just decided with it.
	if e == nil && *body.Uphold {
		e = dropUnusedTag(ctx, tx, body.Tag)
	}
	if e == nil {
		e = tx.Commit(ctx)
	}
	if e != nil {
		internal(w, e)
		return
	}
	web.Respond(w, 200, map[string]any{"chartId": subject.chartID, "tag": body.Tag, "status": status})
}

// tagNames cleans the names a manager gives a tag: only the site's languages, en among them, each
// a tagName; empty ones are left out.
func tagNames(raw map[string]string) (map[string]string, bool) {
	names := map[string]string{}
	for language, value := range raw {
		if !slices.Contains(tagLanguages, language) {
			return nil, false
		}
		if strings.TrimSpace(value) == "" {
			continue
		}
		name, ok := tagName(value)
		if !ok {
			return nil, false
		}
		names[language] = name
	}
	_, ok := names["en"]
	return names, ok
}

// namesTaken says whether another tag than except already has one of the names.
func namesTaken(ctx context.Context, tx pgx.Tx, names map[string]string, except string) (bool, error) {
	values := make([]string, 0, len(names))
	for _, name := range names {
		values = append(values, strings.ToLower(name))
	}
	var taken bool
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM tags g, jsonb_each_text(g.names) n WHERE g.id<>$2 AND lower(n.value)=ANY($1))`, values, except).Scan(&taken)
	return taken, e
}

func readTag(ctx context.Context, db tagQuerier, id string) (Tag, error) {
	t := Tag{ID: id}
	e := db.QueryRow(ctx, `SELECT g.names,g.status,
 (SELECT count(DISTINCT v.chart_id) FROM chart_tag_votes v JOIN charts c ON c.id=v.chart_id WHERE v.tag_id=g.id AND v.vote=1 AND `+publishedChart+`)
 FROM tags g WHERE g.id=$1`, id).Scan(&t.Names, &t.Status, &t.ChartCount)
	return t, e
}

// tagConflict tells a unique-name violation from other failures: two names differing only in
// case in one tag's languages pass namesTaken and meet the index.
func tagConflict(e error) bool {
	var pg *pgconn.PgError
	return errors.As(e, &pg) && pg.Code == "23505"
}

// createTag answers POST /tags {"names": {"en": "…", "zh-Hans": "…"}}, for the source's managers:
// a new official tag at the catalog's end.
func (s *Server) createTag(w http.ResponseWriter, r *http.Request) {
	v, ok := s.tagManager(w, r, true)
	if !ok {
		return
	}
	var body struct {
		Names map[string]string `json:"names"`
	}
	if !decode(w, r, &body) {
		return
	}
	names, ok := tagNames(body.Names)
	if !ok {
		problem(w, 422, "TAG_NAME_INVALID", "需要英文名，每个名字 1–"+strconv.Itoa(maxTagName)+" 个字符")
		return
	}
	ctx := r.Context()
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		internal(w, e)
		return
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, tagTables); e != nil {
		internal(w, e)
		return
	}
	taken, e := namesTaken(ctx, tx, names, "")
	if e != nil {
		internal(w, e)
		return
	}
	if taken {
		problem(w, 409, "TAG_EXISTS", "已经有这个名字的标签")
		return
	}
	id, e := newTag(ctx, tx, names, tagOfficial, v.User.ID)
	if e == nil {
		e = tx.Commit(ctx)
	}
	if e != nil {
		internal(w, e)
		return
	}
	web.Respond(w, 201, Tag{ID: id, Names: names, Status: tagOfficial})
}

// editTag answers PATCH /tags/{tag} {"names"?, "official"?: true}, for the source's managers: new
// names for the tag, or a pending tag made official without waiting for its players.
func (s *Server) editTag(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.tagManager(w, r, true); !ok {
		return
	}
	var body struct {
		Names    map[string]string `json:"names"`
		Official *bool             `json:"official"`
	}
	if !decode(w, r, &body) {
		return
	}
	var names map[string]string
	if body.Names != nil {
		var ok bool
		if names, ok = tagNames(body.Names); !ok {
			problem(w, 422, "TAG_NAME_INVALID", "需要英文名，每个名字 1–"+strconv.Itoa(maxTagName)+" 个字符")
			return
		}
	}
	if body.Official != nil && !*body.Official {
		problem(w, 422, "TAG_STATUS_INVALID", "正式标签不能改回待定")
		return
	}
	ctx := r.Context()
	id := r.PathValue("tag")
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		internal(w, e)
		return
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, tagTables); e != nil {
		internal(w, e)
		return
	}
	var known bool
	if e = tx.QueryRow(ctx, `SELECT true FROM tags WHERE id=$1 FOR UPDATE`, id).Scan(&known); errors.Is(e, pgx.ErrNoRows) {
		problem(w, 404, "TAG_NOT_FOUND", "标签不存在")
		return
	}
	if e == nil && names != nil {
		var taken bool
		if taken, e = namesTaken(ctx, tx, names, id); e == nil && taken {
			problem(w, 409, "TAG_EXISTS", "已经有这个名字的标签")
			return
		}
		if e == nil {
			_, e = tx.Exec(ctx, `UPDATE tags SET names=$2 WHERE id=$1`, id, names)
		}
	}
	if e == nil && body.Official != nil {
		_, e = tx.Exec(ctx, `UPDATE tags SET status='official' WHERE id=$1`, id)
	}
	var tag Tag
	if e == nil {
		tag, e = readTag(ctx, tx, id)
	}
	if e == nil {
		e = tx.Commit(ctx)
	}
	if tagConflict(e) {
		problem(w, 409, "TAG_EXISTS", "已经有这个名字的标签")
		return
	}
	if e != nil {
		internal(w, e)
		return
	}
	web.Respond(w, 200, tag)
}

// deleteTag answers DELETE /tags/{tag}, for the source's managers: the tag leaves the catalog and
// every chart of both sources, with its votes, reports and rejections.
func (s *Server) deleteTag(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.tagManager(w, r, true); !ok {
		return
	}
	done, e := s.DB.Exec(r.Context(), `DELETE FROM tags WHERE id=$1`, r.PathValue("tag"))
	if e != nil {
		internal(w, e)
		return
	}
	if done.RowsAffected() == 0 {
		problem(w, 404, "TAG_NOT_FOUND", "标签不存在")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// mergeTag answers POST /tags/{tag}/merge {"into": "<tag>"}, for the source's managers: every
// vote, report and rejection of the tag, in both sources, becomes the other tag's, and the tag is
// deleted. A player who voted for both keeps one vote; a chart the other tag was taken off stays
// without it.
func (s *Server) mergeTag(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.tagManager(w, r, true); !ok {
		return
	}
	var body struct {
		Into string `json:"into"`
	}
	if !decode(w, r, &body) {
		return
	}
	from := r.PathValue("tag")
	if body.Into == "" || body.Into == from {
		problem(w, 422, "MERGE_INVALID", "要并入另一个标签")
		return
	}
	ctx := r.Context()
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		internal(w, e)
		return
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, tagTables); e != nil {
		internal(w, e)
		return
	}
	// Both rows locked in one order, whichever way two merges name them.
	rows, e := tx.Query(ctx, `SELECT id FROM tags WHERE id IN ($1,$2) ORDER BY id FOR UPDATE`, from, body.Into)
	var locked []string
	if e == nil {
		locked, e = pgx.CollectRows(rows, pgx.RowTo[string])
	}
	if e != nil {
		internal(w, e)
		return
	}
	if len(locked) != 2 {
		problem(w, 404, "TAG_NOT_FOUND", "标签不存在")
		return
	}
	for _, src := range netdb.Sources {
		votes := pgx.Identifier{src, "chart_tag_votes"}.Sanitize()
		rejections := pgx.Identifier{src, "tag_rejections"}.Sanitize()
		reports := pgx.Identifier{src, "chart_tag_reports"}.Sanitize()
		for _, statement := range []string{
			`INSERT INTO ` + rejections + `(chart_id,tag_id,rejected_by,rejected_at) SELECT chart_id,$2,rejected_by,rejected_at FROM ` + rejections + ` WHERE tag_id=$1
 ON CONFLICT (chart_id,tag_id) DO NOTHING`,
			`INSERT INTO ` + votes + `(chart_id,tag_id,user_id,vote,created_at,updated_at) SELECT chart_id,$2,user_id,vote,created_at,updated_at FROM ` + votes + ` WHERE tag_id=$1
 ON CONFLICT (chart_id,tag_id,user_id) DO UPDATE SET vote=GREATEST(` + votes + `.vote,excluded.vote),updated_at=now()`,
			`INSERT INTO ` + reports + `(chart_id,tag_id,user_id,reason,created_at,status,decided_by,decided_at)
 SELECT chart_id,$2,user_id,reason,created_at,status,decided_by,decided_at FROM ` + reports + ` WHERE tag_id=$1 ON CONFLICT (chart_id,tag_id,user_id) DO NOTHING`,
		} {
			if _, e = tx.Exec(ctx, statement, from, body.Into); e != nil {
				internal(w, e)
				return
			}
		}
		// Where either tag was taken off a chart, the merged one is off it.
		if _, e = tx.Exec(ctx, `UPDATE `+votes+` SET vote=0,updated_at=now() WHERE tag_id=$1 AND vote=1 AND chart_id IN (SELECT chart_id FROM `+rejections+` WHERE tag_id=$1)`, body.Into); e != nil {
			internal(w, e)
			return
		}
	}
	if _, e = tx.Exec(ctx, `DELETE FROM tags WHERE id=$1`, from); e == nil {
		// The votes it took over may be what makes a pending tag official.
		_, e = tx.Exec(ctx, `UPDATE tags SET status='official' WHERE id=$1 AND status='pending' AND (SELECT count(*) FROM (`+votesEverywhere+`) u)>=$2`, body.Into, tagPromotionUsers)
	}
	var tag Tag
	if e == nil {
		tag, e = readTag(ctx, tx, body.Into)
	}
	if e == nil {
		e = tx.Commit(ctx)
	}
	if e != nil {
		internal(w, e)
		return
	}
	web.Respond(w, 200, tag)
}

// GameTag is a tag as the game's catalog names it.
type GameTag struct {
	ID    string            `json:"id"`
	Names map[string]string `json:"names"`
}

// gameTags lists the official tags in the catalog's order.
func gameTags(ctx context.Context, db tagQuerier) ([]GameTag, error) {
	rows, e := db.Query(ctx, `SELECT id,names FROM tags WHERE status='official' ORDER BY position`)
	if e != nil {
		return nil, e
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (GameTag, error) {
		var t GameTag
		return t, row.Scan(&t.ID, &t.Names)
	})
}
