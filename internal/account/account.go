// Package account is the site's client of the account center (the SSO): who a token belongs
// to, players' public names and avatars, the OIDC sign-in, and the encryption of the account
// center's tokens that a session keeps. Every part of the site uses the one client.
package account

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/oauth2"
)

type Config struct {
	Issuer, ServiceID, ServiceKey, ClientID, ClientSecret, RedirectURL, EncryptionKey string
	// Optional TCP destination for the issuer, e.g. sso:8090 inside Docker.
	// The public issuer, HTTP Host and TLS certificate verification stay unchanged.
	ConnectAddress string
	AvatarBaseURL  string
}
type Client struct {
	// Config is what the client was made with; HTTP reaches the account center. Tests replace
	// them to stand in for it.
	Config   Config
	HTTP     *http.Client
	cipher   cipher.AEAD
	mu       sync.Mutex
	provider *oidc.Provider
	// kept is where the copy of players' profiles is (KeepProfiles); nil: no copy is kept.
	kept *pgxpool.Pool
}

// errUnreachable is the account center not answering at all, as opposed to answering no.
var errUnreachable = errors.New("SSO connection failed")

// Error is an answer of the account center other than success.
type Error struct{ Status int }

// User is someone signed in, as the account center describes them. IsAdmin is its
// administrator role for the Application the client signs in as.
type User struct {
	PreferredLanguage string `json:"preferredLanguage"`
	ID                string `json:"id"`
	Username          string `json:"username"`
	Nickname          string `json:"nickname"`
	AvatarURL         string `json:"avatarUrl"`
	EmailVerified     bool   `json:"emailVerified"`
	IsAdmin           bool   `json:"isAdmin"`
}

// GameAccounts tells game clients where to trade their OurTaiko account for a
// game session of this service (the SSO's POST /api/v1/game/tokens), so they
// need not send a password here. The catalog carries it as "sso".
func (c *Client) GameAccounts() map[string]string {
	return map[string]string{"issuer": strings.TrimRight(c.Config.Issuer, "/"), "service": c.Config.ServiceID}
}

func (e *Error) Error() string { return fmt.Sprintf("SSO responded with HTTP %d", e.Status) }

// Application client secrets need not be hex. Accept header-safe opaque credentials;
// SSO remains authoritative for verification and application capability checks.
func ValidServiceCredential(value string, minLen, maxLen int) bool {
	if len(value) < minLen || len(value) > maxLen {
		return false
	}
	for _, b := range []byte(value) {
		if b < 0x21 || b > 0x7e {
			return false
		}
	}
	return true
}

