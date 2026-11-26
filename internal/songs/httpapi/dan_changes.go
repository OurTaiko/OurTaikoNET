package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"ourtaiko.dev/net/internal/web"
)

// Fixed 段位 courses (migration 0028_dan_lock). A course's maker fixes it; from then on its songs
// and conditions change only by a request (dan_changes) that a manager of the source approves,
// which fixes it again from that moment, or returns with a reply. Its name, colour and rewards,
// and which of its songs are hidden, stay the maker's to change (updateDan). A manager may undo
// the fixing. Only a fixed course is named by a rule that gives items for passing it (package
// rules, dan_clear), and only the attempts made since it was last fixed count: what a clear of it
// means cannot change behind the rule's back.

// The states of a request.
const (
	danChangePending   = "pending"
	danChangeApproved  = "approved"
	danChangeReturned  = "returned"
	danChangeWithdrawn = "withdrawn"
)

// DanChange is a request to change a fixed course's songs and conditions, with the songs as they
// are asked to be (named as the charts are now).
type DanChange struct {
	ID          string          `json:"id"`
	CourseID    string          `json:"courseId"`
	UserID      string          `json:"userId"`
	Songs       []DanSong       `json:"songs"`
	Exams       json.RawMessage `json:"exams"`
	Request     string          `json:"request"`
	Status      string          `json:"status"`
	ReviewerID  *string         `json:"reviewerId"`
	Reply       string          `json:"reply"`
	CreatedAt   time.Time       `json:"createdAt"`
	DecidedAt   *time.Time      `json:"decidedAt"`
	rawSongs    []byte
	courseOwner string
}

// danSongsJSON is songs as a request keeps them.
func danSongsJSON(songs []danSongInput) []byte {
	if songs == nil {
		songs = []danSongInput{}
	}
	raw, _ := json.Marshal(songs)
	return raw
}

// sameDanSongs says whether in names the course's songs (by chart, course and side, in order)
// and conditions as they are: what a fixed course must be sent as.
func sameDanSongs(ctx context.Context, tx pgx.Tx, course string, storedExams []byte, in danInput) (bool, error) {
	rows, err := tx.Query(ctx, `SELECT chart_id,course,side FROM dan_course_songs WHERE course_id=$1 ORDER BY position`, course)
	if err != nil {
		return false, err
	}
	stored, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (danSongInput, error) {
		var s danSongInput
		return s, row.Scan(&s.ChartID, &s.Course, &s.Side)
	})
	if err != nil || len(stored) != len(in.Songs) {
		return false, err
	}
	for i, s := range stored {
		if s.ChartID != in.Songs[i].ChartID || s.Course != in.Songs[i].Course || s.Side != in.Songs[i].Side {
			return false, nil
		}
	}
	exams, ok := cleanDanExams(in.Exams, len(in.Songs))
	if !ok {
		return false, nil
	}
	var a, b any
	if json.Unmarshal(exams, &a) != nil || json.Unmarshal(storedExams, &b) != nil {
		return false, nil
	}
	return reflect.DeepEqual(a, b), nil
}

// danChangeColumns are a request's columns as scanDanChange reads them, of dan_changes c joined to
// its course d.
const danChangeColumns = `c.id,c.course_id,c.user_id,c.songs,c.exams,c.request,c.status,c.reviewer_id,c.reply,c.created_at,c.decided_at,d.owner_id`

func scanDanChange(row pgx.Row) (DanChange, error) {
	var c DanChange
	err := row.Scan(&c.ID, &c.CourseID, &c.UserID, &c.rawSongs, &c.Exams, &c.Request, &c.Status, &c.ReviewerID, &c.Reply, &c.CreatedAt, &c.DecidedAt, &c.courseOwner)
	return c, err
}

// readDanChanges reads the requests query selects (danChangeColumns) with their songs named.
func (s *Server) readDanChanges(ctx context.Context, query string, args ...any) ([]DanChange, error) {
	rows, err := s.DB.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	changes, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (DanChange, error) { return scanDanChange(row) })
	if err != nil {
		return nil, err
	}
	for i := range changes {
		if changes[i].Songs, err = s.nameDanSongs(ctx, changes[i].rawSongs); err != nil {
			return nil, err
		}
	}
	return changes, nil
}

