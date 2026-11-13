package httpapi

import (
	"context"
	"errors"
	"io"
	"log"
	"mime"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5"
	"ourtaiko.dev/net/internal/kit/cover"
	"ourtaiko.dev/net/internal/kit/tja"
	"ourtaiko.dev/net/internal/web"
)

func encodeCover(w http.ResponseWriter, r *http.Request, name string, data []byte) ([]byte, bool) {
	encoded, err := cover.Encode(r.Context(), name, data)
	if errors.Is(err, cover.ErrInvalid) {
		problem(w, 422, "COVER_INVALID", "封面必须是有效的 JPG、PNG 或 WebP，最大 8 MiB、1600 万像素，单边不超过 8192 像素")
		return nil, false
	}
	if err != nil {
		internal(w, err)
		return nil, false
	}
	return encoded, true
}

func (s *Server) getCover(w http.ResponseWriter, r *http.Request) {
	key, _, digest, ok := s.publishedFile(w, r, "cover_file")
	if !ok {
		return
	}
	if requested := r.URL.Query().Get("v"); requested != "" && requested != digest {
		problem(w, 404, "COVER_NOT_FOUND", "该封面已被替换")
		return
	}
	s.serveCover(w, r, digest, key)
}

// replaceCover sets a published chart's cover on its own and points the stored TJA's PREIMAGE
// at it, as an upload does.
func (s *Server) replaceCover(w http.ResponseWriter, r *http.Request) {
	_, c, ok := s.managed(w, r)
	if !ok {
		return
	}
	// Encoding a large image is as heavy as an upload: one at a time.
	select {
	case s.uploads <- struct{}{}:
		defer func() { <-s.uploads }()
	default:
		problem(w, 503, "UPLOAD_BUSY", "当前正在处理其他上传，请稍后重试")
		return
	}
	encoded, ok := receiveCover(w, r)
	if !ok {
		return
	}
	ctx := context.WithoutCancel(r.Context())
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	var previous *int64
	var hasFiles bool
	err = tx.QueryRow(ctx, `SELECT cover_file,tja_file IS NOT NULL FROM charts c WHERE c.id=$1 AND `+publishedChart+` FOR UPDATE`, c.ID).Scan(&previous, &hasFiles)
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, 404, "CHART_NOT_FOUND", "谱面不存在或已删除")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	file, err := s.storeCover(ctx, tx, encoded)
	if err == nil {
		_, err = tx.Exec(ctx, `UPDATE charts SET cover_file=$2 WHERE id=$1`, c.ID, file)
	}
	// The stored TJA's PREIMAGE names the cover, as an upload writes it; a song registered
	// without files has none.
	var released []int64
	if err == nil && hasFiles {
		name := web.Hash(string(encoded)) + ".webp"
		var tjaFile int64
		tjaFile, released, err = s.rewriteStoredTJA(ctx, tx, c.ID, func(data []byte) ([]byte, bool) {
			stored, ok := tja.SetSongHeader(data, "PREIMAGE", name)
			return stored, ok && tja.SongHeader(stored, "PREIMAGE") == name
		})
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE charts SET tja_file=$2 WHERE id=$1`, c.ID, tjaFile)
		}
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		internal(w, err)
		return
	}
	if previous != nil && *previous != file {
		released = append(released, *previous)
	}
	if len(released) > 0 {
		if err = s.releaseFiles(ctx, released); err != nil {
			log.Printf("replaced cover cleanup pending: %v", err)
		}
	}
	web.Respond(w, 200, map[string]string{"coverHash": web.Hash(string(encoded))})
}

// storeCover stores an encoded WebP cover and returns its file ID.
func (s *Server) storeCover(ctx context.Context, tx pgx.Tx, data []byte) (int64, error) {
	return s.storeImageAs(ctx, tx, "cover", data, "webp", "image/webp")
}

// storeImageAs stores image bytes as {kind}.{extension} (kind: cover, bgimage) and returns the
// file ID.
func (s *Server) storeImageAs(ctx context.Context, tx pgx.Tx, kind string, data []byte, extension, media string) (int64, error) {
	tmp, e := os.CreateTemp(s.Config.Storage, "cover-*."+extension)
	if e != nil {
		return 0, e
	}
	defer os.Remove(tmp.Name())
	_, e = tmp.Write(data)
	if e = errors.Join(e, tmp.Close()); e != nil {
		return 0, e
	}
	return s.storeFile(ctx, tx, tmp.Name(), kind+"."+extension, extension, media, web.Hash(string(data)), int64(len(data)))
}

// receiveCover reads the single multipart file field "cover" and encodes it
// as the stored WebP. It reports false after writing the error response.
func receiveCover(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, cover.MaxBytes+64*1024)
	mr, err := r.MultipartReader()
	if err != nil {
		problem(w, 400, "COVER_INVALID", "请选择封面文件")
		return nil, false
	}
	part, err := mr.NextPart()
	if err != nil {
		uploadReadError(w, err)
		return nil, false
	}
	disposition, params, err := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
	name := params["filename"]
	if err != nil || disposition != "form-data" || params["name"] != "cover" || !tja.SafeFilename(name) || !cover.ValidExtension(name) {
		problem(w, 400, "COVER_INVALID", "只能上传一个 JPG、PNG 或 WebP 封面")
		return nil, false
	}
	data, err := io.ReadAll(io.LimitReader(part, cover.MaxBytes+1))
	if err != nil {
		uploadReadError(w, err)
		return nil, false
	}
	if len(data) > cover.MaxBytes {
		problem(w, 413, "FILE_TOO_LARGE", "封面不能超过 8 MiB")
		return nil, false
	}
	if _, err = mr.NextPart(); err != io.EOF {
		if err != nil {
			uploadReadError(w, err)
		} else {
			problem(w, 400, "COVER_INVALID", "只能上传一个封面文件")
		}
		return nil, false
	}
	return encodeCover(w, r, name, data)
}

// storeGameCover stores the copy of a playlist's cover that the game shows.
func (s *Server) storeGameCover(ctx context.Context, tx pgx.Tx, data []byte) (int64, error) {
	return s.storeImageAs(ctx, tx, "cover", data, cover.GameExtension, cover.GameMediaType)
}

// serveCover sends a cover image, or redirects to its CDN address.
func (s *Server) serveCover(w http.ResponseWriter, r *http.Request, digest, key string) {
	s.serveObject(w, r, key, "cover.webp", digest, "image/webp", "inline")
}

// releaseCovers deletes a playlist's former cover files unless something
// still refers to them; a failure leaves them for the next release.
func (s *Server) releaseCovers(ctx context.Context, covers []*int64) {
	ids := []int64{}
	for _, id := range covers {
		if id != nil {
			ids = append(ids, *id)
		}
	}
	if e := s.releaseFiles(context.WithoutCancel(ctx), ids); e != nil {
		log.Printf("playlist cover cleanup pending: %v", e)
	}
}
