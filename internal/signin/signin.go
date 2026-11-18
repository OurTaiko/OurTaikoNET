// Package signin is the website's sign-in with the account center and the sessions it opens.
//
// A session is a row of net.sessions, named by a cookie. Every part of the site (each song
// source, the players' pages) reads the same rows, so with one cookie name a player who signs
// in once is signed in everywhere. A part makes a Sessions value from its own configuration
// and adds what only it knows about a player, such as whether they manage it.
package signin

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/oauth2"
	"ourtaiko.dev/net/internal/account"
	"ourtaiko.dev/net/internal/requester"
	"ourtaiko.dev/net/internal/titlerules"
	"ourtaiko.dev/net/internal/web"
)

// Sessions signs players in and reads their sessions for one part of the site.
type Sessions struct {
	// DB finds sessions, oidc_flows and users (schema net).
	DB *pgxpool.Pool
	// SSO is nil when no account center is configured; sign-in is then unavailable unless
	// DevUserID is set (local development only).
	SSO       *account.Client
	DevUserID string
	// Origin is the website's address, which sign-in returns to.
	Origin       string
	CookieSecure bool
	// Cookie names the session cookie. FlowCookie ties a sign-in at the account center to the
	// browser that began it.
	Cookie, FlowCookie string
	// DefaultReturn is the page signing in returns to when it was asked for nowhere, or
	// nowhere allowed. ErrorPage is where a sign-in that failed lands.
	DefaultReturn, ErrorPage string
	// Problems answers a request that cannot be served, in the part's own shape.
	web.Problems
}

// Session is a signed-in browser. Token and RefreshToken are the account center's
// credentials for the player; a development session has none.
type Session struct {
	User          account.User
	CSRF          string
	Token         string
	RefreshToken  string
	AccessExpires time.Time
}

// DevUser is the only account of a server running without an account center.
func DevUser(id string) account.User {
	return account.User{ID: id, Username: "dev", Nickname: "本地开发用户", EmailVerified: true, IsAdmin: true}
}

// Local reads the session the request's cookie names, without asking the account center
// whether it still stands. pgx.ErrNoRows: there is none.
func (s *Sessions) Local(r *http.Request) (Session, error) {
	var v Session
	c, e := r.Cookie(s.Cookie)
	if e != nil || len(c.Value) != 64 {
		return v, pgx.ErrNoRows
	}
	var encrypted, refresh []byte
	var accessExpires *time.Time
	e = s.DB.QueryRow(r.Context(), `SELECT user_id,csrf_token,upstream_token,upstream_refresh_token,upstream_expires_at FROM sessions WHERE token_hash=$1 AND expires_at>now()`, web.Hash(c.Value)).Scan(&v.User.ID, &v.CSRF, &encrypted, &refresh, &accessExpires)
	if e != nil {
		return v, e
	}
	if s.SSO == nil {
		// Development sessions carry no account-center credential.
		return v, nil
	}
	v.Token, e = s.SSO.Unseal(encrypted, web.Hash(c.Value))
	if e == nil && len(refresh) > 0 {
		v.RefreshToken, e = s.SSO.Unseal(refresh, "refresh:"+web.Hash(c.Value))
	}
	if accessExpires != nil {
		v.AccessExpires = *accessExpires
	}
	return v, e
}

// Current is the request's session with the player as the account center describes them
// right now; a session whose player the account center no longer vouches for is none
// (pgx.ErrNoRows). It counts as the player being active.
func (s *Sessions) Current(r *http.Request) (Session, error) {
	v, e := s.Local(r)
	if e != nil {
		return v, e
	}
	if s.SSO == nil {
		if s.DevUserID == "" || v.User.ID != s.DevUserID {
			return Session{}, pgx.ErrNoRows
		}
		v.User = DevUser(v.User.ID)
	} else {
		if v.RefreshToken != "" && time.Until(v.AccessExpires) < time.Minute {
			if v, e = s.refresh(r); e != nil {
				return Session{}, e
			}
		}
		u, e := s.SSO.Identity(r.Context(), "web", v.Token)
		if e != nil {
			return Session{}, e
		}
		if u.ID != v.User.ID {
			return Session{}, pgx.ErrNoRows
		}
		v.User = u
	}
	if e = RecordActivity(r.Context(), s.DB, v.User.ID, false, titlerules.SiteLogin); e != nil {
		return Session{}, e
	}
	return v, nil
}

