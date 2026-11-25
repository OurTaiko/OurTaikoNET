package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"ourtaiko.dev/net/internal/web"
)

// TitleNotice is one notice of 称号中心: what a player is told about titles, which the website
// shows as a chat among the player's chats. Kind says what happened and Data is what to show of
// it, as it was then:
//
//   - review_requested, to every title manager: a maker asks for a review. {review, title,
//     userId, userName}: the request, the title as it is, and who asks.
//   - review_approved and review_returned, to the maker: a manager decided their request.
//     {review, title}: the request with the manager's reply, and the title as it is now.
//   - title_removed, to the maker: a manager removed their title. {title}: as it was.
//
// Nobody is told of what they did themselves.
type TitleNotice struct {
	ID        int64           `json:"id"`
	Kind      string          `json:"kind"`
	Data      json.RawMessage `json:"data"`
	Read      bool            `json:"read"`
	CreatedAt time.Time       `json:"createdAt"`
}

// tellOfTitle writes a notice for each of users in tx, the transaction of what it tells of.
func tellOfTitle(ctx context.Context, tx pgx.Tx, users []string, kind string, data any) error {
	if len(users) == 0 {
		return nil
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO title_notices(user_id,kind,data) SELECT u,$2,$3 FROM unnest($1::text[]) u`, users, kind, raw)
	return err
}

const titleNoticePageSize = 30

// titleNotices is GET /api/me/title-notices?page=: the player's notices, newest first, with how
// many there are and how many are unread.
func (s *Server) titleNotices(w http.ResponseWriter, r *http.Request) {
	v, ok := s.required(w, r, false)
	if !ok {
		return
	}
	page, ok := answers.Page(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	var total, unread int
	if err = tx.QueryRow(ctx, `SELECT count(*),count(*) FILTER (WHERE read_at IS NULL) FROM title_notices WHERE user_id=$1`, v.User.ID).Scan(&total, &unread); err != nil {
		internal(w, err)
		return
	}
	rows, err := tx.Query(ctx, `SELECT id,kind,data,read_at IS NOT NULL,created_at FROM title_notices WHERE user_id=$1 ORDER BY id DESC LIMIT $2 OFFSET $3`,
		v.User.ID, titleNoticePageSize, (page-1)*titleNoticePageSize)
	if err != nil {
		internal(w, err)
		return
	}
	defer rows.Close()
	items := []TitleNotice{}
	for rows.Next() {
		var n TitleNotice
		var data []byte
		if err = rows.Scan(&n.ID, &n.Kind, &data, &n.Read, &n.CreatedAt); err != nil {
			internal(w, err)
			return
		}
		n.Data = data
		items = append(items, n)
	}
	if err = rows.Err(); err != nil {
		internal(w, err)
		return
	}
	web.Respond(w, 200, map[string]any{"items": items, "total": total, "unread": unread, "page": page, "pageSize": titleNoticePageSize})
}

// unreadTitleNotices is GET /api/me/title-notices/unread: how many the player has not read.
func (s *Server) unreadTitleNotices(w http.ResponseWriter, r *http.Request) {
	v, ok := s.required(w, r, false)
	if !ok {
		return
	}
	var count int
	if err := s.DB.QueryRow(r.Context(), `SELECT count(*) FROM title_notices WHERE user_id=$1 AND read_at IS NULL`, v.User.ID).Scan(&count); err != nil {
		internal(w, err)
		return
	}
	web.Respond(w, 200, map[string]int{"count": count})
}

// readTitleNotices is POST /api/me/title-notices/read: the player has seen them all.
func (s *Server) readTitleNotices(w http.ResponseWriter, r *http.Request) {
	v, ok := s.required(w, r, true)
	if !ok {
		return
	}
	if _, err := s.DB.Exec(r.Context(), `UPDATE title_notices SET read_at=now() WHERE user_id=$1 AND read_at IS NULL`, v.User.ID); err != nil {
		internal(w, err)
		return
	}
	web.Respond(w, 200, map[string]bool{"ok": true})
}
