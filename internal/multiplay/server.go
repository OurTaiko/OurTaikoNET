package multiplay

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	pb "ourtaiko.dev/net/internal/multiplay/multiplaypb"
)

// The gRPC service ourtaiko.multiplay.v1.MultiEnso: one bidirectional stream per player. It
// runs inside the OurTaikoNet process on a port of its own, in clear text (h2c): the reverse
// proxy in front of it ends TLS.

// Authenticator says whose game session (64 hex digits) a token is. ErrUnauthenticated: the
// account center refuses it. Any other error: the account center could not be asked.
type Authenticator func(ctx context.Context, token string) (Player, error)

// ErrUnauthenticated is an Authenticator's answer for a session that is not valid.
var ErrUnauthenticated = errors.New("game session refused")

const (
	// A stream that has not sent `join` after this long is ended.
	defaultJoinTimeout = 15 * time.Second
	// Messages per second a client may send on average, and how many at once; statuses come
	// about 5 times a second.
	defaultMessageRate = 40
	// The largest message a client may send.
	maxMessageBytes = 64 * 1024
	// Messages waiting to be written to one player. A player this far behind is dropped
	// rather than held in memory: during a play that is about a minute of messages.
	sendQueue = 256
	// How long the account center may take to answer for a session.
	authTimeout = 10 * time.Second
	// How long Stop waits for the streams to end by themselves.
	stopTimeout = 5 * time.Second
)

// ServerConfig is what a Server is made of. Catalog and Authenticate must be given.
type ServerConfig struct {
	Catalog      Catalog
	Authenticate Authenticator
	// Timings of every room; zero is DefaultTimings.
	Timings Timings
	// Logf is where the server logs; nil is log.Printf.
	Logf func(format string, args ...any)
	// JoinTimeout and MessageRate replace the defaults; tests shorten them.
	JoinTimeout time.Duration
	MessageRate float64
}

// Server is the Multi-Enso service and its rooms.
type Server struct {
	pb.UnimplementedMultiEnsoServer
	manager      *Manager
	authenticate Authenticator
	logf         func(format string, args ...any)
	joinTimeout  time.Duration
	messageRate  float64
	grpc         *grpc.Server
	listener     net.Listener
	// quit is closed when the server stops: streams that are in no room yet end.
	quit     chan struct{}
	stopOnce sync.Once
}

// NewServer makes the service; Start or Serve makes it listen.
func NewServer(cfg ServerConfig) *Server {
	manager := NewManager(Deps{Catalog: cfg.Catalog, Timings: cfg.Timings, Logf: cfg.Logf})
	s := &Server{manager: manager, authenticate: cfg.Authenticate, logf: manager.deps.Logf, joinTimeout: cfg.JoinTimeout, messageRate: cfg.MessageRate, quit: make(chan struct{})}
	if s.joinTimeout <= 0 {
		s.joinTimeout = defaultJoinTimeout
	}
	if s.messageRate <= 0 {
		s.messageRate = defaultMessageRate
	}
	s.grpc = grpc.NewServer(
		// Streams stay open for a whole session; pings keep NAT and proxies from dropping
		// idle ones, and find the ones that are gone.
		grpc.KeepaliveParams(keepalive.ServerParameters{Time: 30 * time.Second, Timeout: 10 * time.Second}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{MinTime: 10 * time.Second, PermitWithoutStream: true}),
		grpc.MaxRecvMsgSize(maxMessageBytes),
	)
	pb.RegisterMultiEnsoServer(s.grpc, s)
	return s
}

// Manager is the server's rooms.
func (s *Server) Manager() *Manager { return s.manager }

// Start listens on addr (host:port) and serves in the background.
func (s *Server) Start(addr string) error {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	s.listener = l
	go func() {
		if err := s.grpc.Serve(l); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			s.logf("multiplay: gRPC server stopped: %v", err)
		}
	}()
	return nil
}

// Addr is the address Start listens on; nil before it.
func (s *Server) Addr() net.Addr {
	if s.listener == nil {
		return nil
	}
	return s.listener.Addr()
}

// Stop closes every room (each player is told ROOM_CLOSED and its stream ended) and stops
// the server. Streams that do not end by themselves in a few seconds are cut.
func (s *Server) Stop() {
	s.stopOnce.Do(func() {
		close(s.quit)
		s.manager.Close()
		done := make(chan struct{})
		go func() {
			s.grpc.GracefulStop()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(stopTimeout):
			s.grpc.Stop()
		}
	})
}

// bearer is the game session of the stream's `authorization: Bearer <64 hex>` metadata, or
// empty.
func bearer(ctx context.Context) string {
	md, _ := metadata.FromIncomingContext(ctx)
	values := md.Get("authorization")
	if len(values) == 0 {
		return ""
	}
	scheme, token, ok := strings.Cut(strings.TrimSpace(values[0]), " ")
	token = strings.TrimSpace(token)
	if !ok || !strings.EqualFold(scheme, "Bearer") || len(token) != 64 {
		return ""
	}
	for _, c := range token {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return ""
		}
	}
	return token
}

