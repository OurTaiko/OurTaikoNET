// Package comments is the threaded comments of the site: replies under replies, an upvote or a
// 何意味 on each, five orders, editing, deleting and an administrator's removal. Whatever has
// comments keeps them in tables of its own, named comments and comment_votes in its schema, and
// describes itself as a Host; each song source is one (comments under songs and 段位 courses),
// and the site itself another (comments under announcements).
package comments

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"ourtaiko.dev/net/internal/account"
	"ourtaiko.dev/net/internal/web"
)

const (
	maxRunes  = 10000
	maxDepth  = 999
	pageSize  = 20
	maxThread = 1500
)

// Burst is how many comments one user may post per minute, against reply floods.
const Burst = 8

// WriteLock is how writers lock what a comment or a vote hangs on. Votes and comments also
// change that row (a song's update_at, a course's or an announcement's tallies), so every
// writer takes the row lock it will need up front: two writers holding FOR SHARE and both
// upgrading would deadlock.
const WriteLock = "FOR NO KEY UPDATE"

// Comment is one node of a thread. A comment deleted by its author or removed by an
// administrator keeps its place while replies remain, without author or text.
type Comment struct {
	ID string `json:"id"`
	// What the comment hangs on: one of these, the others empty. A song is named by its chart,
	// which the website reads for its names in each language.
	ChartID          string            `json:"chartId,omitempty"`
	DanID            string            `json:"danId,omitempty"`
	DanTitle         map[string]string `json:"danTitle,omitempty"`
	AnnouncementID   string            `json:"announcementId,omitempty"`
	ParentID         *string           `json:"parentId"`
	Depth            int               `json:"depth"`
	AuthorID         string            `json:"authorId"`
	Author           string            `json:"author"`
	AuthorAvatarPath string            `json:"authorAvatarPath"`
	Body             string            `json:"body"`
	CreatedAt        time.Time         `json:"createdAt"`
	EditedAt         *time.Time        `json:"editedAt"`
	Deleted          bool              `json:"deleted"`
	Removed          bool              `json:"removed"`
	Score            int               `json:"score"`
	Upvotes          int               `json:"upvotes"`
	Downvotes        int               `json:"downvotes"`
	MyVote           int               `json:"myVote"`
	ReplyCount       int               `json:"replyCount"`
	Replies          []*Comment        `json:"replies"`
}

// Kind is one kind of thing a host's comments hang on.
type Kind struct {
	// Column of the host's comments table that names it, such as chart_id.
	Column string
	// Table whose row a writer locks (WriteLock) before the comment.
	Table string
}

// Thread is what the comments of one request hang on.
type Thread struct {
	Kind Kind
	ID   string
	// Who is told about new top-level comments.
	Owners       []string
	CommentCount int
}

// RowScanner is a pool or a transaction, for reading one row.
type RowScanner interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

// A Loader reads the thread r's path names with the given row lock, answering 404 itself
// when there is none.
type Loader func(w http.ResponseWriter, r *http.Request, db RowScanner, lock string) (Thread, bool)

// Writer is the signed-in player who writes, edits, deletes or votes.
type Writer struct {
	ID, Nickname, AvatarPath string
	// Moderator may remove other players' comments.
	Moderator bool
}

// Profile is what is shown of a comment's author.
type Profile struct {
	Nickname, AvatarPath string
}

// What a host may want to tell players about.
const (
	// Posted: a top-level comment, to each owner of the thread in turn.
	Posted = "posted"
	// Replied: a reply, to the author of the comment replied to.
	Replied = "replied"
	// Removed: an administrator removed the comment, to its author; no actor is named.
	Removed = "removed"
	// Hidden: the comment lost its text but keeps its place; what was said about it is stale.
	Hidden = "hidden"
	// Upvoted: the comment now has Upvotes upvotes, to its author.
	Upvoted = "upvoted"
)

