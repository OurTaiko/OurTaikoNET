package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"ourtaiko.dev/net/internal/account"
	"ourtaiko.dev/net/internal/groups"
	"ourtaiko.dev/net/internal/signin"
	"ourtaiko.dev/net/internal/web"
)

const defaultSessionCookie, flowCookie = "ourtaiko_profile_session", "ourtaiko_profile_oidc"

// User is someone signed in: what the account center says about them (account.User), and
// what the site itself knows.
//
// SiteAdmin is the account center's administrator of this site: they manage it, which includes
// every player's groups, and are in no group for it. Groups are the user groups the player is
// in (package groups). With Config.Groups, IsAdmin, which guards the title catalog and grants,
// is membership of TITLE_MANAGER; without, it is SiteAdmin.
type User struct {
	account.User
	SiteAdmin bool     `json:"siteAdmin"`
	Groups    []string `json:"groups"`
}

// known adds to what the account center says of a player what the site itself knows about
// them.
func (s *Server) known(ctx context.Context, who account.User) (User, error) {
	u := User{User: who, SiteAdmin: who.IsAdmin, Groups: []string{}}
	if s.Config.Groups {
		set, e := groups.Of(ctx, s.DB, u.ID)
		if e != nil {
			return u, e
		}
		u.Groups, u.IsAdmin = set.Names(), set.Has(groups.TitleManager)
	}
	return u, nil
}

// session is a signed-in browser (package signin) and its player as this part knows them.
type session struct {
	signin.Session
	User User
}

// sessions is the website's sign-in and sessions, as the players' pages have them configured.
// With one cookie name for the whole site (Config.SessionCookie), signing in here signs in
// everywhere.
func (s *Server) sessions() *signin.Sessions {
	return &signin.Sessions{DB: s.DB, SSO: s.Config.SSO, DevUserID: s.Config.DevUserID, Origin: s.Config.Origin, CookieSecure: s.Config.CookieSecure,
		Cookie: s.sessionCookie(), FlowCookie: flowCookie, DefaultReturn: "/", ErrorPage: "/?error=sso",
		Problems: answers}
}

// OpenSession records that a player has signed in with the account center and returns the
// value of their session cookie and when it ends (signin.Sessions.Open).
func (s *Server) OpenSession(ctx context.Context, user, access, refresh string, accessExpires time.Time) (string, time.Time, error) {
	return s.sessions().Open(ctx, user, access, refresh, accessExpires)
}

// devUser is the only account of a server running without an account center.
func devUser(id string) User {
	return User{User: signin.DevUser(id), SiteAdmin: true, Groups: groups.Known().Names()}
}

// current is who the request comes from. pgx.ErrNoRows: nobody is signed in.
func (s *Server) current(r *http.Request) (session, error) {
	v, e := s.sessions().Current(r)
	if e != nil {
		return session{}, e
	}
	if s.Config.SSO == nil {
		return session{Session: v, User: devUser(v.User.ID)}, nil
	}
	u, e := s.known(r.Context(), v.User)
	if e != nil {
		return session{}, e
	}
	return session{Session: v, User: u}, nil
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
	if csrf && r.Header.Get("X-CSRF-Token") != v.CSRF {
		problem(w, 403, "CSRF_INVALID", "会话已更新，请刷新页面后重试")
		return v, false
	}
	return v, true
}

// admin is required() for administrators of this application.
func (s *Server) admin(w http.ResponseWriter, r *http.Request) (session, bool) {
	v, ok := s.required(w, r, r.Method != "GET")
	if ok && !v.User.IsAdmin {
		problem(w, 403, "FORBIDDEN", "需要管理员权限")
		return v, false
	}
	return v, ok
}

// siteAdmin is required() for the site's administrator.
func (s *Server) siteAdmin(w http.ResponseWriter, r *http.Request) (session, bool) {
	v, ok := s.required(w, r, r.Method != "GET")
	if ok && !v.User.SiteAdmin {
		problem(w, 403, "FORBIDDEN", "需要管理员权限")
		return v, false
	}
	return v, ok
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