// streamConn is one player's stream as its room sees it: a queue the stream's writer empties.
type streamConn struct {
	mu  sync.Mutex
	out chan *pb.ServerMessage
	// ended: nothing more is queued. closing: it ended with `closed`, which the writer still
	// has to send. slow: it ended because the queue was full.
	ended, closing, slow bool
	// overflow is closed when the queue was full.
	overflow chan struct{}

	// The reader's own: who the stream belongs to and, once joined, the room.
	player Player
	room   *Room
}

func (c *streamConn) Send(m *pb.ServerMessage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ended {
		return
	}
	select {
	case c.out <- m:
	default:
		c.drop()
	}
}

func (c *streamConn) Close(code, message string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ended {
		return
	}
	select {
	case c.out <- &pb.ServerMessage{Kind: &pb.ServerMessage_Closed{Closed: &pb.Closed{Code: code, Message: message}}}:
		c.ended, c.closing = true, true
		close(c.out)
	default:
		c.drop()
	}
}

// drop gives up on a player who does not read: c.mu must be held.
func (c *streamConn) drop() {
	c.ended, c.slow = true, true
	close(c.out)
	close(c.overflow)
}

// abort ends the queue of a stream that is over.
func (c *streamConn) abort() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.ended {
		c.ended = true
		close(c.out)
	}
}

func (c *streamConn) state() (ended, closing, slow bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ended, c.closing, c.slow
}

// bucket is a token bucket: rate messages a second on average, rate at once.
type bucket struct {
	tokens, rate float64
	last         time.Time
}

func (b *bucket) take(now time.Time) bool {
	b.tokens = min(b.rate, b.tokens+now.Sub(b.last).Seconds()*b.rate)
	b.last = now
	b.tokens--
	return b.tokens >= 0
}

var errSlow = status.Error(codes.ResourceExhausted, "SLOW_CONSUMER")

// Connect is one player's stream. Three goroutines share it: this one waits for the stream
// to be over; the reader handles the player's messages one at a time, in order, even when
// one waits for the database, and is the only one that asks anything of the room; the writer
// sends what the room queued.
func (s *Server) Connect(stream pb.MultiEnso_ConnectServer) error {
	ctx := stream.Context()
	token := bearer(ctx)
	if token == "" {
		return status.Error(codes.Unauthenticated, "AUTH_REQUIRED")
	}
	authCtx, cancel := context.WithTimeout(ctx, authTimeout)
	player, err := s.authenticate(authCtx, token)
	cancel()
	if errors.Is(err, ErrUnauthenticated) || err == nil && player.UserID == "" {
		return status.Error(codes.Unauthenticated, "AUTH_REQUIRED")
	}
	if err != nil {
		if ctx.Err() == nil {
			s.logf("multiplay: the account center could not be asked: %v", err)
		}
		return status.Error(codes.Unavailable, "AUTH_UNAVAILABLE")
	}

	c := &streamConn{out: make(chan *pb.ServerMessage, sendQueue), overflow: make(chan struct{}), player: player}
	// over: this function has returned or is about to; the writer sends nothing more.
	var over atomic.Bool
	defer func() {
		over.Store(true)
		c.abort()
	}()

	writerDone := make(chan error, 1)
	go func() {
		for m := range c.out {
			if over.Load() {
				break
			}
			if err := stream.Send(m); err != nil {
				writerDone <- err
				return
			}
		}
		writerDone <- nil
	}()

	joined := make(chan struct{})
	readerDone := make(chan error, 1)
	go func() {
		err := s.read(stream, c, joined)
		// The stream is over for this player, however it ended: the room keeps the place for
		// a while. (After `leave` or a replaced connection this finds nothing to do.)
		if c.room != nil {
			c.room.Disconnected(c.player.UserID, c)
		}
		readerDone <- err
	}()

	joinTimer := time.NewTimer(s.joinTimeout)
	defer joinTimer.Stop()
	quit := s.quit
	for {
		select {
		case err := <-writerDone:
			// Everything queued was sent, `closed` last; or the player is gone.
			if _, _, slow := c.state(); err == nil && slow {
				return errSlow
			}
			return err
		case <-c.overflow:
			return errSlow
		case err := <-readerDone:
			// The reader ended the stream with a status, or the player ended it. What the
			// room said last (`closed`) is still sent when the player is there to read it.
			if _, closing, _ := c.state(); err == nil && closing {
				select {
				case <-writerDone:
				case <-time.After(stopTimeout):
				}
			}
			return err
		case <-joinTimer.C:
			select {
			case <-joined:
			default:
				if ended, _, _ := c.state(); !ended {
					return status.Error(codes.DeadlineExceeded, "join was not sent")
				}
			}
		case <-quit:
			// A player in a room is told by the room, which is closing. One in no room yet
			// is not waited for.
			select {
			case <-joined:
				quit = nil
			default:
				if ended, _, _ := c.state(); !ended {
					return status.Error(codes.Unavailable, "SERVER_CLOSING")
				}
				quit = nil
			}
		}
	}
}

