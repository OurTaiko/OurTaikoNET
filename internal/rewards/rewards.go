// Package rewards tells players what they were given: titles, 段位 badges, whatever comes to be
// given later. It is told the same way for every request, the website's and the game's:
//
//   - every answer to a request whose player is known (package requester) carries the header
//     X-Pending-Rewards, the number of rewards they have not seen (Announce);
//   - a client that sees more than 0 fetches them, shows them and marks them seen (Routes).
//
// No answer's body says what was given, so an answer lost on the way loses nothing: the next one
// carries the header again.
//
// The package does not know what a reward is. A reward is a row of net.rewards, a kind and the
// data a client shows, written by the database's triggers on what can be given (migration
// 0009_rewards). Nor does it know how a request is signed in: Routes is given that.
package rewards

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"ourtaiko.dev/net/internal/requester"
	"ourtaiko.dev/net/internal/web"
)

// Header is the answer's header that says how many rewards the player has not seen.
const Header = "X-Pending-Rewards"

// Limit is how many rewards one fetch gives, and how many one call marks seen.
const Limit = 50

// DB is a pool, a connection or a transaction.
type DB interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Store reads and marks the rewards in net.rewards. A Store made by NewStore also keeps each
// player's count once read, while Run listens for the database saying it changed; the zero
// Store with a DB counts every time.
type Store struct {
	DB     DB
	counts *counts
	// Describe, when set, may add to a reward's data as it is fetched what a client needs to show
	// it and the database does not keep (a Don part's name and picture, from the catalog). It
	// returns the data to answer; the stored data does not change.
	Describe func(r *http.Request, kind string, data json.RawMessage) json.RawMessage
}

// NewStore is a Store that keeps counts (see Run).
func NewStore(db DB) Store { return Store{DB: db, counts: &counts{byUser: map[string]count{}}} }

// counts are players' counts of unseen rewards as last read. They are used only while live (Run
// is listening), and a count read across a change is not kept (generation).
type counts struct {
	mu         sync.Mutex
	byUser     map[string]count
	live       bool
	generation uint64
}

type count struct {
	n  int
	at time.Time
}

// countTTL bounds how long a count is used even without a word from the database.
const countTTL = 5 * time.Minute

func (c *counts) get(user string) (int, uint64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.byUser[user]
	if !c.live || !ok || time.Since(v.at) > countTTL {
		return 0, c.generation, false
	}
	return v.n, c.generation, true
}

// put keeps a count read since generation, unless something changed meanwhile.
func (c *counts) put(user string, n int, since uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.live && c.generation == since {
		c.byUser[user] = count{n: n, at: time.Now()}
	}
}

func (c *counts) forget(user string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.generation++
	if user == "" {
		c.byUser = map[string]count{}
	} else {
		delete(c.byUser, user)
	}
}

func (c *counts) setLive(live bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.live = live
	c.generation++
	c.byUser = map[string]count{}
}

// Reward is one thing a player was given, as a client shows it. Data is the kind's own.
type Reward struct {
	ID        int64           `json:"id"`
	Kind      string          `json:"kind"`
	Data      json.RawMessage `json:"data"`
	CreatedAt time.Time       `json:"createdAt"`
}

// Pending is how many rewards the player has not seen. fresh reads it from the database even
// when a count is kept: a request that may have given something counts it.
func (s Store) Pending(ctx context.Context, user string, fresh bool) (int, error) {
	var since uint64
	if s.counts != nil && !fresh {
		n, generation, ok := s.counts.get(user)
		if ok {
			return n, nil
		}
		since = generation
	} else if s.counts != nil {
		_, since, _ = s.counts.get(user)
	}
	var n int
	err := s.DB.QueryRow(ctx, `SELECT count(*) FROM net.rewards WHERE user_id=$1 AND seen_at IS NULL`, user).Scan(&n)
	if err == nil && s.counts != nil {
		s.counts.put(user, n, since)
	}
	return n, err
}

// Unseen is the player's rewards not seen yet, oldest first, at most Limit.
func (s Store) Unseen(ctx context.Context, user string) ([]Reward, error) {
	rows, err := s.DB.Query(ctx, `SELECT id,kind,data,created_at FROM net.rewards WHERE user_id=$1 AND seen_at IS NULL ORDER BY id LIMIT $2`, user, Limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Reward, error) {
		var r Reward
		err := row.Scan(&r.ID, &r.Kind, &r.Data, &r.CreatedAt)
		return r, err
	})
}

// Seen marks the player's rewards of these ids seen. Ids of other players' rewards, or of ones
// seen already, are passed over. It returns how many it marked.
func (s Store) Seen(ctx context.Context, user string, ids []int64) (int, error) {
	tag, err := s.DB.Exec(ctx, `UPDATE net.rewards SET seen_at=now() WHERE user_id=$1 AND id=ANY($2) AND seen_at IS NULL`, user, ids)
	if s.counts != nil {
		s.counts.forget(user)
	}
	return int(tag.RowsAffected()), err
}

// Announce wraps a handler so that every answer to a request whose player became known while
// it was handled (requester.Note) carries Header. It gives each request the place for its
// player (requester.With), so it goes outside everything that authenticates.
//
// The count is read as the answer starts, after the handler has done its work, so an answer
// counts what that very request gave or marked seen. If it cannot be read the header is left
// out: a client then simply hears nothing new from this answer.
func (s Store) Announce(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(requester.With(r.Context()))
		a := &announcer{ResponseWriter: w, store: s, r: r}
		next.ServeHTTP(a, r)
		// A handler that wrote nothing: the header goes on before the server writes its 200.
		a.announce()
	})
}

