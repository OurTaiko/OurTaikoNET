package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"ourtaiko.dev/net/internal/groups"
	"ourtaiko.dev/net/internal/web"
)

// Review is a maker's request that the title managers review a title of theirs: to make a
// private one public (publish), or to change a public one (change). It holds the title as asked
// for (for publish, as it was when asked: a title does not change while its review is pending)
// and Request, what the maker tells the managers: how the title should be obtained, or what the
// change is for. A manager approves it, which copies the name, description and look onto the
// title and makes it public, or returns it; either way with a Reply. Its maker may withdraw it
// while it is pending.
type Review struct {
	ID               string          `json:"id"`
	TitleID          string          `json:"titleId"`
	UserID           string          `json:"userId"`
	Kind             string          `json:"kind"`
	Name             string          `json:"name"`
	NameTranslations json.RawMessage `json:"nameTranslations"`
	Description      string          `json:"description"`
	Layers           Layers          `json:"layers"`
	Request          string          `json:"request"`
	Status           string          `json:"status"`
	Reply            string          `json:"reply"`
	CreatedAt        time.Time       `json:"createdAt"`
	DecidedAt        *time.Time      `json:"decidedAt"`
}

// reviewColumns are a request's columns, read from title_reviews r with its parts joined
// (layerJoins).
var reviewColumns = `r.id,r.title_id,r.user_id,r.kind,r.name,r.name_translations,r.description,` + layerColumns("r") + `,r.request,r.status,r.reply,r.created_at,r.decided_at`

// into is where a row's reviewColumns go.
func (v *Review) into(translations *[]byte) []any {
	into := append([]any{&v.ID, &v.TitleID, &v.UserID, &v.Kind, &v.Name, translations, &v.Description}, v.Layers.into()...)
	return append(into, &v.Request, &v.Status, &v.Reply, &v.CreatedAt, &v.DecidedAt)
}

// requested reads the request a statement writes (INSERT or UPDATE of title_reviews, without
// its RETURNING), with its parts.
func requested(statement string) string {
	return `WITH r AS (` + statement + ` RETURNING *) SELECT ` + reviewColumns + ` FROM r` + layerJoins("r")
}

func scanReview(row rowScanner, extra ...any) (Review, error) {
	var v Review
	var translations []byte
	err := row.Scan(append(v.into(&translations), extra...)...)
	v.NameTranslations = translations
	return v, err
}

// titleManagers are the players who review titles, without except (who is acting). Without user
// groups (Config.Groups) the site's administrator does it, whom the site cannot name.
func (s *Server) titleManagers(ctx context.Context, db queryer, except string) ([]string, error) {
	if !s.Config.Groups {
		return nil, nil
	}
	members, err := groups.In(ctx, db, groups.TitleManager)
	if err != nil {
		return nil, err
	}
	managers := []string{}
	for _, id := range members {
		if id != except {
			managers = append(managers, id)
		}
	}
	return managers, nil
}

