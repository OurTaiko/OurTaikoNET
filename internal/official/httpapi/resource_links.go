package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"ourtaiko.dev/net/internal/kit/audio"
	"ourtaiko.dev/net/internal/kit/objectstore"
	"ourtaiko.dev/net/internal/kit/video"
)

// resourceLinks lists where a client downloads a chart's current files: the
// CDN address when S3 has one, else this API's own file routes under
// APP_ORIGIN. API credentials are never needed for these links.
func (s *Server) resourceLinks(w http.ResponseWriter, r *http.Request) {
	id, ok := chartID(w, r)
	if !ok {
		return
	}
	c, e := readChart(s.DB.QueryRow(r.Context(), chartSelect+` WHERE c.id=$1 AND `+publishedChart, id))
	if errors.Is(e, pgx.ErrNoRows) {
		problem(w, 404, "CHART_NOT_FOUND", "谱面不存在或已删除")
		return
	}
	if e != nil {
		internal(w, e)
		return
	}
	if !c.HasFiles {
		problem(w, 404, "CHART_FILES_MISSING", "该乐曲尚未上传谱面文件")
		return
	}
	link := func(key, route, digest, media string, size int64) (resourceLink, error) {
		target := ""
		if remote, ok := s.Config.Objects.(*objectstore.S3); ok {
			var e error
			if target, e = remote.PublicURL(key); e != nil {
				return resourceLink{}, e
			}
		}
		if target == "" {
			target = s.Config.Origin + "/api/v1/charts/" + c.ID + "/" + route
		}
		return resourceLink{target, target, digest, size, media}, nil
	}
	tjaLink, e := link(c.TJAKey, "tja", c.TJAHash, "application/octet-stream", c.TJASize)
	if e != nil {
		internal(w, e)
		return
	}
	audioLink, e := link(c.AudioKey, "audio", c.AudioHash, audio.MediaType(c.AudioName), c.AudioSize)
	if e != nil {
		internal(w, e)
		return
	}
	resources := map[string]resourceLink{"tja": tjaLink, "audio": audioLink}
	// The song-select preview clip, as Fanmade lists it; a chart waiting for the backfill has none.
	var previewKey, previewHash string
	var previewSize int64
	e = s.DB.QueryRow(r.Context(), `SELECT r.storage_key,r.sha256,r.byte_size FROM charts c JOIN chart_resources r ON r.id=c.preview_file WHERE c.id=$1`, c.ID).Scan(&previewKey, &previewHash, &previewSize)
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		internal(w, e)
		return
	}
	if e == nil {
		if resources["preview"], e = link(previewKey, "preview", previewHash, "audio/ogg", previewSize); e != nil {
			internal(w, e)
			return
		}
	}
	if c.MovieKey != "" {
		if resources["movie"], e = link(c.MovieKey, "movie", c.MovieHash, video.MediaType, c.MovieSize); e != nil {
			internal(w, e)
			return
		}
	}
	if c.BackgroundKey != "" {
		if resources["bgimage"], e = link(c.BackgroundKey, "bgimage", c.BackgroundHash, "image/webp", c.BackgroundSize); e != nil {
			internal(w, e)
			return
		}
	}
	// The cover too, as Fanmade lists it: the website's chart package carries it.
	var coverKey, coverHash string
	var coverSize int64
	e = s.DB.QueryRow(r.Context(), `SELECT r.storage_key,r.sha256,r.byte_size FROM charts c JOIN chart_resources r ON r.id=c.cover_file WHERE c.id=$1`, c.ID).Scan(&coverKey, &coverHash, &coverSize)
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		internal(w, e)
		return
	}
	if e == nil {
		if resources["cover"], e = link(coverKey, "cover", coverHash, "image/webp", coverSize); e != nil {
			internal(w, e)
			return
		}
	}
	respond(w, 200, map[string]any{"chartId": c.ID, "urlsExpire": false, "resources": resources})
}

// resourceLink tells a client where to download one stored file and how to
// verify it.
type resourceLink struct {
	URL         string `json:"url"`
	HeadURL     string `json:"headUrl"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
	ContentType string `json:"contentType"`
}

// directLink is the public address of an object, or one signed for 15 minutes
// when the bucket has no public address.
func directLink(ctx context.Context, remote *objectstore.S3, key, name, media, digest string, size int64) (resourceLink, error) {
	public, e := remote.PublicURL(key)
	if e != nil || public != "" {
		return resourceLink{public, public, digest, size, media}, e
	}
	get, e := remote.Sign(ctx, key, "GET", name, media, 15*time.Minute)
	if e != nil {
		return resourceLink{}, e
	}
	head, e := remote.Sign(ctx, key, "HEAD", name, media, 15*time.Minute)
	return resourceLink{get, head, digest, size, media}, e
}
