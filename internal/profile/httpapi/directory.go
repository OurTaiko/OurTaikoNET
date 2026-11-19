package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"ourtaiko.dev/net/internal/account"
	"ourtaiko.dev/net/internal/files"
	"ourtaiko.dev/net/internal/netdb"
	"ourtaiko.dev/net/internal/web"
)

const directoryPageSize = 12

// PlayerTotals is what a player did on the whole site: every source added together. Never add
// login names, email, roles or credentials here: this is public.
type PlayerTotals struct {
	FirstLoginAt *time.Time `json:"firstLoginAt"`
	LastActiveAt *time.Time `json:"lastActiveAt"`
	ChartCount   int64      `json:"chartCount"`
	ScoreCount   int64      `json:"scoreCount"`
	CommentCount int64      `json:"commentCount"`
	// Karma counts other users' votes on what of the player's is on show: songs, comments and
	// courses.
	Karma int64 `json:"karma"`
	// PlaylistCount counts public playlists only.
	PlaylistCount int64 `json:"playlistCount"`
}

// DirectoryUser is one player of the directory. Nickname is null while the account center
// cannot say it.
type DirectoryUser struct {
	ID         string  `json:"id"`
	Nickname   *string `json:"nickname"`
	AvatarPath string  `json:"avatarPath"`
	PlayerTotals
}

// UserDirectory is GET /api/users.
type UserDirectory struct {
	Items             []DirectoryUser `json:"items"`
	Total             int             `json:"total"`
	Page              int             `json:"page"`
	PageSize          int             `json:"pageSize"`
	ProfilesAvailable bool            `json:"profilesAvailable"`
}

// overSources adds up what each source says.
func overSources(parts []string) string {
	return `(` + strings.Join(parts, `+`) + `)::bigint`
}

// karmaTotal is the karma of a row u of net.users, over all sources.
var karmaTotal = func() string {
	var karma []string
	for _, source := range netdb.Sources {
		schema := pgx.Identifier{source}.Sanitize()
		karma = append(karma,
			`(SELECT COALESCE(sum(v.value),0) FROM `+schema+`.comment_votes v JOIN `+schema+`.comments m ON m.id=v.comment_id LEFT JOIN `+schema+`.charts c ON c.id=m.chart_id LEFT JOIN `+schema+`.dan_courses d ON d.id=m.dan_id
 WHERE m.author_id=u.id AND v.user_id<>u.id AND `+netdb.VisibleComment+`)`,
			`(SELECT COALESCE(sum(v.value),0) FROM `+schema+`.chart_votes v JOIN `+schema+`.charts c ON c.id=v.chart_id WHERE c.owner_ids @> ARRAY[u.id] AND v.user_id<>u.id AND `+netdb.PublishedChart+`)`,
			`(SELECT COALESCE(sum(v.value),0) FROM `+schema+`.dan_votes v JOIN `+schema+`.dan_courses d ON d.id=v.dan_id WHERE d.owner_id=u.id AND v.user_id<>u.id AND `+netdb.PublishedDan+`)`)
	}
	return overSources(karma)
}()

// totalsColumns selects a player's PlayerTotals from a row u of net.users. Official charts
// belong to the built-in account, so adding the sources counts only what players made.
var totalsColumns = func() string {
	var charts, scores, comments, playlists []string
	for _, source := range netdb.Sources {
		schema := pgx.Identifier{source}.Sanitize()
		charts = append(charts, `(SELECT count(*) FROM `+schema+`.charts c WHERE c.owner_ids @> ARRAY[u.id] AND `+netdb.PublishedChart+`)`)
		scores = append(scores, `(SELECT count(*) FROM `+schema+`.scores sc JOIN `+schema+`.chart_difficulties d ON d.id=sc.difficulty_id JOIN `+schema+`.charts c ON c.id=d.chart_id
 WHERE sc.user_id=u.id AND `+netdb.PublishedChart+`)`)
		comments = append(comments, `(SELECT count(*) FROM `+schema+`.comments m LEFT JOIN `+schema+`.charts c ON c.id=m.chart_id LEFT JOIN `+schema+`.dan_courses d ON d.id=m.dan_id
 WHERE m.author_id=u.id AND `+netdb.VisibleComment+`)`)
		playlists = append(playlists, `(SELECT count(*) FROM `+schema+`.playlists p WHERE p.owner_id=u.id AND p.is_public)`)
	}
	return `u.first_login_at,u.last_active_at,` + strings.Join([]string{overSources(charts), overSources(scores), overSources(comments), karmaTotal, overSources(playlists)}, `,`)
}()

