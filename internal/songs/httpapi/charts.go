package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"ourtaiko.dev/net/internal/account"
	"ourtaiko.dev/net/internal/kit/audio"
	"ourtaiko.dev/net/internal/kit/constants"
	"ourtaiko.dev/net/internal/kit/tja"
	"ourtaiko.dev/net/internal/web"
)

// Chart is one song, the same on both song sources. Its JSON has the fields of the chart's
// own source: see MarshalJSON.
type Chart struct {
	ID string `json:"id"`
	// NumberID is the chart's short number, also accepted by GET /charts/{id}.
	NumberID int64 `json:"numberId"`
	// NijiiroID is the song's Nijiiro ID, unique among charts and found by
	// GET /charts/{id}?by=nijiiro; null when unknown.
	NijiiroID *int32 `json:"nijiiroId"`
	// OfficialID is null when unknown. Official source only.
	OfficialID *string             `json:"officialId"`
	OwnerIDs   []string            `json:"ownerIds"`
	Owners     []NotificationActor `json:"owners"`
	// Deprecated: original author only; authorization uses OwnerIDs.
	OwnerID string `json:"ownerId"`
	// Author is who uploaded the chart, AuthorAvatarPath their avatar's file (package files),
	// empty when they have none or SSO is unavailable.
	Author           string `json:"author"`
	AuthorAvatarPath string `json:"authorAvatarPath"`
	Description      string `json:"description"`
	// Votes and comments; MyVote is only filled for the signed-in detail view.
	Score        int       `json:"score"`
	Upvotes      int       `json:"upvotes"`
	Downvotes    int       `json:"downvotes"`
	CommentCount int       `json:"commentCount"`
	MyVote       int       `json:"myVote"`
	CreatedAt    time.Time `json:"createdAt"`
	// UpdateAt is when the chart or anything about it last changed.
	UpdateAt    time.Time `json:"updateAt"`
	Duration    float64   `json:"duration"`
	DemoEnd     float64   `json:"demoEnd"`
	CategoryIDs []string  `json:"categoryIds"`
	TJAName     string    `json:"tjaName"`
	AudioName   string    `json:"audioName"`
	// The stored file's hash: object key and download check, not the chart's identity (that is
	// each block's chartHash). Not in the JSON; the resource manifest carries it as sha256.
	TJAHash string `json:"-"`
	// TJASize is the stored TJA's size in bytes. Official source only.
	TJASize   int64  `json:"tjaSize"`
	AudioHash string `json:"audioHash"`
	AudioSize int64  `json:"audioSize"`
	CoverHash string `json:"coverHash,omitempty"`
	// SongCoverPath is the cover's file (package files), the only way a cover is read.
	SongCoverPath string `json:"songCoverPath,omitempty"`
	// HasFiles is false for a song registered without a TJA and audio, which only the
	// official source accepts: it has no blocks, no duration, and the file fields are empty.
	// Official source only.
	HasFiles bool `json:"hasFiles"`
	// PreviewPath is where the song-select clip is stored, AudioPreview the range of the
	// whole audio that stands for it. Fanmade source only.
	PreviewPath  string        `json:"previewPath,omitempty"`
	AudioPreview *AudioPreview `json:"audioPreview,omitempty"`
	TJAKey       string        `json:"-"`
	AudioKey     string        `json:"-"`
	// Background movie (BGMOVIE), transcoded; empty when the chart has none.
	MovieHash string `json:"movieHash,omitempty"`
	MovieSize int64  `json:"movieSize,omitempty"`
	MovieKey  string `json:"-"`
	// Background image (BGIMAGE), stored as WebP; empty when the chart has none.
	BackgroundHash string `json:"bgimageHash,omitempty"`
	BackgroundSize int64  `json:"bgimageSize,omitempty"`
	BackgroundKey  string `json:"-"`
	// Only in an upload's own response: optional files that were left out.
	FailedFiles []fileFailure `json:"failedFiles,omitempty"`
	tja.Metadata
	// official is whether the chart was read from the official source.
	official bool
}

