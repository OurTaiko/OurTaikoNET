package multiplay

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	pb "ourtaiko.dev/net/internal/multiplay/multiplaypb"
)

// JoinKind is how a player asks to enter a room.
type JoinKind int

const (
	// QuickMatch enters a public room that is waiting, or a new one.
	QuickMatch JoinKind = iota + 1
	// CreatePrivate opens a new room only its number can join.
	CreatePrivate
	// ByCode enters the room with the number given; also how a dropped player comes back.
	ByCode
)

// JoinTarget is the room a player asks for. Code is the room's number, with ByCode only.
// Protocol is the version the player's game speaks (zero: 1).
type JoinTarget struct {
	Kind     JoinKind
	Code     string
	Protocol int32
}

// Manager is all rooms of this server process. Its mutex guards itself and every room.
type Manager struct {
	mu       sync.Mutex
	deps     Deps
	rooms    map[string]*Room
	userRoom map[string]*Room
	// dirty are the rooms that changed since the mutex was taken; unlock sends each one
	// snapshot.
	dirty  []*Room
	closed bool
	// draining: the server is about to be replaced. Rounds under way go on; no room opens, no
	// new player comes in and no round begins (Drain).
	draining bool
	// ctx ends when the manager closes: questions to the catalog stop waiting.
	ctx    context.Context
	cancel context.CancelFunc
}

// NewManager makes a manager with no rooms. What deps leaves zero is the real thing (the
// clock, time.AfterFunc, goroutines, DefaultTimings); the catalog must be given.
func NewManager(deps Deps) *Manager {
	m := &Manager{deps: deps.withDefaults(), rooms: map[string]*Room{}, userRoom: map[string]*Room{}}
	m.ctx, m.cancel = context.WithCancel(context.Background())
	return m
}

// Timings are the timings every room of the manager runs by.
func (m *Manager) Timings() Timings { return m.deps.Timings }

// unlock ends an operation: each room that changed sends its members one snapshot, however
// many changes the operation made, and the mutex is released.
func (m *Manager) unlock() {
	for len(m.dirty) > 0 {
		rooms := m.dirty
		m.dirty = nil
		for _, r := range rooms {
			r.dirty = false
			if !r.closed {
				r.broadcast(&pb.ServerMessage{Kind: &pb.ServerMessage_Room{Room: r.snapshot()}}, nil)
			}
		}
	}
	m.mu.Unlock()
}

// timer is a pending call of Deps.AfterFunc. dead is set, under the mutex, once it fired or
// was cancelled: a timer whose function was already waiting for the mutex when it was
// cancelled does nothing.
type timer struct {
	stop func()
	dead bool
}

// after calls f, with the mutex held, d from now. The mutex must be held.
func (m *Manager) after(d time.Duration, f func()) *timer {
	t := &timer{}
	t.stop = m.deps.AfterFunc(d, func() {
		m.mu.Lock()
		defer m.unlock()
		if t.dead {
			return
		}
		t.dead = true
		f()
	})
	return t
}

// cancel stops a timer; nil and finished timers are fine. The mutex must be held.
func (t *timer) cancel() {
	if t != nil && !t.dead {
		t.dead = true
		t.stop()
	}
}

// RoomCount is how many rooms are open.
func (m *Manager) RoomCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.rooms)
}

// Room is the open room with that number, or nil.
func (m *Manager) Room(code string) *Room {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.rooms[code]
}

// roomOf is the room the player is in, if any. The mutex must be held.
func (m *Manager) roomOf(userID string) *Room {
	r := m.userRoom[userID]
	if r != nil && !r.closed && r.find(userID) != nil {
		return r
	}
	delete(m.userRoom, userID)
	return nil
}