// totalsInto is where a row of totalsColumns is scanned, after any columns before it.
func totalsInto(t *PlayerTotals) []any {
	return []any{&t.FirstLoginAt, &t.LastActiveAt, &t.ChartCount, &t.ScoreCount, &t.CommentCount, &t.Karma, &t.PlaylistCount}
}

// playerTotals reads one player's totals; all zero for a player the site has not seen.
func (s *Server) playerTotals(ctx context.Context, id string) (PlayerTotals, error) {
	var totals PlayerTotals
	err := s.DB.QueryRow(ctx, `SELECT `+totalsColumns+` FROM net.users u WHERE u.id=$1`, id).Scan(totalsInto(&totals)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return PlayerTotals{}, nil
	}
	return totals, err
}

// queryPage reads ?page=, 1 when there is none. It reports false after answering.
func queryPage(w http.ResponseWriter, r *http.Request) (int, bool) {
	raw := r.URL.Query().Get("page")
	if raw == "" {
		return 1, true
	}
	page, err := strconv.Atoi(raw)
	if err != nil || page < 1 || page > 10000 {
		problem(w, 400, "QUERY_INVALID", "页码需要为 1–10000 的整数")
		return 0, false
	}
	return page, true
}

// listUsers is GET /api/users: the site's players with what they did on all sources together.
func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	q := strings.TrimSpace(query.Get("q"))
	page, ok := queryPage(w, r)
	if !ok {
		return
	}
	if len(q) > 200 {
		problem(w, 400, "QUERY_INVALID", "搜索内容过长")
		return
	}
	// sort_value is what the players are ranked by before their last visit, when it is not that.
	order, sortValue := "u.last_active_at DESC NULLS LAST,u.id", `0::bigint`
	switch query.Get("sort") {
	case "", "active":
	case "newest":
		order = "u.first_login_at DESC NULLS LAST,u.id"
	case "karma":
		order, sortValue = "sort_value DESC,"+order, karmaTotal
	default:
		problem(w, 400, "QUERY_INVALID", "排序方式无效")
		return
	}
	matching := []string{}
	if q != "" {
		if s.Config.SSO == nil {
			problem(w, 503, "SSO_NOT_CONFIGURED", "此服务器未接入账号中心，无法搜索用户")
			return
		}
		var err error
		if matching, err = s.Config.SSO.Search(r.Context(), q); err != nil {
			internal(w, err)
			return
		}
	}
	// Count, pagination and totals all describe the same database snapshot.
	tx, err := s.DB.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	result := UserDirectory{Items: []DirectoryUser{}, Page: page, PageSize: directoryPageSize, ProfilesAvailable: s.Config.SSO != nil}
	const filter = ` WHERE u.id<>'` + account.SystemUserID + `' AND ($1='' OR u.id=ANY($2::text[]))`
	if err = tx.QueryRow(r.Context(), `SELECT count(*) FROM net.users u`+filter, q, matching).Scan(&result.Total); err != nil {
		internal(w, err)
		return
	}
	rows, err := tx.Query(r.Context(), `WITH selected AS (
 SELECT u.id,u.first_login_at,u.last_active_at,`+sortValue+` AS sort_value FROM net.users u`+filter+`
 ORDER BY `+order+` LIMIT $3 OFFSET $4)
 SELECT u.id,`+totalsColumns+` FROM selected u ORDER BY `+order, q, matching, directoryPageSize, (page-1)*directoryPageSize)
	if err != nil {
		internal(w, err)
		return
	}
	ids := []string{}
	for rows.Next() {
		var user DirectoryUser
		if err = rows.Scan(append([]any{&user.ID}, totalsInto(&user.PlayerTotals)...)...); err != nil {
			rows.Close()
			internal(w, err)
			return
		}
		result.Items = append(result.Items, user)
		ids = append(ids, user.ID)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		internal(w, err)
		return
	}
	// The transaction is over before the account center is asked. An outage still lists the
	// players and their totals, but never passes for an empty search (above).
	if s.Config.SSO != nil && len(ids) > 0 {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		names, err := s.Config.SSO.Profiles(ctx, ids)
		if err != nil {
			result.ProfilesAvailable = false
		}
		for i := range result.Items {
			if p, ok := names[result.Items[i].ID]; ok {
				result.Items[i].Nickname, result.Items[i].AvatarPath = &p.Nickname, files.Path(p.AvatarURL)
			}
		}
	}
	web.Respond(w, http.StatusOK, result)
}
