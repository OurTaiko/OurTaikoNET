package httpapi

import (
	"net/http"

	"github.com/jackc/pgx/v5"
	"ourtaiko.dev/net/internal/web"
)

// setPlaylistCover points a playlist at its cover, or at none, and returns the
// file it pointed at before.
func setPlaylistCover(r *http.Request, tx pgx.Tx, id string, file *int64) ([]*int64, error) {
	var previous []*int64
	e := tx.QueryRow(r.Context(), `SELECT ARRAY[cover_file] FROM playlists WHERE id=$1`, id).Scan(&previous)
	if e == nil {
		_, e = tx.Exec(r.Context(), `UPDATE playlists SET cover_file=$2,updated_at=now() WHERE id=$1`, id, file)
	}
	return previous, e
}

func (s *Server) replacePlaylistCover(w http.ResponseWriter, r *http.Request) {
	u, ok := s.required(w, r, true)
	if !ok {
		return
	}
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
	var previous []*int64
	// Ownership is checked after encoding, under the playlist's lock.
	_, ok = s.ownPlaylist(w, r, u.User.ID, func(tx pgx.Tx, id string) (bool, error) {
		file, e := s.storeCover(r.Context(), tx, encoded)
		if e != nil {
			return false, e
		}
		previous, e = setPlaylistCover(r, tx, id, &file)
		return true, e
	})
	if !ok {
		return
	}
	s.releaseCovers(r.Context(), previous)
	digest := web.Hash(string(encoded))
	web.Respond(w, 200, map[string]string{"coverHash": digest, "coverUrl": s.coverURL(digest + ".webp")})
}

func (s *Server) removePlaylistCover(w http.ResponseWriter, r *http.Request) {
	u, ok := s.required(w, r, true)
	if !ok {
		return
	}
	var previous []*int64
	_, ok = s.ownPlaylist(w, r, u.User.ID, func(tx pgx.Tx, id string) (bool, error) {
		var e error
		previous, e = setPlaylistCover(r, tx, id, nil)
		return true, e
	})
	if !ok {
		return
	}
	s.releaseCovers(r.Context(), previous)
	web.Respond(w, 200, map[string]string{"coverHash": "", "coverUrl": ""})
}