// Event is something that happened to a comment, inside the transaction that did it.
type Event struct {
	Kind             string
	Recipient, Actor string
	Thread           Kind
	ThreadID         string
	CommentID        string
	Upvotes          int
}

// Host is one place that has comments: its tables are the comments and comment_votes its
// pool's search_path finds.
type Host struct {
	DB *pgxpool.Pool
	// Kinds are everything its comments hang on; each is a nullable column of its comments
	// table, of which exactly one is set.
	Kinds []Kind
	// Joins joins a comment m to what Shown and Title read; Shown says the thing commented on
	// is on show, so comments of what was hidden or deleted stay out of sight. Title, when not
	// empty, is a column with a 段位 course's titles in each language.
	Joins, Shown, Title string
	// Problems is how the host answers a request it cannot serve.
	Problems web.Problems
	// Viewer is the signed-in player by the website cookie alone, "" for a guest: only for
	// showing them their own votes.
	Viewer func(*http.Request) string
	// Writer is the verified signed-in player, CSRF checked; it answers itself when there is
	// none.
	Writer func(http.ResponseWriter, *http.Request) (Writer, bool)
	// Profiles are the public names and pictures of the given players.
	Profiles func(context.Context, []string) map[string]Profile
	// Tell hears of each Event; nil when the host tells nobody.
	Tell func(context.Context, pgx.Tx, Event) error
}

func (h *Host) tell(ctx context.Context, tx pgx.Tx, e Event) error {
	if h.Tell == nil {
		return nil
	}
	return h.Tell(ctx, tx, e)
}

// columns are what scan reads of a comment m: the viewer is always $1 so every query can
// report the viewer's own vote. With title, the host's Title follows.
func (h *Host) columns(title bool) string {
	list := `m.id,m.parent_id,m.depth,m.author_id,m.body,m.created_at,m.edited_at,
 m.deleted_at IS NOT NULL,m.removed_at IS NOT NULL,m.upvotes,m.downvotes,m.reply_count,
 COALESCE((SELECT value FROM comment_votes v WHERE v.comment_id=m.id AND v.user_id=$1),0)`
	for _, kind := range h.Kinds {
		list += `,m.` + kind.Column
	}
	if title && h.Title != "" {
		list += `,` + h.Title
	}
	return list
}

// setThread fills the field of what the comment hangs on.
func (c *Comment) setThread(column, id string) {
	switch column {
	case "chart_id":
		c.ChartID = id
	case "dan_id":
		c.DanID = id
	case "announcement_id":
		c.AnnouncementID = id
	}
}

type rowQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

