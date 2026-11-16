// Package httpapi is the OurTaikoProfile API and the server of its website.
package httpapi

import (
	"context"
	"io/fs"
	"log"
	"net/http"
	"net/netip"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"ourtaiko.dev/net/internal/account"
	"ourtaiko.dev/net/internal/chat"
	"ourtaiko.dev/net/internal/visits"
	"ourtaiko.dev/net/internal/web"
)

type Config struct {
	Origin       string
	CookieSecure bool
	// SSO is nil when no account center is configured; sign-in is then unavailable unless
	// DevUserID is set (local development only).
	SSO            *account.Client
	DevUserID      string
	TrustedProxies []netip.Prefix
	// The song sources whose plays are shown, in display order.
	Sources []Source
	// Groups turns user groups on (package groups): the title catalog and grants are then for
	// TITLE_MANAGER, and the account center's administrator manages players' groups under
	// /api/v1/admin/. Without it the administrator does the titles, as on the old site.
	Groups bool
	// SessionCookie names the cookie a website session is kept in; empty is the players' site's
	// own name. OurTaikoNet gives every part the same name: one sign-in for the whole site.
	SessionCookie string
	// The built website; nil serves only the API.
	Site fs.FS
	// Where the 3D Don's catalog is read (don.go); empty is DefaultDonCatalogURL. DonClient
	// reads it (nil: http.DefaultClient; tests give their own).
	DonCatalogURL string
	DonClient     *http.Client
	// Now is the clock of the visit count (visits.go); nil: time.Now. Tests move it.
	Now func() time.Time
}
type Server struct {
	DB     *pgxpool.Pool
	Config Config
	mu     sync.Mutex
	limits map[string]window
	// The site's latest activity as last read (activity.go).
	feedMu sync.Mutex
	feed   []Activity
	feedAt time.Time
	// The 3D Don's catalog as last read (don.go).
	donMu sync.Mutex
	don   *donCatalog
	donAt time.Time
	// The visit count (visits.go): the totals as last read, and each visitor's views this
	// minute (under mu).
	visitMu     sync.Mutex
	visit       visits.Summary
	visitAt     time.Time
	visitLimits map[string]window
	// Who waits for news of their chats (chats.go).
	chats *chat.Hub
}
type window struct {
	since time.Time
	count int
}

func New(pool *pgxpool.Pool, cfg Config) *Server {
	return &Server{DB: pool, Config: cfg, limits: map[string]window{}, visitLimits: map[string]window{}, chats: &chat.Hub{}}
}

func (s *Server) sessionCookie() string {
	if s.Config.SessionCookie != "" {
		return s.Config.SessionCookie
	}
	return defaultSessionCookie
}

// answers is how the players' part answers a request it cannot serve (web.Problems).
var answers = web.Problems{}