type activityWriter interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

// RecordActivity notes that a player was just seen, and with login that they signed in, and
// tells the request who it is from (requester.Note): every authenticated request comes here. A
// player not known to the site yet becomes known. Call only after authentication. Writes at
// most once a minute for a player.
//
// seen says where they were seen, titlerules.SiteLogin or titlerules.GameLogin, and gives them
// the titles there are for that (as often as this writes, and at every sign-in); "" gives
// none. Failing to give a title is logged and does not fail the request, so do not pass seen
// with a transaction: an error would spoil it.
func RecordActivity(ctx context.Context, db activityWriter, id string, login bool, seen string) error {
	// Whatever runs around the handlers may want to know (package requester).
	requester.Note(ctx, id)
	tag, err := db.Exec(ctx, `INSERT INTO users(id,first_login_at,last_active_at)
 VALUES($1,CASE WHEN $2 THEN now() ELSE NULL END,now())
 ON CONFLICT(id) DO UPDATE SET
 first_login_at=COALESCE(users.first_login_at,EXCLUDED.first_login_at),
 last_active_at=GREATEST(users.last_active_at,EXCLUDED.last_active_at)
 WHERE users.last_active_at IS NULL OR users.last_active_at < now()-interval '1 minute'
 OR (users.first_login_at IS NULL AND EXCLUDED.first_login_at IS NOT NULL)`, id, login)
	if err != nil {
		return err
	}
	if seen != "" && (login || tag.RowsAffected() > 0) {
		if _, e := titlerules.Seen(ctx, db, seen, id); e != nil {
			log.Printf("title rules for a player seen (%s): %v", seen, e)
		}
	}
	return nil
}

// SafeReturn is where signing in returns to: the local path asked for, or DefaultReturn when
// there is none or it leads elsewhere.
func (s *Sessions) SafeReturn(raw string) string {
	u, e := url.Parse(raw)
	if e != nil || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") || strings.ContainsAny(raw, "\\\r\n") || u.IsAbs() || u.Host != "" {
		return s.DefaultReturn
	}
	return raw
}

// SetCookie gives the browser a session opened with Open.
func (s *Sessions) SetCookie(w http.ResponseWriter, raw string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{Name: s.Cookie, Value: raw, Path: "/", HttpOnly: true, Secure: s.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: max(1, int(time.Until(expires).Seconds()))})
}

// Open records that a player has signed in and returns the value of their session cookie and
// when it ends. access and refresh are the account center's tokens for them, kept encrypted;
// a development session (no account center) has none.
func (s *Sessions) Open(ctx context.Context, user, access, refresh string, accessExpires time.Time) (string, time.Time, error) {
	rawSession, csrf := web.NewID()+web.NewID(), web.NewID()+web.NewID()
	digest := web.Hash(rawSession)
	expires := time.Now().Add(7 * 24 * time.Hour)
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return "", expires, e
	}
	defer tx.Rollback(ctx)
	if e = RecordActivity(ctx, tx, user, true, ""); e != nil {
		return "", expires, e
	}
	if s.SSO == nil {
		_, e = tx.Exec(ctx, `INSERT INTO sessions(token_hash,user_id,csrf_token,expires_at,upstream_token) VALUES($1,$2,$3,$4,''::bytea)`, digest, user, csrf, expires)
	} else {
		_, e = tx.Exec(ctx, `INSERT INTO sessions(token_hash,user_id,csrf_token,expires_at,upstream_token,upstream_refresh_token,upstream_expires_at) VALUES($1,$2,$3,$4,$5,$6,$7)`,
			digest, user, csrf, expires, s.SSO.Seal(access, digest), s.SSO.Seal(refresh, "refresh:"+digest), accessExpires)
	}
	if e != nil {
		return "", expires, e
	}
	if e = tx.Commit(ctx); e != nil {
		return "", expires, e
	}
	// The titles for signing in on the website, once the session stands.
	if _, e = titlerules.Seen(ctx, s.DB, titlerules.SiteLogin, user); e != nil {
		log.Printf("title rules for a sign-in: %v", e)
	}
	return rawSession, expires, nil
}