// read scans rows of columns(title).
func (h *Host) read(ctx context.Context, db rowQuerier, title bool, sql string, args ...any) ([]*Comment, error) {
	rows, e := db.Query(ctx, sql, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	items := []*Comment{}
	for rows.Next() {
		c := &Comment{Replies: []*Comment{}}
		threads := make([]*string, len(h.Kinds))
		var danTitle []byte
		dest := []any{&c.ID, &c.ParentID, &c.Depth, &c.AuthorID, &c.Body, &c.CreatedAt, &c.EditedAt, &c.Deleted, &c.Removed, &c.Upvotes, &c.Downvotes, &c.ReplyCount, &c.MyVote}
		for i := range threads {
			dest = append(dest, &threads[i])
		}
		if title && h.Title != "" {
			dest = append(dest, &danTitle)
		}
		if e = rows.Scan(dest...); e != nil {
			return nil, e
		}
		c.Score = c.Upvotes - c.Downvotes
		for i, id := range threads {
			if id != nil {
				c.setThread(h.Kinds[i].Column, *id)
			}
		}
		if len(danTitle) > 0 {
			json.Unmarshal(danTitle, &c.DanTitle)
		}
		if c.Deleted || c.Removed {
			c.AuthorID, c.Body = "", ""
		}
		items = append(items, c)
	}
	return items, rows.Err()
}

var orders = map[string]string{
	"best":          `comment_confidence(m.upvotes,m.downvotes) DESC,m.created_at DESC,m.id`,
	"top":           `m.upvotes-m.downvotes DESC,m.created_at DESC,m.id`,
	"new":           `m.created_at DESC,m.id`,
	"old":           `m.created_at,m.id`,
	"controversial": `comment_controversy(m.upvotes,m.downvotes) DESC,m.created_at DESC,m.id`,
}

func (h *Host) order(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	sort := r.URL.Query().Get("sort")
	if sort == "" {
		sort = "best"
	}
	order, ok := orders[sort]
	if !ok {
		h.Problems.Problem(w, 400, "QUERY_INVALID", "排序方式必须为 best / top / new / old / controversial")
	}
	return sort, order, ok
}

// attach hangs descendants (ordered by depth, then rank) below the given nodes.
// A reply whose parent was cut off by the thread limit is left for its permalink.
func attach(roots []*Comment, descendants []*Comment) {
	nodes := map[string]*Comment{}
	for _, c := range roots {
		nodes[c.ID] = c
	}
	for _, c := range descendants {
		if parent := nodes[*c.ParentID]; parent != nil {
			parent.Replies = append(parent.Replies, c)
			nodes[c.ID] = c
		}
	}
}

// fillAuthors adds public nicknames and avatars to every node of the trees.
func (h *Host) fillAuthors(ctx context.Context, items []*Comment) {
	all := []*Comment{}
	var walk func([]*Comment)
	walk = func(list []*Comment) {
		for _, c := range list {
			all = append(all, c)
			walk(c.Replies)
		}
	}
	walk(items)
	ids := []string{}
	for _, c := range all {
		if c.AuthorID != "" {
			ids = append(ids, c.AuthorID)
		}
	}
	if len(ids) == 0 {
		return
	}
	profiles := h.Profiles(ctx, ids)
	for _, c := range all {
		if c.AuthorID != "" {
			p := profiles[c.AuthorID]
			c.Author, c.AuthorAvatarPath = p.Nickname, p.AvatarPath
		}
	}
}

// body normalizes line endings and surrounding space and rejects empty, overlong or
// control-character text.
func (h *Host) body(w http.ResponseWriter, raw string) (string, bool) {
	body := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(raw, "\r\n", "\n"), "\r", "\n"))
	valid := body != "" && utf8.ValidString(body) && utf8.RuneCountInString(body) <= maxRunes
	for _, c := range body {
		if valid && unicode.IsControl(c) && c != '\n' && c != '\t' {
			valid = false
		}
	}
	if !valid {
		h.Problems.Problem(w, 400, "COMMENT_INVALID", "评论需为 1–10000 字的文本")
	}
	return body, valid
}

type state struct {
	id, authorID   string
	thread         Kind
	threadID       string
	parentID       *string
	depth, replies int
	gone           bool
}

// lock loads the comment r's path names, of something on show, with the given row lock,
// after locking what it hangs on the way writers do, so a comment's writers and a reply's
// (the thread, then the parent comment) always lock in one order.
func (h *Host) lock(w http.ResponseWriter, r *http.Request, tx pgx.Tx, lock string) (state, bool) {
	var c state
	ctx := r.Context()
	names := make([]string, len(h.Kinds))
	threads := make([]*string, len(h.Kinds))
	dest := make([]any, len(h.Kinds))
	for i, kind := range h.Kinds {
		names[i], dest[i] = kind.Column, &threads[i]
	}
	// A comment never moves to another thread, so this read needs no lock.
	e := tx.QueryRow(ctx, `SELECT `+strings.Join(names, ",")+` FROM comments WHERE id=$1`, r.PathValue("id")).Scan(dest...)
	for i, id := range threads {
		if e == nil && id != nil {
			c.thread, c.threadID = h.Kinds[i], *id
			_, e = tx.Exec(ctx, `SELECT 1 FROM `+c.thread.Table+` WHERE id=$1 `+WriteLock, c.threadID)
		}
	}
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		h.Problems.Internal(w, e)
		return c, false
	}
	e = tx.QueryRow(ctx, `SELECT m.id,m.author_id,m.parent_id,m.depth,m.reply_count,m.deleted_at IS NOT NULL OR m.removed_at IS NOT NULL
 FROM comments m`+h.Joins+` WHERE m.id=$1 AND `+h.Shown+` `+lock, r.PathValue("id")).
		Scan(&c.id, &c.authorID, &c.parentID, &c.depth, &c.replies, &c.gone)
	if errors.Is(e, pgx.ErrNoRows) {
		h.Problems.Problem(w, 404, "COMMENT_NOT_FOUND", "评论不存在或已删除")
		return c, false
	}
	if e != nil {
		h.Problems.Internal(w, e)
		return c, false
	}
	return c, true
}

