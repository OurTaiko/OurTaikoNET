package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/text/unicode/norm"
	"ourtaiko.dev/net/internal/kit/audio"
	"ourtaiko.dev/net/internal/kit/cover"
	"ourtaiko.dev/net/internal/kit/tja"
	"ourtaiko.dev/net/internal/kit/video"
)

type stagedFile struct {
	name, path, sha string
	size            int64
}

var keyPattern = regexp.MustCompile(`^[a-zA-Z0-9-]{16,80}$`)

func uploadReadError(w http.ResponseWriter, e error) {
	var tooLarge *http.MaxBytesError
	if errors.As(e, &tooLarge) {
		problem(w, 413, "FILE_TOO_LARGE", "文件超过上传大小限制")
		return
	}
	problem(w, 400, "UPLOAD_INTERRUPTED", "上传中断或请求格式错误，请重试")
}

// uploader requires someone who may add charts: anyone signed in, or on the official source
// only its managers.
func (s *Server) uploader(w http.ResponseWriter, r *http.Request) (session, bool) {
	u, ok := s.required(w, r, true)
	if !ok {
		return u, false
	}
	if s.source().Official && !u.User.IsAdmin {
		problem(w, 403, "UPLOAD_FORBIDDEN", "只有管理员可以上传谱面")
		return u, false
	}
	return u, true
}

// ownIdentifiers refuses the identifiers of the official source's songs on a source whose
// charts have none. It reports false after answering.
func (s *Server) ownIdentifiers(w http.ResponseWriter, fields map[string]string) bool {
	if !s.source().Official && fields["nijiiroId"]+fields["officialId"] != "" {
		problem(w, 422, "IDENTIFIER_INVALID", "这个曲源的谱面没有 Nijiiro ID 和官方 ID")
		return false
	}
	return true
}

// identifiers reads the optional Nijiiro and official IDs; empty means none.
func identifiers(nijiiro, official string) (*int32, *string, bool) {
	var nijiiroID *int32
	var officialID *string
	if nijiiro = strings.TrimSpace(nijiiro); nijiiro != "" {
		n, e := strconv.ParseInt(nijiiro, 10, 32)
		if e != nil || n < 1 {
			return nil, nil, false
		}
		id := int32(n)
		nijiiroID = &id
	}
	if official = strings.TrimSpace(official); official != "" {
		if !officialIDPattern.MatchString(official) {
			return nil, nil, false
		}
		officialID = &official
	}
	return nijiiroID, officialID, true
}

// Text fields a chart submission may carry besides its files. title, subtitle and bpm
// describe a song registered without files; with a TJA they are ignored, as the file is the
// source of those.
var uploadFields = []string{"difficultyMakers", "description", "categoryIds", "nijiiroId", "officialId", "title", "subtitle", "bpm", "demoStart", "demoEnd"}

// A new chart also names the co-creators to invite; an existing one invites from its page.
var newUploadFields = append(append([]string{}, uploadFields...), "collaboratorIds")