// requestReview is POST /api/me/titles/{id}/reviews: the maker asks for a review of their title.
// Of a private one, {request}: that it be made public as it is. Of a public one, {request, name,
// nameTranslations, description, layers}: that it be changed to this. One request at a time for
// a title. The title managers are told in 称号中心.
func (s *Server) requestReview(w http.ResponseWriter, r *http.Request) {
	v, ok := s.required(w, r, true)
	if !ok {
		return
	}
	var body struct {
		titleBody
		Request string `json:"request"`
	}
	if !decode(w, r, &body) {
		return
	}
	request, fits := note(body.Request)
	if !fits || request == "" {
		problem(w, 422, "REVIEW_INVALID", "请填写 1–500 字的申请说明")
		return
	}
	ctx := r.Context()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	t, pending, err := lockOwnTitle(ctx, tx, r.PathValue("id"), v.User.ID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		problem(w, 404, "TITLE_NOT_FOUND", "称号不存在")
		return
	case err != nil:
		internal(w, err)
		return
	case pending:
		problem(w, 409, "TITLE_REVIEW_PENDING", "这个称号已有一份申请在审核中")
		return
	}
	kind, name, translations, description, layers := "publish", t.Name, []byte(t.NameTranslations), t.Description, t.Layers
	if t.Visibility == public {
		var valid, looks bool
		translations, valid = body.text()
		layers, looks = body.look()
		if !valid || !looks {
			problem(w, 422, "TITLE_INVALID", "称号信息无效")
			return
		}
		if _, ok := s.wearable(w, r, tx, layers); !ok {
			return
		}
		kind, name, description = "change", body.Name, body.Description
	} else if body.Layers != nil || body.Name != "" || body.Description != "" || len(body.NameTranslations) > 0 {
		problem(w, 422, "REVIEW_INVALID", "私有称号按现在的样子提审；要改先修改称号")
		return
	}
	review, err := scanReview(tx.QueryRow(ctx, requested(`INSERT INTO title_reviews AS r(id,title_id,user_id,kind,name,name_translations,description,band_id,effect_id,icon_id,effect_overflow,request)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`),
		web.NewID(), t.ID, v.User.ID, kind, name, translations, description, layers.IDs.Band, layers.IDs.Effect, layers.IDs.Icon, layers.EffectOverflow, request))
	if err != nil {
		internal(w, err)
		return
	}
	managers, err := s.titleManagers(ctx, tx, v.User.ID)
	if err == nil {
		err = tellOfTitle(ctx, tx, managers, "review_requested", map[string]any{
			"review": review, "title": t, "userId": v.User.ID, "userName": v.User.Nickname})
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		internal(w, err)
		return
	}
	web.Respond(w, 201, review)
}