// Page is one page of a thread's top-level comments with their reply trees.
type Page struct {
	Items        []*Comment `json:"items"`
	Total        int        `json:"total"`
	CommentCount int        `json:"commentCount"`
	Page         int        `json:"page"`
	PageSize     int        `json:"pageSize"`
	Sort         string     `json:"sort"`
}

// List answers one page of the top-level comments of the thread load reads, with their
// reply trees, in the order ?sort= names.
func (h *Host) List(load Loader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sort, order, ok := h.order(w, r)
		if !ok {
			return
		}
		page, ok := h.Problems.Page(w, r)
		if !ok {
			return
		}
		ctx := r.Context()
		viewer := h.Viewer(r)
		// One snapshot keeps the counts, the page and its replies consistent.
		tx, e := h.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
		if e != nil {
			h.Problems.Internal(w, e)
			return
		}
		defer tx.Rollback(ctx)
		result := Page{Page: page, PageSize: pageSize, Sort: sort}
		thread, ok := load(w, r, tx, "")
		if !ok {
			return
		}
		result.CommentCount = thread.CommentCount
		column := thread.Kind.Column
		if e = tx.QueryRow(ctx, `SELECT count(*) FROM comments WHERE `+column+`=$1 AND parent_id IS NULL`, thread.ID).Scan(&result.Total); e != nil {
			h.Problems.Internal(w, e)
			return
		}
		result.Items, e = h.read(ctx, tx, false, `SELECT `+h.columns(false)+` FROM comments m
 WHERE m.`+column+`=$2 AND m.parent_id IS NULL ORDER BY `+order+` LIMIT $3 OFFSET $4`, viewer, thread.ID, pageSize, (page-1)*pageSize)
		if e != nil {
			h.Problems.Internal(w, e)
			return
		}
		if len(result.Items) > 0 {
			roots := make([]string, len(result.Items))
			for i, c := range result.Items {
				roots[i] = c.ID
			}
			replies, e := h.read(ctx, tx, false, `SELECT `+h.columns(false)+` FROM comments m
 WHERE m.root_id=ANY($2) AND m.parent_id IS NOT NULL ORDER BY m.depth,`+order+` LIMIT $3`, viewer, roots, maxThread)
			if e != nil {
				h.Problems.Internal(w, e)
				return
			}
			attach(result.Items, replies)
		}
		if e = tx.Commit(ctx); e != nil {
			h.Problems.Internal(w, e)
			return
		}
		h.fillAuthors(ctx, result.Items)
		web.Respond(w, 200, result)
	}
}

