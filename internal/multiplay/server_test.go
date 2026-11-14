package multiplay

import (
	"context"
	"errors"
	"io"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	pb "ourtaiko.dev/net/internal/multiplay/multiplaypb"
)

// The gRPC service, from a real client over a loopback connection: an account center that
// knows three game sessions, a catalog of the four test songs, and rooms on the real clock
// with the waits shortened.

var (
	aliceToken  = strings.Repeat("1", 64)
	bobToken    = strings.Repeat("2", 64)
	carolToken  = strings.Repeat("3", 64)
	downToken   = strings.Repeat("d", 64)
	strangerKey = strings.Repeat("9", 64)
	alice       = Player{UserID: strings.Repeat("a", 32), Nickname: "Alice", AvatarURL: "https://avatars.test/alice.webp"}
	bob         = Player{UserID: strings.Repeat("b", 32), Nickname: "Bob"}
	carol       = Player{UserID: strings.Repeat("c", 32), Nickname: "Carol"}
)

// testAccounts answers for the three sessions; downToken finds the account center down.
func testAccounts(ctx context.Context, token string) (Player, error) {
	switch token {
	case aliceToken:
		return alice, nil
	case bobToken:
		return bob, nil
	case carolToken:
		return carol, nil
	case downToken:
		return Player{}, errors.New("connection refused")
	}
	return Player{}, ErrUnauthenticated
}

// staticCatalog is the four test songs; おまかせ draws Beta. Safe to ask from many streams.
type staticCatalog struct{}

func (staticCatalog) Get(ctx context.Context, source, chartID string) (*Song, error) {
	for _, s := range []*Song{songA, songB, songC, songD} {
		if s.Source == source && s.ChartID == chartID {
			return s, nil
		}
	}
	return nil, ErrChartNotFound
}

func (staticCatalog) Random(ctx context.Context) (*Song, error) { return songB, nil }

// startServer runs a server on a loopback port until the test ends.
func startServer(t *testing.T, change func(*ServerConfig)) *Server {
	t.Helper()
	timings := DefaultTimings
	timings.SongDraw = 50 * time.Millisecond
	timings.StatusInterval = 20 * time.Millisecond
	timings.Result = 300 * time.Millisecond
	timings.ReconnectGrace = 3 * time.Second
	cfg := ServerConfig{Catalog: staticCatalog{}, Authenticate: testAccounts, Timings: timings, Logf: t.Logf}
	if change != nil {
		change(&cfg)
	}
	s := NewServer(cfg)
	if err := s.Start("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Stop)
	return s
}

// client is a player's stream: it collects what the server sends and waits for the message
// a test is after.
type client struct {
	t       *testing.T
	stream  pb.MultiEnso_ConnectClient
	cancel  context.CancelFunc
	mu      sync.Mutex
	inbox   []*pb.ServerMessage
	over    bool
	err     error
	changed chan struct{}
}

// connect opens a stream with the given session ("" for none).
func connect(t *testing.T, s *Server, token string) *client {
	t.Helper()
	conn, err := grpc.NewClient(s.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if token != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
	}
	stream, err := pb.NewMultiEnsoClient(conn).Connect(ctx)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	c := &client{t: t, stream: stream, cancel: cancel, changed: make(chan struct{}, 1)}
	t.Cleanup(func() {
		cancel()
		conn.Close()
	})
	go func() {
		for {
			m, err := stream.Recv()
			c.mu.Lock()
			if err != nil {
				c.over = true
				if err != io.EOF {
					c.err = err
				}
			} else {
				c.inbox = append(c.inbox, m)
			}
			c.mu.Unlock()
			select {
			case c.changed <- struct{}{}:
			default:
			}
			if err != nil {
				return
			}
		}
	}()
	return c
}

func (c *client) send(m *pb.ClientMessage) {
	c.t.Helper()
	if err := c.stream.Send(m); err != nil {
		c.t.Fatalf("send: %v", err)
	}
}

// next waits for a message the test accepts and takes it out of the inbox.
func (c *client) next(what string, accept func(*pb.ServerMessage) bool) *pb.ServerMessage {
	c.t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		c.mu.Lock()
		for i, m := range c.inbox {
			if accept(m) {
				c.inbox = append(c.inbox[:i], c.inbox[i+1:]...)
				c.mu.Unlock()
				return m
			}
		}
		over, err := c.over, c.err
		c.mu.Unlock()
		if over {
			c.t.Fatalf("the stream ended (%v) before %s came", err, what)
		}
		select {
		case <-c.changed:
		case <-deadline:
			c.t.Fatalf("timed out waiting for %s", what)
		}
	}
}

