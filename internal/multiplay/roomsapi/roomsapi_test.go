package roomsapi

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"ourtaiko.dev/net/internal/multiplay"
	pb "ourtaiko.dev/net/internal/multiplay/multiplaypb"
)

var (
	token = strings.Repeat("1", 64)
	alice = multiplay.Player{UserID: strings.Repeat("a", 32), Nickname: "Alice", AvatarURL: "https://avatars.test/a.webp"}
	song  = &multiplay.Song{Source: "fanmade", ChartID: strings.Repeat("c", 32), Title: "Song", Subtitle: "Sub", Courses: []multiplay.Course{{Name: "Oni", Level: 9}}, ChartJSON: []byte(`{"id":"c"}`)}
)

type catalog struct{ none bool }

func (c catalog) Get(ctx context.Context, source, chartID string) (*multiplay.Song, error) {
	if source == song.Source && chartID == song.ChartID {
		return song, nil
	}
	return nil, multiplay.ErrChartNotFound
}

func (c catalog) Random(ctx context.Context) (*multiplay.Song, error) {
	if c.none {
		return nil, nil
	}
	return song, nil
}

func identify(ctx context.Context, asked string) (multiplay.Player, error) {
	if asked == token {
		return alice, nil
	}
	return multiplay.Player{}, multiplay.ErrUnauthenticated
}

func site(t *testing.T, c catalog) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(Handler(c, identify))
	t.Cleanup(s.Close)
	return s
}

func quickly(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
	t.Cleanup(cancel)
	return ctx
}

func TestTheSiteAnswersTheRooms(t *testing.T) {
	c := NewClient(site(t, catalog{}).URL)
	ctx := quickly(t)
	if p, err := c.Authenticate(ctx, token); err != nil || p != alice {
		t.Fatalf("identity: %+v, %v", p, err)
	}
	if _, err := c.Authenticate(ctx, strings.Repeat("9", 64)); !errors.Is(err, multiplay.ErrUnauthenticated) {
		t.Fatalf("a session the site refuses: %v", err)
	}
	got, err := c.Get(ctx, song.Source, song.ChartID)
	if err != nil || got.Title != song.Title || got.Subtitle != song.Subtitle || string(got.ChartJSON) != string(song.ChartJSON) || len(got.Courses) != 1 || got.Courses[0] != song.Courses[0] {
		t.Fatalf("chart: %+v, %v", got, err)
	}
	if _, err = c.Get(ctx, "fanmade", strings.Repeat("0", 32)); !errors.Is(err, multiplay.ErrChartNotFound) {
		t.Fatalf("a chart that does not exist: %v", err)
	}
	if got, err = c.Random(ctx); err != nil || got.ChartID != song.ChartID {
		t.Fatalf("random: %+v, %v", got, err)
	}
	if got, err = NewClient(site(t, catalog{none: true}).URL).Random(ctx); err != nil || got != nil {
		t.Fatalf("random on a site without charts: %+v, %v", got, err)
	}
}

