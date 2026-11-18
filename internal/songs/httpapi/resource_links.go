package httpapi

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"ourtaiko.dev/net/internal/kit/audio"
	"ourtaiko.dev/net/internal/kit/objectstore"
	"ourtaiko.dev/net/internal/kit/video"
	"ourtaiko.dev/net/internal/web"
)

// resourceLink is one stored file a client downloads, and how it checks it: Path is the file
// under the distribution root (package files), joined by the client to the root it reads from.
type resourceLink struct {
	Path        string `json:"path"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
	ContentType string `json:"contentType"`
}

// filePath is the file stored at key as clients name it (package files): its path under the
// distribution root, the source's folder there and the key. "" for no key.
func (s *Server) filePath(key string) string {
	if key == "" {
		return ""
	}
	if remote, ok := s.Config.Objects.(*objectstore.S3); ok {
		return remote.Prefix + key
	}
	return s.source().Folder + key
}

// resourceLinks lists a chart's current files, each with what a client checks it by. A client
// reads them from its own root (package files) and never sends its API credentials there.
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
	resources := map[string]resourceLink{}
	add := func(kind, key, name, media, digest string, size int64) error {
		resources[kind] = resourceLink{Path: s.filePath(key), SHA256: digest, Size: size, ContentType: media}
		return nil
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
	web.Respond(w, 200, map[string]any{"chartId": c.ID, "resources": resources})
}

// withCoverPaths fills each playlist's SongListCoverPath.
func (s *Server) withCoverPaths(playlists []Playlist) {
	for i := range playlists {
		playlists[i].SongListCoverPath = s.filePath(playlists[i].coverKey)
	}
}
