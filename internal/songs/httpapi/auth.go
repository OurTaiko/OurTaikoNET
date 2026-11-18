package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"ourtaiko.dev/net/internal/account"
	"ourtaiko.dev/net/internal/signin"
	"ourtaiko.dev/net/internal/titlerules"
	"ourtaiko.dev/net/internal/web"
)

// User is someone signed in, as the account center describes them; what IsAdmin means here
// is decided by Config.Manager where one is set.
type User = account.User

// session is a signed-in browser (package signin), or a game with its token.
type session = signin.Session

// sessions is the website's sign-in and sessions, as this source has them configured.
func (s *Server) sessions() *signin.Sessions {
	src := s.source()
	return &signin.Sessions{DB: s.DB, SSO: s.Config.SSO, DevUserID: s.Config.DevUserID, Origin: s.Config.Origin, CookieSecure: s.Config.CookieSecure,
		Cookie: s.sessionCookie(), FlowCookie: src.OIDCCookie, DefaultReturn: src.DefaultReturn, ErrorPage: "/login?error=sso",
		Problems: answers}
}

// manages says whether u may do what this code calls an administrator's work: the account
// center's answer, unless Config.Manager decides it.
func (s *Server) manages(ctx context.Context, u User) (User, error) {
	if s.Config.Manager == nil {
		return u, nil
	}
	var e error
	u.IsAdmin, e = s.Config.Manager(ctx, u.ID)
	return u, e
}

// devUser is the only account of a server running without an account center.
func devUser(id string) User { return signin.DevUser(id) }

// current is who the request comes from: the game by its token on a game route, the website
// by its session anywhere else. pgx.ErrNoRows: nobody is signed in.
func (s *Server) current(r *http.Request) (session, error) {
	if isGameRequest(r) {
		return s.gameSession(r)
	}
	v, e := s.sessions().Current(r)
	if e != nil || s.Config.SSO == nil {
		return v, e
	}
	if v.User, e = s.manages(r.Context(), v.User); e != nil {
		return session{}, e
	}
	return v, nil
}

// gameSession authenticates a native client by its SSO game token. Browser
// cookies never make a game session, nor a game token a website session.
func (s *Server) gameSession(r *http.Request) (session, error) {
	var v session
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || len(token) != 64 {
		return v, pgx.ErrNoRows
	}
	u, e := s.GameIdentity(r.Context(), token)
	if e == nil && s.Config.SSO != nil {
		u, e = s.manages(r.Context(), u)
	}
	if e != nil {
		return v, e
	}
	v.User = u
	return v, signin.RecordActivity(r.Context(), s.DB, v.User.ID, false, titlerules.GameLogin)
}
func (s *Server) required(w http.ResponseWriter, r *http.Request, csrf bool) (session, bool) {
	v, e := s.current(r)
	if e != nil {
		if errors.Is(e, pgx.ErrNoRows) {
			problem(w, 401, "UNAUTHORIZED", "请先登录")
		} else {
			internal(w, e)
		}
		return v, false
	}
	// A game signs in with a token, not a cookie: there is nothing for CSRF to protect.
	if csrf && !isGameRequest(r) && r.Header.Get("X-CSRF-Token") != v.CSRF {
		problem(w, 403, "CSRF_INVALID", "会话已更新，请刷新页面后重试")
		return v, false
	}
	return v, true
}
func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	v, e := s.current(r)
	if errors.Is(e, pgx.ErrNoRows) {
		web.Respond(w, 200, map[string]any{"user": nil, "csrfToken": ""})
		return
	}
	if e != nil {
		internal(w, e)
		return
	}
	web.Respond(w, 200, map[string]any{"user": v.User, "csrfToken": v.CSRF})
}

// Require is who a request is from, for routes outside this server (such as package rewards'):
// the game by its token on a path under /api/game/, the website by its session elsewhere.
// It answers the request itself when nobody is signed in, or, with write, when a website's
// request lacks its CSRF token.
func (s *Server) Require(w http.ResponseWriter, r *http.Request, write bool) (string, bool) {
	v, ok := s.required(w, r, write)
	return v.User.ID, ok
}
