// Command multiplay-smoke is a player of Multi-Enso (internal/multiplay) for trying the
// service by hand: it connects as the game does, prints every message the server sends, and
// plays along without playing anything.
//
//	go run ./tools/multiplay-smoke                    # a whole round alone, on a local server
//	go run ./tools/multiplay-smoke -do token          # print the game session it would use
//	go run ./tools/multiplay-smoke -do probe          # is the service there? (no session)
//	go run ./tools/multiplay-smoke -do bot -room 12345  # enter a room and play every round
//
// What it does (-do):
//
//	round  Opens a private room, locks the members, picks a song (-song, or おまかせ), is
//	       ready, reports loaded, sends a few statuses, finishes, shows the result and
//	       leaves. Alone, so the server must run with MULTIPLAY_MIN_PLAYERS=1.
//	bot    Enters the room -room (or, without it, a quick match) and does the same in every
//	       round until the room closes or it is interrupted; it never locks the members. A
//	       second player for a game under test. It needs an account of its own: the same
//	       account entering a room twice replaces its first connection.
//	probe  Connects without a session. A server that is there answers UNAUTHENTICATED
//	       AUTH_REQUIRED; that is what the command checks.
//	token  Prints the game session and exits.
//
// Where it connects: it asks the site (-site) where the service is, as the game does
// (GET /api/v1/game/multiplay), unless -addr says. An https address is dialed with TLS, an
// http one or a bare host:port in clear text.
//
// Who it is: -token is a game session (64 hex digits), the one the game sends to the site's
// /game/ routes. Without it, it signs in at the site as the game does
// (POST /api/v1/sources/fanmade/game/login): a local server started with DEV_USER_ID and no
// account center gives every sign-in the development user's session, which is the SHA-256,
// in hex, of "ourtaiko-dev-game:" followed by DEV_USER_ID.
package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	pb "ourtaiko.dev/net/internal/multiplay/multiplaypb"
)

var (
	site     = flag.String("site", "http://127.0.0.1:8090", "the site's API: asked where the service is and, without -token, for a session")
	addr     = flag.String("addr", "", "the service's address (https://host, http://host:port or host:port) instead of what the site says")
	token    = flag.String("token", "", "game session, 64 hex digits; empty: sign in at the site (a development server gives its development session)")
	do       = flag.String("do", "round", "round, bot, probe or token")
	roomCode = flag.String("room", "", "bot: the 5-digit number of the room to enter; empty: quick match")
	songRef  = flag.String("song", "", "the song to pick, as source/chartId (fanmade/0123…); empty: おまかせ")
	course   = flag.String("course", "Oni", "the course to ask for: Easy, Normal, Hard, Oni or Edit")
	statuses = flag.Int("statuses", 5, "how many statuses to send in a play before finishing")
	every    = flag.Duration("every", 300*time.Millisecond, "the time between those statuses")
)

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "multiplay-smoke: "+format+"\n", args...)
	os.Exit(1)
}

