package multiplay

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	pb "ourtaiko.dev/net/internal/multiplay/multiplaypb"
)

// What the tests of the rooms are built from: a clock the test moves (fakeClock), a catalog
// of four songs (fakeCatalog), connections that keep what they were sent (fakeConn), and a
// manager on all of them (world). Nothing here waits or runs by itself: a test is the same
// every time.

// fakeClock is the time of a test and the timers set on it. Advance moves it and calls the
// timers that come due, in order, on the caller's goroutine.
type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*fakeTimer
	seq    int
}

type fakeTimer struct {
	when time.Time
	seq  int
	f    func()
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) AfterFunc(d time.Duration, f func()) func() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seq++
	t := &fakeTimer{when: c.now.Add(d), seq: c.seq, f: f}
	c.timers = append(c.timers, t)
	return func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		for i, x := range c.timers {
			if x == t {
				c.timers = append(c.timers[:i], c.timers[i+1:]...)
				return
			}
		}
	}
}

// next takes the earliest timer due by until, moving the clock to its time.
func (c *fakeClock) next(until time.Time) *fakeTimer {
	c.mu.Lock()
	defer c.mu.Unlock()
	sort.SliceStable(c.timers, func(i, j int) bool {
		if !c.timers[i].when.Equal(c.timers[j].when) {
			return c.timers[i].when.Before(c.timers[j].when)
		}
		return c.timers[i].seq < c.timers[j].seq
	})
	if len(c.timers) == 0 || c.timers[0].when.After(until) {
		c.now = until
		return nil
	}
	t := c.timers[0]
	c.timers = c.timers[1:]
	c.now = t.when
	return t
}

// Songs of the test catalog. Their chart JSON only has to be passed on untouched.
func song(source, id, title string, courses ...Course) *Song {
	return &Song{Source: source, ChartID: strings.Repeat(id, 32), Title: title, Subtitle: "--" + title, Courses: courses, ChartJSON: []byte(`{"title":"` + title + `"}`)}
}

var (
	songA = song("fanmade", "a", "Alpha", Course{"Easy", 3}, Course{"Normal", 5}, Course{"Hard", 7}, Course{"Oni", 9})
	songB = song("official", "b", "Beta", Course{"Oni", 8}, Course{"Edit", 10})
	songC = song("fanmade", "c", "Gamma", Course{"Easy", 2}, Course{"Hard", 6})
	songD = song("official", "d", "Delta", Course{"Hard", 7}, Course{"Edit", 10})
)

// fakeCatalog knows the four songs. random is what おまかせ draws.
type fakeCatalog struct {
	random    *Song
	randomErr error
	getErr    error
	// onGet and onRandom run while the catalog is being asked: what happens to a room
	// meanwhile.
	onGet, onRandom func()
	gets            int
}

func (c *fakeCatalog) Get(ctx context.Context, source, chartID string) (*Song, error) {
	c.gets++
	if c.onGet != nil {
		c.onGet()
	}
	if c.getErr != nil {
		return nil, c.getErr
	}
	for _, s := range []*Song{songA, songB, songC, songD} {
		if s.Source == source && s.ChartID == chartID {
			return s, nil
		}
	}
	return nil, ErrChartNotFound
}

func (c *fakeCatalog) Random(ctx context.Context) (*Song, error) {
	if c.onRandom != nil {
		c.onRandom()
	}
	return c.random, c.randomErr
}

// fakeConn keeps what a player was sent.
type fakeConn struct {
	mu       sync.Mutex
	messages []*pb.ServerMessage
	closed   *pb.Closed
}

func (c *fakeConn) Send(m *pb.ServerMessage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed == nil {
		c.messages = append(c.messages, m)
	}
}

func (c *fakeConn) Close(code, message string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed == nil {
		c.closed = &pb.Closed{Code: code, Message: message}
	}
}

func (c *fakeConn) all() []*pb.ServerMessage {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]*pb.ServerMessage(nil), c.messages...)
}

// closedCode is the code the connection was closed with, empty while it is open.
func (c *fakeConn) closedCode() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed.GetCode()
}

// room is the last snapshot the player was sent.
func (c *fakeConn) room() *pb.Room {
	all := c.all()
	for i := len(all) - 1; i >= 0; i-- {
		if r := all[i].GetRoom(); r != nil {
			return r
		}
	}
	return nil
}

func (c *fakeConn) snapshots() (n int) {
	for _, m := range c.all() {
		if m.GetRoom() != nil {
			n++
		}
	}
	return n
}

func (c *fakeConn) notices() (codes []string) {
	for _, m := range c.all() {
		if n := m.GetNotice(); n != nil {
			codes = append(codes, n.Code)
		}
	}
	return codes
}

func (c *fakeConn) stamps() (out []*pb.StampSent) {
	for _, m := range c.all() {
		if s := m.GetStamp(); s != nil {
			out = append(out, s)
		}
	}
	return out
}

func (c *fakeConn) statuses() (out []*pb.PlayStatuses) {
	for _, m := range c.all() {
		if s := m.GetStatuses(); s != nil {
			out = append(out, s)
		}
	}
	return out
}

