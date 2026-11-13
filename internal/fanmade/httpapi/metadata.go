package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"ourtaiko.dev/net/internal/kit/audio"
	"ourtaiko.dev/net/internal/kit/tja"
	"ourtaiko.dev/net/internal/kit/video"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

type metadataPatch struct {
	Description          json.RawMessage `json:"description"`
	DifficultyMakers     json.RawMessage `json:"difficultyMakers"`
	DemoStart            json.RawMessage `json:"demoStart"`
	DemoEnd              json.RawMessage `json:"demoEnd"`
	MovieOffset          json.RawMessage `json:"movieOffset"`
	CategoryIDs          json.RawMessage `json:"categoryIds"`
	Title                json.RawMessage `json:"title"`
	Subtitle             json.RawMessage `json:"subtitle"`
	TitleTranslations    json.RawMessage `json:"titleTranslations"`
	SubtitleTranslations json.RawMessage `json:"subtitleTranslations"`
}
type metadataTranslations struct{ Titles, Subtitles map[string]string }

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

// sourceMetadata parses the chart's stored TJA.
func (s *Server) sourceMetadata(ctx context.Context, tx pgx.Tx, id string) (tja.Metadata, error) {
	var key string
	if err := tx.QueryRow(ctx, `SELECT r.storage_key FROM charts c JOIN chart_resources r ON r.id=c.tja_file WHERE c.id=$1`, id).Scan(&key); err != nil {
		return tja.Metadata{}, err
	}
	f, err := s.Config.Objects.Open(ctx, key)
	if err != nil {
		return tja.Metadata{}, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, tja.MaxTJA+1))
	if err != nil {
		return tja.Metadata{}, err
	}
	if len(data) > tja.MaxTJA {
		return tja.Metadata{}, fmt.Errorf("source TJA exceeds limit")
	}
	parsed, issue := tja.Parse(data, tja.WaveName(data))
	if issue != nil {
		return tja.Metadata{}, issue
	}
	return parsed, nil
}

