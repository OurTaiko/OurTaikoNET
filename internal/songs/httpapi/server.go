package httpapi

import (
	"context"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"net/netip"
	"os"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"ourtaiko.dev/net/internal/account"
	"ourtaiko.dev/net/internal/kit/cover"
	"ourtaiko.dev/net/internal/kit/objectstore"
	"ourtaiko.dev/net/internal/kit/tja"
	"ourtaiko.dev/net/internal/kit/video"
	"ourtaiko.dev/net/internal/web"
)

type Config struct {
	// Source is the song source this server serves; nil is the Fanmade source.
	Source          *Source
	Origin, Storage string
	// Site is the built website; nil serves the API alone.
	Site         fs.FS
	Objects      objectstore.Store
	CookieSecure bool
	// SSO is nil when no account center is configured; sign-in is then
	// unavailable unless DevUserID is set.
	SSO *account.Client
	// DevUserID signs every "login" in as this user without a password. Local
	// development only: main refuses it with SSO or a non-loopback listener.
	DevUserID      string
	TrustedProxies []netip.Prefix
	// Profile is nil when 段位 attempts earn nothing (no OurTaikoProfile configured).
	Profile *ProfileClient
	// Awards, when set, records what 段位 attempts earn in the attempt's own transaction;
	// Profile is then used for its title catalog only.
	Awards Awarder
	// Manager, when set, says who may do what this code calls an administrator's work
	// (User.IsAdmin): edit or remove anyone's content here, and whatever else is checked
	// against it. The account center's own administrator flag is then not used. OurTaikoNet
	// sets it to membership of a user group.
	Manager func(ctx context.Context, userID string) (bool, error)
	// SessionCookie names the cookie a website session is kept in; empty is this site's own
	// name. OurTaikoNet gives every part the same name: one sign-in for the whole site.
	SessionCookie string
}

func (s *Server) sessionCookie() string {
	if s.Config.SessionCookie != "" {
		return s.Config.SessionCookie
	}
	return s.source().SessionCookie
}

type Server struct {
	DB      *pgxpool.Pool
	Config  Config
	uploads chan struct{}
	// Accepted uploads: pending bounds how many wait with staged files.
	pending chan struct{}
	jobs    chan *uploadJob
	worker  sync.Once
	mu      sync.Mutex
	limits  map[string]window
}
type window struct {
	since time.Time
	count int
}

func New(pool *pgxpool.Pool, cfg Config) *Server {
	if cfg.Objects == nil {
		cfg.Objects = objectstore.Local{Root: cfg.Storage}
	}
	return &Server{DB: pool, Config: cfg, uploads: make(chan struct{}, 2), pending: make(chan struct{}, maxPendingUploads), jobs: make(chan *uploadJob, maxPendingUploads), limits: map[string]window{}}
}

// answers is how a song source answers a request it cannot serve (web.Problems): every such
// answer also says which version of the TJA rules this server judges uploads by.
var answers = web.Problems{Extra: map[string]any{"validationVersion": tja.Version}}

// The names this package calls them by.
var problem, internal, decode, decodeLimit = answers.Problem, answers.Internal, answers.Decode, answers.DecodeLimit