// One is a comment's permalink: the comment and all replies below it.
func (h *Host) One(w http.ResponseWriter, r *http.Request) {
	sort, order, ok := h.order(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	viewer := h.Viewer(r)
	tx, e := h.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		h.Problems.Internal(w, e)
		return
	}
	defer tx.Rollback(ctx)
	items, e := h.read(ctx, tx, true, `SELECT `+h.columns(true)+` FROM comments m`+h.Joins+`
 WHERE m.id=$2 AND `+h.Shown, viewer, r.PathValue("id"))
	if e != nil {
		h.Problems.Internal(w, e)
		return
	}
	if len(items) == 0 {
		h.Problems.Problem(w, 404, "COMMENT_NOT_FOUND", "评论不存在或已删除")
		return
	}
	replies, e := h.read(ctx, tx, false, `WITH RECURSIVE below(id) AS (
 SELECT id FROM comments WHERE parent_id=$2
 UNION ALL SELECT c.id FROM comments c JOIN below b ON c.parent_id=b.id)
 SELECT `+h.columns(false)+` FROM comments m WHERE m.id IN (SELECT id FROM below) ORDER BY m.depth,`+order+` LIMIT $3`, viewer, items[0].ID, maxThread)
	if e != nil {
		h.Problems.Internal(w, e)
		return
	}
	if e = tx.Commit(ctx); e != nil {
		h.Problems.Internal(w, e)
		return
	}
	attach(items, replies)
	h.fillAuthors(ctx, items)
	web.Respond(w, 200, map[string]any{"item": items[0], "sort": sort})
}

// Create posts a comment, or with parentId a reply, on the thread load reads.
func (h *Host) Create(load Loader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := h.Writer(w, r)
		if !ok {
			return
		}
		var req struct {
			Body     string  `json:"body"`
			ParentID *string `json:"parentId"`
		}
		if !h.Problems.DecodeLimit(w, r, &req, 65536) {
			return
		}
		body, ok := h.body(w, req.Body)
		if !ok {
			return
		}
		ctx := r.Context()
		tx, e := h.DB.Begin(ctx)
		if e != nil {
			h.Problems.Internal(w, e)
			return
		}
		defer tx.Rollback(ctx)
		// The lock also serializes with a deletion of the thing commented on, which locks its
		// row FOR UPDATE.
		thread, ok := load(w, r, tx, WriteLock)
		if !ok {
			return
		}
		column := thread.Kind.Column
		// Serialize each author's posts so the burst limit cannot be raced.
		if _, e = tx.Exec(ctx, `SELECT 1 FROM users WHERE id=$1 FOR UPDATE`, u.ID); e != nil {
			h.Problems.Internal(w, e)
			return
		}
		var recent int
		if e = tx.QueryRow(ctx, `SELECT count(*) FROM comments WHERE author_id=$1 AND created_at>now()-interval '1 minute'`, u.ID).Scan(&recent); e != nil {
			h.Problems.Internal(w, e)
			return
		}
		if recent >= Burst {
			w.Header().Set("Retry-After", "60")
			h.Problems.Problem(w, 429, "RATE_LIMITED", "评论过于频繁，请稍后再试")
			return
		}
		id, depth, root := web.NewID(), 0, ""
		// Top-level comments are told to the thread's owners, replies to the parent's author.
		event := Event{Kind: Posted, Actor: u.ID, Thread: thread.Kind, ThreadID: thread.ID, CommentID: id}
		if req.ParentID != nil {
			var parentThread *string
			var gone bool
			// The parent row is updated by the reply counter, so lock it exclusively.
			e = tx.QueryRow(ctx, `SELECT `+column+`,author_id,root_id,depth,deleted_at IS NOT NULL OR removed_at IS NOT NULL FROM comments WHERE id=$1 FOR UPDATE`, *req.ParentID).
				Scan(&parentThread, &event.Recipient, &root, &depth, &gone)
			event.Kind = Replied
			if errors.Is(e, pgx.ErrNoRows) || (e == nil && (parentThread == nil || *parentThread != thread.ID)) {
				h.Problems.Problem(w, 404, "COMMENT_NOT_FOUND", "回复的评论不存在或已删除")
				return
			}
			if e != nil {
				h.Problems.Internal(w, e)
				return
			}
			if gone {
				h.Problems.Problem(w, 409, "COMMENT_DELETED", "不能回复已删除的评论")
				return
			}
			if depth >= maxDepth {
				h.Problems.Problem(w, 400, "COMMENT_TOO_DEEP", "回复层级过深")
				return
			}
			depth++
		} else {
			root = id
		}
		_, e = tx.Exec(ctx, `INSERT INTO comments(id,`+column+`,parent_id,root_id,depth,author_id,body) VALUES($1,$2,$3,$4,$5,$6,$7)`, id, thread.ID, req.ParentID, root, depth, u.ID, body)
		if e != nil {
			h.Problems.Internal(w, e)
			return
		}
		// As on Reddit, an author's own comment starts upvoted by them.
		if _, e = tx.Exec(ctx, `INSERT INTO comment_votes(comment_id,user_id,value) VALUES($1,$2,1)`, id, u.ID); e != nil {
			h.Problems.Internal(w, e)
			return
		}
		if req.ParentID == nil {
			for _, owner := range thread.Owners {
				event.Recipient = owner
				if e = h.tell(ctx, tx, event); e != nil {
					break
				}
			}
		} else {
			e = h.tell(ctx, tx, event)
		}
		if e != nil {
			h.Problems.Internal(w, e)
			return
		}
		h.answerOwn(w, r, tx, u, id, 201)
	}
}

