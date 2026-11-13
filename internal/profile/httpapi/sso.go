package httpapi

import "ourtaiko.dev/net/internal/account"

// The account center's client is the whole site's (package account); these are the names this
// code has always used for it.
type (
	SSOClient     = account.Client
	SSOConfig     = account.Config
	PublicProfile = account.PublicProfile
	ssoError      = account.Error
)

var NewSSO = account.New

func validSubject(id string) bool { return account.ValidSubject(id) }

func validServiceCredential(value string, minLen, maxLen int) bool {
	return account.ValidServiceCredential(value, minLen, maxLen)
}