// storing is in front of the routes that put a file in the store. A server whose store is
// only read (a preview beside the site that owns the files) says so at once, before a
// player has sent a whole chart.
func (s *Server) storing(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.ReadOnlyFiles() {
			problem(w, http.StatusServiceUnavailable, "FILES_READ_ONLY", "预览站不保存文件，请到正式站上传")
			return
		}
		next(w, r)
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { web.Respond(w, 200, map[string]bool{"ok": true}) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if e := s.DB.Ping(ctx); e != nil {
			internal(w, e)
			return
		}
		web.Respond(w, 200, map[string]bool{"ok": true})
	})
	mux.HandleFunc("GET /api/upload-rules", func(w http.ResponseWriter, r *http.Request) {
		web.Respond(w, 200, map[string]any{"validationVersion": tja.Version, "maxTjaBytes": tja.MaxTJA, "maxAudioBytes": tja.MaxAudio, "maxCoverBytes": cover.MaxBytes, "coverExtensions": []string{".jpg", ".png", ".webp"}, "encodings": []string{"utf-8"}, "audioCodecs": []string{"vorbis", "mp3"}, "audioExtensions": []string{".ogg", ".mp3"},
			"maxMovieBytes": video.MaxBytes, "movieExtensions": []string{".mp4", ".webm", ".mov"},
			"movieOutput": map[string]any{"codec": "h264", "extension": "." + video.Extension, "maxWidth": video.MaxWidth, "maxHeight": video.MaxHeight, "maxFps": video.MaxFPS, "maxKbps": video.MaxKbps, "audio": false}})
	})
	mux.HandleFunc("GET /api/categories", s.categories)
	mux.HandleFunc("GET /api/auth/sso/login", func(w http.ResponseWriter, r *http.Request) { s.sessions().Login(w, r) })
	mux.HandleFunc("GET /api/auth/sso/callback", func(w http.ResponseWriter, r *http.Request) { s.sessions().Callback(w, r) })
	mux.HandleFunc("GET /api/auth/account/{page}", func(w http.ResponseWriter, r *http.Request) { s.sessions().AccountRedirect(w, r) })
	mux.HandleFunc("POST /api/auth/logout", func(w http.ResponseWriter, r *http.Request) { s.sessions().Logout(w, r) })
	mux.HandleFunc("GET /api/me", s.me)
	mux.HandleFunc("GET /api/me/charts", s.mine)
	mux.HandleFunc("GET /api/charts", s.list)
	mux.HandleFunc("POST /api/charts", s.storing(s.upload))
	mux.HandleFunc("GET /api/charts/{id}", s.detail)
	mux.HandleFunc("PATCH /api/charts/{id}", s.editMetadata)
	mux.HandleFunc("DELETE /api/charts/{id}", s.remove)
	mux.HandleFunc("PUT /api/charts/{id}/cover", s.storing(s.replaceCover))
	mux.HandleFunc("PUT /api/charts/{id}/movie", s.storing(s.replaceMovie))
	mux.HandleFunc("PUT /api/charts/{id}/files", s.storing(s.replaceFiles))
	mux.HandleFunc("POST /api/charts/{id}/files/preview", s.previewReplacement)
	mux.HandleFunc("GET /api/charts/{id}/tja", s.downloadTJA)
	mux.HandleFunc("GET /api/charts/{id}/audio", s.streamAudio)
	mux.HandleFunc("GET /api/charts/{id}/preview", s.streamPreview)
	mux.HandleFunc("GET /api/charts/{id}/movie", s.streamMovie)
	mux.HandleFunc("GET /api/charts/{id}/bgimage", s.streamBackground)
	mux.HandleFunc("GET /api/charts/{id}/download", s.downloadPackage)
	mux.HandleFunc("GET /api/charts/{id}/resources", s.resourceLinks)
	mux.HandleFunc("POST /api/game/login", s.gameLogin)
	mux.HandleFunc("GET /api/game/catalog", s.gameCatalog)
	mux.HandleFunc("GET /api/game/search", s.gameSearch)
	mux.HandleFunc("GET /api/game/scores", s.gameScoreList)
	mux.HandleFunc("GET /api/users/{id}/scores", s.userScores)
	mux.HandleFunc("GET /api/users/{id}/comments", s.userComments)
	mux.HandleFunc("GET /api/charts/{id}/leaderboard", s.leaderboard)
	mux.HandleFunc("GET /api/scores/{id}/replay", s.scoreReplay)
	mux.HandleFunc("GET /api/replays", s.replayList)
	mux.HandleFunc("PUT /api/charts/{id}/vote", s.voteChart)
	mux.HandleFunc("GET /api/charts/{id}/comments", s.chartComments)
	mux.HandleFunc("POST /api/charts/{id}/comments", s.createComment)
	mux.HandleFunc("GET /api/charts/{id}/collaborations", s.listCollaborations)
	mux.HandleFunc("POST /api/charts/{id}/collaborations", s.inviteCollaborator)
	mux.HandleFunc("DELETE /api/charts/{id}/collaborations/{invitation}", s.revokeCollaboration)
	mux.HandleFunc("DELETE /api/charts/{id}/owners/me", s.leaveChart)
	mux.HandleFunc("PUT /api/dans/{id}/vote", s.voteDan)
	mux.HandleFunc("GET /api/dans/{id}/comments", s.danComments)
	mux.HandleFunc("POST /api/dans/{id}/comments", s.createDanComment)
	mux.HandleFunc("GET /api/comments/{id}", s.commentThread)
	mux.HandleFunc("PATCH /api/comments/{id}", s.editComment)
	mux.HandleFunc("DELETE /api/comments/{id}", s.deleteComment)
	mux.HandleFunc("PUT /api/comments/{id}/vote", s.voteComment)
	mux.HandleFunc("GET /api/me/notifications", s.listNotifications)
	mux.HandleFunc("GET /api/me/notifications/unread", s.unreadNotifications)
	mux.HandleFunc("POST /api/me/notifications/read", s.readAllNotifications)
	mux.HandleFunc("PATCH /api/me/notifications/{id}", s.markNotification)
	mux.HandleFunc("DELETE /api/me/notifications/{id}", s.dismissNotification)
	mux.HandleFunc("POST /api/me/notifications/{id}/respond", s.respondCollaboration)
	mux.HandleFunc("GET /api/me/notification-settings", s.notificationSettings)
	mux.HandleFunc("PUT /api/me/notification-settings", s.updateNotificationSettings)
	mux.HandleFunc("GET /api/dans", s.listDans)
	mux.HandleFunc("GET /api/dans/{id}", s.danDetail)
	mux.HandleFunc("POST /api/dans", s.createDan)
	mux.HandleFunc("PUT /api/dans/{id}", s.updateDan)
	mux.HandleFunc("DELETE /api/dans/{id}", s.deleteDan)
	mux.HandleFunc("PUT /api/dans/{id}/official", s.markDanOfficial)
	mux.HandleFunc("GET /api/game/dans", s.gameDans)
	mux.HandleFunc("GET /api/game/dans/{code}", s.gameDanByCode)
	mux.HandleFunc("POST /api/game/dan-results", s.submitDanResult)
	// Deprecated 2026-11-10: games before dan-results.
	mux.HandleFunc("POST /api/game/dan-attempts", s.submitDanAttempt)
	mux.HandleFunc("POST /api/game/scores", s.submitScore)
	mux.HandleFunc("GET /api/game/playlists", s.gamePlaylistList)
	mux.HandleFunc("GET /api/game/charts/{id}/vote", s.gameChartVote)
	mux.HandleFunc("PUT /api/game/charts/{id}/vote", s.voteChart)
	mux.HandleFunc("GET /api/playlists", s.listPlaylists)
	mux.HandleFunc("POST /api/playlists", s.createPlaylist)
	mux.HandleFunc("GET /api/playlists/system", s.listSystemPlaylists)
	mux.HandleFunc("GET /api/me/playlists", s.myPlaylists)
	mux.HandleFunc("GET /api/playlists/{id}", s.playlistDetail)
	mux.HandleFunc("PATCH /api/playlists/{id}", s.editPlaylist)
	mux.HandleFunc("DELETE /api/playlists/{id}", s.deletePlaylist)
	mux.HandleFunc("PUT /api/playlists/{id}/cover", s.storing(s.replacePlaylistCover))
	mux.HandleFunc("DELETE /api/playlists/{id}/cover", s.removePlaylistCover)
	mux.HandleFunc("PUT /api/playlists/{id}/items/{chart}", s.addPlaylistItem)
	mux.HandleFunc("DELETE /api/playlists/{id}/items/{chart}", s.removePlaylistItem)
	mux.HandleFunc("PUT /api/playlists/{id}/order", s.orderPlaylist)
	mux.HandleFunc("PUT /api/playlists/{id}/favorite", s.favoritePlaylist)
	mux.HandleFunc("DELETE /api/playlists/{id}/favorite", s.unfavoritePlaylist)
	// What only the Fanmade site's older clients still ask for: the account routes it gave up
	// when accounts moved to the account center, and its score route from before the game
	// routes.
	if !s.source().Official {
		mux.HandleFunc("POST /api/auth/email-code", s.retiredAccountAPI)
		mux.HandleFunc("POST /api/auth/register", s.retiredAccountAPI)
		mux.HandleFunc("POST /api/auth/login", s.retiredAccountAPI)
		mux.HandleFunc("PATCH /api/me", s.retiredAccountAPI)
		mux.HandleFunc("POST /api/scores", s.submitScore)
	}
	mux.HandleFunc("/", s.site)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", web.NewID())
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		// Native game requests come without an Origin; a browser always sends one.
		if game := isGameRequest(r); r.Method != "GET" && r.Method != "HEAD" && ((game && r.Header.Get("Origin") != "") || (!game && r.Header.Get("Origin") != s.Config.Origin)) {
			problem(w, 403, "ORIGIN_INVALID", "请求来源无效，请从本站页面操作")
			return
		}
		if (r.Method != "GET" && r.Method != "HEAD") || r.URL.Path == "/api/auth/sso/login" {
			host := web.ClientIP(r, s.Config.TrustedProxies)
			s.mu.Lock()
			now := time.Now()
			for k, v := range s.limits {
				if now.Sub(v.since) > time.Minute {
					delete(s.limits, k)
				}
			}
			limit := s.limits[host]
			if limit.since.IsZero() {
				limit.since = now
			}
			limit.count++
			s.limits[host] = limit
			s.mu.Unlock()
			if limit.count > 40 {
				problem(w, 429, "RATE_LIMITED", "操作过于频繁，请稍后再试")
				return
			}
		}
		w, finish := web.GzipJSON(w, r)
		defer finish()
		defer func() {
			if e := recover(); e != nil {
				log.Printf("panic: %v", e)
				problem(w, 500, "INTERNAL_ERROR", "服务内部错误")
			}
		}()
		mux.ServeHTTP(w, r)
	})
}
func (s *Server) EnsureStorage() error {
	if s.Config.Storage == "" {
		return errors.New("storage directory required")
	}
	return os.MkdirAll(s.Config.Storage, 0700)
}

// retiredAccountAPI answers the account routes a site gave up when accounts moved to the
// account center.
func (s *Server) retiredAccountAPI(w http.ResponseWriter, r *http.Request) {
	problem(w, 410, "SSO_REQUIRED", "账号管理已迁至 OurTaiko 账号中心，请使用 SSO 登录或管理账号")
}