// read handles the player's messages until the stream ends. A non-nil error is the status
// the stream ends with.
func (s *Server) read(stream pb.MultiEnso_ConnectServer, c *streamConn, joined chan struct{}) error {
	limit := bucket{tokens: s.messageRate, rate: s.messageRate, last: time.Now()}
	for {
		message, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if ended, _, _ := c.state(); ended {
			continue
		}
		if !limit.take(time.Now()) {
			return status.Error(codes.ResourceExhausted, "TOO_MANY_MESSAGES")
		}
		if c.room == nil {
			if err := s.join(c, message.GetJoin()); err != nil {
				return err
			}
			if c.room != nil {
				close(joined)
			}
			continue
		}
		s.handle(c, message)
	}
}

// join handles the first message of a stream. A request the rooms refuse ends the stream
// with `closed`; one that is not a join at all, with an error.
func (s *Server) join(c *streamConn, join *pb.Join) error {
	var target JoinTarget
	switch t := join.GetTarget().(type) {
	case *pb.Join_QuickMatch:
		target.Kind = QuickMatch
	case *pb.Join_CreatePrivate:
		target.Kind = CreatePrivate
	case *pb.Join_RoomCode:
		target.Kind, target.Code = ByCode, t.RoomCode
	default:
		return status.Error(codes.FailedPrecondition, "the first message must be join")
	}
	if join.GetProtocol() > Protocol {
		c.Close("UPDATE_REQUIRED", "The server does not speak this version of the protocol yet")
		return nil
	}
	room, err := s.manager.Join(c.player, target, c)
	var refused *Error
	switch {
	case errors.As(err, &refused):
		c.Close(refused.Code, refused.Message)
	case err != nil:
		s.logf("multiplay: join failed: %v", err)
		return status.Error(codes.Internal, "SERVER_ERROR")
	default:
		c.room = room
	}
	return nil
}

// handle is one request of a player who is in a room. What the room refuses comes back as a
// notice; the stream stays open.
func (s *Server) handle(c *streamConn, message *pb.ClientMessage) {
	room, user := c.room, c.player.UserID
	var err error
	switch m := message.GetKind().(type) {
	case *pb.ClientMessage_Ping:
		c.Send(&pb.ServerMessage{Kind: &pb.ServerMessage_Pong{Pong: &pb.Pong{ClientTimeMs: m.Ping.GetClientTimeMs(), ServerTimeMs: s.manager.deps.Now().UnixMilli()}}})
	case *pb.ClientMessage_Leave:
		room.Leave(user)
		c.Close("LEFT", "Left the room")
	case *pb.ClientMessage_Join:
		err = refuse("BAD_REQUEST", "Already in a room")
	case *pb.ClientMessage_LockMembers:
		err = room.LockMembers(user)
	case *pb.ClientMessage_OpenRoom:
		err = room.OpenRoom(user)
	case *pb.ClientMessage_Kick:
		err = room.Kick(user, m.Kick.GetUserId())
	case *pb.ClientMessage_PickSong:
		err = room.PickSong(user, m.PickSong.GetSource(), m.PickSong.GetChartId(), m.PickSong.GetCourse())
	case *pb.ClientMessage_PickCourse:
		err = room.PickCourse(user, m.PickCourse.GetCourse())
	case *pb.ClientMessage_SetReady:
		err = room.SetReady(user, m.SetReady.GetReady())
	case *pb.ClientMessage_Loaded:
		if !m.Loaded.GetOk() {
			reason := m.Loaded.GetError()
			if len(reason) > 200 {
				reason = reason[:200]
			}
			s.logf("multiplay: room %s: player %s could not load the song: %q", room.Code(), user, reason)
		}
		err = room.Loaded(user, m.Loaded.GetOk())
	case *pb.ClientMessage_Status:
		err = room.Status(user, m.Status)
	case *pb.ClientMessage_Finish:
		err = room.Finish(user, m.Finish.GetFinal(), m.Finish.GetCleared())
	case *pb.ClientMessage_Stamp:
		err = room.Stamp(user, m.Stamp.GetStampId())
	default:
		err = refuse("BAD_REQUEST", "Unknown request")
	}
	if err == nil {
		return
	}
	var refused *Error
	if !errors.As(err, &refused) {
		s.logf("multiplay: request failed: %v", err)
		refused = refuse("SERVER_ERROR", "The server could not do that")
	}
	if refused.Code == errNotInRoom.Code {
		// Kicked, or the room closed, while the request was on its way.
		c.Close(refused.Code, "You are no longer in the room")
		return
	}
	c.Send(&pb.ServerMessage{Kind: &pb.ServerMessage_Notice{Notice: &pb.Notice{Code: refused.Code, Message: refused.Message}}})
}