// service asks the site where Multi-Enso is.
func service() string {
	res, err := http.Get(strings.TrimRight(*site, "/") + "/api/v1/game/multiplay")
	if err != nil {
		fail("the site cannot be asked where the service is: %v", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	var where struct {
		URL        string `json:"url"`
		Protocol   int    `json:"protocol"`
		MaxMembers int    `json:"maxMembers"`
	}
	if res.StatusCode != http.StatusOK || json.Unmarshal(body, &where) != nil || where.URL == "" {
		fail("GET /api/v1/game/multiplay answered %d %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	fmt.Printf("# the site says: %s\n", strings.TrimSpace(string(body)))
	return where.URL
}

// session signs in at the site as the game does and returns the game session.
func session() string {
	res, err := http.Post(strings.TrimRight(*site, "/")+"/api/v1/sources/fanmade/game/login", "application/json", bytes.NewReader([]byte(`{"username":"dev","password":"dev"}`)))
	if err != nil {
		fail("cannot sign in at the site: %v", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	var signed struct {
		AccessToken string `json:"accessToken"`
		User        struct {
			ID       string `json:"id"`
			Nickname string `json:"nickname"`
		} `json:"user"`
	}
	if res.StatusCode != http.StatusOK || json.Unmarshal(body, &signed) != nil || len(signed.AccessToken) != 64 {
		fail("the site did not sign the development user in (%d %s); on a server with an account center, give a game session with -token", res.StatusCode, strings.TrimSpace(string(body)))
	}
	fmt.Fprintf(os.Stderr, "# signed in at the site as %s (%s)\n", signed.User.Nickname, signed.User.ID)
	return signed.AccessToken
}

// dial connects to the service: with TLS for an https address, in clear text otherwise.
func dial(address string) *grpc.ClientConn {
	target, secure := address, false
	if strings.Contains(address, "://") {
		u, err := url.Parse(address)
		if err != nil || u.Host == "" || u.Scheme != "http" && u.Scheme != "https" {
			fail("%q is not an address of the service", address)
		}
		secure, target = u.Scheme == "https", u.Host
		if u.Port() == "" {
			target += map[bool]string{true: ":443", false: ":80"}[secure]
		}
	}
	creds := insecure.NewCredentials()
	if secure {
		creds = credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})
	}
	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(creds))
	if err != nil {
		fail("%v", err)
	}
	fmt.Printf("# dialing %s (TLS: %v)\n", target, secure)
	return conn
}

// show prints a message of the server as one line of JSON. A chart's own JSON is long and
// is replaced by its size.
func show(m *pb.ServerMessage) {
	shown := proto.Clone(m).(*pb.ServerMessage)
	note := ""
	if song := shown.GetRoom().GetSong(); song != nil && len(song.ChartJson) > 0 {
		note = fmt.Sprintf("   # chart_json: %d bytes", len(song.ChartJson))
		song.ChartJson = nil
	}
	line, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(shown)
	if err != nil {
		fail("%v", err)
	}
	// protojson varies its spacing on purpose; one form is easier to read and compare.
	var compact bytes.Buffer
	if json.Compact(&compact, line) == nil {
		line = compact.Bytes()
	}
	fmt.Printf("<- %s%s\n", line, note)
}

func main() {
	flag.Parse()
	switch *do {
	case "round", "bot", "probe", "token":
	default:
		fail("-do must be round, bot, probe or token")
	}
	if *do == "token" {
		if *token == "" {
			*token = session()
		}
		fmt.Println(*token)
		return
	}
	address := *addr
	if address == "" {
		address = service()
	}
	conn := dial(address)
	defer conn.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	client := pb.NewMultiEnsoClient(conn)

	if *do == "probe" {
		stream, err := client.Connect(ctx)
		if err == nil {
			_, err = stream.Recv()
		}
		st, _ := status.FromError(err)
		fmt.Printf("<- %s %s\n", strings.ToUpper(st.Code().String()), st.Message())
		if st.Code() != codes.Unauthenticated || st.Message() != "AUTH_REQUIRED" {
			fail("a stream without a session should be refused with UNAUTHENTICATED AUTH_REQUIRED")
		}
		fmt.Println("# the service is there")
		return
	}

	if *token == "" {
		*token = session()
	}
	pick := &pb.PickSong{Course: *course}
	if *songRef != "" {
		source, id, ok := strings.Cut(*songRef, "/")
		if !ok {
			fail("-song is source/chartId")
		}
		pick.Source, pick.ChartId = source, id
	}
	stream, err := client.Connect(metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+*token))
	if err != nil {
		fail("%v", err)
	}
	// One message at a time: the play below sends from a goroutine of its own.
	var sending sync.Mutex
	send := func(m *pb.ClientMessage) {
		sending.Lock()
		defer sending.Unlock()
		line, _ := protojson.MarshalOptions{UseProtoNames: true}.Marshal(m)
		var compact bytes.Buffer
		if json.Compact(&compact, line) == nil {
			line = compact.Bytes()
		}
		fmt.Printf("-> %s\n", line)
		if err := stream.Send(m); err != nil && !errors.Is(err, io.EOF) {
			fail("send: %v", err)
		}
	}
	join := &pb.Join{Protocol: 1}
	switch {
	case *do == "round":
		join.Target = &pb.Join_CreatePrivate{CreatePrivate: &pb.CreatePrivate{}}
	case *roomCode != "":
		join.Target = &pb.Join_RoomCode{RoomCode: *roomCode}
	default:
		join.Target = &pb.Join_QuickMatch{QuickMatch: &pb.QuickMatch{}}
	}
	send(&pb.ClientMessage{Kind: &pb.ClientMessage_Join{Join: join}})

	// What was already done in the round the room is in, so each step is taken once.
	var me string
	var round int32 = -1
	var locked, picked, ready, loaded, left bool
	for {
		m, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			fmt.Println("# the server ended the stream")
			return
		}
		if err != nil {
			st, _ := status.FromError(err)
			if ctx.Err() != nil {
				fmt.Println("# interrupted")
				return
			}
			fail("the stream ended with %s %s", strings.ToUpper(st.Code().String()), st.Message())
		}
		show(m)
		switch {
		case m.GetWelcome() != nil:
			me = m.GetWelcome().UserId
		case m.GetStartPlay() != nil:
			// A play: a score that grows, then the end of the song.
			go func() {
				s := &pb.PlayerStatus{}
				for i := 1; i <= *statuses; i++ {
					time.Sleep(*every)
					s = &pb.PlayerStatus{Score: int32(i) * 12340, Combo: int32(i) * 10, MaxCombo: int32(i) * 10, Good: int32(i) * 8, Ok: int32(i) * 2, Drumroll: int32(i), Gauge: int32(min(10000, i*2500)), MusicTimeMs: int32(i) * int32(every.Milliseconds())}
					send(&pb.ClientMessage{Kind: &pb.ClientMessage_Status{Status: s}})
				}
				time.Sleep(*every)
				send(&pb.ClientMessage{Kind: &pb.ClientMessage_Finish{Finish: &pb.FinishPlay{Final: s, Cleared: s.Gauge >= 8000}}})
			}()
		case m.GetRoom() != nil:
			room := m.GetRoom()
			if room.Round != round {
				round, picked, ready, loaded = room.Round, false, false, false
			}
			switch room.Phase {
			case pb.Phase_LOBBY:
				switch {
				case *do != "round":
				case room.Result != nil && !left:
					// The round is over and the room is back in the lobby.
					left = true
					send(&pb.ClientMessage{Kind: &pb.ClientMessage_Leave{Leave: &pb.Leave{}}})
				case room.Result == nil && !locked && room.HostId == me:
					locked = true
					send(&pb.ClientMessage{Kind: &pb.ClientMessage_LockMembers{LockMembers: &pb.LockMembers{}}})
				}
			case pb.Phase_SONG_SELECT:
				if !picked {
					picked = true
					send(&pb.ClientMessage{Kind: &pb.ClientMessage_PickSong{PickSong: pick}})
				}
			case pb.Phase_COURSE:
				if !ready {
					ready = true
					send(&pb.ClientMessage{Kind: &pb.ClientMessage_SetReady{SetReady: &pb.SetReady{Ready: true}}})
				}
			case pb.Phase_LOADING:
				if !loaded {
					loaded = true
					send(&pb.ClientMessage{Kind: &pb.ClientMessage_Loaded{Loaded: &pb.Loaded{Ok: true}}})
				}
			}
		case m.GetNotice() != nil:
			if *do == "round" {
				fail("the server refused: %s (%s)", m.GetNotice().Code, m.GetNotice().Message)
			}
		}
	}
}
