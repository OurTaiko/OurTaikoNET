// Package roomsapi is how the rooms program (cmd/rooms) asks the site (cmd/server) who a game
// session belongs to and what the songs are. It exists so that the two can be released apart:
// the rooms read neither the database nor anything else whose shape a release of the site may
// change, and live through the site's restarts.
//
//	POST /identity            {"token": "<game session>"}: 200 multiplay.Player, 401 not valid
//	GET  /charts/{source}/{id}  200 multiplay.Song, 404 none that can be played
//	GET  /random              200 multiplay.Song, 204 the site has no playable chart
//
// The site answers on a port of its own that the reverse proxy does not pass on. It asks for no
// key: the charts are public, and /identity tells only the holder of a session whose it is.
package roomsapi

import (
	"bytes"
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"ourtaiko.dev/net/internal/multiplay"
)

const (
	// How long the site may take over one question.
	answerTimeout = 8 * time.Second
	// The longest answer: a chart's description, which is far smaller.
	maxAnswerBytes = 8 << 20

	// One try of a question, and the waits between tries while the site is restarting.
	attemptTimeout = 3 * time.Second
	firstRetry     = 200 * time.Millisecond
	longestRetry   = time.Second
	// A player the site knew is not kept waiting for it: a dropped player's place is kept for
	// 10 s only, and the site may be away for longer.
	knownPlayerWait = 1500 * time.Millisecond
	// How long the site's word about a session stands while the site cannot be asked.
	knownPlayerLife = 12 * time.Hour

	maxKnownPlayers = 4096
	maxKnownSongs   = 128
)

/* ---------- the site's side ---------- */

// Handler answers the rooms' questions from the site's own catalog and accounts.
func Handler(catalog multiplay.Catalog, identify multiplay.Authenticator) http.Handler {
	mux := http.NewServeMux()
	failed := func(w http.ResponseWriter, what string, err error) {
		log.Printf("roomsapi: %s: %v", what, err)
		http.Error(w, "the site could not answer", http.StatusServiceUnavailable)
	}
	reply := func(w http.ResponseWriter, value any) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(value)
	}
	mux.HandleFunc("POST /identity", func(w http.ResponseWriter, r *http.Request) {
		var asked struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&asked); err != nil || asked.Token == "" {
			http.Error(w, "token is required", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), answerTimeout)
		defer cancel()
		player, err := identify(ctx, asked.Token)
		switch {
		case errors.Is(err, multiplay.ErrUnauthenticated):
			w.WriteHeader(http.StatusUnauthorized)
		case err != nil:
			failed(w, "identity", err)
		default:
			reply(w, player)
		}
	})
	mux.HandleFunc("GET /charts/{source}/{id}", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), answerTimeout)
		defer cancel()
		song, err := catalog.Get(ctx, r.PathValue("source"), r.PathValue("id"))
		switch {
		case errors.Is(err, multiplay.ErrChartNotFound):
			w.WriteHeader(http.StatusNotFound)
		case err != nil:
			failed(w, "chart", err)
		default:
			reply(w, song)
		}
	})
	mux.HandleFunc("GET /random", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), answerTimeout)
		defer cancel()
		song, err := catalog.Random(ctx)
		switch {
		case err != nil:
			failed(w, "random chart", err)
		case song == nil:
			w.WriteHeader(http.StatusNoContent)
		default:
			reply(w, song)
		}
	})
	return mux
}

/* ---------- the rooms' side ---------- */

// Client asks the site. It is the rooms' multiplay.Catalog and Authenticate. While the site is
// restarting a question is tried again until the caller gives up, and what the site said last
// stands in for an answer: rooms that are playing never notice.
type Client struct {
	base string
	http *http.Client
	logf func(format string, args ...any)

	mu      sync.Mutex
	players *recent[[sha256.Size]byte, knownPlayer]
	songs   *recent[string, *multiplay.Song]
}

type knownPlayer struct {
	player multiplay.Player
	until  time.Time
}

// NewClient asks the site at base (http://host:port).
func NewClient(base string) *Client {
	return &Client{
		base: strings.TrimRight(base, "/"), http: &http.Client{}, logf: log.Printf,
		players: newRecent[[sha256.Size]byte, knownPlayer](maxKnownPlayers),
		songs:   newRecent[string, *multiplay.Song](maxKnownSongs),
	}
}

// Authenticate is the multiplay.Authenticator of the rooms.
func (c *Client) Authenticate(ctx context.Context, token string) (multiplay.Player, error) {
	key := sha256.Sum256([]byte(token))
	c.mu.Lock()
	known, isKnown := c.players.get(key)
	c.mu.Unlock()
	isKnown = isKnown && time.Now().Before(known.until)
	if isKnown {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, knownPlayerWait)
		defer cancel()
	}
	body, _ := json.Marshal(map[string]string{"token": token})
	status, answer, err := c.ask(ctx, http.MethodPost, "/identity", body)
	switch {
	case err != nil && isKnown:
		c.logf("roomsapi: the site cannot be asked; going by what it last said of a session: %v", err)
		return known.player, nil
	case err != nil:
		return multiplay.Player{}, err
	case status == http.StatusUnauthorized:
		c.mu.Lock()
		c.players.remove(key)
		c.mu.Unlock()
		return multiplay.Player{}, multiplay.ErrUnauthenticated
	case status != http.StatusOK:
		return multiplay.Player{}, fmt.Errorf("the site answered %d about a session", status)
	}
	var player multiplay.Player
	if err = json.Unmarshal(answer, &player); err != nil {
		return multiplay.Player{}, err
	}
	c.mu.Lock()
	c.players.put(key, knownPlayer{player, time.Now().Add(knownPlayerLife)})
	c.mu.Unlock()
	return player, nil
}