func (s *Server) editMetadata(w http.ResponseWriter, r *http.Request) {
	u, ok := s.required(w, r, true)
	if !ok {
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		problem(w, 415, "CONTENT_TYPE_INVALID", "请使用 JSON 请求")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32768)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var patch metadataPatch
	if decoder.Decode(&patch) != nil || decoder.Decode(new(any)) != io.EOF {
		problem(w, 400, "REQUEST_INVALID", "请求只能包含支持的展示字段和一个 JSON 对象")
		return
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var owner []string
	var translations metadataTranslations
	var sourceTitle, sourceSubtitle string
	err = tx.QueryRow(r.Context(), `SELECT owner_ids,title,subtitle,title_translations,subtitle_translations FROM charts c WHERE c.id=$1 AND `+publishedChart+` FOR UPDATE`, r.PathValue("id")).Scan(&owner, &sourceTitle, &sourceSubtitle, &translations.Titles, &translations.Subtitles)
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, 404, "CHART_NOT_FOUND", "作品不存在或已下架")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if !slices.Contains(owner, u.User.ID) && !u.User.IsAdmin {
		problem(w, 403, "FORBIDDEN", "只有作品作者或网站管理员可以修改信息")
		return
	}
	// The names and translations the site shows are the chart's own: an edit replaces them,
	// and nothing here reads the stored TJA's headers.
	isNull := func(raw json.RawMessage) bool { return bytes.Equal(bytes.TrimSpace(raw), []byte("null")) }
	title, subtitle := sourceTitle, sourceSubtitle
	if len(patch.Title) > 0 || len(patch.Subtitle) > 0 {
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
	}
	if translations.Titles == nil {
		translations.Titles = map[string]string{}
	}
	if translations.Subtitles == nil {
		translations.Subtitles = map[string]string{}
	}
	if !patchTranslations(patch.TitleTranslations, translations.Titles) || !patchTranslations(patch.SubtitleTranslations, translations.Subtitles) {
		problem(w, 422, "TRANSLATIONS_INVALID", "译名必须为 en / ja / zh / ko 到不超过 500 字节文本的映射")
		return
	}
	if len(patch.Description) > 0 {
		var description string
		if bytes.Equal(bytes.TrimSpace(patch.Description), []byte("null")) || json.Unmarshal(patch.Description, &description) != nil || len([]rune(description)) > 1000 || strings.ContainsRune(description, 0) {
			problem(w, 422, "DESCRIPTION_INVALID", "谱面介绍最多 1000 字符，不能包含空字符")
			return
		}
		if _, err = tx.Exec(r.Context(), `UPDATE charts SET description=$2 WHERE id=$1`, r.PathValue("id"), description); err != nil {
			internal(w, err)
			return
		}
	}
	if len(patch.DifficultyMakers) > 0 {
		difficulties, err := chartDifficulties(r.Context(), tx, r.PathValue("id"))
		if err != nil {
			internal(w, err)
			return
		}
		if err = applyDifficultyMakers(string(patch.DifficultyMakers), difficulties); err != nil {
			problem(w, 422, "MAKERS_INVALID", err.Error())
			return
		}
		// A changed block refreshes the chart's update_at by trigger.
		for _, d := range difficulties {
			if _, err = tx.Exec(r.Context(), `UPDATE chart_difficulties SET maker=$2 WHERE id=$1`, d.ID, d.Maker); err != nil {
				internal(w, err)
				return
			}
		}
	}
	newPreview := false
	if len(patch.DemoStart)+len(patch.DemoEnd) > 0 {
		c, e := s.readChart(tx.QueryRow(r.Context(), chartSelect+` WHERE c.id=$1`, r.PathValue("id")))
		if e != nil {
			internal(w, e)
			return
		}
		start, end := c.DemoStart, c.DemoEnd
		valid := true
		for _, field := range []struct {
			raw   json.RawMessage
			value *float64
		}{{patch.DemoStart, &start}, {patch.DemoEnd, &end}} {
			if len(field.raw) > 0 && (bytes.Equal(bytes.TrimSpace(field.raw), []byte("null")) || json.Unmarshal(field.raw, field.value) != nil) {
				valid = false
			}
		}
		if !valid || !audio.ValidPreviewRange(start, end, c.Duration) {
			problem(w, 422, "PREVIEW_RANGE_INVALID", "试听起点必须位于音频内，终点必须晚于起点且不超过 1215 秒；超出音频的部分会截到结尾")
			return
		}
		if start != c.DemoStart || end != c.DemoEnd || c.PreviewPath == "" {
			newPreview = true
			if _, e = tx.Exec(r.Context(), `UPDATE charts SET demo_start=$2,demo_end=$3 WHERE id=$1`, c.ID, start, end); e != nil {
				internal(w, e)
				return
			}
		}
	}
	// MOVIEOFFSET is kept in the stored TJA too, where players read it.
	if len(patch.MovieOffset) > 0 {
		var offset, current float64
		var movieSHA string
		if bytes.Equal(bytes.TrimSpace(patch.MovieOffset), []byte("null")) || json.Unmarshal(patch.MovieOffset, &offset) != nil || !video.ValidOffset(offset) {
			problem(w, 422, "TJA_MOVIE_OFFSET_INVALID", "MOVIEOFFSET 必须为秒数，绝对值不超过 1200")
			return
		}
		if err = tx.QueryRow(r.Context(), `SELECT c.movie_offset,COALESCE(mf.sha256,'') FROM charts c LEFT JOIN chart_resources mf ON mf.id=c.movie_file WHERE c.id=$1`, r.PathValue("id")).Scan(&current, &movieSHA); err != nil {
			internal(w, err)
			return
		}
		if movieSHA == "" {
			problem(w, 422, "MOVIE_NOT_FOUND", "该谱面没有背景视频，无法设置 MOVIEOFFSET")
			return
		}
		if offset != current {
			if err = s.rewriteMovie(r.Context(), tx, r.PathValue("id"), movieSHA, nil, offset); err != nil {
				internal(w, err)
				return
			}
		}
	}

	if len(patch.CategoryIDs) > 0 {
		ids, e := categorySelection(patch.CategoryIDs)
		if e != nil {
			problem(w, 422, "CATEGORIES_INVALID", "分类必须为分类 ID 数组")
			return
		}
		if !validCategories(ids) {
			problem(w, 422, "CATEGORIES_INVALID", "包含不存在的分类，请刷新后重试")
			return
		}
		if e = setCategories(r.Context(), tx, r.PathValue("id"), ids); e != nil {
			internal(w, e)
			return
		}
	}
	if len(patch.Title)+len(patch.Subtitle)+len(patch.TitleTranslations)+len(patch.SubtitleTranslations) > 0 {
		_, err = tx.Exec(r.Context(), `UPDATE charts SET title=$4,subtitle=$5,title_translations=$2,subtitle_translations=$3 WHERE id=$1`, r.PathValue("id"), translations.Titles, translations.Subtitles, title, subtitle)
		if err != nil {
			internal(w, err)
			return
		}
	}
	chart, err := s.readChart(tx.QueryRow(r.Context(), chartSelect+` WHERE c.id=$1`, r.PathValue("id")))
	if err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		internal(w, err)
		return
	}
	if newPreview {
		// The clip is cut once the new range stands; a failure is left to the backfill.
		s.previewAfter(r.Context(), chart.ID, "")
		if chart, err = s.readChart(s.DB.QueryRow(r.Context(), chartSelect+` WHERE c.id=$1`, chart.ID)); err != nil {
			internal(w, err)
			return
		}
	}
	chart.setOwners(s.publicProfiles(r.Context(), chart.OwnerIDs))
	respond(w, 200, chart)
}
