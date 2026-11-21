package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"ourtaiko.dev/net/internal/web"
)

// The site's announcements: what its administrator has to tell every visitor. The home page
// shows the first few and /announcements all of them, pinned ones first and then the newest
// (net.announcements). A title and a text of plain lines; neither is HTML or Markdown.

// Announcement is one of them.
type Announcement struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Body        string    `json:"body"`
	Pinned      bool      `json:"pinned"`
	PublishedAt time.Time `json:"publishedAt"`
}

// AnnouncementPage is one page of them, in the order they are shown.
type AnnouncementPage struct {
	Items    []Announcement `json:"items"`
	Total    int            `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"pageSize"`
}

const (
	// In characters, as the table's checks count them.
	maxAnnouncementTitle = 80
	maxAnnouncementBody  = 4000

	defaultAnnouncementPageSize = 20
	maxAnnouncementPageSize     = 50

	announcementColumns = `id,title,body,pinned,published_at`
)

// announcements is GET /api/announcements, public: pinned ones first, then the newest,
// ?pageSize= (1 to 50, 20 when there is none) of them at a time from ?page=.
func (s *Server) announcements(w http.ResponseWriter, r *http.Request) {
	page, ok := queryPage(w, r)
	if !ok {
		return
	}
	pageSize := defaultAnnouncementPageSize
	if raw := r.URL.Query().Get("pageSize"); raw != "" {
		var err error
		if pageSize, err = strconv.Atoi(raw); err != nil || pageSize < 1 || pageSize > maxAnnouncementPageSize {
			problem(w, 400, "QUERY_INVALID", "每页数量必须在 1 到 50 之间")
			return
		}
	}
	ctx := r.Context()
	// One reading of the table, so the count and the page are of the same moment.
	rows, err := s.DB.Query(ctx, `WITH shown AS (SELECT `+announcementColumns+` FROM net.announcements)
 SELECT (SELECT count(*) FROM shown),page.* FROM (SELECT NULL) one
 LEFT JOIN LATERAL (SELECT * FROM shown ORDER BY pinned DESC,published_at DESC,id LIMIT $1 OFFSET $2) page ON true`,
		pageSize, (page-1)*pageSize)
	if err != nil {
		internal(w, err)
		return
	}
	defer rows.Close()
	answer := AnnouncementPage{Items: []Announcement{}, Page: page, PageSize: pageSize}
	for rows.Next() {
		// A page past the end is one row of the count alone.
		var (
			id, title, body *string
			pinned          *bool
			publishedAt     *time.Time
		)
		if err = rows.Scan(&answer.Total, &id, &title, &body, &pinned, &publishedAt); err != nil {
			internal(w, err)
			return
		}
		if id != nil {
			answer.Items = append(answer.Items, Announcement{ID: *id, Title: *title, Body: *body, Pinned: *pinned, PublishedAt: *publishedAt})
		}
	}
	if err = rows.Err(); err != nil {
		internal(w, err)
		return
	}
	web.Respond(w, 200, answer)
}

// announcementInput is what the administrator writes of an announcement.
type announcementInput struct {
	Title  string `json:"title"`
	Body   string `json:"body"`
	Pinned bool   `json:"pinned"`
}

// clean trims what was written and gives its lines one ending. It reports why it cannot be
// kept, or "".
func (in *announcementInput) clean() string {
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" || utf8.RuneCountInString(in.Title) > maxAnnouncementTitle || strings.ContainsFunc(in.Title, unicode.IsControl) {
		return "标题需要 1–80 个字符，且不能换行"
	}
	in.Body = strings.TrimSpace(strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(in.Body))
	if utf8.RuneCountInString(in.Body) > maxAnnouncementBody ||
		strings.ContainsFunc(in.Body, func(r rune) bool { return r != '\n' && unicode.IsControl(r) }) {
		return "正文不能超过 4000 个字符，且不能含有控制字符"
	}
	return ""
}

// readAnnouncement decodes and checks what the administrator sent. It reports false after
// answering.
func (s *Server) readAnnouncement(w http.ResponseWriter, r *http.Request) (session, announcementInput, bool) {
	var in announcementInput
	v, ok := s.siteAdmin(w, r)
	if !ok || !decode(w, r, &in) {
		return v, in, false
	}
	if reason := in.clean(); reason != "" {
		problem(w, 422, "ANNOUNCEMENT_INVALID", reason)
		return v, in, false
	}
	return v, in, true
}

// createAnnouncement is POST /api/admin/announcements with {"title", "body", "pinned"}: the
// site's administrator announces something, as of now.
func (s *Server) createAnnouncement(w http.ResponseWriter, r *http.Request) {
	v, in, ok := s.readAnnouncement(w, r)
	if !ok {
		return
	}
	rows, _ := s.DB.Query(r.Context(), `INSERT INTO net.announcements(id,title,body,pinned,changed_by) VALUES ($1,$2,$3,$4,$5) RETURNING `+announcementColumns,
		web.NewID(), in.Title, in.Body, in.Pinned, v.User.ID)
	made, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByPos[Announcement])
	if err != nil {
		internal(w, err)
		return
	}
	web.Respond(w, 201, made)
}

// saveAnnouncement is PUT /api/admin/announcements/{id} with the same body: the announcement
// now says this. When it was announced stays as it was.
func (s *Server) saveAnnouncement(w http.ResponseWriter, r *http.Request) {
	v, in, ok := s.readAnnouncement(w, r)
	if !ok {
		return
	}
	rows, _ := s.DB.Query(r.Context(), `UPDATE net.announcements SET title=$2,body=$3,pinned=$4,changed_by=$5,changed_at=now() WHERE id=$1 RETURNING `+announcementColumns,
		r.PathValue("id"), in.Title, in.Body, in.Pinned, v.User.ID)
	saved, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByPos[Announcement])
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, 404, "ANNOUNCEMENT_NOT_FOUND", "公告不存在")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	web.Respond(w, 200, saved)
}

// deleteAnnouncement is DELETE /api/admin/announcements/{id}: it is announced no longer.
func (s *Server) deleteAnnouncement(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.siteAdmin(w, r); !ok {
		return
	}
	tag, err := s.DB.Exec(r.Context(), `DELETE FROM net.announcements WHERE id=$1`, r.PathValue("id"))
	if err != nil {
		internal(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		problem(w, 404, "ANNOUNCEMENT_NOT_FOUND", "公告不存在")
		return
	}
	w.WriteHeader(204)
}