// answerOwn commits and answers the writer's own comment as it now reads.
func (h *Host) answerOwn(w http.ResponseWriter, r *http.Request, tx pgx.Tx, u Writer, id string, status int) {
	ctx := r.Context()
	items, e := h.read(ctx, tx, false, `SELECT `+h.columns(false)+` FROM comments m WHERE m.id=$2`, u.ID, id)
	if e == nil {
		e = tx.Commit(ctx)
	}
	if e != nil {
		h.Problems.Internal(w, e)
		return
	}
	items[0].Author, items[0].AuthorAvatarPath = u.Nickname, u.AvatarPath
	web.Respond(w, status, items[0])
}

// Edit changes the text of the writer's own comment.
func (h *Host) Edit(w http.ResponseWriter, r *http.Request) {
	u, ok := h.Writer(w, r)
	if !ok {
		return
	}
	var req struct {
		Body string `json:"body"`
	}
	if !h.Problems.DecodeLimit(w, r, &req, 65536) {
		return
	}
	body, ok := h.body(w, req.Body)
	if !ok {
		return
	}
	ctx := r.Context()
	tx, e := h.DB.Begin(ctx)
	if e != nil {
		h.Problems.Internal(w, e)
		return
	}
	defer tx.Rollback(ctx)
	c, ok := h.lock(w, r, tx, "FOR UPDATE OF m")
	if !ok {
		return
	}
	if c.gone {
		h.Problems.Problem(w, 409, "COMMENT_DELETED", "评论已删除")
		return
	}
	if c.authorID != u.ID {
		h.Problems.Problem(w, 403, "FORBIDDEN", "只能编辑自己的评论")
		return
	}
	if _, e = tx.Exec(ctx, `UPDATE comments SET body=$2,edited_at=now() WHERE id=$1 AND body<>$2`, c.id, body); e != nil {
		h.Problems.Internal(w, e)
		return
	}
	h.answerOwn(w, r, tx, u, c.id, 200)
}

