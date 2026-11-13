package httpapi

// What is this source's own in code it otherwise shares with the fanmade source.
const (
	// The cookie of a website session when the site gives no name of its own.
	defaultSessionCookie = "ourtaiko_db_session"
	// The cookie that ties a sign-in at the account center to the browser that began it.
	oidcCookie = "ourtaiko_db_oidc"
	// Where signing in returns to when it was asked for nowhere, or nowhere allowed.
	defaultReturn = "/"
)