func New(cfg Config) (*Client, error) {
	for _, raw := range []string{cfg.Issuer, cfg.RedirectURL} {
		u, e := url.Parse(raw)
		if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"))) {
			return nil, errors.New("SSO URLs must use HTTPS (HTTP is allowed on loopback only)")
		}
	}
	if cfg.AvatarBaseURL != "" {
		u, err := url.Parse(cfg.AvatarBaseURL)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.ContainsAny(cfg.AvatarBaseURL, "\\\r\n\t ") {
			return nil, errors.New("SSO_AVATAR_BASE_URL must be an HTTPS URL without credentials, query or fragment")
		}
		cfg.AvatarBaseURL = strings.TrimRight(cfg.AvatarBaseURL, "/")
	}
	key, e := hex.DecodeString(cfg.EncryptionKey)
	if e != nil || len(key) != 32 {
		return nil, errors.New("SESSION_ENCRYPTION_KEY must be 64 hex characters")
	}
	if !ValidServiceCredential(cfg.ServiceID, 1, 100) || !ValidServiceCredential(cfg.ServiceKey, 40, 1024) || cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, errors.New("SSO service and OAuth client credentials are required")
	}
	block, e := aes.NewCipher(key)
	if e != nil {
		return nil, e
	}
	aead, e := cipher.NewGCM(block)
	if e != nil {
		return nil, e
	}
	client := &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if cfg.ConnectAddress != "" {
		host, port, err := net.SplitHostPort(cfg.ConnectAddress)
		if err != nil || host == "" || port == "" {
			return nil, errors.New("SSO_CONNECT_ADDRESS must be host:port")
		}
		issuer, _ := url.Parse(cfg.Issuer)
		issuerPort := issuer.Port()
		if issuerPort == "" {
			issuerPort = "443"
			if issuer.Scheme == "http" {
				issuerPort = "80"
			}
		}
		issuerAddress := net.JoinHostPort(issuer.Hostname(), issuerPort)
		transport := http.DefaultTransport.(*http.Transport).Clone()
		// This explicit route must not send private SSO traffic through an env proxy.
		proxy := transport.Proxy
		transport.Proxy = func(r *http.Request) (*url.URL, error) {
			if r.URL.Scheme == issuer.Scheme && r.URL.Host == issuer.Host {
				return nil, nil
			}
			return proxy(r)
		}
		dialer := &net.Dialer{Timeout: 8 * time.Second, KeepAlive: 30 * time.Second}
		transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			if address == issuerAddress {
				address = cfg.ConnectAddress
			}
			return dialer.DialContext(ctx, network, address)
		}
		client.Transport = transport
	}
	return &Client{Config: cfg, cipher: aead, HTTP: client}, nil
}
func (c *Client) Seal(token, aad string) []byte {
	nonce := make([]byte, c.cipher.NonceSize())
	if _, e := rand.Read(nonce); e != nil {
		panic(e)
	}
	return c.cipher.Seal(nonce, nonce, []byte(token), []byte(aad))
}
func (c *Client) Unseal(data []byte, aad string) (string, error) {
	n := c.cipher.NonceSize()
	if len(data) < n {
		return "", errors.New("invalid encrypted session")
	}
	plain, e := c.cipher.Open(nil, data[:n], data[n:], []byte(aad))
	return string(plain), e
}
func (c *Client) Call(ctx context.Context, path string, in, out any) error {
	if c == nil {
		return errors.New("SSO is not configured")
	}
	data, e := json.Marshal(in)
	if e != nil {
		return e
	}
	req, e := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(c.Config.Issuer, "/")+"/internal/v1/"+path, bytes.NewReader(data))
	if e != nil {
		return e
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Service-ID", c.Config.ServiceID)
	req.Header.Set("X-Service-Key", c.Config.ServiceKey)
	res, e := c.HTTP.Do(req)
	if e != nil {
		return errUnreachable
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return &Error{res.StatusCode}
	}
	if out == nil {
		io.Copy(io.Discard, io.LimitReader(res.Body, 8192))
		return nil
	}
	if e = json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(out); e != nil {
		return errors.New("invalid SSO response")
	}
	return nil
}
func (c *Client) Identity(ctx context.Context, kind, token string) (User, error) {
	var result struct {
		User User `json:"user"`
	}
	e := c.callAgain(ctx, kind+"/introspect", map[string]string{"accessToken": token}, &result)
	var remote *Error
	if errors.As(e, &remote) && remote.Status == 401 {
		return User{}, pgx.ErrNoRows
	}
	if e == nil && !ValidSubject(result.User.ID) {
		return User{}, errors.New("invalid SSO subject")
	}
	result.User.AvatarURL = c.AvatarURL(result.User)
	if e == nil {
		c.remember(ctx, result.User)
	}
	return result.User, e
}

// Accept only this user's content-addressed avatar on the issuer or configured CDN.
func (c *Client) AvatarURL(u User) string {
	bases := []string{strings.TrimRight(c.Config.Issuer, "/") + "/avatars"}
	if c.Config.AvatarBaseURL != "" {
		bases = append(bases, strings.TrimRight(c.Config.AvatarBaseURL, "/"))
	}
	for _, base := range bases {
		rest, ok := strings.CutPrefix(u.AvatarURL, base+"/"+u.ID+"/")
		digest, ext := strings.CutSuffix(rest, ".webp")
		if ok && ext && len(digest) == 32 && strings.Trim(digest, "0123456789abcdef") == "" {
			return u.AvatarURL
		}
	}
	return ""
}
func ValidSubject(id string) bool {
	b, e := hex.DecodeString(id)
	return e == nil && len(b) == 16 && strings.ToLower(id) == id
}

// PublicProfile is what SSO publishes about any user: nickname and avatar only.
type PublicProfile struct {
	Nickname  string `json:"nickname"`
	AvatarURL string `json:"avatarUrl"`
}

func (c *Client) Search(ctx context.Context, q string) ([]string, error) {
	ids := []string{}
	after := ""
	for {
		var res struct {
			IDs  []string `json:"userIds"`
			Next string   `json:"next"`
		}
		if e := c.Call(ctx, "users/search", map[string]string{"query": q, "after": after}, &res); e != nil {
			return nil, e
		}
		ids = append(ids, res.IDs...)
		if res.Next == "" {
			return ids, nil
		}
		if res.Next <= after || len(ids) > 100000 {
			return nil, errors.New("SSO search exceeded limit")
		}
		after = res.Next
	}
}
func (c *Client) OIDC(ctx context.Context) (*oidc.Provider, *oauth2.Config, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ctx = oidc.ClientContext(ctx, c.HTTP)
	if c.provider == nil {
		p, e := oidc.NewProvider(ctx, c.Config.Issuer)
		if e != nil {
			return nil, nil, errors.New("SSO discovery failed")
		}
		c.provider = p
	}
	cfg := &oauth2.Config{ClientID: c.Config.ClientID, ClientSecret: c.Config.ClientSecret, RedirectURL: c.Config.RedirectURL, Endpoint: c.provider.Endpoint(), Scopes: []string{oidc.ScopeOpenID, "profile"}}
	return c.provider, cfg, nil
}
