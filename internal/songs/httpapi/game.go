package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"ourtaiko.dev/net/internal/account"
	"ourtaiko.dev/net/internal/signin"
	"ourtaiko.dev/net/internal/titlerules"
	"ourtaiko.dev/net/internal/web"
)

// The game API, as OurTaikoPlay speaks it: chart blocks as slots of course × side
// (chartSlotVersion 1), resource manifests (resourceDownloadVersion 1) and scores with an
// input recording (scoreReplayVersion 1). The game sends its account-center session as a
// bearer token to the game routes, where a browser cookie authorizes nothing; on every other
// route only the website's session counts, so a token kept in a game cannot do what the
// website can.
func isGameRequest(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, "/api/game/")
}

func (s *Server) gameLogin(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decode(w, r, &c) {
		return
	}
	var result struct {
		User    User   `json:"user"`
		Token   string `json:"accessToken"`
		Expires int    `json:"expiresIn"`
	}
	if s.Config.SSO == nil {
		if s.Config.DevUserID == "" {
			problem(w, 503, "LOGIN_UNAVAILABLE", "此服务器未接入账号中心")
			return
		}
		result.User, result.Token, result.Expires = devUser(s.Config.DevUserID), s.devGameToken(), 86400
	} else {
		e := s.Config.SSO.Call(r.Context(), "game/login", c, &result)
		var remote *account.Error
		if errors.As(e, &remote) {
			switch remote.Status {
			case 401:
				problem(w, 401, "LOGIN_INVALID", "用户名或密码不正确")
				return
			case 429:
				w.Header().Set("Retry-After", "600")
				problem(w, 429, "RATE_LIMITED", "尝试次数过多，请稍后重试")
				return
			case 400:
				problem(w, 400, "REQUEST_INVALID", "登录信息格式不正确")
				return
			}
		}
		if e != nil {
			internal(w, e)
			return
		}
		if !account.ValidSubject(result.User.ID) || len(result.Token) != 64 {
			internal(w, errors.New("invalid SSO login response"))
			return
		}
		result.User.AvatarURL = s.Config.SSO.AvatarURL(result.User)
	}
	shown := struct {
		User    account.Shown `json:"user"`
		Token   string        `json:"accessToken"`
		Expires int           `json:"expiresIn"`
	}{account.Show(result.User), result.Token, result.Expires}
	if e := signin.RecordActivity(r.Context(), s.DB, result.User.ID, true, titlerules.GameLogin); e != nil {
		internal(w, e)
		return
	}
	web.Respond(w, 200, shown)
}

// gameCatalog is the complete song list for a game client: the categories, every published
// chart with files that is in at least one, and the playlists everyone gets, read in one
// snapshot so the counts match the charts. It is the same for everyone and ignores any
// session; an account's scores are GET /game/scores and its own playlists GET /game/playlists.
func (s *Server) gameCatalog(w http.ResponseWriter, r *http.Request) {
	tx, err := s.DB.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	rows, err := tx.Query(r.Context(), chartSelect+` WHERE `+gameChart+` ORDER BY c.created_at DESC,c.id DESC`)
	if err != nil {
		internal(w, err)
		return
	}
	charts, err := s.readCharts(rows)
	if err != nil {
		internal(w, err)
		return
	}
	playlists, err := s.gamePlaylists(r.Context(), tx, nil)
	if err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		internal(w, err)
		return
	}
	counts := map[string]int{}
	for _, c := range charts {
		for _, id := range c.CategoryIDs {
			counts[id]++
		}
	}
	categories := make([]Category, 0, len(s.source().Categories))
	for _, entry := range s.source().Categories {
		category := entry.Category
		category.ChartCount = counts[category.ID]
		categories = append(categories, category)
	}
	s.chartNames(r.Context(), charts)
	if err = s.gamePlaylistDetails(r.Context(), playlists); err != nil {
		internal(w, err)
		return
	}
	body := map[string]any{
		"categories": categories, "charts": charts, "chartCount": len(charts), "playlists": playlists, "playlistVersion": 1,
		"chartSlotVersion": 1, "resourceDownloadVersion": 1, "scoreReplayVersion": 1, "danVersion": 1, "danResultsVersion": 1,
		"genreFrame": s.source().GenreFrame,
	}
	// Where charts carry their preview clip (audioPreview).
	if !s.source().Official {
		body["audioPreviewVersion"] = 1
	}
	if s.Config.SSO != nil {
		body["sso"] = s.Config.SSO.GameAccounts()
	}
	web.Respond(w, 200, body)
}

// gameScoreList returns the signed-in account's scores on published charts,
// those of replaced files included; the client prefers current ones.
func (s *Server) gameScoreList(w http.ResponseWriter, r *http.Request) {
	u, ok := s.required(w, r, false)
	if !ok {
		return
	}
	rows, err := s.DB.Query(r.Context(), scoreSelect+` JOIN charts c ON c.id=d.chart_id WHERE sc.user_id=$1 AND `+publishedChart+` ORDER BY sc.submitted_at,sc.id`, u.User.ID)
	if err != nil {
		internal(w, err)
		return
	}
	scores, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Score, error) { return readScore(row) })
	if err != nil {
		internal(w, err)
		return
	}
	if scores == nil {
		scores = []Score{}
	}
	web.Respond(w, 200, map[string]any{"scores": scores})
}

// devGameToken stands for the account center's game session of DEV_USER_ID on a local server
// without an account center.
func (s *Server) devGameToken() string {
	h := sha256.Sum256([]byte("ourtaiko-dev-game:" + s.Config.DevUserID))
	return hex.EncodeToString(h[:])
}