func validRoomCode(code string) bool {
	if len(code) != 5 {
		return false
	}
	for _, c := range code {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// Join puts the player in the room asked for and answers the connection with `welcome`; the
// room's snapshot follows. A player is in one room at a time: entering another leaves the
// first. Entering the room the player is already in, or dropped from, takes the place back,
// and a connection still open there is closed (REPLACED).
func (m *Manager) Join(player Player, target JoinTarget, conn Conn) (*Room, error) {
	m.mu.Lock()
	defer m.unlock()
	if m.closed {
		return nil, refuse("ROOM_CLOSED", "The server is shutting down")
	}
	current := m.roomOf(player.UserID)
	// A member coming back to their room is the one thing a server about to restart still does.
	if m.draining && (target.Kind != ByCode || current == nil || current.code != strings.TrimSpace(target.Code)) {
		return nil, refuse("SERVER_FULL", "The server is about to restart; try again in a moment")
	}
	var room *Room
	switch target.Kind {
	case ByCode:
		code := strings.TrimSpace(target.Code)
		if !validRoomCode(code) {
			return nil, refuse("BAD_ROOM_CODE", "A room number is 5 digits")
		}
		if room = m.rooms[code]; room == nil || room.closed {
			return nil, refuse("ROOM_NOT_FOUND", "No such room")
		}
		if current != nil && current != room {
			current.leave(player.UserID)
		}
	case QuickMatch, CreatePrivate:
		if current != nil {
			current.leave(player.UserID)
		}
		if target.Kind == QuickMatch {
			// The fullest waiting public room, so rooms fill up and start sooner; among equals
			// the lowest number, so the choice does not depend on map order.
			for _, r := range m.rooms {
				if r.acceptsQuickMatch() && (room == nil || len(r.members) > len(room.members) || len(r.members) == len(room.members) && r.code < room.code) {
					room = r
				}
			}
		}
		if room == nil {
			var err error
			if room, err = m.create(target.Kind == QuickMatch); err != nil {
				return nil, err
			}
		}
	default:
		return nil, refuse("BAD_REQUEST", "join names no room")
	}
	if err := room.join(player, conn, target.Protocol); err != nil {
		// A room opened for this player and refused to them is not left behind empty.
		if len(room.members) == 0 {
			room.close()
		}
		return nil, err
	}
	m.userRoom[player.UserID] = room
	// Welcome goes out before the room's snapshot, which unlock sends.
	conn.Send(&pb.ServerMessage{Kind: &pb.ServerMessage_Welcome{Welcome: &pb.Welcome{UserId: player.UserID, Nickname: player.Nickname, Protocol: Protocol}}})
	return room, nil
}

// Leave takes the player out of the room they are in, if any.
func (m *Manager) Leave(userID string) {
	m.mu.Lock()
	defer m.unlock()
	if r := m.roomOf(userID); r != nil {
		r.leave(userID)
	}
	delete(m.userRoom, userID)
}

// create opens an empty room with a number no open room has. The mutex must be held.
func (m *Manager) create(public bool) (*Room, error) {
	full := refuse("SERVER_FULL", "The server has no room left; try again later")
	if len(m.rooms) >= MaxRooms {
		return nil, full
	}
	code := ""
	for i := 0; i < 100 && (code == "" || m.rooms[code] != nil); i++ {
		code = fmt.Sprintf("%05d", 10000+m.deps.Intn(90000))
	}
	if m.rooms[code] != nil {
		return nil, full
	}
	r := &Room{m: m, code: code, public: public, phase: pb.Phase_LOBBY}
	m.rooms[code] = r
	return r, nil
}

// DrainState is how far a server about to be replaced has come.
type DrainState struct {
	Draining bool `json:"draining"`
	// Rooms are open; Busy of them are in a round, which replacing the server would cut short.
	Rooms int `json:"rooms"`
	Busy  int `json:"busy"`
}

// Drain makes the server ready to be replaced (on), or takes that back (off), when a full room
// that was kept waiting begins its round.
func (m *Manager) Drain(on bool) {
	m.mu.Lock()
	defer m.unlock()
	if m.draining == on {
		return
	}
	m.draining = on
	if !on {
		for _, r := range m.rooms {
			r.maybeStartFull()
		}
	}
}

// DrainState says whether the server is draining and what is still going on.
func (m *Manager) DrainState() DrainState {
	m.mu.Lock()
	defer m.mu.Unlock()
	state := DrainState{Draining: m.draining, Rooms: len(m.rooms)}
	for _, r := range m.rooms {
		if r.phase != pb.Phase_LOBBY {
			state.Busy++
		}
	}
	return state
}

// Close closes every room (each member is told ROOM_CLOSED) and refuses new ones.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.unlock()
	m.closed = true
	m.cancel()
	for _, r := range m.rooms {
		r.close()
	}
}
