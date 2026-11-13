package httpapi

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"ourtaiko.dev/net/internal/kit/audio"
	"ourtaiko.dev/net/internal/kit/constants"
	"ourtaiko.dev/net/internal/kit/objectstore"
	"ourtaiko.dev/net/internal/kit/tja"
	"ourtaiko.dev/net/internal/kit/tjatitles"
	"ourtaiko.dev/net/internal/kit/video"
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
	// Deprecated: original uploader only; authorization uses OwnerIDs.
	OwnerID  string `json:"ownerId"`
	Uploader string `json:"uploader"`
	// UploaderAvatarURL is empty when the uploader has no avatar or SSO is unavailable.
	UploaderAvatarURL string `json:"uploaderAvatarUrl"`
	Description       string `json:"description"`
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
	URL             string  `json:"url"`
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
		URL:             "/api/v1/charts/" + url.PathEscape(c.ID) + "/audio",
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
const chartSelect = `SELECT c.id,c.number_id,c.nijiiro_id,` + officialIDColumn + `,c.owner_ids,c.description,c.created_at,c.update_at,c.duration,c.demo_end,c.category_flags,
 COALESCE(tf.original_filename,''),COALESCE(af.original_filename,''),COALESCE(tf.sha256,''),COALESCE(tf.byte_size,0),COALESCE(af.sha256,''),COALESCE(af.byte_size,0),COALESCE(tf.storage_key,''),COALESCE(af.storage_key,''),
 COALESCE(cf.sha256,''),COALESCE(pf.storage_key,''),
 c.title,c.subtitle,c.bpm,c.offset_seconds,c.demo_start,COALESCE(af.sha256||'.'||af.extension,''),c.title_translations,c.subtitle_translations,
 COALESCE((SELECT jsonb_agg(jsonb_build_object('id',d.id,'course',d.course,'side',d.side,'level',d.level,'maker',d.maker,'branching',d.branching,'chartHash',d.content_hash) ORDER BY ` + slotOrder + `) FROM chart_difficulties d WHERE d.chart_id=c.id),'[]'),
 COALESCE((SELECT upvotes FROM chart_stats WHERE chart_id=c.id),0),COALESCE((SELECT downvotes FROM chart_stats WHERE chart_id=c.id),0),COALESCE((SELECT comment_count FROM chart_stats WHERE chart_id=c.id),0),
 c.movie_offset,COALESCE(mf.sha256,''),COALESCE(mf.byte_size,0),COALESCE(mf.storage_key,''),
 COALESCE(bf.sha256,''),COALESCE(bf.byte_size,0),COALESCE(bf.storage_key,'')
 FROM charts c LEFT JOIN chart_resources tf ON tf.id=c.tja_file LEFT JOIN chart_resources af ON af.id=c.audio_file
 LEFT JOIN chart_resources cf ON cf.id=c.cover_file LEFT JOIN chart_resources pf ON pf.id=c.preview_file
 LEFT JOIN chart_resources mf ON mf.id=c.movie_file LEFT JOIN chart_resources bf ON bf.id=c.bgimage_file `

