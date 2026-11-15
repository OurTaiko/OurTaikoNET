package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"ourtaiko.dev/net/internal/multiplay"
	pb "ourtaiko.dev/net/internal/multiplay/multiplaypb"
	"ourtaiko.dev/net/internal/netdb/netdbtest"
)

// Multi-Enso in the program: the game is told where it is, a game session of the site opens
// it, and its rooms play the charts of both sources, described as each source's API
// describes them.
func TestMultiEnsoPlaysBothSources(t *testing.T) {
	pool, url := netdbtest.NewURL(t, "fanmade")
	ctx := context.Background()
	fan, unlisted, official, extra, bare := strings.Repeat("f", 32), strings.Repeat("e", 32), strings.Repeat("d", 32), strings.Repeat("c", 32), strings.Repeat("b", 32)
	if _, err := pool.Exec(ctx, `
INSERT INTO net.users(id) VALUES (repeat('a',32));
INSERT INTO fanmade.chart_resources(storage_key,original_filename,sha256,byte_size,extension) VALUES
 (repeat('1',64)||'.tja','a.tja',repeat('1',64),10,'tja'), (repeat('2',64)||'.ogg','a.ogg',repeat('2',64),10,'ogg');
INSERT INTO official.chart_resources(storage_key,original_filename,sha256,byte_size,extension) VALUES
 (repeat('3',64)||'.tja','b.tja',repeat('3',64),10,'tja'), (repeat('4',64)||'.ogg','b.ogg',repeat('4',64),10,'ogg');
-- A Fanmade chart is in a category from the start; this one was taken out of all of them.
INSERT INTO fanmade.charts(id,owner_ids,title,subtitle,bpm,duration,demo_end,tja_file,audio_file,category_flags)
 SELECT id, ARRAY[repeat('a',32)], title, '--'||title, 120, 60, 10, (SELECT min(id) FROM fanmade.chart_resources), (SELECT max(id) FROM fanmade.chart_resources), flags
 FROM (VALUES (repeat('f',32),'Fan song',16), (repeat('e',32),'Unlisted',0)) v(id,title,flags);
-- The official source lists a chart in the game only once it has a category; one of these
-- has none, and one song is registered without files.
INSERT INTO official.charts(id,owner_ids,title,bpm,duration,demo_end,tja_file,audio_file,category_flags)
 SELECT id, ARRAY[repeat('a',32)], title, 120, 60, 10, (SELECT min(id) FROM official.chart_resources), (SELECT max(id) FROM official.chart_resources), flags
 FROM (VALUES (repeat('d',32),'Official song',1), (repeat('c',32),'Not in the game yet',0)) v(id,title,flags);
INSERT INTO official.charts(id,owner_ids,title,bpm,duration,demo_end,category_flags) VALUES (repeat('b',32), ARRAY[repeat('a',32)], 'No files', 120, 0, 0, 1);
INSERT INTO fanmade.chart_difficulties(chart_id,course,side,level,content_hash) VALUES
 (repeat('f',32),'Oni','single',8,'v2:'||repeat('0',64)), (repeat('f',32),'Easy','single',3,'v2:'||repeat('1',64)),
 (repeat('f',32),'Oni','p1',9,'v2:'||repeat('2',64)), (repeat('f',32),'Oni','p2',10,'v2:'||repeat('3',64)),
 (repeat('e',32),'Hard','single',6,'v2:'||repeat('4',64));
INSERT INTO official.chart_difficulties(chart_id,course,side,level,content_hash) VALUES
 (repeat('d',32),'Oni','single',9,'v2:'||repeat('5',64)), (repeat('d',32),'Edit','p2',7,'v2:'||repeat('6',64)), (repeat('d',32),'Edit','p1',10,'v2:'||repeat('7',64)),
 (repeat('c',32),'Normal','single',4,'v2:'||repeat('8',64));
`); err != nil {
		t.Fatal(err)
	}
	dev := strings.Repeat("a", 32)
	a, err := New(ctx, Config{DatabaseURL: url, Origin: "http://localhost", StorageDir: t.TempDir(), DevUserID: dev,
		Multiplay: Multiplay{ListenAddr: "127.0.0.1:0", PublicURL: "http://127.0.0.1:50061", MinPlayers: 1}})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	h := a.Handler()
	get := func(path string) (int, string) {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		return w.Code, strings.TrimSuffix(w.Body.String(), "\n")
	}

	// The game is told where the rooms are.
	if status, body := get("/api/v1/game/multiplay"); status != 200 || body != `{"url":"http://127.0.0.1:50061","protocol":1,"maxProtocol":2,"maxMembers":5}` {
		t.Fatalf("%d %s", status, body)
	}

	// A chart is what its source's API says of it, with the courses of its blocks: the
	// level of a course is its single-play block's, else its first block's (p1).
	songs := newRoomSongs(a.Fanmade, a.Official)
	courses := func(s *multiplay.Song) string {
		out, _ := json.Marshal(s.Courses)
		return string(out)
	}
	song, err := songs.Get(ctx, "fanmade", fan)
	if err != nil {
		t.Fatal(err)
	}
	if song.Source != "fanmade" || song.ChartID != fan || song.Title != "Fan song" || song.Subtitle != "--Fan song" || courses(song) != `[{"Name":"Easy","Level":3},{"Name":"Oni","Level":8}]` {
		t.Fatalf("%+v %s", song, courses(song))
	}
	if status, body := get("/api/v1/sources/fanmade/charts/" + fan); status != 200 || string(song.ChartJSON) != body {
		t.Fatalf("a room is given\n%s\nand the source answers %d\n%s", song.ChartJSON, status, body)
	}
	if song, err = songs.Get(ctx, "official", official); err != nil || song.Title != "Official song" || courses(song) != `[{"Name":"Oni","Level":9},{"Name":"Edit","Level":10}]` {
		t.Fatalf("%+v, %v", song, err)
	}
	if status, body := get("/api/v1/sources/official/charts/" + official); status != 200 || string(song.ChartJSON) != body {
		t.Fatalf("a room is given\n%s\nand the source answers %d\n%s", song.ChartJSON, status, body)
	}
	// A chart in no category can be picked, though chance never draws it.
	if song, err = songs.Get(ctx, "official", extra); err != nil || courses(song) != `[{"Name":"Normal","Level":4}]` {
		t.Fatalf("%+v, %v", song, err)
	}
	if song, err = songs.Get(ctx, "fanmade", unlisted); err != nil || song.Title != "Unlisted" || courses(song) != `[{"Name":"Hard","Level":6}]` {
		t.Fatalf("%+v, %v", song, err)
	}
	for name, asked := range map[string][2]string{
		"a chart of the other source": {"official", fan},
		"a song without files":        {"official", bare},
		"an unknown source":           {"elsewhere", fan},
		"no source":                   {"", fan},
		"an unknown chart":            {"fanmade", strings.Repeat("0", 32)},
		"a number":                    {"fanmade", "1"},
	} {
		if _, err = songs.Get(ctx, asked[0], asked[1]); !errors.Is(err, multiplay.ErrChartNotFound) {
			t.Errorf("%s: %v", name, err)
		}
	}

	// Chance draws among the charts the game lists, of both sources as one list.
	drawn := map[string]int{}
	for n := 0; n < 4; n++ {
		songs.intn = func(total int) int {
			if total != 2 {
				t.Fatalf("drawing among %d charts", total)
			}
			return n % total
		}
		if song, err = songs.Random(ctx); err != nil || song == nil {
			t.Fatal(song, err)
		}
		drawn[song.Source+" "+song.Title]++
	}
	if len(drawn) != 2 || drawn["fanmade Fan song"] != 2 || drawn["official Official song"] != 2 {
		t.Fatalf("drawn: %v", drawn)
	}
	if _, err = pool.Exec(ctx, `UPDATE fanmade.charts SET status='hidden'; UPDATE official.charts SET status='hidden'`); err != nil {
		t.Fatal(err)
	}
	songs.intn = func(int) int { t.Fatal("drawing among no charts"); return 0 }
	if song, err = songs.Random(ctx); err != nil || song != nil {
		t.Fatalf("with no chart to draw: %+v, %v", song, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE fanmade.charts SET status='published'; UPDATE official.charts SET status='published'`); err != nil {
		t.Fatal(err)
	}

	// The session the game has for the site's API is the one that opens the rooms.
	login := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/sources/fanmade/game/login", strings.NewReader(`{"username":"dev","password":"dev"}`))
	r.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(login, r)
	var session struct {
		AccessToken string `json:"accessToken"`
	}
	if err = json.Unmarshal(login.Body.Bytes(), &session); err != nil || login.Code != http.StatusOK || len(session.AccessToken) != 64 {
		t.Fatal(login.Code, login.Body.String())
	}
	who := gamePlayer(a.Fanmade)
	if p, err := who(ctx, session.AccessToken); err != nil || p.UserID != dev || p.Nickname == "" {
		t.Fatalf("%+v, %v", p, err)
	}
	if _, err = who(ctx, strings.Repeat("0", 64)); !errors.Is(err, multiplay.ErrUnauthenticated) {
		t.Fatalf("another session: %v", err)
	}

	// And over the wire: alone in a room (this server lets one player play), leaving the
	// song to chance.
	if err = a.startRooms(); err != nil {
		t.Fatal(err)
	}
	conn, err := grpc.NewClient(a.RoomsAddr(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	client := pb.NewMultiEnsoClient(conn)
	wire, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	refused, err := client.Connect(metadata.AppendToOutgoingContext(wire, "authorization", "Bearer "+strings.Repeat("0", 64)))
	if err == nil {
		_, err = refused.Recv()
	}
	if st, _ := status.FromError(err); st.Code() != codes.Unauthenticated || st.Message() != "AUTH_REQUIRED" {
		t.Fatalf("a session the site does not know: %v", err)
	}
	stream, err := client.Connect(metadata.AppendToOutgoingContext(wire, "authorization", "Bearer "+session.AccessToken))
	if err != nil {
		t.Fatal(err)
	}
	send := func(m *pb.ClientMessage) {
		t.Helper()
		if err := stream.Send(m); err != nil {
			t.Fatal(err)
		}
	}
	room := func(phase pb.Phase) *pb.Room {
		t.Helper()
		for {
			m, err := stream.Recv()
			if err != nil {
				t.Fatalf("waiting for %v: %v", phase, err)
			}
			if n := m.GetNotice(); n != nil {
				t.Fatalf("waiting for %v: notice %v", phase, n)
			}
			if r := m.GetRoom(); r != nil && r.Phase == phase {
				return r
			}
		}
	}
	send(&pb.ClientMessage{Kind: &pb.ClientMessage_Join{Join: &pb.Join{Target: &pb.Join_CreatePrivate{CreatePrivate: &pb.CreatePrivate{}}, Protocol: 1}}})
	welcome, err := stream.Recv()
	if err != nil || welcome.GetWelcome().GetUserId() != dev {
		t.Fatal(welcome, err)
	}
	if lobby := room(pb.Phase_LOBBY); lobby.MinPlayers != 1 || lobby.MaxMembers != 5 || lobby.HostId != dev {
		t.Fatalf("lobby: %v", lobby)
	}
	send(&pb.ClientMessage{Kind: &pb.ClientMessage_LockMembers{LockMembers: &pb.LockMembers{}}})
	room(pb.Phase_SONG_SELECT)
	send(&pb.ClientMessage{Kind: &pb.ClientMessage_PickSong{PickSong: &pb.PickSong{Course: "Edit"}}})
	draw := room(pb.Phase_SONG_DRAW)
	if draw.SongPickedBy != "" || draw.Song == nil {
		t.Fatalf("draw: %v", draw)
	}
	// Whichever was drawn: the chart as its source describes it, and the nearest course to
	// Edit it has.
	want := map[string]string{fan: "Oni", official: "Edit"}[draw.Song.ChartId]
	if want == "" || draw.Members[0].Course != want {
		t.Fatalf("drew %s %s, to be played on %q", draw.Song.Source, draw.Song.ChartId, draw.Members[0].Course)
	}
	if status, body := get("/api/v1/sources/" + draw.Song.Source + "/charts/" + draw.Song.ChartId); status != 200 || string(draw.Song.ChartJson) != body {
		t.Fatalf("the room shows\n%s\nand the source answers %d\n%s", draw.Song.ChartJson, status, body)
	}
	// Stopping tells the player the room is closed.
	a.StopRooms()
	for {
		m, err := stream.Recv()
		if err != nil {
			t.Fatalf("the stream ended without `closed`: %v", err)
		}
		if c := m.GetClosed(); c != nil {
			if c.Code != "ROOM_CLOSED" {
				t.Fatalf("closed: %v", c)
			}
			break
		}
	}
}

// Without MULTIPLAY_LISTEN_ADDR there are no rooms, and the game is told so.
func TestMultiEnsoIsOffUnlessConfigured(t *testing.T) {
	_, url := netdbtest.NewURL(t, "fanmade")
	ctx := context.Background()
	a, err := New(ctx, Config{DatabaseURL: url, Origin: "http://localhost", StorageDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if err = a.startRooms(); err != nil || a.RoomsAddr() != "" {
		t.Fatalf("listening on %q, %v", a.RoomsAddr(), err)
	}
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/game/multiplay", nil))
	if w.Code != 404 || !strings.Contains(w.Body.String(), `"code":"MULTIPLAY_UNAVAILABLE"`) {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	a.StopRooms()

	// The public address is the site's unless another is given; a round needs a sensible
	// number of players.
	on, err := New(ctx, Config{DatabaseURL: url, Origin: "https://net.example", StorageDir: t.TempDir(), Multiplay: Multiplay{ListenAddr: "127.0.0.1:0"}})
	if err != nil {
		t.Fatal(err)
	}
	defer on.Close()
	w = httptest.NewRecorder()
	on.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/game/multiplay", nil))
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"url":"https://net.example","protocol":1,"maxProtocol":2,"maxMembers":5}` {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	for _, players := range []int{-1, 6} {
		if bad, err := New(ctx, Config{DatabaseURL: url, Origin: "http://localhost", StorageDir: t.TempDir(), Multiplay: Multiplay{ListenAddr: "127.0.0.1:0", MinPlayers: players}}); err == nil {
			bad.Close()
			t.Fatalf("a round of at least %d players was accepted", players)
		}
	}
}