// finished waits for the stream to end and returns the error it ended with, nil for a
// clean end.
func (c *client) finished() error {
	c.t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		c.mu.Lock()
		over, err := c.over, c.err
		c.mu.Unlock()
		if over {
			return err
		}
		select {
		case <-c.changed:
		case <-deadline:
			c.t.Fatal("timed out waiting for the stream to end")
		}
	}
}

// refusedWith checks that the stream ended with the gRPC status and message given.
func (c *client) refusedWith(code codes.Code, message string) {
	c.t.Helper()
	err := c.finished()
	if st, _ := status.FromError(err); err == nil || st.Code() != code || !strings.Contains(st.Message(), message) {
		c.t.Fatalf("the stream ended with %v, want %v %s", err, code, message)
	}
}

// closedWith checks that the server said `closed` with the code and then ended the stream
// cleanly.
func (c *client) closedWith(code string) {
	c.t.Helper()
	m := c.next("closed "+code, func(m *pb.ServerMessage) bool { return m.GetClosed() != nil })
	if m.GetClosed().Code != code || m.GetClosed().Message == "" {
		c.t.Fatalf("closed with %v, want %s", m.GetClosed(), code)
	}
	if err := c.finished(); err != nil {
		c.t.Fatalf("after closed %s the stream ended with %v", code, err)
	}
}

func (c *client) room(what string, accept func(*pb.Room) bool) *pb.Room {
	c.t.Helper()
	return c.next(what, func(m *pb.ServerMessage) bool { return m.GetRoom() != nil && accept(m.GetRoom()) }).GetRoom()
}

func (c *client) phase(p pb.Phase) *pb.Room {
	c.t.Helper()
	return c.room("the room in "+p.String(), func(r *pb.Room) bool { return r.Phase == p })
}

func (c *client) notice(code string) {
	c.t.Helper()
	m := c.next("notice "+code, func(m *pb.ServerMessage) bool { return m.GetNotice() != nil })
	if m.GetNotice().Code != code || m.GetNotice().Message == "" {
		c.t.Fatalf("notice %v, want %s", m.GetNotice(), code)
	}
}