// Login begins a sign-in: GET …/auth/sso/login?returnTo=<page>. Without an account center it
// signs the development user in at once, if there is one.
func (s *Sessions) Login(w http.ResponseWriter, r *http.Request) {
	destination := s.SafeReturn(r.URL.Query().Get("returnTo"))
	if strings.HasPrefix(destination, "/api/") {
		s.Problem(w, 400, "RETURN_PATH_INVALID", "返回地址必须是网站页面")
		return
	}
	if s.SSO == nil {
		if s.DevUserID == "" {
			http.Redirect(w, r, s.Origin+s.ErrorPage, http.StatusSeeOther)
			return
		}
		rawSession, expires, e := s.Open(r.Context(), s.DevUserID, "", "", time.Time{})
		if e != nil {
			s.Internal(w, e)
			return
		}
		s.SetCookie(w, rawSession, expires)
		http.Redirect(w, r, s.Origin+destination, http.StatusSeeOther)
		return
	}
	_, cfg, e := s.SSO.OIDC(r.Context())
	if e != nil {
		s.Internal(w, e)
		return
	}
	state, binding, nonce, verifier := web.NewID()+web.NewID(), web.NewID()+web.NewID(), web.NewID()+web.NewID(), oauth2.GenerateVerifier()
	if _, e = s.DB.Exec(r.Context(), `DELETE FROM oidc_flows WHERE expires_at<now()`); e != nil {
		s.Internal(w, e)
		return
	}
	_, e = s.DB.Exec(r.Context(), `INSERT INTO oidc_flows(state_hash,binding_hash,nonce,verifier,return_path,expires_at) VALUES($1,$2,$3,$4,$5,now()+interval '5 minutes')`, web.Hash(state), web.Hash(binding), nonce, verifier, destination)
	if e != nil {
		s.Internal(w, e)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: s.FlowCookie, Value: binding, Path: "/api", HttpOnly: true, Secure: s.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: 300})
	http.Redirect(w, r, cfg.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)), http.StatusSeeOther)
}