// receiveUploadForm stages the multipart files tja, audio and cover of a
// submission in dir and collects the text fields named in allowed. On a bad
// request it has answered and returns false.
func receiveUploadForm(w http.ResponseWriter, r *http.Request, dir string, allowed []string) (map[string]*stagedFile, map[string]string, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 112*1024*1024)
	mr, e := r.MultipartReader()
	if e != nil {
		problem(w, 400, "UPLOAD_FILES_INVALID", "请选择 TJA 和 OGG 或 MP3 音频文件")
		return nil, nil, false
	}
	files := map[string]*stagedFile{}
	fields := map[string]string{}
	for {
		part, e := mr.NextPart()
		if e == io.EOF {
			break
		}
		if e != nil {
			uploadReadError(w, e)
			return nil, nil, false
		}
		disposition, params, e := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
		if e != nil || disposition != "form-data" {
			problem(w, 400, "UPLOAD_FILES_INVALID", "上传字段格式错误")
			return nil, nil, false
		}
		field, name := params["name"], params["filename"]
		switch {
		case field == "tja" || field == "audio" || field == "cover" || field == "bgimage" || field == "movie":
			validExtension := strings.EqualFold(filepath.Ext(name), ".tja")
			maxBytes := int64(tja.MaxTJA)
			if field == "audio" {
				validExtension = audio.MediaType(name) != ""
				maxBytes = tja.MaxAudio
			}
			if field == "cover" || field == "bgimage" {
				validExtension = cover.ValidExtension(name)
				maxBytes = cover.MaxBytes
			}
			if field == "movie" {
				validExtension = video.ValidExtension(name)
				maxBytes = video.MaxBytes
			}
			if files[field] != nil || !tja.SafeFilename(name) || !validExtension {
				problem(w, 400, "UPLOAD_FILES_INVALID", "文件类型、数量或文件名无效；封面与背景图片仅支持 JPG / PNG / WebP，视频仅支持 MP4 / WebM / MOV，文件名不能包含路径")
				return nil, nil, false
			}
			f := &stagedFile{name: name, path: filepath.Join(dir, field)}
			out, e := os.OpenFile(f.path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if e != nil {
				internal(w, e)
				return nil, nil, false
			}
			h := sha256.New()
			n, copyErr := io.Copy(io.MultiWriter(out, h), io.LimitReader(part, maxBytes+1))
			closeErr := out.Close()
			if copyErr != nil {
				uploadReadError(w, copyErr)
				return nil, nil, false
			}
			if closeErr != nil {
				internal(w, closeErr)
				return nil, nil, false
			}
			if n == 0 || n > maxBytes {
				problem(w, 413, "FILE_TOO_LARGE", "文件为空或超过上传大小限制")
				return nil, nil, false
			}
			f.size, f.sha = n, hex.EncodeToString(h.Sum(nil))
			files[field] = f
		case slices.Contains(allowed, field):
			if _, exists := fields[field]; exists || name != "" {
				problem(w, 400, "UPLOAD_FILES_INVALID", "上传字段重复或格式错误")
				return nil, nil, false
			}
			limit := 4000
			if field == "difficultyMakers" {
				limit = 64 * 1024
			}
			data, e := io.ReadAll(io.LimitReader(part, int64(limit)+1))
			if e != nil {
				uploadReadError(w, e)
				return nil, nil, false
			}
			if len(data) > limit {
				if field == "difficultyMakers" {
					problem(w, 400, "UPLOAD_FIELD_TOO_LONG", "制作者表格不能超过 64 KiB")
				} else {
					problem(w, 400, "DESCRIPTION_TOO_LONG", "说明不能超过 4000 字节")
				}
				return nil, nil, false
			}
			fields[field] = string(data)
		default:
			problem(w, 400, "UPLOAD_FILES_INVALID", "包含不支持的上传字段")
			return nil, nil, false
		}
		part.Close()
	}
	return files, fields, true
}