// Client messages.
func joinQuick() *pb.ClientMessage {
	return &pb.ClientMessage{Kind: &pb.ClientMessage_Join{Join: &pb.Join{Target: &pb.Join_QuickMatch{QuickMatch: &pb.QuickMatch{}}, Protocol: 1}}}
}
func joinPrivate() *pb.ClientMessage {
	return &pb.ClientMessage{Kind: &pb.ClientMessage_Join{Join: &pb.Join{Target: &pb.Join_CreatePrivate{CreatePrivate: &pb.CreatePrivate{}}, Protocol: 1}}}
}
func joinCode(code string) *pb.ClientMessage {
	return &pb.ClientMessage{Kind: &pb.ClientMessage_Join{Join: &pb.Join{Target: &pb.Join_RoomCode{RoomCode: code}}}}
}
func lockMembers() *pb.ClientMessage {
	return &pb.ClientMessage{Kind: &pb.ClientMessage_LockMembers{LockMembers: &pb.LockMembers{}}}
}
func pickSong(s *Song, course string) *pb.ClientMessage {
	pick := &pb.PickSong{Course: course}
	if s != nil {
		pick.Source, pick.ChartId = s.Source, s.ChartID
	}
	return &pb.ClientMessage{Kind: &pb.ClientMessage_PickSong{PickSong: pick}}
}
func pickCourse(course string) *pb.ClientMessage {
	return &pb.ClientMessage{Kind: &pb.ClientMessage_PickCourse{PickCourse: &pb.PickCourse{Course: course}}}
}
func setReady() *pb.ClientMessage {
	return &pb.ClientMessage{Kind: &pb.ClientMessage_SetReady{SetReady: &pb.SetReady{Ready: true}}}
}
func loaded() *pb.ClientMessage {
	return &pb.ClientMessage{Kind: &pb.ClientMessage_Loaded{Loaded: &pb.Loaded{Ok: true}}}
}
func playStatus(s *pb.PlayerStatus) *pb.ClientMessage {
	return &pb.ClientMessage{Kind: &pb.ClientMessage_Status{Status: s}}
}
func finish(s *pb.PlayerStatus, cleared bool) *pb.ClientMessage {
	return &pb.ClientMessage{Kind: &pb.ClientMessage_Finish{Finish: &pb.FinishPlay{Final: s, Cleared: cleared}}}
}
func stamp(id int32) *pb.ClientMessage {
	return &pb.ClientMessage{Kind: &pb.ClientMessage_Stamp{Stamp: &pb.SendStamp{StampId: id}}}
}
func ping(at int64) *pb.ClientMessage {
	return &pb.ClientMessage{Kind: &pb.ClientMessage_Ping{Ping: &pb.Ping{ClientTimeMs: at}}}
}
func leave() *pb.ClientMessage {
	return &pb.ClientMessage{Kind: &pb.ClientMessage_Leave{Leave: &pb.Leave{}}}
}
func kick(userID string) *pb.ClientMessage {
	return &pb.ClientMessage{Kind: &pb.ClientMessage_Kick{Kick: &pb.Kick{UserId: userID}}}
}

// roomOfTwo is Alice in a private room she opened and Bob entered by its number.
func roomOfTwo(t *testing.T, s *Server) (a, b *client, code string) {
	t.Helper()
	a = connect(t, s, aliceToken)
	a.send(joinPrivate())
	code = a.phase(pb.Phase_LOBBY).Code
	b = connect(t, s, bobToken)
	b.send(joinCode(code))
	b.room("the room with two members", func(r *pb.Room) bool { return len(r.Members) == 2 })
	a.room("the room with two members", func(r *pb.Room) bool { return len(r.Members) == 2 })
	return a, b, code
}

func TestStreamNeedsAGameSession(t *testing.T) {
	s := startServer(t, nil)
	// Refused at once, before anything is sent.
	connect(t, s, "").refusedWith(codes.Unauthenticated, "AUTH_REQUIRED")
	connect(t, s, "not-a-session").refusedWith(codes.Unauthenticated, "AUTH_REQUIRED")
	connect(t, s, strings.Repeat("z", 64)).refusedWith(codes.Unauthenticated, "AUTH_REQUIRED")
	// A session the account center does not know.
	stranger := connect(t, s, strangerKey)
	stranger.stream.Send(joinQuick()) // the stream may be over already
	stranger.refusedWith(codes.Unauthenticated, "AUTH_REQUIRED")
	if s.Manager().RoomCount() != 0 {
		t.Fatal("a room was opened for nobody")
	}
}

func TestAccountCenterDownIsNotARefusedSession(t *testing.T) {
	s := startServer(t, nil)
	c := connect(t, s, downToken)
	c.stream.Send(joinQuick()) // the stream may be over already
	c.refusedWith(codes.Unavailable, "AUTH_UNAVAILABLE")
}

func TestFirstMessageMustBeJoin(t *testing.T) {
	s := startServer(t, nil)
	c := connect(t, s, aliceToken)
	c.send(lockMembers())
	c.refusedWith(codes.FailedPrecondition, "first message must be join")
	// A join that names no room is none.
	c = connect(t, s, aliceToken)
	c.send(&pb.ClientMessage{Kind: &pb.ClientMessage_Join{Join: &pb.Join{Protocol: 1}}})
	c.refusedWith(codes.FailedPrecondition, "first message must be join")
	c = connect(t, s, aliceToken)
	c.send(&pb.ClientMessage{})
	c.refusedWith(codes.FailedPrecondition, "first message must be join")
}

