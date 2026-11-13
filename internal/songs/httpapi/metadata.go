package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"math"
	"net/http"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
	"ourtaiko.dev/net/internal/kit/audio"
	"ourtaiko.dev/net/internal/kit/tja"
	"ourtaiko.dev/net/internal/kit/video"
)

// editMetadata changes the site's own description of a chart; the stored
// TJA and audio are untouched. Omitted fields keep their value. The title and
// subtitle can be changed at any time (R042): pages, packages and the game use
// these, and put them into the TJA headers they hand out. The BPM is the TJA's
// and can be set only on a song registered without files.
func (s *Server) editMetadata(w http.ResponseWriter, r *http.Request) {
	_, c, ok := s.managed(w, r)
	if !ok {
		return
	}
	var patch metadataPatch
	if !decode(w, r, &patch) {
		return
	}
	isNull := func(raw json.RawMessage) bool { return bytes.Equal(bytes.TrimSpace(raw), []byte("null")) }
	title, subtitle, bpm := c.Title, c.Subtitle, c.BPM
	if len(patch.BPM) > 0 && c.HasFiles {
		problem(w, 422, "METADATA_LOCKED", "有谱面文件时，BPM 来自 TJA，不能在此修改")
		return
	}
	if len(patch.Title) > 0 || len(patch.Subtitle) > 0 || len(patch.BPM) > 0 {
		if len(patch.Title) > 0 {
			if json.Unmarshal(patch.Title, &title) != nil || isNull(patch.Title) {
				problem(w, 422, "METADATA_INVALID", "曲名必须为文本")
				return
			}
			title = strings.TrimSpace(title)
		}
		if len(patch.Subtitle) > 0 {
			if json.Unmarshal(patch.Subtitle, &subtitle) != nil || isNull(patch.Subtitle) {
				problem(w, 422, "METADATA_INVALID", "副标题必须为文本")
				return
			}
			subtitle = strings.TrimSpace(subtitle)
		}
		if title == "" || !validDisplayText(title) || !validDisplayText(subtitle) {
			problem(w, 422, "METADATA_INVALID", "曲名不能为空；曲名和副标题不能超过 500 字节或包含控制字符")
			return
		}
		if len(patch.BPM) > 0 {
			bpm = 0
			if !isNull(patch.BPM) && (json.Unmarshal(patch.BPM, &bpm) != nil || bpm <= 0 || !validBPM(bpm)) {
				problem(w, 422, "METADATA_INVALID", "BPM 必须为正数或 null")
				return
			}
		}
	}
	description := c.Description
	if len(patch.Description) > 0 {
		if json.Unmarshal(patch.Description, &description) != nil || isNull(patch.Description) || !validDescription(description) {
			problem(w, 422, "DESCRIPTION_INVALID", "说明必须为不超过 4000 字节的文本")
			return
		}
	}
	flags, _ := s.source().encodeCategories(c.CategoryIDs)
	if len(patch.CategoryIDs) > 0 {
		ids, e := s.source().categorySelection(patch.CategoryIDs)
		if e != nil {
			problem(w, 422, "CATEGORIES_INVALID", "分类必须为已有分类 ID 的数组")
			return
		}
		flags, _ = s.source().encodeCategories(ids)
	}
	// Only the official source's songs carry the official game's identifiers.
	if !s.source().Official && len(patch.NijiiroID)+len(patch.OfficialID) > 0 {
		problem(w, 422, "IDENTIFIER_INVALID", "这个曲源的谱面没有 Nijiiro ID 和官方 ID")
		return
	}
	nijiiroID, officialID := c.NijiiroID, c.OfficialID
	if len(patch.NijiiroID) > 0 {
		nijiiroID = nil
		if !isNull(patch.NijiiroID) {
			var n int32
			if json.Unmarshal(patch.NijiiroID, &n) != nil || n < 1 {
				problem(w, 422, "IDENTIFIER_INVALID", "Nijiiro ID 必须为正整数或 null")
				return
			}
			nijiiroID = &n
		}
		if !s.nijiiroFree(w, r, nijiiroID, c.ID) {
			return
		}
	}
	if len(patch.OfficialID) > 0 {
		officialID = nil
		if !isNull(patch.OfficialID) {
			var value string
			if json.Unmarshal(patch.OfficialID, &value) != nil || (value != "" && !officialIDPattern.MatchString(value)) {
				problem(w, 422, "IDENTIFIER_INVALID", "官方 ID 只能包含字母、数字、点、下划线和连字符，最长 64 个字符")
				return
			}
			if value != "" {
				officialID = &value
			}
		}
	}
	demoStart, demoEnd := c.DemoStart, c.DemoEnd
	if len(patch.DemoStart)+len(patch.DemoEnd) > 0 {
		if !c.HasFiles {
			problem(w, 422, "METADATA_LOCKED", "没有音频的乐曲不能设置试听范围")
			return
		}
		for _, field := range []struct {
			raw   json.RawMessage
			value *float64
		}{{patch.DemoStart, &demoStart}, {patch.DemoEnd, &demoEnd}} {
			if len(field.raw) > 0 && (isNull(field.raw) || json.Unmarshal(field.raw, field.value) != nil) {
				*field.value = math.NaN()
			}
		}
		if !audio.ValidPreviewRange(demoStart, demoEnd, c.Duration) {
			problem(w, 422, "PREVIEW_RANGE_INVALID", "试听起点必须位于音频内，终点须晚于起点且不超过 1215 秒")
			return
		}
	}
	movieOffset := c.MovieOffset
	if len(patch.MovieOffset) > 0 {
		if isNull(patch.MovieOffset) || json.Unmarshal(patch.MovieOffset, &movieOffset) != nil || !video.ValidOffset(movieOffset) {
			problem(w, 422, "TJA_MOVIE_OFFSET_INVALID", "MOVIEOFFSET 必须为秒数，绝对值不超过 1200")
			return
		}
		if c.MovieHash == "" {
			problem(w, 422, "MOVIE_NOT_FOUND", "该谱面没有背景视频，无法设置 MOVIEOFFSET")
			return
		}
	}
	if !patchTranslations(patch.TitleTranslations, c.TitleTranslations) || !patchTranslations(patch.SubtitleTranslations, c.SubtitleTranslations) {
		problem(w, 422, "TRANSLATIONS_INVALID", "译名必须为 en / ja / zh / ko 到不超过 500 字节文本的映射")
		return
	}
	makers := c.Difficulties
	if len(patch.DifficultyMakers) > 0 {
		makers = append([]tja.Difficulty(nil), c.Difficulties...)
		if e := applyDifficultyMakers(string(patch.DifficultyMakers), makers); e != nil {
			problem(w, 422, "MAKERS_INVALID", e.Error())
			return
		}
	}
	titles, _ := json.Marshal(c.TitleTranslations)
	subtitles, _ := json.Marshal(c.SubtitleTranslations)
	tx, e := s.DB.Begin(r.Context())
	if e != nil {
		internal(w, e)
		return
	}
	defer tx.Rollback(r.Context())
	// update_at is refreshed by the charts trigger only when something changed.
	tag, e := tx.Exec(r.Context(), `UPDATE charts c SET description=$2,category_flags=$3,nijiiro_id=$4,title_translations=$5,subtitle_translations=$6,title=$7,subtitle=$8,bpm=$9,demo_start=$10,demo_end=$11 WHERE c.id=$1 AND `+publishedChart,
		c.ID, description, flags, nijiiroID, titles, subtitles, title, subtitle, bpm, demoStart, demoEnd)
	if nijiiroConflict(e) {
		problem(w, 409, "NIJIIRO_ID_TAKEN", nijiiroTakenMessage)
		return
	}
	if e != nil {
		internal(w, e)
		return
	}
	if tag.RowsAffected() == 0 {
		problem(w, 404, "CHART_NOT_FOUND", "谱面不存在或已删除")
		return
	}
	// The column is the official source's alone.
	if s.source().Official {
		if _, e = tx.Exec(r.Context(), `UPDATE charts SET official_id=$2 WHERE id=$1`, c.ID, officialID); e != nil {
			internal(w, e)
			return
		}
	}
	// A changed block refreshes the chart's update_at by trigger.
	for i, d := range makers {
		if d.Maker != c.Difficulties[i].Maker {
			if _, e = tx.Exec(r.Context(), `UPDATE chart_difficulties SET maker=$2 WHERE id=$1`, d.ID, d.Maker); e != nil {
				internal(w, e)
				return
			}
		}
	}
	var retired []int64
	if movieOffset != c.MovieOffset {
		// The UPDATE above holds the row; the movie is read again under it.
		var movieSHA string
		if e = tx.QueryRow(r.Context(), `SELECT COALESCE(mf.sha256,'') FROM charts c LEFT JOIN chart_resources mf ON mf.id=c.movie_file WHERE c.id=$1`, c.ID).Scan(&movieSHA); e != nil {
			internal(w, e)
			return
		}
		if movieSHA == "" {
			problem(w, 422, "MOVIE_NOT_FOUND", "该谱面没有背景视频，无法设置 MOVIEOFFSET")
			return
		}
		if retired, e = s.rewriteMovie(r.Context(), tx, c.ID, movieSHA, nil, movieOffset); e != nil {
			internal(w, e)
			return
		}
	}
	if e = tx.Commit(r.Context()); e != nil {
		internal(w, e)
		return
	}
	if e = s.releaseFiles(r.Context(), retired); e != nil {
		log.Printf("replaced TJA cleanup pending: %v", e)
	}
	if demoStart != c.DemoStart || demoEnd != c.DemoEnd {
		s.previewAfter(r.Context(), c.ID, "")
	}
	updated, e := s.chart(r.Context(), c.ID)
	if e != nil {
		internal(w, e)
		return
	}
	respond(w, 200, updated)
}

