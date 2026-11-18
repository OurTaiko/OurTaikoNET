// Package awards is what a song source needs of the part that keeps players' titles and 段位
// badges: the title catalog, and what a 段位 attempt earns.
package awards

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"ourtaiko.dev/net/internal/account"
)

// Config is how this service asks OurTaikoProfile (profile.ourtaiko.org) for the
// titles and 段位 badges a 段位 attempt earns: its address and this service's key there
// (registered with ourtaiko-profile -register-service).
type Config struct {
	URL, ServiceID, ServiceKey string
}

// Client calls POST /internal/v1/awards and reads the title catalog.
type Client struct {
	config Config
	http   *http.Client

	mu       sync.Mutex
	titles   map[string]Title
	titlesAt time.Time
}

func New(cfg Config) (*Client, error) {
	u, err := url.Parse(cfg.URL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost" || u.Hostname() == "::1"))) {
		return nil, errors.New("PROFILE_URL must use HTTPS (HTTP is allowed on loopback only)")
	}
	if !account.ValidServiceCredential(cfg.ServiceID, 1, 40) || !account.ValidServiceCredential(cfg.ServiceKey, 32, 1024) {
		return nil, errors.New("PROFILE_SERVICE_ID and PROFILE_SERVICE_KEY are required with PROFILE_URL")
	}
	cfg.URL = strings.TrimRight(cfg.URL, "/")
	return &Client{config: cfg, http: &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

// UseTransport makes the client reach Profile through rt instead of the network. OurTaikoNet
// uses it: Profile's features run in the same program.
func (c *Client) UseTransport(rt http.RoundTripper) { c.http.Transport = rt }

// Awarder records what a 段位 attempt earns inside the attempt's own transaction and returns
// what Award returns. A *Rejection means the attempt will never be taken and leaves
// the transaction usable; any other error fails the request.
//
// It replaces Client.Award where the titles and badges are kept in the same database
// (OurTaikoNet). Client.Award, with its separate call after the attempt is saved and
// the profileError retries that go with it, remains for a Profile that is another service.
type Awarder interface {
	Grant(ctx context.Context, tx pgx.Tx, request Request) (json.RawMessage, error)
}

// Request is one 段位 attempt as Profile records it.
type Request struct {
	Key         string   `json:"key"`
	UserID      string   `json:"userId"`
	CourseID    string   `json:"courseId"`
	CourseTitle string   `json:"courseTitle"`
	CourseOwner string   `json:"courseOwnerId"`
	Official    bool     `json:"official"`
	Verdict     int      `json:"verdict"`
	Dan         *int     `json:"dan"`
	Titles      []string `json:"titles"`
}

// Award asks Profile for what an attempt earns: its reply (granted and the new nameplate).
func (c *Client) Award(ctx context.Context, request Request) (json.RawMessage, error) {
	data, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.config.URL+"/internal/v1/awards", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Service-ID", c.config.ServiceID)
	req.Header.Set("X-Service-Key", c.config.ServiceKey)
	res, err := c.http.Do(req)
	if err != nil {
		return nil, errors.New("OurTaikoProfile unreachable")
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode == 400 || res.StatusCode == 409 || res.StatusCode == 422 {
		var problem struct {
			Code string `json:"code"`
		}
		json.Unmarshal(body, &problem)
		return nil, &Rejection{Status: res.StatusCode, Code: problem.Code}
	}
	if res.StatusCode != 200 || !json.Valid(body) {
		return nil, fmt.Errorf("OurTaikoProfile answered HTTP %d", res.StatusCode)
	}
	return body, nil
}

// Rejection is Profile turning an award down for good because the request itself is
// wrong, unlike Profile being unreachable or failing for a while: asking again cannot help.
type Rejection struct {
	Status int
	Code   string
	// Message is for people; the code is what programs go by.
	Message string
}

func (e *Rejection) Error() string {
	return fmt.Sprintf("OurTaikoProfile rejected the award: HTTP %d %s", e.Status, e.Code)
}

// Title is what a course needs to know of a title of Profile's catalog: whether it is
// official, and the player who made it (none for the titles Profile's administrators keep).
type Title struct {
	ID       string  `json:"id"`
	Official bool    `json:"official"`
	OwnerID  *string `json:"ownerId"`
}

// Titles is Profile's title catalog (GET /api/titles) by id, kept for a minute; fresh reads
// it again.
func (c *Client) Titles(ctx context.Context, fresh bool) (map[string]Title, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !fresh && c.titles != nil && time.Since(c.titlesAt) < time.Minute {
		return c.titles, nil
	}
	req, err := http.NewRequestWithContext(ctx, "GET", c.config.URL+"/api/titles", nil)
	if err != nil {
		return nil, err
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, errors.New("OurTaikoProfile unreachable")
	}
	defer res.Body.Close()
	var catalog struct {
		Titles []Title `json:"titles"`
	}
	if res.StatusCode != 200 || json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&catalog) != nil {
		return nil, fmt.Errorf("OurTaikoProfile answered HTTP %d", res.StatusCode)
	}
	titles := make(map[string]Title, len(catalog.Titles))
	for _, title := range catalog.Titles {
		titles[title.ID] = title
	}
	c.titles, c.titlesAt = titles, time.Now()
	return titles, nil
}