func TestJoinMustComeInTime(t *testing.T) {
	s := startServer(t, func(cfg *ServerConfig) { cfg.JoinTimeout = 150 * time.Millisecond })
	connect(t, s, aliceToken).refusedWith(codes.DeadlineExceeded, "join was not sent")
	// A player who joined in time stays.
	c := connect(t, s, bobToken)
	c.send(joinPrivate())
	c.phase(pb.Phase_LOBBY)
	time.Sleep(300 * time.Millisecond)
	c.send(ping(1))
	c.next("pong", func(m *pb.ServerMessage) bool { return m.GetPong() != nil })
}

func TestNewerProtocolIsRefused(t *testing.T) {
	s := startServer(t, nil)
	c := connect(t, s, aliceToken)
	c.send(&pb.ClientMessage{Kind: &pb.ClientMessage_Join{Join: &pb.Join{Target: &pb.Join_QuickMatch{QuickMatch: &pb.QuickMatch{}}, Protocol: Protocol + 1}}})
	c.closedWith("UPDATE_REQUIRED")
	if s.Manager().RoomCount() != 0 {
		t.Fatal("a room was opened")
	}
	// An older client, or one that says nothing, is served.
	c = connect(t, s, aliceToken)
	c.send(&pb.ClientMessage{Kind: &pb.ClientMessage_Join{Join: &pb.Join{Target: &pb.Join_QuickMatch{QuickMatch: &pb.QuickMatch{}}}}})
	if w := c.next("welcome", func(m *pb.ServerMessage) bool { return m.GetWelcome() != nil }).GetWelcome(); w.Protocol != Protocol {
		t.Fatalf("welcome: %v", w)
	}
}

func TestJoinThatIsRefusedClosesTheStream(t *testing.T) {
	s := startServer(t, nil)
	c := connect(t, s, aliceToken)
	c.send(joinCode("123"))
	c.closedWith("BAD_ROOM_CODE")
	c = connect(t, s, aliceToken)
	c.send(joinCode("54321"))
	c.closedWith("ROOM_NOT_FOUND")
	// A room in a round.
	a, _, code := roomOfTwo(t, s)
	a.send(lockMembers())
	a.phase(pb.Phase_SONG_SELECT)
	c = connect(t, s, carolToken)
	c.send(joinCode(code))
	c.closedWith("ROOM_BUSY")
}

