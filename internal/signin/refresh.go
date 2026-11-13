package signin

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/jackc/pgx/v5"
	"golang.org/x/oauth2"
	"ourtaiko.dev/net/internal/web"
)

// refresh renews the account center's tokens of the request's session when they are about to
// run out. Rotations are serialized across requests and API instances, and a rotated
// credential is saved even if the tab cancels its request, without extending the session.
func (s *Sessions) refresh(r *http.Request) (Session, error) {
	var v Session
	cookie, err := r.Cookie(s.Cookie)
	if err != nil {
		return v, pgx.ErrNoRows
	}
	digest := web.Hash(cookie.Value)
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 20*time.Second)
	defer cancel()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return v, err
	}
	defer tx.Rollback(ctx)
	var access, refresh []byte
	var accessExpiry *time.Time
	var deadline time.Time
	err = tx.QueryRow(ctx, `SELECT user_id,csrf_token,upstream_token,upstream_refresh_token,upstream_expires_at,expires_at FROM sessions WHERE token_hash=$1 FOR UPDATE`, digest).Scan(&v.User.ID, &v.CSRF, &access, &refresh, &accessExpiry, &deadline)
	if err != nil {
		return v, err
	}
	if !deadline.After(time.Now()) {
		return v, pgx.ErrNoRows
	}
	v.Token, err = s.SSO.Unseal(access, digest)
	if err != nil {
		return v, err
	}
	if len(refresh) == 0 {
		return v, pgx.ErrNoRows
	}
	v.RefreshToken, err = s.SSO.Unseal(refresh, "refresh:"+digest)
	if err != nil {
		return v, err
	}
	if accessExpiry != nil && time.Until(*accessExpiry) >= time.Minute {
		v.AccessExpires = *accessExpiry
		return v, nil
	}
	_, cfg, err := s.SSO.OIDC(ctx)
	if err != nil {
		return v, err
	}
	// Force renewal inside our own expiry window, not oauth2's implicit window.
	token, err := cfg.TokenSource(oidc.ClientContext(ctx, s.SSO.HTTP), &oauth2.Token{RefreshToken: v.RefreshToken}).Token()
	if err != nil {
		var remote *oauth2.RetrieveError
		if errors.As(err, &remote) && remote.ErrorCode == "invalid_grant" {
			if _, err = tx.Exec(ctx, `DELETE FROM sessions WHERE token_hash=$1`, digest); err != nil {
				return Session{}, err
			}
			if err = tx.Commit(ctx); err != nil {
				return Session{}, err
			}
			return Session{}, pgx.ErrNoRows
		}
		// Never log token endpoint bodies or clear sessions for transient outages.
		return Session{}, errors.New("SSO session renewal failed")
	}
	if token.AccessToken == "" || token.RefreshToken == "" || !token.Expiry.After(time.Now()) {
		return Session{}, errors.New("invalid SSO renewal response")
	}
	// Current asks the account center about the player before anything is authorized.
	_, err = tx.Exec(ctx, `UPDATE sessions SET upstream_token=$2,upstream_refresh_token=$3,upstream_expires_at=$4 WHERE token_hash=$1`, digest, s.SSO.Seal(token.AccessToken, digest), s.SSO.Seal(token.RefreshToken, "refresh:"+digest), token.Expiry)
	if err != nil {
		return Session{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Session{}, err
	}
	v.Token, v.RefreshToken, v.AccessExpires = token.AccessToken, token.RefreshToken, token.Expiry
	return v, nil
}
