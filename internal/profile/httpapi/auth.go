package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/jackc/pgx/v5"
	"golang.org/x/oauth2"
	"ourtaiko.dev/net/internal/account"
	"ourtaiko.dev/net/internal/groups"
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

// identity asks the account center who a token belongs to and adds what the site itself
// knows about them.
func (s *Server) identity(ctx context.Context, kind, token string) (User, error) {
	who, e := s.Config.SSO.Identity(ctx, kind, token)
	if e != nil {
		return User{}, e
	}
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

type session struct {
	User          User
	CSRF          string
	Token         string
	RefreshToken  string
	AccessExpires time.Time
}

func (s *Server) localSession(r *http.Request) (session, error) {
	var v session
	c, e := r.Cookie(s.sessionCookie())
	if e != nil || len(c.Value) != 64 {
		return v, pgx.ErrNoRows
	}
	var encrypted, refresh []byte
	var accessExpires *time.Time
	e = s.DB.QueryRow(r.Context(), `SELECT user_id,csrf_token,upstream_token,upstream_refresh_token,upstream_expires_at FROM sessions WHERE token_hash=$1 AND expires_at>now()`, hash(c.Value)).Scan(&v.User.ID, &v.CSRF, &encrypted, &refresh, &accessExpires)
	if e != nil {
		return v, e
	}
	if s.Config.SSO == nil {
		// Development sessions carry no account-center credential.
		return v, nil
	}
	v.Token, e = s.Config.SSO.Unseal(encrypted, hash(c.Value))
	if e == nil && len(refresh) > 0 {
		v.RefreshToken, e = s.Config.SSO.Unseal(refresh, "refresh:"+hash(c.Value))
	}
	if accessExpires != nil {
		v.AccessExpires = *accessExpires
	}
	return v, e
}

// devUser is the only account of a server running without an account center.
func devUser(id string) User {
	return User{User: account.User{ID: id, Username: "dev", Nickname: "本地开发用户", EmailVerified: true, IsAdmin: true}, SiteAdmin: true, Groups: groups.Known().Names()}
}

func (s *Server) current(r *http.Request) (session, error) {
	v, e := s.localSession(r)
	if e != nil {
		return v, e
	}
	if s.Config.SSO == nil {
		if s.Config.DevUserID == "" || v.User.ID != s.Config.DevUserID {
			return session{}, pgx.ErrNoRows
		}
		v.User = devUser(v.User.ID)
		return v, nil
	}
	if v.RefreshToken != "" && time.Until(v.AccessExpires) < time.Minute {
		v, e = s.refreshWebSession(r)
		if e != nil {
			return session{}, e
		}
	}
	u, e := s.identity(r.Context(), "web", v.Token)
	if e != nil {
		return session{}, e
	}
	if u.ID != v.User.ID {
		return session{}, pgx.ErrNoRows
	}
	v.User = u
	return v, nil
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
		respond(w, 200, map[string]any{"user": nil, "csrfToken": ""})
		return
	}
	if e != nil {
		internal(w, e)
		return
	}
	respond(w, 200, map[string]any{"user": v.User, "csrfToken": v.CSRF})
}

func (s *Server) accountRedirect(w http.ResponseWriter, r *http.Request) {
	if s.Config.SSO == nil {
		problem(w, 404, "SSO_NOT_CONFIGURED", "此服务器未接入账号中心")
		return
	}
	path := "/"
	switch r.PathValue("page") {
	case "register":
		path = "/register/"
	case "profile":
		path = "/profile/"
	}
	http.Redirect(w, r, s.Config.SSO.Config.Issuer+path, http.StatusSeeOther)
}

func safeReturn(raw string) string {
	u, e := url.Parse(raw)
	if e != nil || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") || strings.ContainsAny(raw, "\\\r\n") || u.IsAbs() || u.Host != "" {
		return "/"
	}
	return raw
}

func (s *Server) setSession(w http.ResponseWriter, raw string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{Name: s.sessionCookie(), Value: raw, Path: "/", HttpOnly: true, Secure: s.Config.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: max(1, int(time.Until(expires).Seconds()))})
}

