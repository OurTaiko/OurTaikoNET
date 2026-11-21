package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"ourtaiko.dev/net/internal/comments"
	"ourtaiko.dev/net/internal/files"
	"ourtaiko.dev/net/internal/netdb"
)

// The comments under a source's songs and 段位 courses are package comments' threads, kept in
// the source's own comments and comment_votes tables. What is of the source is here: what a
// thread hangs on, who may see it, who writes, and whom the source's notices tell.

// The shapes of package comments, by the names this package and its clients know them.
type (
	Comment     = comments.Comment
	CommentPage = comments.Page
	CommentList = comments.Flat
)

var (
	chartThreads = comments.Kind{Column: "chart_id", Table: "charts"}
	danThreads   = comments.Kind{Column: "dan_id", Table: "dan_courses"}
)

// subjectWriteLock is how writers lock a comment's or vote's song or course (comments.WriteLock).
const subjectWriteLock = comments.WriteLock

// commentSubject is what a thread hangs on: a published song or 段位 course.
type commentSubject struct {
	chartID, danID string
	// Who is told about new top-level comments and upvote milestones.
	owners       []string
	commentCount int
}

func (t commentSubject) column() string {
	if t.danID != "" {
		return "dan_id"
	}
	return "chart_id"
}

func (t commentSubject) id() any {
	if t.danID != "" {
		return t.danID
	}
	return t.chartID
}

// A subjectLoader reads the subject in r's path with the given row lock,
// answering 404 itself when there is none.
type subjectLoader func(w http.ResponseWriter, r *http.Request, db rowScanner, lock string) (commentSubject, bool)

type rowScanner = comments.RowScanner

func chartSubject(w http.ResponseWriter, r *http.Request, db rowScanner, lock string) (commentSubject, bool) {
	var t commentSubject
	e := db.QueryRow(r.Context(), `SELECT c.id,c.owner_ids,COALESCE((SELECT comment_count FROM chart_stats WHERE chart_id=c.id),0)
	 FROM charts c WHERE c.id=$1 AND `+publishedChart+` `+lock, r.PathValue("id")).Scan(&t.chartID, &t.owners, &t.commentCount)
	if errors.Is(e, pgx.ErrNoRows) {
		problem(w, 404, "CHART_NOT_FOUND", "作品不存在或已删除")
		return t, false
	}
	if e != nil {
		internal(w, e)
		return t, false
	}
	return t, true
}

func danSubject(w http.ResponseWriter, r *http.Request, db rowScanner, lock string) (commentSubject, bool) {
	var t commentSubject
	id := r.PathValue("id")
	var e error = pgx.ErrNoRows
	if validID(id) {
		var owner string
		e = db.QueryRow(r.Context(), `SELECT d.id,d.owner_id,d.comment_count FROM dan_courses d WHERE d.id=$1 AND d.status='published' `+lock, id).Scan(&t.danID, &owner, &t.commentCount)
		t.owners = []string{owner}
	}
	if errors.Is(e, pgx.ErrNoRows) {
		problem(w, 404, "DAN_NOT_FOUND", "段位不存在")
		return t, false
	}
	if e != nil {
		internal(w, e)
		return t, false
	}
	return t, true
}

// thread is the subject as package comments knows it.
func (t commentSubject) thread() comments.Thread {
	kind := chartThreads
	if t.danID != "" {
		kind = danThreads
	}
	return comments.Thread{Kind: kind, ID: t.id().(string), Owners: t.owners, CommentCount: t.commentCount}
}

// threadOf makes a subject's loader the loader of its comments.
func threadOf(load subjectLoader) comments.Loader {
	return func(w http.ResponseWriter, r *http.Request, db comments.RowScanner, lock string) (comments.Thread, bool) {
		subject, ok := load(w, r, db, lock)
		return subject.thread(), ok
	}
}

// comments is this source as a host of comments.
func (s *Server) comments() *comments.Host {
	return &comments.Host{
		DB:    s.DB,
		Kinds: []comments.Kind{chartThreads, danThreads},
		// A comment m beside its song c or its course d.
		Joins: ` LEFT JOIN charts c ON c.id=m.chart_id LEFT JOIN dan_courses d ON d.id=m.dan_id`,
		// Comments of hidden songs and deleted courses stay out of sight.
		Shown:    netdb.PublishedCommentSubject,
		Title:    `d.title`,
		Problems: answers,
		Viewer:   s.viewer,
		Writer: func(w http.ResponseWriter, r *http.Request) (comments.Writer, bool) {
			u, ok := s.required(w, r, true)
			return comments.Writer{ID: u.User.ID, Nickname: u.User.Nickname, AvatarPath: files.Path(s.avatarOf(u.User)), Moderator: u.User.IsAdmin}, ok
		},
		Profiles: func(ctx context.Context, ids []string) map[string]comments.Profile {
			shown := map[string]comments.Profile{}
			for id, p := range s.publicProfiles(ctx, ids) {
				shown[id] = comments.Profile{Nickname: p.Nickname, AvatarPath: files.Path(p.AvatarURL)}
			}
			return shown
		},
		Tell: tellOfComment,
	}
}

// tellOfComment turns what happened to a comment into the source's notices: a top-level
// comment tells the song's uploaders or the course's maker, a reply the parent's author, a
// removal the comment's author, and an upvote milestone its author too.
func tellOfComment(ctx context.Context, tx pgx.Tx, e comments.Event) error {
	notice := Notice{Recipient: e.Recipient, Actor: e.Actor, CommentID: e.CommentID}
	if e.Thread == danThreads {
		notice.DanID = e.ThreadID
	} else {
		notice.ChartID = e.ThreadID
	}
	switch e.Kind {
	case comments.Posted:
		notice.Kind = "chart_comment"
		if notice.DanID != "" {
			notice.Kind = "dan_comment"
		}
	case comments.Replied:
		notice.Kind = "comment_reply"
	case comments.Removed:
		// The notice outlives the comment, so it does not point at it.
		notice.Kind, notice.CommentID, notice.DedupeKey = "comment_removed", "", "comment_removed:"+e.CommentID
	case comments.Hidden:
		_, err := tx.Exec(ctx, `DELETE FROM notifications WHERE comment_id=$1`, e.CommentID)
		return err
	case comments.Upvoted:
		return notifyUpvoteMilestone(ctx, tx, "comment_upvotes", e.Recipient, notice, e.Upvotes)
	default:
		return errors.New("unknown comment event " + e.Kind)
	}
	return notify(ctx, tx, notice)
}
