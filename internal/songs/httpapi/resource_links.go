package httpapi

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"ourtaiko.dev/net/internal/kit/audio"
	"ourtaiko.dev/net/internal/kit/objectstore"
	"ourtaiko.dev/net/internal/kit/video"
	"ourtaiko.dev/net/internal/web"
)

// resourceLink tells a client where to download one stored file and how to
// verify it.
type resourceLink struct {
	URL         string `json:"url"`
	HeadURL     string `json:"headUrl"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
	ContentType string `json:"contentType"`
	FallbackURL string `json:"fallbackUrl,omitempty"`
}

// directLink is the public address of an object, or one signed for 15 minutes
// when the bucket has no public address.
func directLink(ctx context.Context, remote *objectstore.S3, key, name, media, digest string, size int64) (resourceLink, error) {
	public, e := remote.PublicURL(key)
	if e != nil || public != "" {
		return resourceLink{URL: public, HeadURL: public, SHA256: digest, Size: size, ContentType: media}, e
	}
	get, e := remote.Sign(ctx, key, "GET", name, media, 15*time.Minute)
	if e != nil {
		return resourceLink{}, e
	}
	head, e := remote.Sign(ctx, key, "HEAD", name, media, 15*time.Minute)
	return resourceLink{URL: get, HeadURL: head, SHA256: digest, Size: size, ContentType: media}, e
}

func (s *Server) routeResource(r *http.Request, link resourceLink) resourceLink {
	if r.URL.Query().Get("origin") == "1" {
		return link
	}
	target := s.Config.Distribution.Select(r, link.URL, link.SHA256, link.Size)
	if target != link.URL {
		link.FallbackURL, link.URL, link.HeadURL = link.URL, target, target
	}
	return link
}

// resourceLinks lists where a client downloads a chart's current files and how to verify
// them: the bucket's public address, a link signed for 15 minutes when the bucket has none
// (urlsExpire), or this API's own file routes (Config.APIBase) when files are kept on this
// server's disk. A client never sends its API credentials to these links.
func (s *Server) resourceLinks(w http.ResponseWriter, r *http.Request) {
	// One snapshot: every key, hash and size is of one committed upload, even if a
	// replacement commits while the list is being put together.
	tx, e := s.DB.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		internal(w, e)
		return
	}
	defer tx.Rollback(r.Context())
	c, e := s.readChart(tx.QueryRow(r.Context(), chartSelect+` WHERE c.id=$1 AND `+publishedChart, r.PathValue("id")))
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
	remote, _ := s.Config.Objects.(*objectstore.S3)
	expire := false
	resources := map[string]resourceLink{}
	// kind is also the last part of this API's route for the file.
	add := func(kind, key, name, media, digest string, size int64) error {
		if remote == nil {
			route := s.apiBase() + "/charts/" + c.ID + "/" + kind
			resources[kind] = resourceLink{URL: route, HeadURL: route, SHA256: digest, Size: size, ContentType: media}
			return nil
		}
		link, e := directLink(r.Context(), remote, key, name, media, digest, size)
		resources[kind] = link
		expire = expire || link.URL != link.HeadURL
		return e
	}
	// stored reads the files a chart read does not carry: its cover and preview clip.
	stored := func(kind, column, name, media string) error {
		var key, digest string
		var size int64
		e := tx.QueryRow(r.Context(), `SELECT f.storage_key,f.sha256,f.byte_size FROM charts c JOIN chart_resources f ON f.id=c.`+column+` WHERE c.id=$1`, c.ID).Scan(&key, &digest, &size)
		if errors.Is(e, pgx.ErrNoRows) {
			return nil
		}
		if e != nil {
			return e
		}
		return add(kind, key, name, media, digest, size)
	}
	e = add("tja", c.TJAKey, c.TJAName, "application/octet-stream", c.TJAHash, c.TJASize)
	if e == nil {
		e = add("audio", c.AudioKey, c.AudioName, audio.MediaType(c.AudioName), c.AudioHash, c.AudioSize)
	}
	if e == nil {
		e = stored("cover", "cover_file", "cover.webp", "image/webp")
	}
	// A chart waiting for its preview clip to be cut has none yet.
	if e == nil {
		e = stored("preview", "preview_file", "preview.ogg", "audio/ogg")
	}
	if e == nil && c.BackgroundKey != "" {
		e = add("bgimage", c.BackgroundKey, c.BackgroundHash+".webp", "image/webp", c.BackgroundHash, c.BackgroundSize)
	}
	if e == nil && c.MovieKey != "" {
		e = add("movie", c.MovieKey, c.MovieHash+"."+video.Extension, video.MediaType, c.MovieHash, c.MovieSize)
	}
	if e == nil {
		e = tx.Commit(r.Context())
	}
	if e != nil {
		internal(w, e)
		return
	}
	// Probe only after releasing the DB snapshot, with one shared deadline for all files.
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	request := r.WithContext(ctx)
	var wait sync.WaitGroup
	var lock sync.Mutex
	routed := make(map[string]resourceLink, len(resources))
	for kind, link := range resources {
		wait.Go(func() {
			selected := s.routeResource(request, link)
			lock.Lock()
			routed[kind] = selected
			lock.Unlock()
		})
	}
	wait.Wait()
	resources = routed
	web.Respond(w, 200, map[string]any{"urlsExpire": expire, "chartId": c.ID, "resources": resources})
}