func TestWholeRoundBetweenTwoPlayers(t *testing.T) {
	s := startServer(t, nil)
	a := connect(t, s, aliceToken)
	a.send(joinPrivate())
	// Welcome first, then the room.
	first := a.next("the first message", func(*pb.ServerMessage) bool { return true })
	if w := first.GetWelcome(); w == nil || w.UserId != alice.UserID || w.Nickname != "Alice" || w.Protocol != 1 {
		t.Fatalf("the first message: %v", first)
	}
	lobby := a.phase(pb.Phase_LOBBY)
	if !regexp.MustCompile(`^\d{5}$`).MatchString(lobby.Code) || lobby.Public || lobby.HostId != alice.UserID || lobby.MaxMembers != 5 || lobby.MinPlayers != 2 {
		t.Fatalf("lobby: %v", lobby)
	}
	if me := lobby.Members[0]; me.AvatarUrl != alice.AvatarURL || !me.Connected {
		t.Fatalf("member: %v", me)
	}

	b := connect(t, s, bobToken)
	b.send(joinCode(lobby.Code))
	b.room("the room with two members", func(r *pb.Room) bool { return len(r.Members) == 2 })

	// What the room refuses comes back as a notice; the stream stays open.
	b.send(lockMembers())
	b.notice("NOT_HOST")
	b.send(joinQuick())
	b.notice("BAD_REQUEST")
	b.send(&pb.ClientMessage{})
	b.notice("BAD_REQUEST")
	b.send(ping(4242))
	if pong := b.next("pong", func(m *pb.ServerMessage) bool { return m.GetPong() != nil }).GetPong(); pong.ClientTimeMs != 4242 || pong.ServerTimeMs < time.Now().Add(-time.Minute).UnixMilli() {
		t.Fatalf("pong: %v", pong)
	}

	a.send(lockMembers())
	if r := a.phase(pb.Phase_SONG_SELECT); r.Round != 1 || r.DeadlineMs <= r.ServerTimeMs {
		t.Fatalf("song select: %v", r)
	}
	b.send(pickSong(&Song{Source: "fanmade", ChartID: strings.Repeat("f", 32)}, "Oni"))
	b.notice("CHART_NOT_FOUND")
	// Alice picks Alpha on Normal, Bob leaves it to chance on Hard: either way the draw has
	// both courses, as Beta, what chance draws here, has neither.
	a.send(pickSong(songA, "Normal"))
	b.send(pickSong(nil, "Hard"))
	draw := b.phase(pb.Phase_SONG_DRAW)
	if draw.Song == nil || len(draw.Song.ChartJson) == 0 {
		t.Fatalf("draw: %v", draw)
	}
	switch draw.Song.ChartId {
	case songA.ChartID:
		if draw.SongPickedBy != alice.UserID || draw.Members[0].Course != "Normal" || draw.Members[1].Course != "Hard" {
			t.Fatalf("draw of Alpha: %v", draw)
		}
	case songB.ChartID:
		// Beta has Oni and Edit: nothing easier than Normal or Hard, so the nearest harder.
		if draw.SongPickedBy != "" || draw.Members[0].Course != "Oni" || draw.Members[1].Course != "Oni" {
			t.Fatalf("draw of Beta: %v", draw)
		}
	default:
		t.Fatalf("drew %v", draw.Song)
	}
	if draw.Members[0].PickedChartId != songA.ChartID || draw.Members[1].PickedChartId != "" {
		t.Fatalf("picks: %v", draw.Members)
	}

	b.phase(pb.Phase_COURSE)
	b.send(pickCourse("Oni"))
	b.send(pickCourse("Expert"))
	b.notice("BAD_REQUEST")
	a.send(setReady())
	b.send(setReady())
	loading := a.phase(pb.Phase_LOADING)
	if loading.Members[1].Course != "Oni" || !loading.Members[0].Playing || !loading.Members[1].Playing {
		t.Fatalf("loading: %v", loading)
	}

	a.send(loaded())
	b.send(loaded())
	if start := a.next("start_play", func(m *pb.ServerMessage) bool { return m.GetStartPlay() != nil }).GetStartPlay(); start.Round != 1 {
		t.Fatalf("start_play: %v", start)
	}
	b.next("start_play", func(m *pb.ServerMessage) bool { return m.GetStartPlay() != nil })
	a.phase(pb.Phase_PLAYING)

	a.send(playStatus(&pb.PlayerStatus{Score: 12345, Combo: 50, MaxCombo: 50, Good: 40, Ok: 10, Gauge: 20_000, MusicTimeMs: 9000}))
	statuses := b.next("Alice's status", func(m *pb.ServerMessage) bool {
		return len(m.GetStatuses().GetMembers()) == 2 && m.GetStatuses().Members[0].Status.Score == 12345
	}).GetStatuses()
	if got := statuses.Members[0]; got.UserId != alice.UserID || got.Status.Gauge != 10_000 || got.Status.Combo != 50 || !got.Connected || got.Finished {
		t.Fatalf("status: %v", got)
	}
	// No stamps from a player in the play.
	a.send(stamp(1))

	a.send(finish(&pb.PlayerStatus{Score: 950_000, MaxCombo: 700, Good: 600, Ok: 100, Gauge: 9000}, true))
	b.send(finish(&pb.PlayerStatus{Score: 700_000, MaxCombo: 200, Good: 400, Ok: 30, Bad: 4, Gauge: 4000}, false))
	result := b.phase(pb.Phase_RESULT).Result
	if result.Round != 1 || result.TeamScore != 1_650_000 || result.ChartId != draw.Song.ChartId || result.Title != draw.Song.Title || len(result.Players) != 2 {
		t.Fatalf("result: %v", result)
	}
	p1, p2 := result.Players[0], result.Players[1]
	if p1.Nickname != "Alice" || p1.Rank != 1 || p1.Award != pb.Award_SUPER_STAR || !p1.Cleared || !p1.FullCombo || p1.AllGood || p1.Slot != 0 {
		t.Fatalf("first: %v", p1)
	}
	if p2.Nickname != "Bob" || p2.Rank != 2 || p2.Award != pb.Award_NO_AWARD || p2.Cleared || p2.FullCombo || p2.Course != "Oni" || p2.Level == 0 || p2.Slot != 1 {
		t.Fatalf("second: %v", p2)
	}

	// Stamps go to everyone, the sender too.
	b.send(stamp(2))
	for _, c := range []*client{a, b} {
		if got := c.next("the stamp", func(m *pb.ServerMessage) bool { return m.GetStamp() != nil }).GetStamp(); got.StampId != 2 || got.UserId != bob.UserID {
			t.Fatalf("stamp: %v", got)
		}
	}
	back := a.room("the lobby after the round", func(r *pb.Room) bool { return r.Phase == pb.Phase_LOBBY && r.Round == 1 })
	if back.Result.GetRound() != 1 || len(back.Members) != 2 {
		t.Fatalf("back in the lobby: %v", back)
	}

	b.send(leave())
	b.closedWith("LEFT")
	a.room("the room without Bob", func(r *pb.Room) bool { return len(r.Members) == 1 })
	if s.Manager().RoomCount() != 1 {
		t.Fatalf("%d rooms", s.Manager().RoomCount())
	}
	a.send(leave())
	a.closedWith("LEFT")
	if s.Manager().RoomCount() != 0 {
		t.Fatalf("%d rooms after everyone left", s.Manager().RoomCount())
	}
}