// The names this package calls them by.
var problem, internal, decode = answers.Problem, answers.Internal, answers.Decode

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
	mux.HandleFunc("GET /api/v1/auth/sso/login", func(w http.ResponseWriter, r *http.Request) { s.sessions().Login(w, r) })
	mux.HandleFunc("GET /api/v1/auth/sso/callback", func(w http.ResponseWriter, r *http.Request) { s.sessions().Callback(w, r) })
	mux.HandleFunc("GET /api/v1/auth/account/{page}", func(w http.ResponseWriter, r *http.Request) { s.sessions().AccountRedirect(w, r) })
	mux.HandleFunc("POST /api/v1/auth/logout", func(w http.ResponseWriter, r *http.Request) { s.sessions().Logout(w, r) })
	mux.HandleFunc("GET /api/v1/me", s.me)
	mux.HandleFunc("PUT /api/v1/me/nameplate", s.wearNameplate)
	mux.HandleFunc("POST /api/v1/me/titles", s.createOwnTitle)
	mux.HandleFunc("PUT /api/v1/me/titles/{id}", s.saveOwnTitle)
	mux.HandleFunc("DELETE /api/v1/me/titles/{id}", s.deleteOwnTitle)
	mux.HandleFunc("GET /api/v1/sources", s.sources)
	mux.HandleFunc("GET /api/v1/titles", s.titleCatalog)
	mux.HandleFunc("GET /api/v1/activity", s.activity)
	mux.HandleFunc("GET /api/v1/users/{id}", s.userProfile)
	mux.HandleFunc("GET /api/v1/users/{id}/nameplate", s.userNameplate)
	mux.HandleFunc("GET /api/v1/users/{id}/don", s.userDon)
	mux.HandleFunc("GET /api/v1/me/don", s.myDon)
	mux.HandleFunc("PUT /api/v1/me/don", s.wearDon)
	mux.HandleFunc("GET /api/v1/don/catalog", s.donCatalogAnswer)
	mux.HandleFunc("GET /api/v1/nameplates", s.nameplates)
	mux.HandleFunc("GET /api/v1/users/{id}/scores", s.userScores)
	mux.HandleFunc("GET /api/v1/users/{id}/titles", s.userTitles)
	mux.HandleFunc("GET /api/v1/users/{id}/dans", s.userDans)
	mux.HandleFunc("GET /api/v1/chats", s.listChats)
	mux.HandleFunc("POST /api/v1/chats", s.openChat)
	mux.HandleFunc("GET /api/v1/chats/unread", s.unreadChats)
	mux.HandleFunc("GET /api/v1/chats/updates", s.chatUpdates)
	mux.HandleFunc("GET /api/v1/chats/{id}", s.getChat)
	mux.HandleFunc("GET /api/v1/chats/{id}/messages", s.chatMessages)
	mux.HandleFunc("POST /api/v1/chats/{id}/messages", s.sendChatMessage)
	mux.HandleFunc("POST /api/v1/chats/{id}/read", s.readChat)
	mux.HandleFunc("PUT /api/v1/chats/{id}/block", s.blockChat)
	mux.HandleFunc("POST /api/v1/visits", s.countVisit)
	mux.HandleFunc("GET /api/v1/visits", s.visitSummary)
	mux.HandleFunc("GET /api/v1/admin/visits", s.visitReport)
	mux.HandleFunc("PUT /api/v1/admin/titles/{id}", s.saveTitle)
	mux.HandleFunc("DELETE /api/v1/admin/titles/{id}", s.deleteTitle)
	mux.HandleFunc("POST /api/v1/admin/grants", s.grantTitle)
	mux.HandleFunc("DELETE /api/v1/admin/grants", s.revokeTitle)
	mux.HandleFunc("GET /api/v1/admin/groups", s.groupCatalog)
	mux.HandleFunc("GET /api/v1/admin/users", s.groupMembers)
	mux.HandleFunc("PUT /api/v1/admin/users/{id}/groups", s.giveGroups)
	mux.HandleFunc("POST /internal/v1/awards", s.award)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { problem(w, 404, "NOT_FOUND", "接口不存在") })
	mux.HandleFunc("/internal/", func(w http.ResponseWriter, r *http.Request) { problem(w, 404, "NOT_FOUND", "接口不存在") })
	mux.HandleFunc("/", s.site)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", web.NewID())
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		api := strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/internal/")
		if api {
			w.Header().Set("Cache-Control", "no-store")
		}
		// Services call /internal/ from their servers, without an Origin; a browser always
		// sends one with a write.
		service := strings.HasPrefix(r.URL.Path, "/internal/")
		if r.Method != "GET" && r.Method != "HEAD" && ((service && r.Header.Get("Origin") != "") || (!service && r.Header.Get("Origin") != s.Config.Origin)) {
			problem(w, 403, "ORIGIN_INVALID", "请求来源无效，请从本站页面操作")
			return
		}
		// Showing a page is no write of the visitor's: it has a count of its own (visits.go).
		// Nor is having read a chat, which a page says each time a message arrives (chats.go).
		chatRead := strings.HasPrefix(r.URL.Path, "/api/v1/chats/") && strings.HasSuffix(r.URL.Path, "/read")
		if !service && ((r.Method != "GET" && r.Method != "HEAD") || r.URL.Path == "/api/v1/auth/sso/login") && r.URL.Path != "/api/v1/visits" && !chatRead && !s.allow(r) {
			problem(w, 429, "RATE_LIMITED", "操作过于频繁，请稍后再试")
			return
		}
		if api {
			var finish func()
			w, finish = web.GzipJSON(w, r)
			defer finish()
		}
		defer func() {
			if e := recover(); e != nil {
				log.Printf("panic: %v", e)
				problem(w, 500, "INTERNAL_ERROR", "服务内部错误")
			}
		}()
		mux.ServeHTTP(w, r)
	})
}

// allow counts a write against its client's 40 a minute.
func (s *Server) allow(r *http.Request) bool {
	host := web.ClientIP(r, s.Config.TrustedProxies)
	s.mu.Lock()
	defer s.mu.Unlock()
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
	return limit.count <= 40
}

// site serves the built website: a file that exists, else index.html for the page routes.
// Hashed assets are cached for a year, index.html never.
func (s *Server) site(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.Config.Site == nil {
		http.NotFound(w, r)
		return
	}
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name != "" && name != "index.html" {
		if info, err := fs.Stat(s.Config.Site, name); err == nil && !info.IsDir() {
			if strings.HasPrefix(name, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				w.Header().Set("Cache-Control", "public, max-age=3600")
			}
			http.ServeFileFS(w, r, s.Config.Site, name)
			return
		}
		if path.Ext(name) != "" {
			http.NotFound(w, r)
			return
		}
	}
	page, err := fs.ReadFile(s.Config.Site, "index.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' https: data:; style-src 'self' 'unsafe-inline'; font-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
	w.Write(page)
}
