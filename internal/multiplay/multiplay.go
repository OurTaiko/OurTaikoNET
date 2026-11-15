// Package multiplay is Multi-Enso (マルチ演奏): OurTaikoPlay's online rooms. Up to five players
// share a room with a 5-digit number; the host locks the members, everyone picks a song (one
// pick is drawn), each player picks a course of that song, the song is played together with
// everyone's score shown, and the result ranks the players. The protocol is
// proto/ourtaiko/multiplay/v1/multiplay.proto.
//
// Rooms live in memory: a restart closes them, and only one instance of the server can run.
//
// The room state machine (room.go, manager.go) does no I/O. Time, timers, randomness and the
// song catalog come in through Deps, connections are Conns. One mutex, the Manager's, guards
// the manager and every room: rooms are few and what is done under it is tiny. Nothing that
// can wait is done while it is held. The catalog is asked outside it, and what the room does
// with the answer first checks that the room is still in the round and phase the question
// was asked in; a Conn's Send only puts the message in that connection's queue.
//
// server.go is the gRPC service around it.
package multiplay

import (
	"context"
	"errors"
	"log"
	"math/rand/v2"
	"slices"
	"time"

	pb "ourtaiko.dev/net/internal/multiplay/multiplaypb"
)

// Protocol is the newest version of the protocol this server speaks. A client that asks for
// a newer one is told to wait for the server (Closed UPDATE_REQUIRED); an older client is
// served, down to MinProtocol.
//
//	2 (2026-11-15): `returned` and Member.away: after a play the room knows who is back at
//	its screen. A full room begins its next round once everyone is; the host's start leaves
//	those who are not behind (LEFT_BEHIND). A member that speaks 1 is never away.
const (
	Protocol    = 2
	MinProtocol = 1
)

// StampCount is how many stamps there are: ids 0 to StampCount-1.
const StampCount = 16

// MaxRooms is the most rooms open at once; more would only come from a misbehaving client.
const MaxRooms = 2000

// lookupTimeout bounds one question to the catalog.
const lookupTimeout = 10 * time.Second

// Timings are a room's limits and the length of each phase.
type Timings struct {
	MaxMembers int
	// MinPlayers is the fewest players a round needs.
	MinPlayers int
	SongSelect time.Duration
	SongDraw   time.Duration
	Course     time.Duration
	Loading    time.Duration
	// A play ends for everyone this long after the first player finished.
	FinishGrace time.Duration
	Result      time.Duration
	// A dropped player keeps its place this long.
	ReconnectGrace time.Duration
	// How often everyone's statuses are sent during a play.
	StatusInterval time.Duration
	// A player's stamps are at least this far apart.
	StampInterval time.Duration
	// No play is longer than this; a round still playing then is ended.
	MaxPlay time.Duration
}

// DefaultTimings are the timings of the production server.
var DefaultTimings = Timings{
	MaxMembers:     5,
	MinPlayers:     2,
	SongSelect:     90 * time.Second,
	SongDraw:       4 * time.Second,
	Course:         15 * time.Second,
	Loading:        90 * time.Second,
	FinishGrace:    30 * time.Second,
	Result:         10 * time.Second,
	ReconnectGrace: 10 * time.Second,
	StatusInterval: 250 * time.Millisecond,
	StampInterval:  time.Second,
	MaxPlay:        15 * time.Minute,
}

// Course is one course of a song and its star level.
type Course struct {
	// Easy, Normal, Hard, Oni or Edit.
	Name  string
	Level int
}

// Song is a chart a room can play.
type Song struct {
	// Source is the site's song source the chart is in ("fanmade", "official").
	Source  string
	ChartID string
	Title   string
	// Subtitle may be empty.
	Subtitle string
	// Courses are the chart's courses, easiest first. Never empty.
	Courses []Course
	// ChartJSON is the chart as the source's API describes it (GET /charts/{id}).
	ChartJSON []byte
}

// ErrChartNotFound is a Catalog's answer for a chart that does not exist or cannot be played.
var ErrChartNotFound = errors.New("no such playable chart")

