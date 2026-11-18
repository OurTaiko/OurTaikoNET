package httpapi

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5"
	"ourtaiko.dev/net/internal/web"
)

// GamePlaylist is a folder of the game's song select: the user's own and
// favorite playlists and the system ones, as IDs of songs in the catalog.
type GamePlaylist struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Genre string `json:"genre"`
	// "owned", "favorite" or "system".
	Kind    string `json:"kind"`
	OwnerID string `json:"ownerId,omitempty"`
	Owner   string `json:"owner"`
	// The folder's picture, the WebP the website shows (the game decodes WebP): its file
	// (package files), SHA-256 and size; absent without one.
	SongListCoverPath   string   `json:"songListCoverPath,omitempty"`
	SongListCoverSHA256 string   `json:"songListCoverSha256,omitempty"`
	SongListCoverSize   int64    `json:"songListCoverSize,omitempty"`
	ChartIDs            []string `json:"chartIds"`
	// The stored cover, resolved after the snapshot.
	coverKey, coverHash string
	coverSize           int64
}

// gamePlaylists reads the folders for user (nil for a guest) from the
// catalog snapshot. Empty playlists are left out; owner names and cover
// links are filled later by gamePlaylistDetails.
func (s *Server) gamePlaylists(ctx context.Context, tx pgx.Tx, user *User) ([]GamePlaylist, error) {
	items := []GamePlaylist{}
	viewer := ""
	if user != nil {
		viewer = user.ID
		rows, e := tx.Query(ctx, `SELECT p.id,p.name,p.category_flag,p.owner_id,p.owner_id=$1,COALESCE(gc.storage_key,''),COALESCE(gc.sha256,''),COALESCE(gc.byte_size,0),
 ARRAY(SELECT i.chart_id FROM playlist_items i JOIN charts c ON c.id=i.chart_id WHERE i.playlist_id=p.id AND `+systemChart+` ORDER BY i.position,i.added_at,i.chart_id)
 FROM playlists p LEFT JOIN playlist_favorites mine ON mine.playlist_id=p.id AND mine.user_id=$1
 LEFT JOIN chart_resources gc ON gc.id=p.cover_file
 WHERE p.owner_id=$1 OR (mine.user_id IS NOT NULL AND p.is_public)
 ORDER BY p.owner_id<>$1,CASE WHEN p.owner_id=$1 THEN p.updated_at ELSE mine.favorited_at END DESC,p.id`, viewer)
		if e != nil {
			return nil, e
		}
		for rows.Next() {
			p := GamePlaylist{Kind: "favorite"}
			var flag CategoryFlags
			var own bool
			if e = rows.Scan(&p.ID, &p.Title, &flag, &p.OwnerID, &own, &p.coverKey, &p.coverHash, &p.coverSize, &p.ChartIDs); e != nil {
				rows.Close()
				return nil, e
			}
			if own {
				p.Kind = "owned"
			}
			p.Genre = s.source().categoryByFlag(flag).Genre
			if len(p.ChartIDs) > 0 {
				items = append(items, p)
			}
		}
		rows.Close()
		if e = rows.Err(); e != nil {
			return nil, e
		}
	}
	for i := range systemPlaylists {
		p := &systemPlaylists[i]
		if p.personal && user == nil {
			continue
		}
		ids, e := p.chartIDs(ctx, tx, viewer)
		if e != nil {
			return nil, e
		}
		if len(ids) > 0 {
			items = append(items, GamePlaylist{ID: p.id, Title: p.name, Genre: s.source().playlistCategory(p.id).Genre, Kind: "system", ChartIDs: ids})
		}
	}
	return items, nil
}

// personalGamePlaylists keeps the folders that exist only for an account: its
// own and favorite playlists and the personal system ones. The public system
// playlists are part of the catalog.
func personalGamePlaylists(items []GamePlaylist) []GamePlaylist {
	personal := []GamePlaylist{}
	for _, p := range items {
		if system := systemPlaylistByID(p.ID); p.Kind != "system" || (system != nil && system.personal) {
			personal = append(personal, p)
		}
	}
	return personal
}

// gamePlaylistList returns the signed-in account's own folders, which the
// public catalog cannot carry. A client adds them to the catalog's playlists.
func (s *Server) gamePlaylistList(w http.ResponseWriter, r *http.Request) {
	u, ok := s.required(w, r, false)
	if !ok {
		return
	}
	tx, err := s.DB.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	playlists, err := s.gamePlaylists(r.Context(), tx, &u.User)
	if err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		internal(w, err)
		return
	}
	playlists = personalGamePlaylists(playlists)
	if err = s.gamePlaylistDetails(r.Context(), playlists); err != nil {
		internal(w, err)
		return
	}
	web.Respond(w, 200, map[string]any{"playlists": playlists, "playlistVersion": 1})
}

func (s *Server) gamePlaylistDetails(ctx context.Context, items []GamePlaylist) error {
	ids := []string{}
	for i, p := range items {
		if p.OwnerID != "" {
			ids = append(ids, p.OwnerID)
		}
		if p.coverKey != "" {
			items[i].SongListCoverPath, items[i].SongListCoverSHA256, items[i].SongListCoverSize = s.filePath(p.coverKey), p.coverHash, p.coverSize
		}
	}
	if len(ids) == 0 {
		return nil
	}
	profiles := s.publicProfiles(ctx, ids)
	for i := range items {
		if items[i].OwnerID != "" {
			items[i].Owner = profiles[items[i].OwnerID].Nickname
		}
	}
	return nil
}