// nameDanSongs is the songs a request keeps as a course shows them: each block with its chart's
// names and level as they are now, Valid when the block is there with content to play.
func (s *Server) nameDanSongs(ctx context.Context, raw []byte) ([]DanSong, error) {
	rows, err := s.DB.Query(ctx, `SELECT e.v->>'chartId',e.v->>'course',e.v->>'side',COALESCE((e.v->>'hidden')::boolean,false),
 COALESCE(c.title,''),COALESCE(c.subtitle,''),COALESCE(c.title_translations,'{}'::jsonb),COALESCE(c.subtitle_translations,'{}'::jsonb),
 COALESCE(b.level,0),COALESCE(b.content_hash IS NOT NULL,false),COALESCE(c.status='published' AND b.content_hash IS NOT NULL,false)
 FROM jsonb_array_elements($1::jsonb) WITH ORDINALITY e(v,n)
 LEFT JOIN charts c ON c.id=e.v->>'chartId'
 LEFT JOIN chart_difficulties b ON b.chart_id=c.id AND b.course=e.v->>'course' AND b.side=e.v->>'side'
 ORDER BY e.n`, raw)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (DanSong, error) {
		var song DanSong
		err := row.Scan(&song.Chart.ID, &song.Chart.Course, &song.Chart.Side, &song.Hidden, &song.Title, &song.Subtitle,
			&song.TitleTranslations, &song.SubtitleTranslations, &song.Level, &song.Valid, &song.Available)
		return song, err
	})
}

// danReviewer answers a request for a manager's work with who it is from, or answers it itself
// for anyone else.
func (s *Server) danReviewer(w http.ResponseWriter, r *http.Request, csrf bool) (session, bool) {
	v, ok := s.required(w, r, csrf)
	if ok && !v.User.IsAdmin {
		problem(w, 403, "FORBIDDEN", "只有曲库管理员可以处理段位修改申请")
		return v, false
	}
	return v, ok
}

// fixDan is POST /api/dans/{id}/fix, by the course's maker or a manager: the course is fixed from
// now. Every song must be playable: a course that cannot be played is not worth fixing.
func (s *Server) fixDan(w http.ResponseWriter, r *http.Request) {
	_, id, ok := s.ownDan(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	courses, err := s.loadDans(ctx, `d.id=$1`, id)
	if err != nil || len(courses) == 0 {
		internal(w, err)
		return
	}
	if courses[0].FixedAt != nil {
		problem(w, 409, "DAN_FIXED", "段位已经固定了曲目")
		return
	}
	if !danPlayable(courses[0]) {
		problem(w, 422, "DAN_NOT_PLAYABLE", "段位里有无法游玩的歌曲，请先修改曲目")
		return
	}
	tag, err := s.DB.Exec(ctx, `UPDATE dan_courses SET fixed_at=now(),updated_at=now() WHERE id=$1 AND fixed_at IS NULL AND updated_at=$2`, id, courses[0].UpdatedAt)
	if err != nil {
		internal(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		// Changed between the reading and the fixing: what was checked is not what would be fixed.
		problem(w, 409, "DAN_CHANGED", "段位刚刚被修改过，请刷新后重试")
		return
	}
	s.respondDan(w, ctx, id)
}

// unfixDan is DELETE /api/dans/{id}/fix, by a manager: the course is no longer fixed, and its
// maker changes it freely. A request waiting is withdrawn. The rules that name it give nothing
// until it is fixed again, and then count only the attempts since.
func (s *Server) unfixDan(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.danReviewer(w, r, true); !ok {
		return
	}
	id := r.PathValue("id")
	ctx := r.Context()
	var found bool
	err := s.DB.QueryRow(ctx, `WITH d AS (UPDATE dan_courses SET fixed_at=NULL,updated_at=now() WHERE id=$1 AND status='published' RETURNING id),
 w AS (UPDATE dan_changes SET status='withdrawn',decided_at=now() WHERE course_id IN (SELECT id FROM d) AND status='pending')
 SELECT EXISTS(SELECT 1 FROM d)`, id).Scan(&found)
	if err != nil {
		internal(w, err)
		return
	}
	if !found {
		problem(w, 404, "DAN_NOT_FOUND", "段位不存在")
		return
	}
	s.respondDan(w, ctx, id)
}

func (s *Server) respondDan(w http.ResponseWriter, ctx context.Context, id string) {
	courses, err := s.loadDans(ctx, `d.id=$1`, id)
	if err != nil || len(courses) == 0 {
		internal(w, err)
		return
	}
	web.Respond(w, 200, courses[0])
}

// danChanges is GET /api/dans/{id}/changes, for the course's maker or a manager: its requests,
// newest first.
func (s *Server) danChanges(w http.ResponseWriter, r *http.Request) {
	_, id, ok := s.ownDanRead(w, r)
	if !ok {
		return
	}
	changes, err := s.readDanChanges(r.Context(), `SELECT `+danChangeColumns+` FROM dan_changes c JOIN dan_courses d ON d.id=c.course_id
 WHERE c.course_id=$1 ORDER BY c.created_at DESC,c.id LIMIT 50`, id)
	if err != nil {
		internal(w, err)
		return
	}
	web.Respond(w, 200, map[string]any{"changes": changes})
}

// ownDanRead is ownDan for a request that changes nothing.
func (s *Server) ownDanRead(w http.ResponseWriter, r *http.Request) (session, string, bool) {
	u, ok := s.required(w, r, false)
	if !ok {
		return u, "", false
	}
	id := r.PathValue("id")
	var owner string
	err := pgx.ErrNoRows
	if validID(id) {
		err = s.DB.QueryRow(r.Context(), `SELECT owner_id FROM dan_courses WHERE id=$1 AND status='published'`, id).Scan(&owner)
	}
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			problem(w, 404, "DAN_NOT_FOUND", "段位不存在")
		} else {
			internal(w, err)
		}
		return u, "", false
	}
	if owner != u.User.ID && !u.User.IsAdmin {
		problem(w, 403, "FORBIDDEN", "只有创建者或管理员可以查看")
		return u, "", false
	}
	return u, id, true
}