// Catalog is the site's songs, as rooms need them. Its methods may take a while (they read
// the database) and are never called with the manager's mutex held.
type Catalog interface {
	// Get is the playable chart chartID of source; ErrChartNotFound when there is none.
	Get(ctx context.Context, source, chartID string) (*Song, error)
	// Random is any chart the game lists, for おまかせ; nil when the site has none.
	Random(ctx context.Context) (*Song, error)
}

// Deps is everything a Manager and its rooms take from outside. NewManager fills what is left
// zero with the real thing; tests give a clock of their own.
type Deps struct {
	Catalog Catalog
	Timings Timings
	Now     func() time.Time
	// Intn is a random number in [0, n).
	Intn func(n int) int
	// AfterFunc calls f once, d from now, without the manager's mutex held (f takes it), and
	// returns what stops it. A timer stopped too late does no harm: what it calls checks.
	AfterFunc func(d time.Duration, f func()) (stop func())
	// Go runs f without the manager's mutex held; the server starts a goroutine. It is how a
	// room asks the catalog for a random song from inside a timer.
	Go   func(f func())
	Logf func(format string, args ...any)
}

func (d Deps) withDefaults() Deps {
	if d.Timings == (Timings{}) {
		d.Timings = DefaultTimings
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Intn == nil {
		d.Intn = rand.IntN
	}
	if d.AfterFunc == nil {
		d.AfterFunc = func(wait time.Duration, f func()) func() {
			t := time.AfterFunc(wait, f)
			return func() { t.Stop() }
		}
	}
	if d.Go == nil {
		d.Go = func(f func()) { go f() }
	}
	if d.Logf == nil {
		d.Logf = log.Printf
	}
	return d
}

// Conn is one player's connection, as a room sees it. Both methods return at once and may be
// called with the manager's mutex held.
type Conn interface {
	// Send queues a message for the player. A connection whose queue is full is dropped.
	Send(*pb.ServerMessage)
	// Close sends `closed` and ends the stream. Nothing is sent after it.
	Close(code, message string)
}

// Player is who a connection belongs to, as the account center describes them.
type Player struct {
	UserID   string
	Nickname string
	// AvatarURL may be empty.
	AvatarURL string
}

// Error is a request a room refuses. Code is what the game shows in the player's language
// (Notice.code, Closed.code); Message is for logs.
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func refuse(code, message string) *Error { return &Error{Code: code, Message: message} }

var errNotInRoom = refuse("NOT_IN_ROOM", "You are not in this room")

// The courses, easiest first.
var courseOrder = []string{"Easy", "Normal", "Hard", "Oni", "Edit"}

// courseIndex is the course's place in courseOrder, -1 for a name that is no course.
func courseIndex(name string) int {
	for i, c := range courseOrder {
		if c == name {
			return i
		}
	}
	return -1
}

// CourseNames are the courses a chart can have, easiest first.
func CourseNames() []string { return slices.Clone(courseOrder) }

// nearestCourse is the course of courses a player who asked for preferred plays: that one
// when the song has it, else the nearest easier one, else the nearest harder one. An unknown
// or empty preferred is Oni. Empty only when the song has no course at all.
func nearestCourse(courses []Course, preferred string) string {
	want := courseIndex(preferred)
	if want < 0 {
		want = courseIndex("Oni")
	}
	easier, harder := -1, -1
	for _, c := range courses {
		i := courseIndex(c.Name)
		switch {
		case i < 0:
		case i == want:
			return c.Name
		case i < want && i > easier:
			easier = i
		case i > want && (harder < 0 || i < harder):
			harder = i
		}
	}
	if easier >= 0 {
		return courseOrder[easier]
	}
	if harder >= 0 {
		return courseOrder[harder]
	}
	return ""
}

func (s *Song) hasCourse(name string) bool {
	for _, c := range s.Courses {
		if c.Name == name {
			return true
		}
	}
	return false
}

func (s *Song) level(course string) int32 {
	for _, c := range s.Courses {
		if c.Name == course {
			return int32(c.Level)
		}
	}
	return 0
}

func (s *Song) message() *pb.Song {
	out := &pb.Song{Source: s.Source, ChartId: s.ChartID, Title: s.Title, Subtitle: s.Subtitle, ChartJson: s.ChartJSON}
	for _, c := range s.Courses {
		out.Courses = append(out.Courses, &pb.Course{Course: c.Name, Level: int32(c.Level)})
	}
	return out
}