// What the site last said stands while it is away; nothing is made up for what it never said.
func TestRoomsGoByWhatTheSiteLastSaidWhileItIsAway(t *testing.T) {
	s := site(t, catalog{})
	c := NewClient(s.URL)
	c.logf = t.Logf
	ctx := quickly(t)
	if _, err := c.Authenticate(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(ctx, song.Source, song.ChartID); err != nil {
		t.Fatal(err)
	}
	s.Close()

	if p, err := c.Authenticate(quickly(t), token); err != nil || p != alice {
		t.Fatalf("a known session: %+v, %v", p, err)
	}
	if got, err := c.Get(quickly(t), song.Source, song.ChartID); err != nil || got.ChartID != song.ChartID {
		t.Fatalf("a known chart: %+v, %v", got, err)
	}
	if got, err := c.Random(quickly(t)); err != nil || got.ChartID != song.ChartID {
		t.Fatalf("a draw: %+v, %v", got, err)
	}
	if _, err := c.Authenticate(quickly(t), strings.Repeat("2", 64)); err == nil || errors.Is(err, multiplay.ErrUnauthenticated) {
		t.Fatalf("an unknown session must be an outage, not a refusal: %v", err)
	}
	if _, err := c.Get(quickly(t), "fanmade", strings.Repeat("0", 32)); err == nil || errors.Is(err, multiplay.ErrChartNotFound) {
		t.Fatalf("an unknown chart must be an outage, not a refusal: %v", err)
	}
}

// A session the site has since revoked is not let in once the site can say so.
func TestARevokedSessionIsForgotten(t *testing.T) {
	var revoked atomic.Bool
	s := httptest.NewServer(Handler(catalog{}, func(ctx context.Context, asked string) (multiplay.Player, error) {
		if revoked.Load() {
			return multiplay.Player{}, multiplay.ErrUnauthenticated
		}
		return alice, nil
	}))
	c := NewClient(s.URL)
	if _, err := c.Authenticate(quickly(t), token); err != nil {
		t.Fatal(err)
	}
	revoked.Store(true)
	if _, err := c.Authenticate(quickly(t), token); !errors.Is(err, multiplay.ErrUnauthenticated) {
		t.Fatalf("revoked: %v", err)
	}
	s.Close()
	if _, err := c.Authenticate(quickly(t), token); err == nil {
		t.Fatal("a revoked session came back while the site was away")
	}
}

// A question asked while the site restarts is answered once it is back.
func TestAQuestionWaitsForTheSite(t *testing.T) {
	var fails atomic.Int32
	fails.Store(2)
	h := Handler(catalog{}, identify)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fails.Add(-1) >= 0 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		h.ServeHTTP(w, r)
	}))
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := NewClient(s.URL).Get(ctx, song.Source, song.ChartID); err != nil {
		t.Fatal(err)
	}
}

// The point of it all: the site goes away and comes back while a player is in a room, the
// stream stays up, and the player can drop and come back meanwhile.
func TestARoomLivesThroughARestartOfTheSite(t *testing.T) {
	s := site(t, catalog{})
	client := NewClient(s.URL)
	client.logf = t.Logf
	rooms := multiplay.NewServer(multiplay.ServerConfig{Catalog: client, Authenticate: client.Authenticate, Logf: t.Logf})
	if err := rooms.Start("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rooms.Stop)
	conn, err := grpc.NewClient(rooms.Addr().(*net.TCPAddr).String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })

	ctx, cancel := context.WithTimeout(metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+token), 15*time.Second)
	defer cancel()
	connect := func(ctx context.Context, join *pb.Join) (pb.MultiEnso_ConnectClient, *pb.Room) {
		t.Helper()
		stream, err := pb.NewMultiEnsoClient(conn).Connect(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err = stream.Send(&pb.ClientMessage{Kind: &pb.ClientMessage_Join{Join: join}}); err != nil {
			t.Fatal(err)
		}
		for {
			m, err := stream.Recv()
			if err != nil {
				t.Fatal(err)
			}
			if m.GetClosed() != nil {
				t.Fatalf("closed: %v", m.GetClosed())
			}
			if m.GetRoom() != nil {
				return stream, m.GetRoom()
			}
		}
	}
	first, cancelFirst := context.WithCancel(ctx)
	defer cancelFirst()
	stream, room := connect(first, &pb.Join{Target: &pb.Join_CreatePrivate{CreatePrivate: &pb.CreatePrivate{}}, Protocol: multiplay.Protocol})

	s.Close()

	// Still in the room with the site gone: the stream answers.
	if err = stream.Send(&pb.ClientMessage{Kind: &pb.ClientMessage_Ping{Ping: &pb.Ping{ClientTimeMs: 1}}}); err != nil {
		t.Fatal(err)
	}
	for {
		m, err := stream.Recv()
		if err != nil {
			t.Fatalf("the stream ended with the site gone: %v", err)
		}
		if m.GetPong() != nil {
			break
		}
	}

	// A player who drops now gets the place back well inside the 10 s it is kept.
	cancelFirst()
	began := time.Now()
	_, again := connect(ctx, &pb.Join{Target: &pb.Join_RoomCode{RoomCode: room.Code}, Protocol: multiplay.Protocol})
	if again.Code != room.Code {
		t.Fatalf("came back to room %s, not %s", again.Code, room.Code)
	}
	if took := time.Since(began); took > 5*time.Second {
		t.Fatalf("coming back took %s", took)
	}
}
