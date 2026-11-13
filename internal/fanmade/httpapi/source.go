package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
)

// What is this source's own in code it otherwise shares with the official source.
const (
	// The cookie of a website session when the site gives no name of its own.
	defaultSessionCookie = "ourtaiko_session"
	// The cookie that ties a sign-in at the account center to the browser that began it.
	oidcCookie = "ourtaiko_oidc"
	// Where signing in returns to when it was asked for nowhere, or nowhere allowed.
	defaultReturn = "/upload"
)

// devGameToken stands for the SSO game token of DEV_USER_ID on a local server without an
// account center.
func (s *Server) devGameToken() string {
	h := sha256.Sum256([]byte("ourtaiko-fanmade-dev-game:" + s.Config.DevUserID))
	return hex.EncodeToString(h[:])
}