// devLogin opens a session for the configured development user.
func (s *Server) devLogin(w http.ResponseWriter, r *http.Request, destination string) {
	rawSession, csrf := ID()+ID(), ID()+ID()
	expires := time.Now().Add(7 * 24 * time.Hour)
	if _, e := s.DB.Exec(r.Context(), `INSERT INTO sessions(token_hash,user_id,csrf_token,expires_at,upstream_token) VALUES($1,$2,$3,$4,''::bytea)`, hash(rawSession), s.Config.DevUserID, csrf, expires); e != nil {
		internal(w, e)
		return
	}
	s.setSession(w, rawSession, expires)
	http.Redirect(w, r, s.Config.Origin+destination, http.StatusSeeOther)
}

func (s *Server) ssoLogin(w http.ResponseWriter, r *http.Request) {
	destination := safeReturn(r.URL.Query().Get("returnTo"))
	if strings.HasPrefix(destination, "/api/") {
		problem(w, 400, "RETURN_PATH_INVALID", "返回地址必须是网站页面")
		return
	}
	if s.Config.SSO == nil {
		if s.Config.DevUserID != "" {
			s.devLogin(w, r, destination)
			return
		}
		http.Redirect(w, r, s.Config.Origin+"/?error=sso", http.StatusSeeOther)
		return
	}
	_, cfg, e := s.Config.SSO.OIDC(r.Context())
	if e != nil {
		internal(w, e)
		return
	}
	state, binding, nonce, verifier := ID()+ID(), ID()+ID(), ID()+ID(), oauth2.GenerateVerifier()
	if _, e = s.DB.Exec(r.Context(), `DELETE FROM oidc_flows WHERE expires_at<now()`); e != nil {
		internal(w, e)
		return
	}
	_, e = s.DB.Exec(r.Context(), `INSERT INTO oidc_flows(state_hash,binding_hash,nonce,verifier,return_path,expires_at) VALUES($1,$2,$3,$4,$5,now()+interval '5 minutes')`, hash(state), hash(binding), nonce, verifier, destination)
	if e != nil {
		internal(w, e)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: flowCookie, Value: binding, Path: "/api/v1/auth/sso", HttpOnly: true, Secure: s.Config.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: 300})
	http.Redirect(w, r, cfg.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)), http.StatusSeeOther)
}