func (c *fakeConn) starts() (out []*pb.StartPlay) {
	for _, m := range c.all() {
		if s := m.GetStartPlay(); s != nil {
			out = append(out, s)
		}
	}
	return out
}

// member is the player's entry in the last snapshot.
func (c *fakeConn) member(userID string) *pb.Member {
	for _, m := range c.room().GetMembers() {
		if m.UserId == userID {
			return m
		}
	}
	return nil
}

// world is a manager on a fake clock and catalog.
type world struct {
	t       *testing.T
	clock   *fakeClock
	catalog *fakeCatalog
	m       *Manager
	// draw is what the next draw among n picks returns (modulo n).
	draw int
	// hold keeps what the rooms start in the background (Deps.Go) until settle is called;
	// otherwise it runs as soon as the operation that started it is over.
	hold    bool
	mu      sync.Mutex
	pending []func()
	codes   int
	logs    []string
}

func newWorld(t *testing.T, change func(*Timings)) *world {
	t.Helper()
	w := &world{t: t, clock: &fakeClock{now: time.UnixMilli(1_800_000_000_000)}, catalog: &fakeCatalog{random: songB}}
	timings := DefaultTimings
	if change != nil {
		change(&timings)
	}
	w.m = NewManager(Deps{
		Catalog: w.catalog, Timings: timings, Now: w.clock.Now, AfterFunc: w.clock.AfterFunc,
		Intn: func(n int) int {
			// Room numbers come one after another (10001, 10002, …); draws are the test's.
			if n > 1000 {
				w.codes++
				return w.codes
			}
			return w.draw % n
		},
		Go: func(f func()) {
			w.mu.Lock()
			w.pending = append(w.pending, f)
			w.mu.Unlock()
		},
		Logf: func(format string, args ...any) { w.logs = append(w.logs, format) },
	})
	return w
}

// settle runs what the rooms started in the background.
func (w *world) settle() {
	for {
		w.mu.Lock()
		pending := w.pending
		w.pending = nil
		w.mu.Unlock()
		if len(pending) == 0 {
			return
		}
		for _, f := range pending {
			f()
		}
	}
}

// done is called after every operation of a test: background work runs unless held.
func (w *world) done() {
	if !w.hold {
		w.settle()
	}
}

// advance moves the clock, calling the timers that come due one by one.
func (w *world) advance(d time.Duration) {
	until := w.clock.Now().Add(d)
	w.done()
	for {
		t := w.clock.next(until)
		if t == nil {
			return
		}
		t.f()
		w.done()
	}
}

func player(n int) Player {
	id := strings.Repeat(string(rune('0'+n)), 32)
	return Player{UserID: id, Nickname: "P" + string(rune('0'+n)), AvatarURL: "https://avatars.test/" + id + ".webp"}
}

func uid(n int) string { return player(n).UserID }

// join puts a player in a room and fails the test when the room refuses.
func (w *world) join(n int, target JoinTarget) (*Room, *fakeConn) {
	w.t.Helper()
	conn := &fakeConn{}
	room, err := w.m.Join(player(n), target, conn)
	if err != nil {
		w.t.Fatalf("player %d could not join: %v", n, err)
	}
	w.done()
	return room, conn
}

// private opens a private room with players 1 to n in it; player 1 is the host.
func (w *world) private(n int) (*Room, []*fakeConn) {
	w.t.Helper()
	room, first := w.join(1, JoinTarget{Kind: CreatePrivate})
	conns := []*fakeConn{nil, first}
	for i := 2; i <= n; i++ {
		_, c := w.join(i, JoinTarget{Kind: ByCode, Code: room.Code()})
		conns = append(conns, c)
	}
	return room, conns
}

// must fails the test when an operation a room should accept is refused.
func (w *world) must(err error) {
	w.t.Helper()
	if err != nil {
		w.t.Fatal(err)
	}
	w.done()
}

// refused checks that the room refused with the given code.
func (w *world) refused(err error, code string) {
	w.t.Helper()
	var e *Error
	if !errors.As(err, &e) || e.Code != code {
		w.t.Fatalf("got %v, want a refusal with code %s", err, code)
	}
	w.done()
}

func (w *world) phase(room *Room, want pb.Phase) {
	w.t.Helper()
	if got := room.Phase(); got != want {
		w.t.Fatalf("the room is in %v, want %v", got, want)
	}
}

// playing brings a private room of two to the play: members locked, player 1 picks Alpha on
// Oni and wins the draw, player 2 picks Beta, everyone is ready and loaded.
func (w *world) playing() (*Room, []*fakeConn) {
	w.t.Helper()
	room, conns := w.private(2)
	w.must(room.LockMembers(uid(1)))
	w.draw = 0
	w.must(room.PickSong(uid(1), songA.Source, songA.ChartID, "Oni"))
	w.must(room.PickSong(uid(2), songB.Source, songB.ChartID, "Hard"))
	w.phase(room, pb.Phase_SONG_DRAW)
	w.advance(w.m.Timings().SongDraw)
	w.must(room.SetReady(uid(1), true))
	w.must(room.SetReady(uid(2), true))
	w.must(room.Loaded(uid(1), true))
	w.must(room.Loaded(uid(2), true))
	w.phase(room, pb.Phase_PLAYING)
	return room, conns
}