// readChart reads a row of chartSelect.
func (s *Server) readChart(row pgx.Row) (Chart, error) {
	c := Chart{official: officialSource}
	var difficulties []byte
	var flags CategoryFlags
	e := row.Scan(&c.ID, &c.NumberID, &c.NijiiroID, &c.OfficialID, &c.OwnerIDs, &c.Description, &c.CreatedAt, &c.UpdateAt, &c.Duration, &c.DemoEnd, &flags,
		&c.TJAName, &c.AudioName, &c.TJAHash, &c.TJASize, &c.AudioHash, &c.AudioSize, &c.TJAKey, &c.AudioKey, &c.CoverHash, &c.PreviewPath,
		&c.Title, &c.Subtitle, &c.BPM, &c.Offset, &c.DemoStart, &c.Wave, &c.TitleTranslations, &c.SubtitleTranslations, &difficulties,
		&c.Upvotes, &c.Downvotes, &c.CommentCount, &c.MovieOffset, &c.MovieHash, &c.MovieSize, &c.MovieKey,
		&c.BackgroundHash, &c.BackgroundSize, &c.BackgroundKey)
	if e == nil {
		c.OwnerID = c.OwnerIDs[0]
		c.Score = c.Upvotes - c.Downvotes
		c.HasFiles = c.TJAKey != ""
		c.CategoryIDs = flags.IDs()
		e = json.Unmarshal(difficulties, &c.Difficulties)
		c.Maker = difficultyMakers(c.Difficulties)
		c.AudioPreview = audioPreview(c)
		// A bucket's files are reached under its public address.
		if remote, ok := s.Config.Objects.(*objectstore.S3); ok && c.PreviewPath != "" {
			c.PreviewPath = remote.Prefix + c.PreviewPath
		}
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
func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	owner := r.URL.Query().Get("owner")
	if owner != "" && !validSubject(owner) {
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
	// The difficulty constants, for the chart page only (lists and the game catalog go without).
	ids := make([]int64, len(c.Difficulties))
	for i, d := range c.Difficulties {
		ids[i] = d.ID
	}
	shown, e := constants.ForBlocks(r.Context(), s.DB, ids)
	if e != nil {
		internal(w, e)
		return
	}
	for i := range c.Difficulties {
		c.Difficulties[i].Constants = shown[c.Difficulties[i].ID]
	}
	if viewer := s.viewer(r); viewer != "" {
		e = s.DB.QueryRow(r.Context(), `SELECT value FROM chart_votes WHERE chart_id=$1 AND user_id=$2`, c.ID, viewer).Scan(&c.MyVote)
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			internal(w, e)
			return
		}
	}
	respond(w, 200, c)
}

// canManage: a song's owners and whoever manages this source may change or delete it.
func canManage(u User, c Chart) bool {
	return u.IsAdmin || c.hasOwner(u.ID)
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
	respond(w, 200, map[string]bool{"ok": true})
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
func (s *Server) download(w http.ResponseWriter, r *http.Request) {
	c, e := s.chart(r.Context(), r.PathValue("id"))
	if errors.Is(e, pgx.ErrNoRows) {
		problem(w, 404, "CHART_NOT_FOUND", "作品不存在或已删除")
		return
	}
	if e != nil {
		internal(w, e)
		return
	}
	kind := r.PathValue("kind")
	if kind == "download" {
		// The ZIP is built per request from the stored TJA and audio.
		tf, e := s.Config.Objects.Open(r.Context(), c.TJAKey)
		if e != nil {
			internal(w, e)
			return
		}
		defer tf.Close()
		af, e := s.Config.Objects.Open(r.Context(), c.AudioKey)
		if e != nil {
			internal(w, e)
			return
		}
		defer af.Close()
		// Name the audio as this TJA does; a file awaiting rekeying still
		// declares its uploaded name.
		data, e := io.ReadAll(io.LimitReader(tf, tja.MaxTJA+1))
		if e != nil {
			internal(w, e)
			return
		}
		// The package names the chart as the site does, whatever the uploaded file says.
		chartText := tjatitles.WithTitles(data, c.Title, c.Subtitle, c.TitleTranslations, c.SubtitleTranslations)
		entries := []struct {
			name string
			file io.Reader
		}{{c.TJAName, bytes.NewReader(chartText)}, {tja.WaveName(data), af}}
		if c.MovieKey != "" {
			mf, e := s.Config.Objects.Open(r.Context(), c.MovieKey)
			if e != nil {
				internal(w, e)
				return
			}
			defer mf.Close()
			entries = append(entries, struct {
				name string
				file io.Reader
			}{tja.MovieName(data), mf})
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": strings.TrimSuffix(c.TJAName, filepath.Ext(c.TJAName)) + ".zip"}))
		z := zip.NewWriter(w)
		for _, f := range entries {
			dst, e := z.CreateHeader(&zip.FileHeader{Name: f.name, Method: zip.Store})
			if e != nil {
				return
			}
			if _, e = io.Copy(dst, f.file); e != nil {
				return
			}
		}
		z.Close()
		return
	}
	var key, name, contentType, etag string
	switch kind {
	case "tja":
		key, name, contentType, etag = c.TJAKey, c.TJAName, "application/octet-stream", c.TJAHash
	case "movie":
		if c.MovieKey == "" {
			problem(w, 404, "RESOURCE_NOT_FOUND", "该谱面没有背景视频")
			return
		}
		key, name, contentType, etag = c.MovieKey, c.MovieHash+"."+video.Extension, video.MediaType, c.MovieHash
	case "bgimage":
		if c.BackgroundKey == "" {
			problem(w, 404, "RESOURCE_NOT_FOUND", "该谱面没有背景图片")
			return
		}
		key, name, contentType, etag = c.BackgroundKey, c.BackgroundHash+".webp", "image/webp", c.BackgroundHash
	default:
		problem(w, 404, "RESOURCE_NOT_FOUND", "资源不存在")
		return
	}
	if s.redirectResource(w, r, key, name) {
		return
	}
	f, e := s.Config.Objects.Open(r.Context(), key)
	if e != nil {
		internal(w, e)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("ETag", `"`+etag+`"`)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	http.ServeContent(w, r, name, f.ModTime(), f)
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

const pageSize = 24

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