// Get is the playable chart chartID of source.
func (c *Client) Get(ctx context.Context, source, chartID string) (*multiplay.Song, error) {
	key := source + "/" + chartID
	status, answer, err := c.ask(ctx, http.MethodGet, "/charts/"+url.PathEscape(source)+"/"+url.PathEscape(chartID), nil)
	switch {
	case err != nil:
		c.mu.Lock()
		song, known := c.songs.get(key)
		c.mu.Unlock()
		if known {
			c.logf("roomsapi: the site cannot be asked; going by what it last said of chart %s: %v", key, err)
			return song, nil
		}
		return nil, err
	case status == http.StatusNotFound:
		c.mu.Lock()
		c.songs.remove(key)
		c.mu.Unlock()
		return nil, multiplay.ErrChartNotFound
	case status != http.StatusOK:
		return nil, fmt.Errorf("the site answered %d about chart %s", status, key)
	}
	return c.keep(answer)
}

// Random is any chart the game lists; nil when the site has none.
func (c *Client) Random(ctx context.Context) (*multiplay.Song, error) {
	status, answer, err := c.ask(ctx, http.MethodGet, "/random", nil)
	switch {
	case err != nil:
		c.mu.Lock()
		song, known := c.songs.any()
		c.mu.Unlock()
		if known {
			c.logf("roomsapi: the site cannot be asked; drawing from the charts it last named: %v", err)
			return song, nil
		}
		return nil, err
	case status == http.StatusNoContent:
		return nil, nil
	case status != http.StatusOK:
		return nil, fmt.Errorf("the site answered %d for a random chart", status)
	}
	return c.keep(answer)
}

// keep reads a song the site described and remembers it.
func (c *Client) keep(answer []byte) (*multiplay.Song, error) {
	song := &multiplay.Song{}
	if err := json.Unmarshal(answer, song); err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.songs.put(song.Source+"/"+song.ChartID, song)
	c.mu.Unlock()
	return song, nil
}

// ask puts a question to the site until it answers, or ctx ends. The site not being there and
// the site failing (5xx) are the same thing: it may be on its way down or up.
func (c *Client) ask(ctx context.Context, method, path string, body []byte) (int, []byte, error) {
	wait := firstRetry
	for {
		status, answer, err := c.try(ctx, method, path, body)
		if err == nil && status < 500 {
			return status, answer, nil
		}
		if err == nil {
			err = fmt.Errorf("the site answered %d", status)
		}
		select {
		case <-ctx.Done():
			return 0, nil, fmt.Errorf("%w (last: %v)", ctx.Err(), err)
		case <-time.After(wait):
		}
		wait = min(wait*2, longestRetry)
	}
}

func (c *Client) try(ctx context.Context, method, path string, body []byte) (int, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, attemptTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, method, c.base+path, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	response, err := c.http.Do(request)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	answer, err := io.ReadAll(io.LimitReader(response.Body, maxAnswerBytes))
	return response.StatusCode, answer, err
}

// recent keeps the values last put, up to max of them. Not safe for concurrent use.
type recent[K comparable, V any] struct {
	max   int
	order *list.List
	items map[K]*list.Element
}

type recentItem[K comparable, V any] struct {
	key   K
	value V
}

func newRecent[K comparable, V any](max int) *recent[K, V] {
	return &recent[K, V]{max: max, order: list.New(), items: map[K]*list.Element{}}
}

func (r *recent[K, V]) get(key K) (value V, ok bool) {
	if e := r.items[key]; e != nil {
		return e.Value.(recentItem[K, V]).value, true
	}
	return value, false
}

func (r *recent[K, V]) put(key K, value V) {
	r.remove(key)
	r.items[key] = r.order.PushFront(recentItem[K, V]{key, value})
	if r.order.Len() > r.max {
		r.remove(r.order.Back().Value.(recentItem[K, V]).key)
	}
}

func (r *recent[K, V]) remove(key K) {
	if e := r.items[key]; e != nil {
		r.order.Remove(e)
		delete(r.items, key)
	}
}

// any is one of the values, each with the same chance.
func (r *recent[K, V]) any() (value V, ok bool) {
	if r.order.Len() == 0 {
		return value, false
	}
	e := r.order.Front()
	for n := rand.IntN(r.order.Len()); n > 0; n-- {
		e = e.Next()
	}
	return e.Value.(recentItem[K, V]).value, true
}