// MarshalJSON writes the fields of the chart's own source. A Fanmade chart goes without
// officialId, hasFiles and tjaSize; an official one without previewPath and audioPreview.
// A field declared here hides the chart's field of the same JSON name, and is itself left
// out because it is empty.
func (c Chart) MarshalJSON() ([]byte, error) {
	type fields Chart // the same fields without this method
	type absent *struct{}
	if c.official {
		return json.Marshal(struct {
			fields
			PreviewPath  absent `json:"previewPath,omitempty"`
			AudioPreview absent `json:"audioPreview,omitempty"`
		}{fields: fields(c)})
	}
	return json.Marshal(struct {
		fields
		OfficialID absent `json:"officialId,omitempty"`
		HasFiles   absent `json:"hasFiles,omitempty"`
		TJASize    absent `json:"tjaSize,omitempty"`
	}{fields: fields(c)})
}

// AudioPreview describes the original audio stream, not a transcoded excerpt.
// Times are in seconds; SHA-256 and byte size remain Chart.AudioHash/AudioSize.
type AudioPreview struct {
	// Path is the audio's file (package files).
	Path            string  `json:"path"`
	ContentType     string  `json:"contentType"`
	StartSeconds    float64 `json:"startSeconds"`
	DurationSeconds float64 `json:"durationSeconds"`
}

func audioPreview(c Chart) *AudioPreview {
	if c.Duration <= 0 || math.IsNaN(c.Duration) || math.IsInf(c.Duration, 0) {
		return nil
	}
	start := c.DemoStart
	if start < 0 || start >= c.Duration || math.IsNaN(start) || math.IsInf(start, 0) {
		start = 0
	}
	return &AudioPreview{
		ContentType:     audio.MediaType(c.AudioName),
		StartSeconds:    start,
		DurationSeconds: math.Min(15, c.Duration-start),
	}
}

const publishedChart = `c.status='published'`

// playableChart is a published chart with files: what the game can play.
const playableChart = publishedChart + ` AND c.tja_file IS NOT NULL`

// Blocks are listed Easy to Edit, each as single, p1, p2.
const slotOrder = `array_position(ARRAY['Easy','Normal','Hard','Oni','Edit'],d.course),array_position(ARRAY['single','p1','p2'],d.side)`

// chartSelect reads charts for readChart. A song without files (official source) has empty
// file fields.
const chartSelect = `SELECT c.id,c.number_id,c.nijiiro_id,c.official_id,c.owner_ids,c.description,c.created_at,c.update_at,c.duration,c.demo_end,c.category_flags,
 COALESCE(tf.original_filename,''),COALESCE(af.original_filename,''),COALESCE(tf.sha256,''),COALESCE(tf.byte_size,0),COALESCE(af.sha256,''),COALESCE(af.byte_size,0),COALESCE(tf.storage_key,''),COALESCE(af.storage_key,''),
 COALESCE(cf.sha256,''),COALESCE(pf.storage_key,''),
 c.title,c.subtitle,c.bpm,c.offset_seconds,c.demo_start,COALESCE(af.sha256||'.'||af.extension,''),c.title_translations,c.subtitle_translations,
 COALESCE((SELECT jsonb_agg(jsonb_build_object('id',d.id,'course',d.course,'side',d.side,'level',d.level,'maker',d.maker,'branching',d.branching,'chartHash',d.content_hash) ORDER BY ` + slotOrder + `) FROM chart_difficulties d WHERE d.chart_id=c.id),'[]'),
 COALESCE((SELECT upvotes FROM chart_stats WHERE chart_id=c.id),0),COALESCE((SELECT downvotes FROM chart_stats WHERE chart_id=c.id),0),COALESCE((SELECT comment_count FROM chart_stats WHERE chart_id=c.id),0),
 c.movie_offset,COALESCE(mf.sha256,''),COALESCE(mf.byte_size,0),COALESCE(mf.storage_key,''),
 COALESCE(bf.sha256,''),COALESCE(bf.byte_size,0),COALESCE(bf.storage_key,''),COALESCE(cf.storage_key,'')
 FROM charts c LEFT JOIN chart_resources tf ON tf.id=c.tja_file LEFT JOIN chart_resources af ON af.id=c.audio_file
 LEFT JOIN chart_resources cf ON cf.id=c.cover_file LEFT JOIN chart_resources pf ON pf.id=c.preview_file
 LEFT JOIN chart_resources mf ON mf.id=c.movie_file LEFT JOIN chart_resources bf ON bf.id=c.bgimage_file `