func (s *Server) ssoCallback(w http.ResponseWriter, r *http.Request) {
	fail := func() { http.Redirect(w, r, s.Config.Origin+"/?error=sso", http.StatusSeeOther) }
	if s.Config.SSO == nil {
		fail()
		return
	}
	c, e := r.Cookie(flowCookie)
	state := r.URL.Query().Get("state")
	if e != nil || len(c.Value) != 64 || len(state) != 64 {
		fail()
		return
	}
	var nonce, verifier, destination string
	e = s.DB.QueryRow(r.Context(), `DELETE FROM oidc_flows WHERE state_hash=$1 AND binding_hash=$2 AND expires_at>now() RETURNING nonce,verifier,return_path`, hash(state), hash(c.Value)).Scan(&nonce, &verifier, &destination)
	http.SetCookie(w, &http.Cookie{Name: flowCookie, Path: "/api/v1/auth/sso", HttpOnly: true, Secure: s.Config.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	if e != nil || r.URL.Query().Get("error") != "" || r.URL.Query().Get("code") == "" {
		fail()
		return
	}
	provider, cfg, e := s.Config.SSO.OIDC(r.Context())
	if e != nil {
		fail()
		return
	}
	ctx := oidc.ClientContext(r.Context(), s.Config.SSO.HTTP)
	token, e := cfg.Exchange(ctx, r.URL.Query().Get("code"), oauth2.VerifierOption(verifier))
	if e != nil {
		fail()
		return
	}
	revoke := func() {
		_ = s.Config.SSO.Call(ctx, "web/revoke", map[string]string{"accessToken": token.AccessToken, "refreshToken": token.RefreshToken}, nil)
	}
	raw, _ := token.Extra("id_token").(string)
	idToken, e := provider.Verifier(&oidc.Config{ClientID: cfg.ClientID}).Verify(ctx, raw)
	if e != nil || idToken.Nonce != nonce || !validSubject(idToken.Subject) {
		revoke()
		fail()
		return
	}
	if idToken.AccessTokenHash != "" && idToken.VerifyAccessToken(token.AccessToken) != nil {
		revoke()
		fail()
		return
	}
	u, e := s.identity(ctx, "web", token.AccessToken)
	if e != nil || u.ID != idToken.Subject {
		revoke()
		fail()
		return
	}
	if token.RefreshToken == "" || token.Expiry.IsZero() || !token.Expiry.After(time.Now()) {
		revoke()
		fail()
		return
	}
	rawSession, expires, e := s.OpenSession(ctx, u.ID, token.AccessToken, token.RefreshToken, token.Expiry)
	if e != nil {
		revoke()
		fail()
		return
	}
	if old, e := r.Cookie(s.sessionCookie()); e == nil {
		_, _ = s.DB.Exec(ctx, `DELETE FROM sessions WHERE token_hash=$1`, hash(old.Value))
	}
	s.setSession(w, rawSession, expires)
	http.Redirect(w, r, s.Config.Origin+safeReturn(destination), http.StatusSeeOther)
}

// OpenSession records that a player has signed in with the account center and returns the
// value of their session cookie and when it ends. access and refresh are the account center's
// tokens for them, kept encrypted. The player becomes known to the site if they were not, and
// this counts as their sign-in for the first-login and last-active times the sources show.
//
// A session is the same row to every part of the site: with one cookie name (Config
// .SessionCookie), signing in here signs in everywhere.
func (s *Server) OpenSession(ctx context.Context, user, access, refresh string, accessExpires time.Time) (string, time.Time, error) {
	rawSession, csrf := ID()+ID(), ID()+ID()
	digest := hash(rawSession)
	expires := time.Now().Add(7 * 24 * time.Hour)
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return "", expires, e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `INSERT INTO users(id,first_login_at,last_active_at) VALUES($1,now(),now())
 ON CONFLICT(id) DO UPDATE SET first_login_at=COALESCE(users.first_login_at,EXCLUDED.first_login_at),
 last_active_at=GREATEST(users.last_active_at,EXCLUDED.last_active_at)`, user); e != nil {
		return "", expires, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO sessions(token_hash,user_id,csrf_token,expires_at,upstream_token,upstream_refresh_token,upstream_expires_at) VALUES($1,$2,$3,$4,$5,$6,$7)`,
		digest, user, csrf, expires, s.Config.SSO.Seal(access, digest), s.Config.SSO.Seal(refresh, "refresh:"+digest), accessExpires); e != nil {
		return "", expires, e
	}
	return rawSession, expires, tx.Commit(ctx)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	// Local logout still works when the account center is unavailable.
	v, e := s.localSession(r)
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		internal(w, e)
		return
	}
	if e == nil {
		if r.Header.Get("X-CSRF-Token") != v.CSRF {
			problem(w, 403, "CSRF_INVALID", "会话已更新，请刷新页面后重试")
			return
		}
		c, _ := r.Cookie(s.sessionCookie())
		var access, refresh []byte
		e = s.DB.QueryRow(r.Context(), `DELETE FROM sessions WHERE token_hash=$1 RETURNING upstream_token,upstream_refresh_token`, hash(c.Value)).Scan(&access, &refresh)
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			internal(w, e)
			return
		}
		if e == nil && s.Config.SSO != nil {
			v.Token, _ = s.Config.SSO.Unseal(access, hash(c.Value))
			v.RefreshToken = ""
			if len(refresh) > 0 {
				v.RefreshToken, _ = s.Config.SSO.Unseal(refresh, "refresh:"+hash(c.Value))
			}
			ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 2*time.Second)
			defer cancel()
			_ = s.Config.SSO.Call(ctx, "web/revoke", map[string]string{"accessToken": v.Token, "refreshToken": v.RefreshToken}, nil)
		}
	}
	http.SetCookie(w, &http.Cookie{Name: s.sessionCookie(), Path: "/", HttpOnly: true, Secure: s.Config.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	respond(w, 200, map[string]bool{"ok": true})
}