// withdrawReview is DELETE /api/me/title-reviews/{id}: the maker takes back a request that is
// still pending. Their title can be changed again.
func (s *Server) withdrawReview(w http.ResponseWriter, r *http.Request) {
	v, ok := s.required(w, r, true)
	if !ok {
		return
	}
	tag, err := s.DB.Exec(r.Context(), `UPDATE title_reviews SET status='withdrawn',decided_at=now() WHERE id=$1 AND user_id=$2 AND status='pending'`, r.PathValue("id"), v.User.ID)
	if err != nil {
		internal(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		problem(w, 404, "REVIEW_NOT_FOUND", "没有这份待审核的申请")
		return
	}
	web.Respond(w, 200, map[string]bool{"ok": true})
}

// QueuedReview is a review as the title managers see it: with who asked and who decided, by the
// nicknames the site keeps, and the title as it is now, to compare a change against.
type QueuedReview struct {
	Review
	UserName     string `json:"userName"`
	ReviewerName string `json:"reviewerName"`
	Title        Title  `json:"title"`
}

const reviewPageSize = 20

// listReviews is GET /api/admin/title-reviews?status=pending|decided&page=: the requests waiting
// for a title manager, oldest first, or the ones decided or withdrawn, newest first.
func (s *Server) listReviews(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.admin(w, r); !ok {
		return
	}
	page, ok := answers.Page(w, r)
	if !ok {
		return
	}
	var filter, order string
	switch r.URL.Query().Get("status") {
	case "", "pending":
		filter, order = `r.status='pending'`, `r.created_at,r.id`
	case "decided":
		filter, order = `r.status<>'pending'`, `r.decided_at DESC,r.id`
	default:
		problem(w, 400, "QUERY_INVALID", "status 必须为 pending / decided")
		return
	}
	ctx := r.Context()
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	var total int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM title_reviews r WHERE `+filter).Scan(&total); err != nil {
		internal(w, err)
		return
	}
	rows, err := tx.Query(ctx, `SELECT `+titleColumns+`,`+reviewColumns+`,COALESCE(u.nickname,''),COALESCE(d.nickname,'')
 FROM title_reviews r`+layerJoins("r")+` JOIN titles t ON t.id=r.title_id`+layerJoins("t")+` LEFT JOIN users u ON u.id=r.user_id LEFT JOIN users d ON d.id=r.reviewer_id
 WHERE `+filter+` ORDER BY `+order+` LIMIT $1 OFFSET $2`, reviewPageSize, (page-1)*reviewPageSize)
	if err != nil {
		internal(w, err)
		return
	}
	defer rows.Close()
	items := []QueuedReview{}
	for rows.Next() {
		var q QueuedReview
		var translations []byte
		if q.Title, err = scanTitle(rows, append(q.Review.into(&translations), &q.UserName, &q.ReviewerName)...); err != nil {
			internal(w, err)
			return
		}
		q.NameTranslations = translations
		items = append(items, q)
	}
	if err = rows.Err(); err != nil {
		internal(w, err)
		return
	}
	web.Respond(w, 200, map[string]any{"items": items, "total": total, "page": page, "pageSize": reviewPageSize})
}

// decideReview is POST /api/admin/title-reviews/{id} {approve, reply, obtain}: a title manager
// approves a pending request or returns it, with a reply to its maker. Approving makes the title
// what was asked for and public, with obtain as the words the catalog gives for how it is
// obtained (the manager's, written from the request); what gives it to players (rules, grants)
// the managers set afterwards. The maker is told in 称号中心.
func (s *Server) decideReview(w http.ResponseWriter, r *http.Request) {
	v, ok := s.admin(w, r)
	if !ok {
		return
	}
	var body struct {
		Approve *bool  `json:"approve"`
		Reply   string `json:"reply"`
		Obtain  string `json:"obtain"`
	}
	if !decode(w, r, &body) {
		return
	}
	reply, replyFits := note(body.Reply)
	obtain, obtainFits := note(body.Obtain)
	if body.Approve == nil || !replyFits || !obtainFits {
		problem(w, 422, "REVIEW_INVALID", "需要 approve，回复和获取方式各至多 500 字")
		return
	}
	ctx := r.Context()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	review, err := scanReview(tx.QueryRow(ctx, `SELECT `+reviewColumns+` FROM title_reviews r`+layerJoins("r")+` WHERE r.id=$1 FOR UPDATE OF r`, r.PathValue("id")))
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		problem(w, 404, "REVIEW_NOT_FOUND", "没有这份申请")
		return
	case err != nil:
		internal(w, err)
		return
	case review.Status != "pending":
		problem(w, 409, "REVIEW_DECIDED", "这份申请已经处理过了")
		return
	}
	var t Title
	status, kind := "returned", "review_returned"
	if *body.Approve {
		status, kind = "approved", "review_approved"
		t, err = scanTitle(tx.QueryRow(ctx, written(`UPDATE titles t SET name=$2,name_translations=$3,description=$4,style=$5,band_id=$6,effect_id=$7,icon_id=$8,effect_overflow=$9,visibility='public',obtain=$10
 WHERE t.id=$1`), review.TitleID, review.Name, []byte(review.NameTranslations), review.Description,
			legacyStyle(review.Layers.Band), review.Layers.IDs.Band, review.Layers.IDs.Effect, review.Layers.IDs.Icon, review.Layers.EffectOverflow, obtain))
	} else {
		t, err = scanTitle(tx.QueryRow(ctx, `SELECT `+titleColumns+titlesFrom+` WHERE t.id=$1`, review.TitleID))
	}
	if err == nil {
		review, err = scanReview(tx.QueryRow(ctx, requested(`UPDATE title_reviews r SET status=$2,reviewer_id=$3,reply=$4,decided_at=now() WHERE r.id=$1`),
			review.ID, status, v.User.ID, reply))
	}
	if err == nil && review.UserID != v.User.ID {
		err = tellOfTitle(ctx, tx, []string{review.UserID}, kind, map[string]any{"review": review, "title": t})
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		internal(w, err)
		return
	}
	web.Respond(w, 200, map[string]any{"review": review, "title": t})
}