type metadataPatch struct {
	// Title, Subtitle and BPM are the TJA's for a chart with files and can be
	// changed only on a song registered without them (R038); null BPM means
	// unknown.
	Title       json.RawMessage `json:"title"`
	Subtitle    json.RawMessage `json:"subtitle"`
	BPM         json.RawMessage `json:"bpm"`
	Description json.RawMessage `json:"description"`
	CategoryIDs json.RawMessage `json:"categoryIds"`
	// null clears an identifier.
	NijiiroID            json.RawMessage `json:"nijiiroId"`
	OfficialID           json.RawMessage `json:"officialId"`
	TitleTranslations    json.RawMessage `json:"titleTranslations"`
	SubtitleTranslations json.RawMessage `json:"subtitleTranslations"`
	// The preview range in seconds, within the audio; a chart without files has none to set.
	DemoStart json.RawMessage `json:"demoStart"`
	DemoEnd   json.RawMessage `json:"demoEnd"`
	// MOVIEOFFSET in seconds, for a chart with a movie; it is rewritten in the stored TJA,
	// where players read it.
	MovieOffset json.RawMessage `json:"movieOffset"`
	// Every block's maker, as [{course, side, maker}].
	DifficultyMakers json.RawMessage `json:"difficultyMakers"`
}

// patchTranslations merges locale → text into dst; null or "" removes a
// locale. The TJA's own title and subtitle are not editable here.
func patchTranslations(raw json.RawMessage, dst map[string]string) bool {
	if len(raw) == 0 {
		return true
	}
	var patch map[string]*string
	if json.Unmarshal(raw, &patch) != nil {
		return false
	}
	for locale, value := range patch {
		if locale != "en" && locale != "ja" && locale != "zh" && locale != "ko" {
			return false
		}
		if value == nil || strings.TrimSpace(*value) == "" {
			delete(dst, locale)
			continue
		}
		if !validDisplayText(*value) {
			return false
		}
		dst[locale] = strings.TrimSpace(*value)
	}
	return true
}