// readChart reads a row of chartSelect.
func (s *Server) readChart(row pgx.Row) (Chart, error) {
	c := Chart{official: s.source().Official}
	var difficulties []byte
	var flags CategoryFlags
	var coverKey string
	e := row.Scan(&c.ID, &c.NumberID, &c.NijiiroID, &c.OfficialID, &c.OwnerIDs, &c.Description, &c.CreatedAt, &c.UpdateAt, &c.Duration, &c.DemoEnd, &flags,
		&c.TJAName, &c.AudioName, &c.TJAHash, &c.TJASize, &c.AudioHash, &c.AudioSize, &c.TJAKey, &c.AudioKey, &c.CoverHash, &c.PreviewPath,
		&c.Title, &c.Subtitle, &c.BPM, &c.Offset, &c.DemoStart, &c.Wave, &c.TitleTranslations, &c.SubtitleTranslations, &difficulties,
		&c.Upvotes, &c.Downvotes, &c.CommentCount, &c.MovieOffset, &c.MovieHash, &c.MovieSize, &c.MovieKey,
		&c.BackgroundHash, &c.BackgroundSize, &c.BackgroundKey, &coverKey)
	if e == nil {
		c.SongCoverPath = s.filePath(coverKey)
		c.OwnerID = c.OwnerIDs[0]
		c.Score = c.Upvotes - c.Downvotes
		c.HasFiles = c.TJAKey != ""
		c.CategoryIDs = s.source().categoryIDs(flags)
		e = json.Unmarshal(difficulties, &c.Difficulties)
		c.Maker = difficultyMakers(c.Difficulties)
		if c.AudioPreview = audioPreview(c); c.AudioPreview != nil {
			c.AudioPreview.Path = s.filePath(c.AudioKey)
		}
		c.PreviewPath = s.filePath(c.PreviewPath)
	}
	return c, e
}

func (s *Server) readCharts(rows pgx.Rows) ([]Chart, error) {
	defer rows.Close()
	items := []Chart{}
	for rows.Next() {
		c, e := s.readChart(rows)
		if e != nil {
			return nil, e
		}
		items = append(items, c)
	}
	return items, rows.Err()
}

// chartID checks the {id} path value; chart IDs are 32 lowercase hex digits, as Fanmade's.
func chartID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	if !validID(id) {
		problem(w, 404, "CHART_NOT_FOUND", "谱面不存在或已删除")
		return "", false
	}
	return id, true
}