func TestQuickMatchAndKickOverTheWire(t *testing.T) {
	s := startServer(t, nil)
	a := connect(t, s, aliceToken)
	a.send(joinQuick())
	lobby := a.phase(pb.Phase_LOBBY)
	if !lobby.Public {
		t.Fatalf("lobby: %v", lobby)
	}
	b := connect(t, s, bobToken)
	b.send(joinQuick())
	with := b.room("the room with two members", func(r *pb.Room) bool { return len(r.Members) == 2 })
	if with.Code != lobby.Code || with.DeadlineMs == 0 {
		t.Fatalf("quick match: %v", with)
	}
	b.send(kick(alice.UserID))
	b.notice("NOT_HOST")
	a.send(kick(bob.UserID))
	b.closedWith("KICKED")
	a.room("the room without Bob", func(r *pb.Room) bool { return len(r.Members) == 1 && r.DeadlineMs == 0 })
}

func TestTooManyMessages(t *testing.T) {
	s := startServer(t, func(cfg *ServerConfig) { cfg.MessageRate = 20 })
	a, b, _ := roomOfTwo(t, s)
	// A burst within the allowance is fine.
	for i := 0; i < 10; i++ {
		b.send(ping(int64(i)))
	}
	b.next("the last pong", func(m *pb.ServerMessage) bool { return m.GetPong().GetClientTimeMs() == 9 })
	for i := 0; i < 200; i++ {
		// The stream may be cut while these are being sent.
		if b.stream.Send(ping(0)) != nil {
			break
		}
	}
	b.refusedWith(codes.ResourceExhausted, "TOO_MANY_MESSAGES")
	// To the room the player dropped: the place is kept for a while.
	a.room("Bob shown as disconnected", func(r *pb.Room) bool { return len(r.Members) == 2 && !r.Members[1].Connected })
}

func TestMessageTooLargeEndsTheStream(t *testing.T) {
	s := startServer(t, nil)
	c := connect(t, s, aliceToken)
	c.send(joinPrivate())
	c.phase(pb.Phase_LOBBY)
	c.send(&pb.ClientMessage{Kind: &pb.ClientMessage_Loaded{Loaded: &pb.Loaded{Error: strings.Repeat("x", 70*1024)}}})
	c.refusedWith(codes.ResourceExhausted, "larger than max")
}