// upload publishes a new chart from a TJA, its audio, and an optional cover, background image
// and background movie: the request makes the quick checks and keeps the files; decoding,
// transcoding and storing follow in the background (upload_job.go). On the official source a
// form with neither a TJA nor an audio registers a song without files (createSong).
func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	u, ok := s.uploader(w, r)
	if !ok {
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if !keyPattern.MatchString(key) {
		problem(w, 400, "IDEMPOTENCY_KEY_INVALID", "需要有效的上传请求标识")
		return
	}
	select {
	case s.uploads <- struct{}{}:
		defer func() { <-s.uploads }()
	default:
		problem(w, 503, "UPLOAD_BUSY", "当前正在处理其他上传，请稍后重试")
		return
	}
	dir, e := os.MkdirTemp(s.Config.Storage, "staging-")
	if e != nil {
		internal(w, e)
		return
	}
	// The staged files belong to the request until a job accepts them.
	accepted := false
	defer func() {
		if !accepted {
			os.RemoveAll(dir)
		}
	}()
	files, fields, ok := receiveUploadForm(w, r, dir, newUploadFields)
	if !ok {
		return
	}
	categoryIDs, e := s.source().categorySelection(json.RawMessage(fields["categoryIds"]))
	if e != nil {
		problem(w, 422, "CATEGORIES_INVALID", "分类必须为已有分类 ID 的数组")
		return
	}
	flags, _ := s.source().encodeCategories(categoryIDs)
	if !s.ownIdentifiers(w, fields) {
		return
	}
	nijiiroID, officialID, ok := identifiers(fields["nijiiroId"], fields["officialId"])
	if !ok {
		problem(w, 422, "IDENTIFIER_INVALID", "Nijiiro ID 必须为正整数；官方 ID 只能包含字母、数字、点、下划线和连字符，最长 64 个字符")
		return
	}
	if !s.nijiiroFree(w, r, nijiiroID, "") {
		return
	}
	description := fields["description"]
	if !validDescription(description) {
		problem(w, 422, "DESCRIPTION_INVALID", "说明包含无效字符")
		return
	}
	collaborators, ok := s.uploadCollaborators(w, r, fields["collaboratorIds"], u.User.ID)
	if !ok {
		return
	}
	tf, af, cf, mv, bg := files["tja"], files["audio"], files["cover"], files["movie"], files["bgimage"]
	if s.source().Official && tf == nil && af == nil && mv == nil && bg == nil {
		s.createSong(w, r, u, key, fields, cf, categoryIDs, flags, nijiiroID, officialID, collaborators)
		return
	}
	if tf == nil || af == nil {
		message := "必须同时选择 TJA 和一个 OGG 或 MP3 音频"
		if s.source().Official {
			message += "，或两者都不选以仅登记乐曲"
		}
		problem(w, 400, "UPLOAD_FILES_INVALID", message)
		return
	}
	data, ok := normalizedTJA(w, tf)
	if !ok {
		return
	}
	meta, ok := parsedTJA(w, data, af.name)
	if !ok {
		return
	}
	if e := applyDifficultyMakers(fields["difficultyMakers"], meta.Difficulties); e != nil {
		problem(w, 422, "DIFFICULTY_MAKERS_INVALID", e.Error())
		return
	}
	keepMovie, ok := checkMovie(w, meta, mv, nil)
	if !ok {
		return
	}
	keepBackground, ok := checkImages(w, data, cf, bg, nil)
	if !ok {
		return
	}
	if !reservePending(w, s) {
		return
	}
	// Makers count as they were understood: the same makers listed in another order are the
	// same upload.
	makers := ""
	if fields["difficultyMakers"] != "" {
		for _, d := range meta.Difficulties {
			makers += d.Course + "/" + d.Side + "=" + d.Maker + "\n"
		}
	}
	parts := []string{tf.name, tf.sha, af.name, af.sha, description, strings.Join(categoryIDs, ","), fields["nijiiroId"], fields["officialId"], fields["demoStart"], fields["demoEnd"], makers}
	// A background image is listed last, so requests without one keep the digest they had.
	for _, f := range []*stagedFile{cf, mv, bg} {
		if f != nil {
			parts = append(parts, f.name, f.sha)
		}
	}
	// Requests without co-creators keep the digest they always had.
	if len(collaborators) > 0 {
		parts = append(parts, "collaborators", strings.Join(collaborators, ","))
	}
	job := &uploadJob{
		user: u.User.ID, key: key, dir: dir, tja: tf, audio: af, cover: cf, background: bg, movie: mv, normalized: data, meta: meta,
		description: description, flags: flags, nijiiroID: nijiiroID, officialID: officialID, fields: fields, keepMovie: keepMovie, keepBackground: keepBackground, collaborators: collaborators,
		// RFC 7240: the client asks not to wait for processing.
		async: strings.Contains(strings.ToLower(r.Header.Get("Prefer")), "respond-async"), done: make(chan uploadResult, 1),
	}
	accepted = s.acceptUpload(w, r, job, hash(strings.Join(parts, "\n")), nil)
}

// invalidTJA answers an upload whose TJA has an issue.
func invalidTJA(w http.ResponseWriter, issue *tja.Issue) {
	respond(w, 422, map[string]any{"code": issue.Code, "message": issue.Message, "errors": []*tja.Issue{issue}, "requestId": w.Header().Get("X-Request-ID"), "validationVersion": tja.Version})
}

// normalizedTJA reads an uploaded TJA as UTF-8 text. From here on the file is known by that
// text's hash, so the same chart sent in another encoding is the same upload. It reports
// false after answering.
func normalizedTJA(w http.ResponseWriter, tf *stagedFile) ([]byte, bool) {
	raw, e := os.ReadFile(tf.path)
	if e != nil {
		internal(w, e)
		return nil, false
	}
	data, issue := tja.NormalizeUTF8(raw)
	if issue != nil {
		invalidTJA(w, issue)
		return nil, false
	}
	tf.sha = hash(string(data))
	return data, true
}

