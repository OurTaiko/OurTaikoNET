package httpapi

import (
	"net/http"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"ourtaiko.dev/net/internal/web"
)

// The playlists the home page recommends. The site's administrator chooses a few public
// playlists of any song source, puts them in order and may say a line about each. Only the
// choice is kept here (net.featured_playlists): a playlist's name, cover and songs are its
// source's to answer, at /api/sources/<source>/playlists.

// FeaturedPlaylist is one recommended playlist: which source it is in, its id there, and the
// administrator's line about it (empty for none).
type FeaturedPlaylist struct {
	Source     string `json:"source"`
	PlaylistID string `json:"playlistId"`
	Note       string `json:"note"`
}

const (
	maxFeaturedPlaylists = 6
	// In characters, as the table's check counts them.
	maxFeaturedNote = 40
)

// A stored playlist's id in its source: 32 hex digits. A system playlist's is a word, and
// those are every player's own or the server's reckoning, not something to recommend.
var storedPlaylistID = regexp.MustCompile(`^[0-9a-f]{32}$`)

// featuredPlaylists is GET /api/featured-playlists, public: the recommended playlists in the
// order they are shown, and how many there may be.
func (s *Server) featuredPlaylists(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.Query(r.Context(), `SELECT source,playlist_id,note FROM net.featured_playlists ORDER BY position`)
	if err != nil {
		internal(w, err)
		return
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByPos[FeaturedPlaylist])
	if err != nil {
		internal(w, err)
		return
	}
	web.Respond(w, 200, map[string]any{"items": items, "max": maxFeaturedPlaylists})
}

// cleanFeatured checks a list the administrator sent and trims its notes. It reports why
// the list cannot be kept, or "".
func (s *Server) cleanFeatured(items []FeaturedPlaylist) string {
	if len(items) > maxFeaturedPlaylists {
		return "推荐歌单数量超出上限"
	}
	seen := map[FeaturedPlaylist]bool{}
	for i := range items {
		item := &items[i]
		known := false
		for _, source := range s.Config.Sources {
			known = known || source.ID == item.Source
		}
		if !known || !storedPlaylistID.MatchString(item.PlaylistID) {
			return "歌单不存在"
		}
		item.Note = strings.TrimSpace(item.Note)
		if utf8.RuneCountInString(item.Note) > maxFeaturedNote || strings.ContainsFunc(item.Note, unicode.IsControl) {
			return "推荐语过长或含有换行"
		}
		key := FeaturedPlaylist{Source: item.Source, PlaylistID: item.PlaylistID}
		if seen[key] {
			return "同一个歌单只能推荐一次"
		}
		seen[key] = true
	}
	return ""
}

// setFeaturedPlaylists is PUT /api/admin/featured-playlists with
// {"items": [{"source": "fanmade", "playlistId": "…", "note": "…"}, …]}: from now on the home
// page recommends exactly these, in this order. Only the site's administrator may; every
// playlist named must be a public one of its source. An empty list recommends nothing.
func (s *Server) setFeaturedPlaylists(w http.ResponseWriter, r *http.Request) {
	v, ok := s.siteAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Items *[]FeaturedPlaylist `json:"items"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Items == nil {
		problem(w, 422, "FEATURED_INVALID", "需要 items 列表")
		return
	}
	items := *body.Items
	if reason := s.cleanFeatured(items); reason != "" {
		problem(w, 422, "FEATURED_INVALID", reason)
		return
	}
	ctx := r.Context()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	// One administrator's list at a time replaces the other's. Not a lock on the table: the
	// playlists' trigger writes to it while holding a playlist this is about to wait for.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('featured-playlists',0))`); err != nil {
		internal(w, err)
		return
	}
	for _, source := range s.Config.Sources {
		ids := []string{}
		for _, item := range items {
			if item.Source == source.ID {
				ids = append(ids, item.PlaylistID)
			}
		}
		if len(ids) == 0 {
			continue
		}
		// Held until the list is kept: a playlist turned private meanwhile waits, and its
		// trigger then takes it off the list.
		var public int
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 FROM `+pgx.Identifier{source.ID}.Sanitize()+`.playlists WHERE id=ANY($1) AND is_public FOR SHARE) found`, ids).Scan(&public); err != nil {
			internal(w, err)
			return
		}
		if public != len(ids) {
			problem(w, 422, "FEATURED_PLAYLIST_UNAVAILABLE", "有歌单不存在或未公开")
			return
		}
	}
	if _, err = tx.Exec(ctx, `DELETE FROM net.featured_playlists`); err != nil {
		internal(w, err)
		return
	}
	for position, item := range items {
		if _, err = tx.Exec(ctx, `INSERT INTO net.featured_playlists(source,playlist_id,position,note,changed_by) VALUES ($1,$2,$3,$4,$5)`,
			item.Source, item.PlaylistID, position, item.Note, v.User.ID); err != nil {
			internal(w, err)
			return
		}
	}
	if err = tx.Commit(ctx); err != nil {
		internal(w, err)
		return
	}
	web.Respond(w, 200, map[string]any{"items": items, "max": maxFeaturedPlaylists})
}