func TestSecondStreamOfAnAccountReplacesTheFirst(t *testing.T) {
	s := startServer(t, nil)
	a, b, code := roomOfTwo(t, s)
	again := connect(t, s, bobToken)
	again.send(joinCode(code))
	b.closedWith("REPLACED")
	if r := again.phase(pb.Phase_LOBBY); len(r.Members) != 2 || !r.Members[1].Connected {
		t.Fatalf("the room on the second stream: %v", r)
	}
	// The first stream's end does not drop the player.
	again.send(stamp(7))
	a.next("Bob's stamp", func(m *pb.ServerMessage) bool { return m.GetStamp().GetUserId() == bob.UserID })
	time.Sleep(100 * time.Millisecond)
	a.send(ping(1))
	a.next("pong", func(m *pb.ServerMessage) bool { return m.GetPong() != nil })
	if room := s.Manager().Room(code); room == nil || !room.Has(bob.UserID) {
		t.Fatal("Bob lost his place")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, m := range a.inbox {
		if r := m.GetRoom(); r != nil && len(r.Members) == 2 && !r.Members[1].Connected {
			t.Fatalf("Bob was shown as disconnected: %v", r)
		}
	}
}

func TestDroppedPlayerComesBackByTheRoomNumber(t *testing.T) {
	s := startServer(t, nil)
	a, b, code := roomOfTwo(t, s)
	// Bob's connection is lost.
	b.cancel()
	a.room("Bob shown as disconnected", func(r *pb.Room) bool { return len(r.Members) == 2 && !r.Members[1].Connected })
	back := connect(t, s, bobToken)
	back.send(joinCode(code))
	if w := back.next("welcome", func(m *pb.ServerMessage) bool { return m.GetWelcome() != nil }).GetWelcome(); w.UserId != bob.UserID {
		t.Fatalf("welcome: %v", w)
	}
	if r := back.phase(pb.Phase_LOBBY); len(r.Members) != 2 || r.Members[1].Slot != 1 || !r.Members[1].Connected || r.HostId != alice.UserID {
		t.Fatalf("the room on coming back: %v", r)
	}
	a.room("Bob shown as connected", func(r *pb.Room) bool { return len(r.Members) == 2 && r.Members[1].Connected })
}

func TestDroppedPlayerLosesThePlaceAfterTheGracePeriod(t *testing.T) {
	s := startServer(t, func(cfg *ServerConfig) { cfg.Timings.ReconnectGrace = 100 * time.Millisecond })
	a, b, _ := roomOfTwo(t, s)
	// Ending the stream without `leave` is a drop too.
	if err := b.stream.CloseSend(); err != nil {
		t.Fatal(err)
	}
	if err := b.finished(); err != nil {
		t.Fatalf("the stream ended with %v", err)
	}
	a.room("the room without Bob", func(r *pb.Room) bool { return len(r.Members) == 1 })
}

func TestStoppingTheServerClosesTheRooms(t *testing.T) {
	s := startServer(t, nil)
	a, b, _ := roomOfTwo(t, s)
	waiting := connect(t, s, carolToken)
	// Carol's stream is open (the server has answered for her session) but in no room.
	time.Sleep(100 * time.Millisecond)
	stopped := make(chan struct{})
	go func() {
		s.Stop()
		close(stopped)
	}()
	a.closedWith("ROOM_CLOSED")
	b.closedWith("ROOM_CLOSED")
	waiting.refusedWith(codes.Unavailable, "SERVER_CLOSING")
	select {
	case <-stopped:
	case <-time.After(stopTimeout - time.Second):
		t.Fatal("Stop waited though every stream had ended")
	}
	if s.Manager().RoomCount() != 0 {
		t.Fatal("rooms are left")
	}
}

// A player who is no longer in the room (removed while a request was on its way) is told so
// and the stream ended, rather than answered with notices for ever.
func TestRequestFromOutsideTheRoomEndsTheStream(t *testing.T) {
	s := NewServer(ServerConfig{Catalog: staticCatalog{}, Authenticate: testAccounts, Logf: t.Logf})
	defer s.Stop()
	host := &fakeConn{}
	room, err := s.Manager().Join(alice, private, host)
	if err != nil {
		t.Fatal(err)
	}
	c := &streamConn{out: make(chan *pb.ServerMessage, sendQueue), overflow: make(chan struct{}), player: bob}
	if c.room, err = s.Manager().Join(bob, byCode(room.Code()), c); err != nil {
		t.Fatal(err)
	}
	// Bob is taken out behind the stream's back.
	s.Manager().mu.Lock()
	room.removeMember(room.find(bob.UserID))
	s.Manager().unlock()
	s.handle(c, lockMembers())
	var last *pb.ServerMessage
	for m := range c.out {
		last = m
	}
	if last.GetClosed().GetCode() != "NOT_IN_ROOM" {
		t.Fatalf("the last message: %v", last)
	}
	if ended, closing, slow := c.state(); !ended || !closing || slow {
		t.Fatalf("ended %v, closing %v, slow %v", ended, closing, slow)
	}
}

// A player who does not read is dropped once its queue is full; the room is never held up.
func TestSlowConsumerIsDropped(t *testing.T) {
	c := &streamConn{out: make(chan *pb.ServerMessage, sendQueue), overflow: make(chan struct{})}
	m := &pb.ServerMessage{Kind: &pb.ServerMessage_Pong{Pong: &pb.Pong{}}}
	for i := 0; i < sendQueue; i++ {
		c.Send(m)
	}
	if ended, _, _ := c.state(); ended {
		t.Fatal("dropped with the queue just full")
	}
	c.Send(m)
	select {
	case <-c.overflow:
	default:
		t.Fatal("not dropped with the queue overflowing")
	}
	if ended, closing, slow := c.state(); !ended || closing || !slow {
		t.Fatalf("ended %v, closing %v, slow %v", ended, closing, slow)
	}
	// Nothing more is queued, and nothing blocks or panics.
	c.Send(m)
	c.Close("LEFT", "Left the room")
	c.abort()
	n := 0
	for range c.out {
		n++
	}
	if n != sendQueue {
		t.Fatalf("%d messages queued", n)
	}
	// A full queue at Close is the same.
	c = &streamConn{out: make(chan *pb.ServerMessage, 1), overflow: make(chan struct{})}
	c.Send(m)
	c.Close("ROOM_CLOSED", "The room was closed")
	if _, closing, slow := c.state(); closing || !slow {
		t.Fatalf("closing %v, slow %v", closing, slow)
	}
}

func TestBearer(t *testing.T) {
	token := strings.Repeat("0a", 32)
	for value, want := range map[string]string{
		"Bearer " + token:                  token,
		"bearer " + token:                  token,
		"  Bearer   " + token + " ":        token,
		"Bearer " + strings.ToUpper(token): strings.ToUpper(token),
		"Bearer " + token[:63]:             "",
		"Bearer " + token + "0":            "",
		"Bearer " + token[:63] + "g":       "",
		"Basic " + token:                   "",
		token:                              "",
		"":                                 "",
	} {
		ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", value))
		if got := bearer(ctx); got != want {
			t.Errorf("%q: got %q, want %q", value, got, want)
		}
	}
	if got := bearer(context.Background()); got != "" {
		t.Errorf("no metadata: got %q", got)
	}
}

func TestMessageRate(t *testing.T) {
	start := time.Unix(1_800_000_000, 0)
	b := bucket{tokens: 40, rate: 40, last: start}
	for i := 0; i < 40; i++ {
		if !b.take(start) {
			t.Fatalf("message %d of a burst of 40 was refused", i+1)
		}
	}
	if b.take(start) {
		t.Fatal("the 41st message at once was let through")
	}
	// It fills again at 40 a second, up to 40.
	b = bucket{tokens: 0, rate: 40, last: start}
	if !b.take(start.Add(25 * time.Millisecond)) {
		t.Fatal("a message 25 ms later was refused")
	}
	if b.take(start.Add(26 * time.Millisecond)) {
		t.Fatal("two messages in 26 ms were let through")
	}
	b = bucket{tokens: 0, rate: 40, last: start}
	n := 0
	for b.take(start.Add(time.Hour)) {
		n++
	}
	if n != 40 {
		t.Fatalf("after an hour of silence %d messages at once", n)
	}
}
