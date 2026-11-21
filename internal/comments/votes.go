package comments

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5"
	"ourtaiko.dev/net/internal/web"
)

// A vote is an upvote (1) or a 何意味 "what?" (-1), which the API and the tables still call a
// downvote; 0 withdraws it. The website shows the two counts side by side.

// VoteResult is the target's tally after the viewer's vote was applied.
type VoteResult struct {
	Score     int `json:"score"`
	Upvotes   int `json:"upvotes"`
	Downvotes int `json:"downvotes"`
	MyVote    int `json:"myVote"`
}

// ReadVote reads the vote a request carries, {"value": 1 | 0 | -1}. It reports false after
// answering.
func ReadVote(p web.Problems, w http.ResponseWriter, r *http.Request) (int, bool) {
	var body struct {
		Value *int `json:"value"`
	}
	if !p.Decode(w, r, &body) {
		return 0, false
	}
	if body.Value == nil || *body.Value < -1 || *body.Value > 1 {
		p.Problem(w, 400, "VOTE_INVALID", "投票值必须为 1、0 或 -1")
		return 0, false
	}
	return *body.Value, true
}

// ApplyVote sets, changes or withdraws (value 0) one user's vote on a row.
func ApplyVote(ctx context.Context, tx pgx.Tx, table, column string, target any, user string, value int) error {
	var e error
	if value == 0 {
		_, e = tx.Exec(ctx, `DELETE FROM `+table+` WHERE `+column+`=$1 AND user_id=$2`, target, user)
	} else {
		_, e = tx.Exec(ctx, `INSERT INTO `+table+`(`+column+`,user_id,value) VALUES($1,$2,$3)
 ON CONFLICT(`+column+`,user_id) DO UPDATE SET value=EXCLUDED.value,voted_at=now() WHERE `+table+`.value<>EXCLUDED.value`, target, user, value)
	}
	return e
}

// Vote is the writer's vote on the comment r's path names.
func (h *Host) Vote(w http.ResponseWriter, r *http.Request) {
	u, ok := h.Writer(w, r)
	if !ok {
		return
	}
	value, ok := ReadVote(h.Problems, w, r)
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
	// Withdrawing a vote stays possible after a comment was deleted.
	if value != 0 && c.gone {
		h.Problems.Problem(w, 409, "COMMENT_DELETED", "评论已删除")
		return
	}
	if e = ApplyVote(ctx, tx, "comment_votes", "comment_id", c.id, u.ID, value); e != nil {
		h.Problems.Internal(w, e)
		return
	}
	result := VoteResult{MyVote: value}
	e = tx.QueryRow(ctx, `SELECT upvotes,downvotes FROM comments WHERE id=$1`, c.id).Scan(&result.Upvotes, &result.Downvotes)
	// The author's own vote is nothing to tell them.
	if e == nil && value == 1 && c.authorID != u.ID {
		e = h.tell(ctx, tx, Event{Kind: Upvoted, Recipient: c.authorID, Thread: c.thread, ThreadID: c.threadID, CommentID: c.id, Upvotes: result.Upvotes})
	}
	if e == nil {
		e = tx.Commit(ctx)
	}
	if e != nil {
		h.Problems.Internal(w, e)
		return
	}
	result.Score = result.Upvotes - result.Downvotes
	web.Respond(w, 200, result)
}
