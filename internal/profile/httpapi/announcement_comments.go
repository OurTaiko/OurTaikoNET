package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"ourtaiko.dev/net/internal/comments"
	"ourtaiko.dev/net/internal/files"
)

// The comments under an announcement are package comments' threads, as under a song of a
// song source: replies, votes, orders, editing and removal are that package's. They are kept in
// net.comments and net.comment_votes, which this server's connection finds by those names.
// What is of the site is here: the thread is an announcement, the site's administrator may
// remove a comment, and nobody is sent a notice (the site keeps none of its own).

var announcementThreads = comments.Kind{Column: "announcement_id", Table: "announcements"}

// announcementThread reads the announcement r's path names as what comments hang on.
func announcementThread(w http.ResponseWriter, r *http.Request, db comments.RowScanner, lock string) (comments.Thread, bool) {
	t := comments.Thread{Kind: announcementThreads}
	err := db.QueryRow(r.Context(), `SELECT id,comment_count FROM net.announcements WHERE id=$1 `+lock, r.PathValue("id")).Scan(&t.ID, &t.CommentCount)
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, 404, "ANNOUNCEMENT_NOT_FOUND", "公告不存在")
		return t, false
	}
	if err != nil {
		internal(w, err)
		return t, false
	}
	return t, true
}

// comments is the site as a host of comments.
func (s *Server) comments() *comments.Host {
	return &comments.Host{
		DB:    s.DB,
		Kinds: []comments.Kind{announcementThreads},
		// A comment goes with its announcement (the foreign key deletes it), so every comment
		// there is hangs on one that is on show.
		Shown:    `true`,
		Problems: answers,
		Viewer: func(r *http.Request) string {
			v, err := s.sessions().Local(r)
			if err != nil {
				return ""
			}
			return v.User.ID
		},
		Writer: func(w http.ResponseWriter, r *http.Request) (comments.Writer, bool) {
			v, ok := s.required(w, r, true)
			return comments.Writer{ID: v.User.ID, Nickname: v.User.Nickname, AvatarPath: files.Path(v.User.AvatarURL), Moderator: v.User.SiteAdmin}, ok
		},
		Profiles: func(ctx context.Context, ids []string) map[string]comments.Profile {
			known := s.profiles(ctx, ids)
			shown := map[string]comments.Profile{}
			for _, id := range ids {
				p, ok := known[id]
				if !ok {
					p.Nickname = "未知用户"
				}
				shown[id] = comments.Profile{Nickname: p.Nickname, AvatarPath: files.Path(p.AvatarURL)}
			}
			return shown
		},
	}
}