// requestDanChange is POST /api/dans/{id}/changes {songs, exams, request}, by the maker of a fixed
// course or a manager: the songs and conditions it should have, and why. One waits at a time.
func (s *Server) requestDanChange(w http.ResponseWriter, r *http.Request) {
	u, id, ok := s.ownDan(w, r)
	if !ok {
		return
	}
	var in struct {
		Songs   []danSongInput  `json:"songs"`
		Exams   json.RawMessage `json:"exams"`
		Request string          `json:"request"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Request = strings.TrimSpace(in.Request)
	if in.Request == "" || utf8.RuneCountInString(in.Request) > 500 {
		problem(w, 422, "DAN_CHANGE_INVALID", "请用 1–500 字说明修改的理由")
		return
	}
	ctx := r.Context()
	songs, exams, ok := s.checkDanSongs(ctx, w, in.Songs, in.Exams)
	if !ok {
		return
	}
	kept := make([]danSongInput, len(songs))
	for i, song := range songs {
		kept[i] = danSongInput{ChartID: song.chartID, Course: song.course, Side: song.side, Hidden: song.hidden}
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	var fixed bool
	var stored []byte
	if err = tx.QueryRow(ctx, `SELECT fixed_at IS NOT NULL,exams FROM dan_courses WHERE id=$1 FOR SHARE`, id).Scan(&fixed, &stored); err != nil {
		internal(w, err)
		return
	}
	if !fixed {
		problem(w, 409, "DAN_NOT_FIXED", "段位没有固定曲目，可以直接修改")
		return
	}
	same, err := sameDanSongs(ctx, tx, id, stored, danInput{Songs: kept, Exams: exams})
	if err != nil {
		internal(w, err)
		return
	}
	if same {
		problem(w, 422, "DAN_CHANGE_EMPTY", "申请的曲目和合格条件与现在相同")
		return
	}
	change := web.NewID()
	_, err = tx.Exec(ctx, `INSERT INTO dan_changes(id,course_id,user_id,songs,exams,request) VALUES($1,$2,$3,$4,$5,$6)`, change, id, u.User.ID, danSongsJSON(kept), exams, in.Request)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "dan_changes_pending" {
		problem(w, 409, "DAN_CHANGE_PENDING", "已有一个等待审核的修改申请")
		return
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		internal(w, err)
		return
	}
	s.respondDanChange(w, ctx, 201, change)
}

func (s *Server) respondDanChange(w http.ResponseWriter, ctx context.Context, status int, id string) {
	changes, err := s.readDanChanges(ctx, `SELECT `+danChangeColumns+` FROM dan_changes c JOIN dan_courses d ON d.id=c.course_id WHERE c.id=$1`, id)
	if err != nil || len(changes) == 0 {
		internal(w, err)
		return
	}
	web.Respond(w, status, changes[0])
}

// withdrawDanChange is DELETE /api/dans/{id}/changes/{change}, by the course's maker or a
// manager: a request waiting is withdrawn.
func (s *Server) withdrawDanChange(w http.ResponseWriter, r *http.Request) {
	_, id, ok := s.ownDan(w, r)
	if !ok {
		return
	}
	change := r.PathValue("change")
	tag, err := s.DB.Exec(r.Context(), `UPDATE dan_changes SET status='withdrawn',decided_at=now() WHERE id=$1 AND course_id=$2 AND status='pending'`, change, id)
	if err != nil {
		internal(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		problem(w, 404, "DAN_CHANGE_NOT_FOUND", "没有等待审核的这个申请")
		return
	}
	s.respondDanChange(w, r.Context(), 200, change)
}

// DanChangeReview is a request in the managers' queue, with its course as it is now.
type DanChangeReview struct {
	DanChange
	Course DanCourse `json:"course"`
}

// danChangeQueue is GET /api/dan-changes, for managers: the requests waiting, oldest first, each
// with its course as it is now.
func (s *Server) danChangeQueue(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.danReviewer(w, r, false); !ok {
		return
	}
	ctx := r.Context()
	changes, err := s.readDanChanges(ctx, `SELECT `+danChangeColumns+` FROM dan_changes c JOIN dan_courses d ON d.id=c.course_id
 WHERE c.status='pending' AND d.status='published' ORDER BY c.created_at,c.id LIMIT 100`)
	if err != nil {
		internal(w, err)
		return
	}
	ids := []string{}
	for _, c := range changes {
		ids = append(ids, c.CourseID)
	}
	courses, err := s.loadDans(ctx, `d.id=ANY($1)`, ids)
	if err != nil {
		internal(w, err)
		return
	}
	byID := map[string]DanCourse{}
	for _, c := range courses {
		byID[c.ID] = c
	}
	queue := []DanChangeReview{}
	for _, c := range changes {
		if course, ok := byID[c.CourseID]; ok {
			queue = append(queue, DanChangeReview{DanChange: c, Course: course})
		}
	}
	web.Respond(w, 200, map[string]any{"changes": queue})
}

// decideDanChange is POST /api/dan-changes/{change}/decision {approve, reply}, by a manager.
// Approving it gives the course the songs and conditions asked for, which must still be blocks
// the charts have, and fixes it again from now; returning it needs a reply. Either way the
// course's maker is told.
func (s *Server) decideDanChange(w http.ResponseWriter, r *http.Request) {
	v, ok := s.danReviewer(w, r, true)
	if !ok {
		return
	}
	var in struct {
		Approve bool   `json:"approve"`
		Reply   string `json:"reply"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Reply = strings.TrimSpace(in.Reply)
	if utf8.RuneCountInString(in.Reply) > 500 || (!in.Approve && in.Reply == "") {
		problem(w, 422, "DAN_CHANGE_REPLY_INVALID", "退回申请需要 1–500 字的回复")
		return
	}
	ctx := r.Context()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	id := r.PathValue("change")
	change, err := scanDanChange(tx.QueryRow(ctx, `SELECT `+danChangeColumns+` FROM dan_changes c JOIN dan_courses d ON d.id=c.course_id
 WHERE c.id=$1 AND c.status='pending' AND d.status='published' FOR UPDATE OF c,d`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, 404, "DAN_CHANGE_NOT_FOUND", "没有等待审核的这个申请")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	status := danChangeReturned
	if in.Approve {
		status = danChangeApproved
		var asked []danSongInput
		if err = json.Unmarshal(change.rawSongs, &asked); err != nil {
			internal(w, err)
			return
		}
		songs, exams, ok := s.checkDanSongs(ctx, w, asked, change.Exams)
		if !ok {
			return
		}
		if err = s.saveDanSongs(ctx, tx, change.CourseID, songs); err != nil {
			internal(w, err)
			return
		}
		if _, err = tx.Exec(ctx, `UPDATE dan_courses SET exams=$2,fixed_at=now(),updated_at=now() WHERE id=$1`, change.CourseID, exams); err != nil {
			internal(w, err)
			return
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE dan_changes SET status=$2,reviewer_id=$3,reply=$4,decided_at=now() WHERE id=$1`, id, status, v.User.ID, in.Reply); err != nil {
		internal(w, err)
		return
	}
	err = notify(ctx, tx, Notice{Recipient: change.courseOwner, Kind: "dan_change_result", Actor: v.User.ID, DanID: change.CourseID,
		Data: map[string]any{"approved": in.Approve, "reply": in.Reply}})
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		internal(w, err)
		return
	}
	s.respondDanChange(w, ctx, 200, id)
}