func validDisplayText(s string) bool {
	if len(s) > 500 || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func validDescription(s string) bool {
	if len(s) > 4000 || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return false
		}
	}
	return true
}

// validBPM accepts the BPM an uploader gives a song without files: positive
// and finite, or 0 for unknown.
func validBPM(n float64) bool {
	return n >= 0 && n <= 10000 && !math.IsNaN(n)
}

var officialIDPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)

// nijiiroTaken reports whether a chart other than except has the Nijiiro ID; each names one
// chart (unique index charts_nijiiro_id, migration 027).
func (s *Server) nijiiroTaken(ctx context.Context, id *int32, except string) (bool, error) {
	if id == nil {
		return false, nil
	}
	var taken bool
	e := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM charts WHERE nijiiro_id=$1 AND id<>$2)`, *id, except).Scan(&taken)
	return taken, e
}

// nijiiroConflict tells the unique index's refusal: another writer took the Nijiiro ID after
// nijiiroTaken looked.
func nijiiroConflict(e error) bool {
	var pg *pgconn.PgError
	return errors.As(e, &pg) && pg.Code == "23505" && pg.ConstraintName == "charts_nijiiro_id"
}

const nijiiroTakenMessage = "这个 Nijiiro ID 已被其他谱面使用"

// nijiiroFree answers 409 NIJIIRO_ID_TAKEN and reports false when another chart has the ID.
func (s *Server) nijiiroFree(w http.ResponseWriter, r *http.Request, id *int32, except string) bool {
	taken, e := s.nijiiroTaken(r.Context(), id, except)
	if e != nil {
		internal(w, e)
		return false
	}
	if taken {
		problem(w, 409, "NIJIIRO_ID_TAKEN", nijiiroTakenMessage)
	}
	return !taken
}