// Callback finishes a sign-in the account center sends back: GET …/auth/sso/callback. It
// opens a session only for a player the account center has just vouched for, in the browser
// that began the sign-in, and withdraws the account center's tokens if anything is amiss.
func (s *Sessions) Callback(w http.ResponseWriter, r *http.Request) {
	fail := func() { http.Redirect(w, r, s.Origin+s.ErrorPage, http.StatusSeeOther) }
	if s.SSO == nil {
		fail()
		return
	}
	c, e := r.Cookie(s.FlowCookie)
	state := r.URL.Query().Get("state")
	if e != nil || len(c.Value) != 64 || len(state) != 64 {
		fail()
		return
	}
	var nonce, verifier, destination string
	e = s.DB.QueryRow(r.Context(), `DELETE FROM oidc_flows WHERE state_hash=$1 AND binding_hash=$2 AND expires_at>now() RETURNING nonce,verifier,return_path`, web.Hash(state), web.Hash(c.Value)).Scan(&nonce, &verifier, &destination)
	http.SetCookie(w, &http.Cookie{Name: s.FlowCookie, Path: "/api", HttpOnly: true, Secure: s.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	if e != nil || r.URL.Query().Get("error") != "" || r.URL.Query().Get("code") == "" {
		fail()
		return
	}
	provider, cfg, e := s.SSO.OIDC(r.Context())
	if e != nil {
		fail()
		return
	}
	ctx := oidc.ClientContext(r.Context(), s.SSO.HTTP)
	token, e := cfg.Exchange(ctx, r.URL.Query().Get("code"), oauth2.VerifierOption(verifier))
	if e != nil {
		fail()
		return
	}
	revoke := func() {
		_ = s.SSO.Call(ctx, "web/revoke", map[string]string{"accessToken": token.AccessToken, "refreshToken": token.RefreshToken}, nil)
	}
	raw, _ := token.Extra("id_token").(string)
	idToken, e := provider.Verifier(&oidc.Config{ClientID: cfg.ClientID}).Verify(ctx, raw)
	if e != nil || idToken.Nonce != nonce || !account.ValidSubject(idToken.Subject) {
		revoke()
		fail()
		return
	}
	if idToken.AccessTokenHash != "" && idToken.VerifyAccessToken(token.AccessToken) != nil {
		revoke()
		fail()
		return
	}
	u, e := s.SSO.Identity(ctx, "web", token.AccessToken)
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
	rawSession, expires, e := s.Open(ctx, u.ID, token.AccessToken, token.RefreshToken, token.Expiry)
	if e != nil {
		revoke()
		fail()
		return
	}
	if old, e := r.Cookie(s.Cookie); e == nil {
		_, _ = s.DB.Exec(ctx, `DELETE FROM sessions WHERE token_hash=$1`, web.Hash(old.Value))
	}
	s.SetCookie(w, rawSession, expires)
	http.Redirect(w, r, s.Origin+s.SafeReturn(destination), http.StatusSeeOther)
}

// Logout ends the request's session: POST …/auth/logout. It works when the account center
// is unavailable, and never needs a remote refresh first.
func (s *Sessions) Logout(w http.ResponseWriter, r *http.Request) {
	v, e := s.Local(r)
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		s.Internal(w, e)
		return
	}
	if e == nil {
		if r.Header.Get("X-CSRF-Token") != v.CSRF {
			s.Problem(w, 403, "CSRF_INVALID", "会话已更新，请刷新页面后重试")
			return
		}
		c, _ := r.Cookie(s.Cookie)
		var access, refresh []byte
		e = s.DB.QueryRow(r.Context(), `DELETE FROM sessions WHERE token_hash=$1 RETURNING upstream_token,upstream_refresh_token`, web.Hash(c.Value)).Scan(&access, &refresh)
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			s.Internal(w, e)
			return
		}
		if e == nil && s.SSO != nil {
			v.Token, _ = s.SSO.Unseal(access, web.Hash(c.Value))
			v.RefreshToken = ""
			if len(refresh) > 0 {
				v.RefreshToken, _ = s.SSO.Unseal(refresh, "refresh:"+web.Hash(c.Value))
			}
			ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 2*time.Second)
			defer cancel()
			_ = s.SSO.Call(ctx, "web/revoke", map[string]string{"accessToken": v.Token, "refreshToken": v.RefreshToken}, nil)
		}
	}
	http.SetCookie(w, &http.Cookie{Name: s.Cookie, Path: "/", HttpOnly: true, Secure: s.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(200)
	_, _ = w.Write([]byte("{\"ok\":true}\n"))
}

// AccountRedirect sends the browser to a page of the account center:
// GET …/auth/account/{page}, page being register, profile, or anything else for its front page.
func (s *Sessions) AccountRedirect(w http.ResponseWriter, r *http.Request) {
	if s.SSO == nil {
		s.Problem(w, 404, "SSO_NOT_CONFIGURED", "此服务器未接入账号中心")
		return
	}
	path := "/"
	switch r.PathValue("page") {
	case "register":
		path = "/register/"
	case "profile":
		path = "/profile/"
	}
	http.Redirect(w, r, s.SSO.Config.Issuer+path, http.StatusSeeOther)
}