type announcer struct {
	http.ResponseWriter
	store Store
	r     *http.Request
	done  bool
}

func (a *announcer) announce() {
	if a.done {
		return
	}
	a.done = true
	user := requester.Of(a.r.Context())
	if user == "" {
		return
	}
	// The request may have been given up; the count is still worth the answer's header.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(a.r.Context()), 2*time.Second)
	defer cancel()
	// GET and HEAD give nothing; any other request may have, so it counts afresh.
	fresh := a.r.Method != http.MethodGet && a.r.Method != http.MethodHead
	n, err := a.store.Pending(ctx, user, fresh)
	if err != nil {
		log.Printf("rewards: counting for the answer: %v", err)
		return
	}
	a.Header().Set(Header, strconv.Itoa(n))
}

func (a *announcer) WriteHeader(status int) {
	a.announce()
	a.ResponseWriter.WriteHeader(status)
}

func (a *announcer) Write(b []byte) (int, error) {
	a.announce()
	return a.ResponseWriter.Write(b)
}

// Flush keeps streaming answers streaming.
func (a *announcer) Flush() {
	a.announce()
	if f, ok := a.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the writer underneath.
func (a *announcer) Unwrap() http.ResponseWriter { return a.ResponseWriter }

// Retention is how long a reward is kept once seen.
const Retention = 90 * 24 * time.Hour

// Listener is a connection Run listens on (a pool's connection).
type Listener interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	WaitForNotification(ctx context.Context) (*pgconn.Notification, error)
}

// Run keeps the store's counts right and its table small until ctx ends: it listens for the
// database saying a player's rewards changed (migration 0009's net_rewards), and counts are kept
// only while it does; every hour it deletes what was seen more than Retention ago. connect gives
// a connection to listen on and a function to give it back; when the connection fails, counts
// stop being kept until another is listening.
func (s Store) Run(ctx context.Context, connect func(context.Context) (Listener, func(), error)) {
	prune := time.NewTicker(time.Hour)
	defer prune.Stop()
	go func() {
		for {
			if _, err := s.Prune(ctx); err != nil && ctx.Err() == nil {
				log.Printf("rewards: pruning: %v", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-prune.C:
			}
		}
	}()
	if s.counts == nil {
		<-ctx.Done()
		return
	}
	for ctx.Err() == nil {
		err := s.listen(ctx, connect)
		s.counts.setLive(false)
		if ctx.Err() != nil {
			return
		}
		log.Printf("rewards: listening stopped, counting every answer until it is back: %v", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}
}

func (s Store) listen(ctx context.Context, connect func(context.Context) (Listener, func(), error)) error {
	conn, release, err := connect(ctx)
	if err != nil {
		return err
	}
	defer release()
	if _, err = conn.Exec(ctx, `LISTEN net_rewards`); err != nil {
		return err
	}
	s.counts.setLive(true)
	for {
		n, err := conn.WaitForNotification(ctx)
		if err != nil {
			return err
		}
		s.counts.forget(n.Payload)
	}
}

// Prune deletes the rewards seen more than Retention ago and returns how many.
func (s Store) Prune(ctx context.Context) (int64, error) {
	tag, err := s.DB.Exec(ctx, `DELETE FROM net.rewards WHERE seen_at < now() - make_interval(secs => $1)`, Retention.Seconds())
	return tag.RowsAffected(), err
}

// Auth says who a request is from, answering it itself when nobody is (or, with write, when the
// request may not change anything, such as a website's request without its CSRF token).
type Auth func(w http.ResponseWriter, r *http.Request, write bool) (user string, ok bool)

var answers = web.Problems{}

// Routes is the rewards' API under prefix, for players auth signs in:
//
//	GET  <prefix>       → {"rewards": [Reward…]}, the ones not seen, oldest first
//	POST <prefix>/seen  {"ids": [id…]} → {"marked": n}
//
// The site mounts it once for the website and once for the game, each with its own Auth.
func (s Store) Routes(mux *http.ServeMux, prefix string, auth Auth) {
	mux.HandleFunc("GET "+prefix, func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth(w, r, false)
		if !ok {
			return
		}
		list, err := s.Unseen(r.Context(), user)
		if err != nil {
			answers.Internal(w, err)
			return
		}
		if s.Describe != nil {
			for i := range list {
				list[i].Data = s.Describe(r, list[i].Kind, list[i].Data)
			}
		}
		web.Respond(w, 200, map[string]any{"rewards": list})
	})
	mux.HandleFunc("POST "+prefix+"/seen", func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth(w, r, true)
		if !ok {
			return
		}
		var body struct {
			IDs []int64 `json:"ids"`
		}
		if !answers.DecodeLimit(w, r, &body, 16<<10) {
			return
		}
		if len(body.IDs) == 0 || len(body.IDs) > Limit {
			answers.Problem(w, 422, "REWARDS_INVALID", "需要 1–50 个奖励 ID")
			return
		}
		n, err := s.Seen(r.Context(), user, body.IDs)
		if err != nil {
			answers.Internal(w, err)
			return
		}
		web.Respond(w, 200, map[string]int{"marked": n})
	})
}