// parsedTJA validates an uploaded TJA against the audio it names (waveCheck, or none when
// the stored audio stays). It reports false after answering.
func parsedTJA(w http.ResponseWriter, data []byte, waveCheck string) (tja.Metadata, bool) {
	meta, issue := tja.Parse(data, waveCheck)
	if issue != nil {
		invalidTJA(w, issue)
		return tja.Metadata{}, false
	}
	return meta, true
}

// checkMovie matches the BGMOVIE a TJA declares with the movie sent; a
// replacement may keep the chart's stored movie (keep reports that). It reports false after
// answering.
func checkMovie(w http.ResponseWriter, meta tja.Metadata, mv *stagedFile, existing *Chart) (keep, ok bool) {
	invalid := func(code, message, expected, actual string) {
		invalidTJA(w, &tja.Issue{Code: code, Message: message, Expected: expected, Actual: actual})
	}
	switch {
	case meta.Movie == "" && mv != nil:
		problem(w, 400, "UPLOAD_FILES_INVALID", "TJA 没有声明 BGMOVIE，不能上传视频")
		return false, false
	case meta.Movie != "" && mv == nil:
		// Declared without a file: a replacement keeps the stored movie, and a chart without
		// one is published without (BGMOVIE is cleared).
		return existing != nil && existing.MovieHash != "", true
	case mv != nil && norm.NFC.String(meta.Movie) != norm.NFC.String(mv.name):
		invalid("TJA_MOVIE_MISMATCH", fmt.Sprintf("BGMOVIE 引用了「%s」，但你选择的是「%s」。请选择对应文件，或修改 TJA 的 BGMOVIE 后重新选择。", meta.Movie, mv.name), meta.Movie, mv.name)
		return false, false
	}
	return false, true
}

// checkImages matches the images sent with the ones the TJA names: a cover
// must be PREIMAGE's when it declares one, and a background image BGIMAGE's, which it must
// declare. A replacement still declaring BGIMAGE without a new file keeps the chart's stored
// image (keep reports that). It reports false after answering.
func checkImages(w http.ResponseWriter, data []byte, cf, bg *stagedFile, existing *Chart) (keep, ok bool) {
	invalid := func(code, message, expected, actual string) {
		invalidTJA(w, &tja.Issue{Code: code, Message: message, Expected: expected, Actual: actual})
	}
	if declared := tja.HeaderFile(tja.SongHeader(data, "PREIMAGE")); cf != nil && declared != "" && norm.NFC.String(declared) != norm.NFC.String(cf.name) {
		invalid("TJA_PREIMAGE_MISMATCH", fmt.Sprintf("PREIMAGE 引用了「%s」，但你选择的封面是「%s」。请选择对应图片，或修改 TJA 的 PREIMAGE 后重新选择。", declared, cf.name), declared, cf.name)
		return false, false
	}
	switch declared := tja.HeaderFile(tja.SongHeader(data, "BGIMAGE")); {
	case declared == "" && bg != nil:
		problem(w, 400, "UPLOAD_FILES_INVALID", "TJA 没有声明 BGIMAGE，不能上传背景图片")
		return false, false
	case bg != nil && norm.NFC.String(declared) != norm.NFC.String(bg.name):
		invalid("TJA_BGIMAGE_MISMATCH", fmt.Sprintf("BGIMAGE 引用了「%s」，但你选择的背景图片是「%s」。请选择对应图片，或修改 TJA 的 BGIMAGE 后重新选择。", declared, bg.name), declared, bg.name)
		return false, false
	case declared != "" && bg == nil:
		return existing != nil && existing.BackgroundHash != "", true
	}
	return false, true
}

// reservePending takes a place in the bounded queue of accepted uploads.
func reservePending(w http.ResponseWriter, s *Server) bool {
	select {
	case s.pending <- struct{}{}:
		return true
	default:
		problem(w, 503, "UPLOAD_BUSY", "当前排队处理的上传较多，请稍后重试")
		return false
	}
}