// Delete lets an author delete, or a moderator remove, a comment. Without replies the row
// disappears, together with any placeholder ancestors that only remained for this branch;
// otherwise it becomes a placeholder.
func (h *Host) Delete(w http.ResponseWriter, r *http.Request) {
	u, ok := h.Writer(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	tx, e := h.DB.Begin(ctx)
	if e != nil {
		h.Problems.Internal(w, e)
		return
	}
	defer tx.Rollback(ctx)
	c, ok := h.lock(w, r, tx, "FOR UPDATE OF m")
	if !ok {
		return
	}
	column := "deleted_at"
	if c.authorID != u.ID {
		if !u.Moderator {
			h.Problems.Problem(w, 403, "FORBIDDEN", "只能删除自己的评论")
			return
		}
		column = "removed_at"
	}
	event := Event{Thread: c.thread, ThreadID: c.threadID, CommentID: c.id}
	purged := false
	if c.gone {
		// Already a placeholder: repeating the request changes nothing.
	} else if c.replies > 0 {
		_, e = tx.Exec(ctx, `UPDATE comments SET body='',`+column+`=now() WHERE id=$1`, c.id)
		// A purged row takes what was said about it along; a placeholder's is dropped here.
		if e == nil {
			event.Kind = Hidden
			e = h.tell(ctx, tx, event)
		}
	} else {
		purged = true
		_, e = tx.Exec(ctx, `DELETE FROM comments WHERE id=$1`, c.id)
		for parent := c.parentID; e == nil && parent != nil; {
			var next *string
			e = tx.QueryRow(ctx, `DELETE FROM comments WHERE id=$1 AND reply_count=0
 AND (deleted_at IS NOT NULL OR removed_at IS NOT NULL) RETURNING parent_id`, *parent).Scan(&next)
			if errors.Is(e, pgx.ErrNoRows) {
				e = nil
				break
			}
			parent = next
		}
	}
	// The author learns of a removal, but not which moderator removed it.
	if e == nil && !c.gone && column == "removed_at" {
		event.Kind, event.Recipient = Removed, c.authorID
		e = h.tell(ctx, tx, event)
	}
	if e == nil {
		e = tx.Commit(ctx)
	}
	if e != nil {
		h.Problems.Internal(w, e)
		return
	}
	web.Respond(w, 200, map[string]bool{"ok": true, "purged": purged})
}

// Flat is a page of comments out of their threads, such as one player's.
type Flat struct {
	Items    []*Comment `json:"items"`
	Total    int        `json:"total"`
	Page     int        `json:"page"`
	PageSize int        `json:"pageSize"`
}

// ByAuthor is the public comment history of the player r's path names, newest (?sort=new) or
// top first: live comments on things on show.
func (h *Host) ByAuthor(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !account.ValidSubject(id) {
		h.Problems.Problem(w, 404, "USER_NOT_FOUND", "用户不存在")
		return
	}
	order := "m.created_at DESC,m.id"
	switch r.URL.Query().Get("sort") {
	case "", "new":
	case "top":
		order = "m.upvotes-m.downvotes DESC,m.created_at DESC,m.id"
	default:
		h.Problems.Problem(w, 400, "QUERY_INVALID", "排序方式必须为 new / top")
		return
	}
	page, ok := h.Problems.Page(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	viewer := h.Viewer(r)
	from := ` FROM comments m` + h.Joins + ` WHERE m.author_id=$2 AND m.deleted_at IS NULL AND m.removed_at IS NULL AND ` + h.Shown
	tx, e := h.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		h.Problems.Internal(w, e)
		return
	}
	defer tx.Rollback(ctx)
	result := Flat{Page: page, PageSize: pageSize}
	// The count does not report the viewer's votes, so the author becomes $1.
	if e = tx.QueryRow(ctx, `SELECT count(*)`+strings.ReplaceAll(from, "$2", "$1"), id).Scan(&result.Total); e != nil {
		h.Problems.Internal(w, e)
		return
	}
	result.Items, e = h.read(ctx, tx, true, `SELECT `+h.columns(true)+from+` ORDER BY `+order+` LIMIT $3 OFFSET $4`, viewer, id, pageSize, (page-1)*pageSize)
	if e == nil {
		e = tx.Commit(ctx)
	}
	if e != nil {
		h.Problems.Internal(w, e)
		return
	}
	h.fillAuthors(ctx, result.Items)
	web.Respond(w, 200, result)
}