// validID reports whether id has the form of every random ID here (charts, courses,
// playlists): 32 lowercase hex digits.
func validID(id string) bool {
	if len(id) != 32 {
		return false
	}
	for _, c := range id {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func (s *Server) chart(ctx context.Context, id string) (Chart, error) {
	return s.chartWhere(ctx, "c.id", id)
}

// chartWhere reads the published chart whose column (c.id, c.number_id or c.nijiiro_id) is value.
func (s *Server) chartWhere(ctx context.Context, column string, value any) (Chart, error) {
	c, e := s.readChart(s.DB.QueryRow(ctx, chartSelect+` WHERE `+column+`=$1 AND `+publishedChart, value))
	if e == nil {
		items := []Chart{c}
		s.chartNames(ctx, items)
		c = items[0]
	}
	return c, e
}

// keywordClause matches a chart whose title, subtitle, any translation,
// official ID (where the source has one) or any block's maker contains the keyword q; number is
// keywordNumber(q). An empty keyword matches every chart.
func keywordClause(q, number string) string {
	return `(` + q + `='' OR c.title ILIKE '%'||` + q + `||'%' OR c.subtitle ILIKE '%'||` + q + `||'%' OR c.official_id ILIKE '%'||` + q + `||'%'
 OR c.nijiiro_id=` + number + ` OR c.number_id=` + number + `
 OR EXISTS(SELECT 1 FROM chart_difficulties dm WHERE dm.chart_id=c.id AND dm.maker ILIKE '%'||` + q + `||'%')
 OR EXISTS(SELECT 1 FROM jsonb_each_text(c.title_translations) t WHERE t.value ILIKE '%'||` + q + `||'%')
 OR EXISTS(SELECT 1 FROM jsonb_each_text(c.subtitle_translations) t WHERE t.value ILIKE '%'||` + q + `||'%'))`
}

// keywordNumber lets a number also find the chart with that Nijiiro ID or
// number ID; -1 matches none.
func keywordNumber(q string) int64 {
	if n, e := strconv.ParseInt(q, 10, 64); e == nil && n > 0 {
		return n
	}
	return -1
}

func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	owner := r.URL.Query().Get("owner")
	if owner != "" && !account.ValidSubject(owner) {
		problem(w, 400, "QUERY_INVALID", "用户标识无效")
		return
	}
	s.search(w, r, owner)
}
func (s *Server) mine(w http.ResponseWriter, r *http.Request) {
	u, ok := s.required(w, r, false)
	if !ok {
		return
	}
	s.search(w, r, u.User.ID)
}

// How many charts a page of the chart list has when the client does not say, on both sources,
// and the most a client can ask for.
const (
	defaultChartPageSize = 24
	maxChartPageSize     = 100
)

// search pages the chart list, pageSize charts at a time. Filters: keyword, owner, category, a
// course and star level that must hold for the same block (any side), whether the chart has a
// Nijiiro ID, and whether it has files. Every filter and order is there on both sources; which
// of them a source's website offers is the website's business.
func (s *Server) search(w http.ResponseWriter, r *http.Request, owner string) {
	query := r.URL.Query()
	q := strings.TrimSpace(query.Get("q"))
	if len(q) > 200 {
		problem(w, 400, "QUERY_INVALID", "搜索内容过长")
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
	pageSize := defaultChartPageSize
	if raw := query.Get("pageSize"); raw != "" {
		var e error
		if pageSize, e = strconv.Atoi(raw); e != nil || pageSize < 1 || pageSize > maxChartPageSize {
			problem(w, 400, "QUERY_INVALID", "每页数量必须在 1 到 100 之间")
			return
		}
	}
	var flag CategoryFlags
	if id := query.Get("category"); id != "" {
		var ok bool
		if flag, ok = s.source().categoryFlag(id); !ok {
			problem(w, 400, "QUERY_INVALID", "分类不存在")
			return
		}
	}
	course := query.Get("course")
	switch course {
	case "", "Easy", "Normal", "Hard", "Oni", "Edit":
	default:
		problem(w, 400, "QUERY_INVALID", "难度必须为 Easy / Normal / Hard / Oni / Edit")
		return
	}
	level := 0
	if raw := query.Get("level"); raw != "" {
		var e error
		if level, e = strconv.Atoi(raw); e != nil || level < 1 || level > 10 {
			problem(w, 400, "QUERY_INVALID", "星数必须在 1 到 10 之间")
			return
		}
	}
	mapped := query.Get("nijiiro")
	if mapped != "" && mapped != "yes" && mapped != "no" {
		problem(w, 400, "QUERY_INVALID", "nijiiro 必须为 yes / no")
		return
	}
	files := query.Get("files")
	if files != "" && files != "yes" && files != "no" {
		problem(w, 400, "QUERY_INVALID", "files 必须为 yes / no")
		return
	}
	// The star level a level sort ranks by: the filtered course's, else the hardest.
	const stars = `(SELECT max(d.level) FROM chart_difficulties d WHERE d.chart_id=c.id AND ($4='' OR d.course=$4))`
	ordering := ` ORDER BY c.created_at DESC,c.id DESC`
	switch query.Get("order") {
	case "", "default":
	case "nijiiro":
		ordering = ` ORDER BY c.nijiiro_id ASC NULLS LAST,c.id ASC`
	case "title":
		ordering = ` ORDER BY lower(c.title) ASC,c.id ASC`
	case "bpm":
		ordering = ` ORDER BY c.bpm DESC,c.id ASC`
	case "level":
		ordering = ` ORDER BY ` + stars + ` DESC NULLS LAST,c.id ASC`
	case "updated":
		ordering = ` ORDER BY c.update_at DESC,c.id DESC`
	// Reddit's "hot": the vote score counts logarithmically
	// against age, so every tenfold score keeps a chart as high as one 12.5 hours newer.
	case "hot":
		ordering = ` ORDER BY sign(` + chartScoreSQL + `)*log(greatest(abs(` + chartScoreSQL + `),1))+extract(epoch FROM c.created_at)/45000 DESC,c.created_at DESC,c.id DESC`
	case "top":
		ordering = ` ORDER BY ` + chartScoreSQL + ` DESC,c.created_at DESC,c.id DESC`
	case "comments":
		ordering = ` ORDER BY COALESCE((SELECT comment_count FROM chart_stats WHERE chart_id=c.id),0) DESC,c.created_at DESC,c.id DESC`
	default:
		problem(w, 400, "QUERY_INVALID", "顺序必须为 default / nijiiro / title / bpm / level / updated / hot / top / comments")
		return
	}
	number := keywordNumber(q)
	where := ` WHERE ` + publishedChart + ` AND ` + keywordClause("$1", "$7") + `
 AND ($2='' OR c.owner_ids @> ARRAY[$2::text])
 AND ($3::int=0 OR (c.category_flags & $3::int)<>0)
 AND (($4='' AND $5::int=0) OR EXISTS(SELECT 1 FROM chart_difficulties d
   WHERE d.chart_id=c.id AND ($4='' OR d.course=$4) AND ($5::int=0 OR d.level=$5)))
 AND ($6='' OR (c.nijiiro_id IS NOT NULL)=($6='yes'))
 AND ($8='' OR (c.tja_file IS NOT NULL)=($8='yes'))`
	args := []any{q, owner, int(flag), course, level, mapped, number, files}
	var total int
	if e := s.DB.QueryRow(r.Context(), `SELECT count(*) FROM charts c`+where, args...).Scan(&total); e != nil {
		internal(w, e)
		return
	}
	rows, e := s.DB.Query(r.Context(), chartSelect+where+ordering+` LIMIT $9 OFFSET $10`, append(args, pageSize, (page-1)*pageSize)...)
	if e != nil {
		internal(w, e)
		return
	}
	items, e := s.readCharts(rows)
	if e != nil {
		internal(w, e)
		return
	}
	s.chartNames(r.Context(), items)
	web.Respond(w, 200, map[string]any{"items": items, "total": total, "page": page, "pageSize": pageSize})
}

// nijiiroID reads a Nijiiro ID as text: a positive int4 without leading zeros.
func nijiiroID(raw string) (int32, bool) {
	n, e := strconv.ParseInt(raw, 10, 32)
	return int32(n), e == nil && n > 0 && raw[0] != '0' && raw[0] != '+'
}

// detail answers GET /charts/{id}: by=number looks {id} up as the chart's number, by=id as its
// ID, by=nijiiro as its Nijiiro ID, and without by its form decides between ID and number.
func (s *Server) detail(w http.ResponseWriter, r *http.Request) {
	raw, column := r.PathValue("id"), ""
	var value any = raw
	by := r.URL.Query().Get("by")
	if by == "" {
		// Without by, the form tells: an ID has 32 hex digits, a number at most 19.
		by = "number"
		if validID(raw) {
			by = "id"
		}
	}
	switch by {
	case "number":
		if n, e := strconv.ParseInt(raw, 10, 64); e == nil && n > 0 && raw[0] != '0' {
			column, value = "c.number_id", n
		}
	case "id":
		column = "c.id"
	case "nijiiro":
		if n, ok := nijiiroID(raw); ok {
			column, value = "c.nijiiro_id", n
		}
	default:
		problem(w, 400, "QUERY_INVALID", "by 只能是 number、id 或 nijiiro")
		return
	}
	e := pgx.ErrNoRows
	var c Chart
	if column != "" {
		c, e = s.chartWhere(r.Context(), column, value)
	}
	if errors.Is(e, pgx.ErrNoRows) {
		problem(w, 404, "CHART_NOT_FOUND", "谱面不存在或已删除")
		return
	}
	if e != nil {
		internal(w, e)
		return
	}
	s.respondChart(w, r, 200, c)
}

// respondChart answers a request with one chart. Every route that answers with a single chart
// ends here, reads and writes alike, so a client can take any of those answers as the chart:
// they are the representation GET /charts/{id} gives (TestChartAnswersMatchDetail), plus an
// upload's own failedFiles. Lists and the game catalog have their own, smaller form.
func (s *Server) respondChart(w http.ResponseWriter, r *http.Request, status int, c Chart) {
	if e := s.addDetail(r, &c); e != nil {
		internal(w, e)
		return
	}
	web.Respond(w, status, c)
}

// addDetail adds what a single chart has and a list's do not: the difficulty constants and the
// viewer's vote.
func (s *Server) addDetail(r *http.Request, c *Chart) error {
	ids := make([]int64, len(c.Difficulties))
	for i, d := range c.Difficulties {
		ids[i] = d.ID
	}
	shown, e := constants.ForBlocks(r.Context(), s.DB, ids)
	if e != nil {
		return e
	}
	for i := range c.Difficulties {
		c.Difficulties[i].Constants = shown[c.Difficulties[i].ID]
	}
	if viewer := s.viewer(r); viewer != "" {
		e = s.DB.QueryRow(r.Context(), `SELECT value FROM chart_votes WHERE chart_id=$1 AND user_id=$2`, c.ID, viewer).Scan(&c.MyVote)
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
	}
	return nil
}

// canManage: a song's owners and whoever manages this source may change or delete it.
func canManage(u User, c Chart) bool {
	return u.IsAdmin || c.hasOwner(u.ID)
}

// managed answers a request to change a chart: the signed-in user and the published chart
// the path names, when the user may manage it.
func (s *Server) managed(w http.ResponseWriter, r *http.Request) (session, Chart, bool) {
	u, ok := s.required(w, r, true)
	if !ok {
		return u, Chart{}, false
	}
	c, e := s.chart(r.Context(), r.PathValue("id"))
	if errors.Is(e, pgx.ErrNoRows) {
		problem(w, 404, "CHART_NOT_FOUND", "谱面不存在或已删除")
		return u, c, false
	}
	if e != nil {
		internal(w, e)
		return u, c, false
	}
	if !canManage(u.User, c) {
		problem(w, 403, "FORBIDDEN", "只有所有者或管理员可以修改谱面")
		return u, c, false
	}
	return u, c, true
}

func (s *Server) remove(w http.ResponseWriter, r *http.Request) {
	u, ok := s.required(w, r, true)
	if !ok {
		return
	}
	c, e := s.chart(r.Context(), r.PathValue("id"))
	if errors.Is(e, pgx.ErrNoRows) {
		problem(w, 404, "CHART_NOT_FOUND", "谱面不存在")
		return
	}
	if e != nil {
		internal(w, e)
		return
	}
	if !canManage(u.User, c) {
		problem(w, 403, "FORBIDDEN", "只有所有者或管理员可以删除谱面")
		return
	}
	files, e := s.deleteChart(r.Context(), c.ID)
	if errors.Is(e, pgx.ErrNoRows) {
		problem(w, 404, "CHART_NOT_FOUND", "谱面不存在")
		return
	}
	if e != nil {
		internal(w, e)
		return
	}
	// The song is gone either way. A file another chart shares stays; one that could not be
	// deleted is retried later.
	if e = s.releaseFiles(context.WithoutCancel(r.Context()), files); e != nil {
		log.Printf("deleted chart file cleanup pending: %v", e)
	}
	web.Respond(w, 200, map[string]bool{"ok": true})
}

// deleteChart removes a chart's scores and row, and returns the files it referred to.
func (s *Server) deleteChart(ctx context.Context, id string) ([]int64, error) {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	// Serialize with score submissions, which hold the chart row FOR SHARE.
	var files []*int64
	if e = tx.QueryRow(ctx, `SELECT ARRAY[tja_file,audio_file,cover_file,preview_file,movie_file,bgimage_file] FROM charts WHERE id=$1 FOR UPDATE`, id).Scan(&files); e != nil {
		return nil, e
	}
	if _, e = tx.Exec(ctx, `DELETE FROM scores WHERE difficulty_id IN (SELECT id FROM chart_difficulties WHERE chart_id=$1)`, id); e != nil {
		return nil, e
	}
	if _, e = tx.Exec(ctx, `DELETE FROM charts WHERE id=$1`, id); e != nil {
		return nil, e
	}
	ids := []int64{}
	for _, file := range files {
		if file != nil {
			ids = append(ids, *file)
		}
	}
	return ids, tx.Commit(ctx)
}

// playlistCharts reads published charts in the order of ids.
func (s *Server) playlistCharts(ctx context.Context, ids []string) ([]Chart, error) {
	rows, e := s.DB.Query(ctx, chartSelect+` JOIN unnest($1::text[]) WITH ORDINALITY listed(id,ord) ON listed.id=c.id WHERE `+publishedChart+` ORDER BY listed.ord`, ids)
	if e != nil {
		return nil, e
	}
	charts, e := s.readCharts(rows)
	if e != nil {
		return nil, e
	}
	s.chartNames(ctx, charts)
	return charts, nil
}
