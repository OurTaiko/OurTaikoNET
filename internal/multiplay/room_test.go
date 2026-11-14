package multiplay

import (
	"errors"
	"reflect"
	"regexp"
	"testing"
	"time"

	pb "ourtaiko.dev/net/internal/multiplay/multiplaypb"
)

var quick, private = JoinTarget{Kind: QuickMatch}, JoinTarget{Kind: CreatePrivate}

func byCode(code string) JoinTarget { return JoinTarget{Kind: ByCode, Code: code} }

/* ---------- joining ---------- */

func TestPrivateRoomHasANumberAndAHost(t *testing.T) {
	w := newWorld(t, nil)
	room, c1 := w.join(1, private)
	if !regexp.MustCompile(`^\d{5}$`).MatchString(room.Code()) {
		t.Fatalf("room number %q", room.Code())
	}
	// The player is welcomed, then shown the room: once.
	all := c1.all()
	if len(all) != 2 || all[0].GetWelcome().GetUserId() != uid(1) || all[0].GetWelcome().GetNickname() != "P1" || all[0].GetWelcome().GetProtocol() != Protocol || all[1].GetRoom() == nil {
		t.Fatalf("after joining: %v", all)
	}
	snap := c1.room()
	if snap.Code != room.Code() || snap.Public || snap.Phase != pb.Phase_LOBBY || snap.HostId != uid(1) || snap.MaxMembers != 5 || snap.MinPlayers != 2 || snap.Round != 0 || snap.DeadlineMs != 0 {
		t.Fatalf("snapshot: %v", snap)
	}
	if snap.ServerTimeMs != w.clock.Now().UnixMilli() {
		t.Fatalf("server time %d", snap.ServerTimeMs)
	}
	me := snap.Members[0]
	if me.UserId != uid(1) || me.Nickname != "P1" || me.AvatarUrl != player(1).AvatarURL || me.Slot != 0 || !me.Connected {
		t.Fatalf("member: %v", me)
	}
}

func TestQuickMatchFillsTheFullestWaitingRoom(t *testing.T) {
	w := newWorld(t, func(tm *Timings) { tm.MinPlayers = 3 })
	a, _ := w.join(1, quick)
	if b, _ := w.join(2, quick); b != a {
		t.Fatal("the second player opened a room instead of entering the waiting one")
	}
	// A second public room with one member: player 3 leaves a and opens it.
	w.m.Leave(uid(2))
	other, _ := w.join(2, private)
	w.must(other.OpenRoom(uid(2)))
	_, _ = w.join(3, byCode(other.Code()))
	_, _ = w.join(4, byCode(other.Code()))
	// a has one member, other three: the next quick match goes to other.
	if got, _ := w.join(5, quick); got != other {
		t.Fatalf("quick match entered room %s, want the fullest, %s", got.Code(), other.Code())
	}
	if w.m.RoomCount() != 2 {
		t.Fatalf("%d rooms", w.m.RoomCount())
	}
}

func TestQuickMatchStaysOutOfPrivateAndBusyRooms(t *testing.T) {
	w := newWorld(t, nil)
	priv, _ := w.join(1, private)
	pub, _ := w.join(2, quick)
	if pub == priv {
		t.Fatal("quick match entered a private room")
	}
	_, _ = w.join(3, quick)
	w.advance(w.m.Timings().LobbyCountdown)
	w.phase(pub, pb.Phase_SONG_SELECT)
	if again, _ := w.join(4, quick); again == pub || again == priv {
		t.Fatal("quick match entered a room in a round")
	}
}

func TestPrivateRoomIsEnteredByItsNumberOnly(t *testing.T) {
	w := newWorld(t, nil)
	room, conns := w.private(5)
	if got := len(conns[1].room().Members); got != 5 {
		t.Fatalf("%d members", got)
	}
	// Slots are the joined order.
	for i, m := range conns[5].room().Members {
		if m.Slot != int32(i) || m.UserId != uid(i+1) {
			t.Fatalf("member %d: %v", i, m)
		}
	}
	_, err := w.m.Join(player(6), byCode(room.Code()), &fakeConn{})
	w.refused(err, "ROOM_FULL")
	_, err = w.m.Join(player(6), byCode("1234"), &fakeConn{})
	w.refused(err, "BAD_ROOM_CODE")
	_, err = w.m.Join(player(6), byCode("12a45"), &fakeConn{})
	w.refused(err, "BAD_ROOM_CODE")
	_, err = w.m.Join(player(6), byCode("99999"), &fakeConn{})
	w.refused(err, "ROOM_NOT_FOUND")
	// A full private room does not start by itself.
	w.advance(time.Hour)
	w.phase(room, pb.Phase_LOBBY)
}

func TestRoomInARoundIsBusy(t *testing.T) {
	w := newWorld(t, nil)
	room, _ := w.private(2)
	w.must(room.LockMembers(uid(1)))
	_, err := w.m.Join(player(3), byCode(room.Code()), &fakeConn{})
	w.refused(err, "ROOM_BUSY")
}

func TestPublicRoomLocksItselfAfterTheCountdown(t *testing.T) {
	w := newWorld(t, nil)
	room, c1 := w.join(1, quick)
	if c1.room().DeadlineMs != 0 {
		t.Fatal("the countdown began with one player")
	}
	_, _ = w.join(2, quick)
	want := w.clock.Now().Add(30 * time.Second).UnixMilli()
	if got := c1.room().DeadlineMs; got != want {
		t.Fatalf("deadline %d, want %d", got, want)
	}
	// A third player does not start it over.
	w.advance(10 * time.Second)
	_, _ = w.join(3, quick)
	if got := c1.room().DeadlineMs; got != want {
		t.Fatalf("deadline moved to %d", got)
	}
	w.advance(20*time.Second - time.Millisecond)
	w.phase(room, pb.Phase_LOBBY)
	w.advance(time.Millisecond)
	w.phase(room, pb.Phase_SONG_SELECT)
	if snap := c1.room(); snap.Round != 1 || snap.DeadlineMs != w.clock.Now().Add(90*time.Second).UnixMilli() {
		t.Fatalf("snapshot: %v", snap)
	}
}

func TestCountdownStopsWhenTooFewArePresent(t *testing.T) {
	w := newWorld(t, nil)
	room, c1 := w.join(1, quick)
	_, c2 := w.join(2, quick)
	room.Disconnected(uid(2), c2)
	if c1.room().DeadlineMs != 0 {
		t.Fatal("the countdown runs with one player present")
	}
	w.advance(time.Hour)
	w.phase(room, pb.Phase_LOBBY)
	if len(c1.room().Members) != 1 {
		t.Fatal("the dropped player was kept")
	}
}

func TestPublicRoomLocksAtOnceWhenFull(t *testing.T) {
	w := newWorld(t, nil)
	room, c1 := w.join(1, quick)
	for i := 2; i <= 4; i++ {
		w.join(i, quick)
	}
	w.phase(room, pb.Phase_LOBBY)
	before := c1.snapshots()
	_, c5 := w.join(5, quick)
	w.phase(room, pb.Phase_SONG_SELECT)
	// Joining and the start of the round are one change: one snapshot each.
	if c1.snapshots() != before+1 || c5.snapshots() != 1 {
		t.Fatalf("snapshots: %d more for player 1, %d for player 5", c1.snapshots()-before, c5.snapshots())
	}
	if snap := c5.room(); snap.Phase != pb.Phase_SONG_SELECT || len(snap.Members) != 5 {
		t.Fatalf("snapshot: %v", snap)
	}
}

func TestOpenRoomMakesAPrivateRoomPublic(t *testing.T) {
	w := newWorld(t, nil)
	room, conns := w.private(2)
	w.refused(room.OpenRoom(uid(2)), "NOT_HOST")
	w.must(room.OpenRoom(uid(1)))
	if snap := conns[2].room(); !snap.Public || snap.DeadlineMs == 0 {
		t.Fatalf("snapshot: %v", snap)
	}
	if got, _ := w.join(3, quick); got != room {
		t.Fatal("quick match did not find the opened room")
	}
	w.advance(w.m.Timings().LobbyCountdown)
	w.phase(room, pb.Phase_SONG_SELECT)
}

func TestHostPassesOnAndAnEmptyRoomCloses(t *testing.T) {
	w := newWorld(t, nil)
	room, conns := w.private(3)
	// The next host is the longest-standing member who is connected.
	room.Disconnected(uid(2), conns[2])
	w.m.Leave(uid(1))
	if conns[1].closedCode() != "LEFT" {
		t.Fatalf("the leaver was closed with %q", conns[1].closedCode())
	}
	if got := conns[3].room().HostId; got != uid(3) {
		t.Fatalf("host %s", got)
	}
	w.m.Leave(uid(3))
	// Only the dropped player is left; its place lasts the grace period.
	if room.Closed() || w.m.RoomCount() != 1 {
		t.Fatal("the room closed while a dropped player could come back")
	}
	w.advance(w.m.Timings().ReconnectGrace)
	if !room.Closed() || w.m.RoomCount() != 0 {
		t.Fatal("the empty room stayed open")
	}
	_, err := w.m.Join(player(4), byCode(room.Code()), &fakeConn{})
	w.refused(err, "ROOM_NOT_FOUND")
}

func TestOnlyTheHostLocksAndOnlyWithEnoughPlayers(t *testing.T) {
	w := newWorld(t, nil)
	room, _ := w.join(1, private)
	w.refused(room.LockMembers(uid(1)), "NOT_ENOUGH_PLAYERS")
	_, c2 := w.join(2, byCode(room.Code()))
	w.refused(room.LockMembers(uid(2)), "NOT_HOST")
	w.refused(room.LockMembers(uid(9)), "NOT_IN_ROOM")
	// A member who is not connected does not count.
	room.Disconnected(uid(2), c2)
	w.refused(room.LockMembers(uid(1)), "NOT_ENOUGH_PLAYERS")
	_, _ = w.join(2, byCode(room.Code()))
	w.must(room.LockMembers(uid(1)))
	w.phase(room, pb.Phase_SONG_SELECT)
	w.refused(room.LockMembers(uid(1)), "BAD_PHASE")
}

func TestOnePlayerIsEnoughWhenTheServerSaysSo(t *testing.T) {
	w := newWorld(t, func(tm *Timings) { tm.MinPlayers = 1 })
	room, c1 := w.join(1, private)
	if c1.room().MinPlayers != 1 {
		t.Fatal(c1.room())
	}
	w.must(room.LockMembers(uid(1)))
	w.phase(room, pb.Phase_SONG_SELECT)
}

func TestKick(t *testing.T) {
	w := newWorld(t, nil)
	room, conns := w.private(3)
	w.refused(room.Kick(uid(2), uid(3)), "NOT_HOST")
	w.refused(room.Kick(uid(1), uid(1)), "BAD_REQUEST")
	w.refused(room.Kick(uid(1), uid(9)), "BAD_REQUEST")
	w.must(room.Kick(uid(1), uid(2)))
	if conns[2].closedCode() != "KICKED" || room.Has(uid(2)) {
		t.Fatalf("closed %q, still a member: %v", conns[2].closedCode(), room.Has(uid(2)))
	}
	if got := len(conns[3].room().Members); got != 2 {
		t.Fatalf("%d members", got)
	}
	// The place is not given again: slots only grow.
	_, c4 := w.join(4, byCode(room.Code()))
	if got := c4.member(uid(4)).Slot; got != 3 {
		t.Fatalf("slot %d", got)
	}
	w.must(room.LockMembers(uid(1)))
	w.refused(room.Kick(uid(1), uid(3)), "BAD_PHASE")
}

func TestDroppedPlayerKeepsItsPlaceForAWhile(t *testing.T) {
	w := newWorld(t, nil)
	room, conns := w.private(2)
	room.Disconnected(uid(2), conns[2])
	if conns[1].member(uid(2)).Connected {
		t.Fatal("still shown as connected")
	}
	w.advance(w.m.Timings().ReconnectGrace - time.Millisecond)
	if !room.Has(uid(2)) {
		t.Fatal("removed before the grace period was over")
	}
	back, again := w.join(2, byCode(room.Code()))
	if back != room {
		t.Fatal("came back to another room")
	}
	w.advance(w.m.Timings().ReconnectGrace)
	if !room.Has(uid(2)) || !again.member(uid(2)).Connected || again.member(uid(2)).Slot != 1 {
		t.Fatalf("after coming back: %v", again.room())
	}
	// The old stream ending late changes nothing.
	room.Disconnected(uid(2), conns[2])
	if !conns[1].member(uid(2)).Connected {
		t.Fatal("the old connection's end dropped the player")
	}
}

func TestDroppedPlayerIsRemovedAfterTheGracePeriod(t *testing.T) {
	w := newWorld(t, nil)
	room, conns := w.private(2)
	room.Disconnected(uid(2), conns[2])
	w.advance(w.m.Timings().ReconnectGrace)
	if room.Has(uid(2)) || len(conns[1].room().Members) != 1 {
		t.Fatal("the dropped player was kept")
	}
}

func TestSecondConnectionReplacesTheFirst(t *testing.T) {
	w := newWorld(t, nil)
	room, conns := w.private(2)
	_, second := w.join(2, byCode(room.Code()))
	if conns[2].closedCode() != "REPLACED" {
		t.Fatalf("the first connection was closed with %q", conns[2].closedCode())
	}
	if len(second.room().Members) != 2 || !second.member(uid(2)).Connected {
		t.Fatalf("snapshot: %v", second.room())
	}
	// The first stream's end must not drop the player now on the second.
	room.Disconnected(uid(2), conns[2])
	w.advance(w.m.Timings().ReconnectGrace)
	if !room.Has(uid(2)) {
		t.Fatal("the replaced connection's end removed the player")
	}
}

func TestEnteringAnotherRoomLeavesTheFirst(t *testing.T) {
	w := newWorld(t, nil)
	first, conns := w.private(2)
	second, _ := w.join(2, private)
	if second == first || first.Has(uid(2)) || conns[2].closedCode() != "LEFT" {
		t.Fatalf("still in the first room: %v, closed %q", first.Has(uid(2)), conns[2].closedCode())
	}
	// And back by number: the second room, now empty, closes.
	w.join(2, byCode(first.Code()))
	if !second.Closed() || !first.Has(uid(2)) {
		t.Fatal("the player is not back in the first room alone")
	}
}

func TestServerFull(t *testing.T) {
	w := newWorld(t, nil)
	for i := 0; i < MaxRooms; i++ {
		p := Player{UserID: "u" + string(rune(0x4e00+i)), Nickname: "n"}
		if _, err := w.m.Join(p, private, &fakeConn{}); err != nil {
			t.Fatal(i, err)
		}
	}
	_, err := w.m.Join(player(1), private, &fakeConn{})
	w.refused(err, "SERVER_FULL")
	_, err = w.m.Join(player(1), quick, &fakeConn{})
	w.refused(err, "SERVER_FULL")
}

func TestClosingTheManagerClosesEveryRoom(t *testing.T) {
	w := newWorld(t, nil)
	room, conns := w.private(2)
	_, other := w.join(3, quick)
	w.m.Close()
	for i, c := range []*fakeConn{conns[1], conns[2], other} {
		if c.closedCode() != "ROOM_CLOSED" {
			t.Fatalf("connection %d was closed with %q", i, c.closedCode())
		}
	}
	if !room.Closed() || w.m.RoomCount() != 0 {
		t.Fatal("rooms are still open")
	}
	_, err := w.m.Join(player(4), private, &fakeConn{})
	w.refused(err, "ROOM_CLOSED")
	// Nothing is left to fire.
	w.advance(time.Hour)
}

/* ---------- a round ---------- */

func TestPicksAreHiddenUntilTheDrawAndEachPlayerPicksACourse(t *testing.T) {
	w := newWorld(t, nil)
	room, conns := w.private(2)
	w.must(room.LockMembers(uid(1)))
	w.must(room.PickSong(uid(1), songA.Source, songA.ChartID, "Hard"))
	m := conns[2].member(uid(1))
	if !m.Picked || m.PickedSource != "" || m.PickedChartId != "" || m.Course != "" || conns[2].room().Song != nil {
		t.Fatalf("a pick shows before the draw: %v", m)
	}
	if conns[2].member(uid(2)).Picked {
		t.Fatal("player 2 has not picked")
	}

	w.draw = 0 // player 1's pick wins
	w.must(room.PickSong(uid(2), songB.Source, songB.ChartID, "Edit"))
	w.phase(room, pb.Phase_SONG_DRAW)
	snap := conns[1].room()
	if snap.Song.Source != "fanmade" || snap.Song.ChartId != songA.ChartID || snap.Song.Title != "Alpha" || snap.Song.Subtitle != "--Alpha" || string(snap.Song.ChartJson) != `{"title":"Alpha"}` {
		t.Fatalf("song: %v", snap.Song)
	}
	var courses []string
	for _, c := range snap.Song.Courses {
		courses = append(courses, c.Course+" "+string(rune('0'+c.Level)))
	}
	if !reflect.DeepEqual(courses, []string{"Easy 3", "Normal 5", "Hard 7", "Oni 9"}) {
		t.Fatalf("courses: %v", courses)
	}
	if snap.SongPickedBy != uid(1) || snap.DeadlineMs != w.clock.Now().Add(4*time.Second).UnixMilli() {
		t.Fatalf("snapshot: %v", snap)
	}
	// Player 1 plays the course asked for. Alpha has no Edit: player 2 gets the nearest
	// easier one. What each picked shows now.
	p1, p2 := snap.Members[0], snap.Members[1]
	if p1.Course != "Hard" || p2.Course != "Oni" {
		t.Fatalf("courses %s and %s", p1.Course, p2.Course)
	}
	if p1.PickedSource != "fanmade" || p1.PickedChartId != songA.ChartID || p2.PickedSource != "official" || p2.PickedChartId != songB.ChartID {
		t.Fatalf("picks: %v, %v", p1, p2)
	}

	// A course may be picked already while the draw is shown.
	w.must(room.PickCourse(uid(1), "Easy"))
	w.advance(w.m.Timings().SongDraw)
	w.phase(room, pb.Phase_COURSE)
	if got := conns[1].room().DeadlineMs; got != w.clock.Now().Add(15*time.Second).UnixMilli() {
		t.Fatalf("deadline %d", got)
	}
	w.must(room.PickCourse(uid(2), "Normal"))
	w.refused(room.PickCourse(uid(2), "Edit"), "BAD_REQUEST")
	w.refused(room.PickCourse(uid(2), "Expert"), "BAD_REQUEST")
	w.must(room.SetReady(uid(2), true))
	w.refused(room.PickCourse(uid(2), "Hard"), "BAD_PHASE")
	// Not ready any more: the course can change again.
	w.must(room.SetReady(uid(2), false))
	w.must(room.PickCourse(uid(2), "Hard"))
	w.must(room.SetReady(uid(2), true))
	w.phase(room, pb.Phase_COURSE)
	w.must(room.SetReady(uid(1), true))
	w.phase(room, pb.Phase_LOADING)
	snap = conns[2].room()
	if snap.Members[0].Course != "Easy" || snap.Members[1].Course != "Hard" || !snap.Members[0].Playing || !snap.Members[1].Playing || snap.Members[0].Loaded {
		t.Fatalf("snapshot: %v", snap)
	}
	if snap.DeadlineMs != w.clock.Now().Add(90*time.Second).UnixMilli() {
		t.Fatalf("deadline %d", snap.DeadlineMs)
	}
	w.refused(room.PickCourse(uid(1), "Oni"), "BAD_PHASE")
	w.refused(room.SetReady(uid(1), false), "BAD_PHASE")
	w.refused(room.PickSong(uid(1), "", "", "Oni"), "BAD_PHASE")
}

func TestTheDrawCanFallOnAnyPick(t *testing.T) {
	w := newWorld(t, nil)
	room, conns := w.private(2)
	w.must(room.LockMembers(uid(1)))
	w.must(room.PickSong(uid(1), songA.Source, songA.ChartID, "Oni"))
	w.draw = 1 // player 2's pick wins
	w.must(room.PickSong(uid(2), songB.Source, songB.ChartID, "Edit"))
	snap := conns[1].room()
	if snap.Song.ChartId != songB.ChartID || snap.SongPickedBy != uid(2) || snap.Members[0].Course != "Oni" || snap.Members[1].Course != "Edit" {
		t.Fatalf("snapshot: %v", snap)
	}
}

func TestAPickCanBeChangedUntilTheDraw(t *testing.T) {
	w := newWorld(t, nil)
	room, conns := w.private(2)
	w.must(room.LockMembers(uid(1)))
	w.must(room.PickSong(uid(1), songA.Source, songA.ChartID, "Oni"))
	w.must(room.PickSong(uid(1), songC.Source, songC.ChartID, "Easy"))
	w.must(room.PickSong(uid(2), "", "", ""))
	if snap := conns[2].room(); snap.Song.ChartId != songC.ChartID || snap.Members[0].Course != "Easy" {
		t.Fatalf("snapshot: %v", snap)
	}
}

func TestOmakaseDrawsAnySong(t *testing.T) {
	w := newWorld(t, nil)
	room, conns := w.private(2)
	w.must(room.LockMembers(uid(1)))
	// Nobody picks: when the time is up, everyone has left it to chance.
	w.advance(w.m.Timings().SongSelect)
	w.phase(room, pb.Phase_SONG_DRAW)
	snap := conns[1].room()
	if snap.Song.ChartId != songB.ChartID || snap.SongPickedBy != "" {
		t.Fatalf("snapshot: %v", snap)
	}
	for _, m := range snap.Members {
		// Nothing asked for and no earlier round: Oni.
		if !m.Picked || m.PickedChartId != "" || m.PickedSource != "" || m.Course != "Oni" {
			t.Fatalf("member: %v", m)
		}
	}
}

func TestOmakaseKeepsTheCourseAskedFor(t *testing.T) {
	w := newWorld(t, nil)
	room, conns := w.private(2)
	w.must(room.LockMembers(uid(1)))
	w.catalog.random = songA
	// Player 2's おまかせ wins over player 1's song.
	w.draw = 1
	w.must(room.PickSong(uid(1), songB.Source, songB.ChartID, "Edit"))
	w.must(room.PickSong(uid(2), "", "", "Normal"))
	snap := conns[1].room()
	if snap.Song.ChartId != songA.ChartID || snap.SongPickedBy != "" {
		t.Fatalf("snapshot: %v", snap)
	}
	// Alpha has Normal; it has no Edit, so player 1 gets Oni.
	if snap.Members[0].Course != "Oni" || snap.Members[1].Course != "Normal" || snap.Members[0].PickedChartId != songB.ChartID {
		t.Fatalf("members: %v", snap.Members)
	}
}

func TestCourseDefaults(t *testing.T) {
	all := []Course{{"Easy", 1}, {"Normal", 2}, {"Hard", 3}, {"Oni", 4}, {"Edit", 5}}
	for _, c := range []struct {
		courses   []Course
		preferred string
		want      string
	}{
		{all, "Normal", "Normal"},
		{all, "Edit", "Edit"},
		{all, "", "Oni"},
		{all, "Expert", "Oni"},
		// Not there: the nearest easier one.
		{songA.Courses, "Edit", "Oni"},
		{songC.Courses, "Oni", "Hard"},
		{songC.Courses, "Normal", "Easy"},
		{songC.Courses, "Edit", "Hard"},
		// Nothing easier: the nearest harder one.
		{songB.Courses, "Easy", "Oni"},
		{songD.Courses, "Easy", "Hard"},
		{songD.Courses, "Normal", "Hard"},
		{songD.Courses, "Oni", "Hard"},
		{[]Course{{"Edit", 10}}, "Easy", "Edit"},
		{[]Course{{"Edit", 10}}, "", "Edit"},
		{nil, "Oni", ""},
	} {
		if got := nearestCourse(c.courses, c.preferred); got != c.want {
			t.Errorf("asked for %q of %v: got %q, want %q", c.preferred, c.courses, got, c.want)
		}
	}
}

func TestCoursePlayedLastRoundIsTheNextDefault(t *testing.T) {
	w := newWorld(t, nil)
	room, conns := w.playing()
	// Round 1 is Alpha: player 1 on Oni; player 2 asked for Hard.
	if snap := conns[1].room(); snap.Members[0].Course != "Oni" || snap.Members[1].Course != "Hard" {
		t.Fatalf("round 1: %v", snap.Members)
	}
	w.must(room.Finish(uid(1), nil, false))
	w.must(room.Finish(uid(2), nil, false))
	w.advance(w.m.Timings().Result)
	w.must(room.LockMembers(uid(1)))
	// Round 2: Gamma (Easy, Hard) by おまかせ, nobody says a course.
	w.catalog.random = songC
	w.advance(w.m.Timings().SongSelect)
	snap := conns[1].room()
	if snap.Round != 2 || snap.Song.ChartId != songC.ChartID {
		t.Fatalf("round 2: %v", snap)
	}
	// Player 1 played Oni: Gamma's nearest easier is Hard. Player 2 played Hard.
	if snap.Members[0].Course != "Hard" || snap.Members[1].Course != "Hard" {
		t.Fatalf("round 2 courses: %v", snap.Members)
	}
}

func TestUnknownAndMalformedPicksAreRefused(t *testing.T) {
	w := newWorld(t, nil)
	room, conns := w.private(2)
	w.refused(room.PickSong(uid(1), songA.Source, songA.ChartID, "Oni"), "BAD_PHASE")
	w.must(room.LockMembers(uid(1)))
	w.refused(room.PickSong(uid(1), "fanmade", songB.ChartID, "Oni"), "CHART_NOT_FOUND")
	w.refused(room.PickSong(uid(1), "elsewhere", songA.ChartID, "Oni"), "CHART_NOT_FOUND")
	asked := w.catalog.gets
	w.refused(room.PickSong(uid(1), "fanmade", "", "Oni"), "BAD_REQUEST")
	w.refused(room.PickSong(uid(1), "", songA.ChartID, "Oni"), "BAD_REQUEST")
	w.refused(room.PickSong(uid(1), songA.Source, songA.ChartID, "Expert"), "BAD_REQUEST")
	w.refused(room.PickSong(uid(9), songA.Source, songA.ChartID, "Oni"), "NOT_IN_ROOM")
	if w.catalog.gets != asked {
		t.Fatal("the catalog was asked about a request that could not stand")
	}
	w.catalog.getErr = errors.New("database is down")
	w.refused(room.PickSong(uid(1), songA.Source, songA.ChartID, "Oni"), "SERVER_ERROR")
	if conns[1].member(uid(1)).Picked {
		t.Fatal("a refused pick counts")
	}
	w.phase(room, pb.Phase_SONG_SELECT)
}

func TestPickThatComesBackTooLateIsDropped(t *testing.T) {
	w := newWorld(t, nil)
	room, conns := w.private(2)
	w.must(room.LockMembers(uid(1)))
	// While the catalog looks player 1's chart up, player 2 leaves: back to the lobby.
	w.catalog.onGet = func() { w.m.Leave(uid(2)) }
	w.must(room.PickSong(uid(1), songA.Source, songA.ChartID, "Oni"))
	w.phase(room, pb.Phase_LOBBY)
	if conns[1].member(uid(1)).Picked {
		t.Fatal("the late pick was kept")
	}
	// The same, when the room has begun another round meanwhile.
	w.catalog.onGet = nil
	_, _ = w.join(2, byCode(room.Code()))
	w.must(room.LockMembers(uid(1)))
	w.catalog.onGet = func() {
		w.catalog.onGet = nil
		w.m.Leave(uid(2))
		w.join(2, byCode(room.Code()))
		w.must(room.LockMembers(uid(1)))
	}
	w.must(room.PickSong(uid(1), songA.Source, songA.ChartID, "Oni"))
	w.phase(room, pb.Phase_SONG_SELECT)
	if snap := conns[1].room(); snap.Round != 3 || snap.Members[0].Picked {
		t.Fatalf("a pick of round 2 counts in round 3: %v", snap)
	}
}

func TestRandomSongThatComesBackTooLateIsDropped(t *testing.T) {
	w := newWorld(t, nil)
	room, conns := w.private(2)
	w.must(room.LockMembers(uid(1)))
	w.hold = true
	w.must(room.PickSong(uid(1), "", "", "Oni"))
	w.must(room.PickSong(uid(2), "", "", "Oni"))
	// The catalog is being asked: the countdown is over, the phase is not.
	w.phase(room, pb.Phase_SONG_SELECT)
	if snap := conns[1].room(); snap.DeadlineMs != 0 || !snap.Members[1].Picked {
		t.Fatalf("while drawing: %v", snap)
	}
	// A pick that comes now is too late.
	w.must(room.PickSong(uid(1), songA.Source, songA.ChartID, "Easy"))
	// Player 2 leaves before the answer: the round is given up.
	w.m.Leave(uid(2))
	w.phase(room, pb.Phase_LOBBY)
	w.settle()
	w.phase(room, pb.Phase_LOBBY)
	if snap := conns[1].room(); snap.Song != nil {
		t.Fatalf("a song was drawn for a round that was given up: %v", snap)
	}
	// And when another round has begun by the time the answer comes, it is not that round's.
	w.join(2, byCode(room.Code()))
	w.must(room.LockMembers(uid(1)))
	w.must(room.PickSong(uid(1), "", "", ""))
	w.must(room.PickSong(uid(2), "", "", ""))
	w.m.Leave(uid(2))
	w.join(2, byCode(room.Code()))
	w.must(room.LockMembers(uid(1)))
	w.must(room.PickSong(uid(1), songA.Source, songA.ChartID, "Easy"))
	w.settle()
	w.phase(room, pb.Phase_SONG_SELECT)
	if snap := conns[1].room(); snap.Round != 3 || snap.Song != nil {
		t.Fatalf("round 2's song was drawn in round 3: %v", snap)
	}
	// The held answer drawn in time is used.
	w.must(room.PickSong(uid(1), "", "", "Easy"))
	w.must(room.PickSong(uid(2), "", "", ""))
	w.phase(room, pb.Phase_SONG_SELECT)
	w.settle()
	w.phase(room, pb.Phase_SONG_DRAW)
	if snap := conns[1].room(); snap.Song.ChartId != songB.ChartID || snap.Members[0].Course != "Oni" {
		t.Fatalf("round 3: %v", snap)
	}
}

func TestNoChartsAndCatalogFailureGoBackToTheLobby(t *testing.T) {
	w := newWorld(t, nil)
	room, conns := w.private(2)
	w.must(room.LockMembers(uid(1)))
	w.catalog.random = nil
	w.advance(w.m.Timings().SongSelect)
	w.phase(room, pb.Phase_LOBBY)
	if got := conns[1].notices(); !reflect.DeepEqual(got, []string{"NO_CHARTS"}) {
		t.Fatalf("notices: %v", got)
	}
	w.must(room.LockMembers(uid(1)))
	w.catalog.randomErr = errors.New("database is down")
	w.must(room.PickSong(uid(1), "", "", ""))
	w.must(room.PickSong(uid(2), "", "", ""))
	w.phase(room, pb.Phase_LOBBY)
	if got := conns[2].notices(); !reflect.DeepEqual(got, []string{"NO_CHARTS", "SERVER_ERROR"}) {
		t.Fatalf("notices: %v", got)
	}
	if len(w.logs) != 1 {
		t.Fatalf("logged: %v", w.logs)
	}
}

func TestCoursePhaseEndsByItselfWithTheDefaults(t *testing.T) {
	w := newWorld(t, nil)
	room, conns := w.private(2)
	w.must(room.LockMembers(uid(1)))
	tm := w.m.Timings()
	w.advance(tm.SongSelect + tm.SongDraw)
	w.phase(room, pb.Phase_COURSE)
	w.advance(tm.Course)
	w.phase(room, pb.Phase_LOADING)
	for _, m := range conns[1].room().Members {
		if m.Course != "Oni" || !m.Playing || !m.Ready {
			t.Fatalf("member: %v", m)
		}
	}
}

func TestPlayStartsOnceEveryoneLoaded(t *testing.T) {
	w := newWorld(t, nil)
	room, conns := w.private(2)
	w.must(room.LockMembers(uid(1)))
	tm := w.m.Timings()
	w.advance(tm.SongSelect + tm.SongDraw + tm.Course)
	w.phase(room, pb.Phase_LOADING)
	w.must(room.Loaded(uid(1), true))
	w.phase(room, pb.Phase_LOADING)
	if !conns[2].member(uid(1)).Loaded || len(conns[1].starts()) != 0 {
		t.Fatal("loaded is not shown, or the play began early")
	}
	w.must(room.Loaded(uid(2), true))
	w.phase(room, pb.Phase_PLAYING)
	for i := 1; i <= 2; i++ {
		if got := conns[i].starts(); len(got) != 1 || got[0].Round != 1 {
			t.Fatalf("player %d was told to start: %v", i, got)
		}
	}
	// start_play comes before the snapshot that says PLAYING.
	all := conns[1].all()
	if all[len(all)-2].GetStartPlay() == nil || all[len(all)-1].GetRoom().GetPhase() != pb.Phase_PLAYING {
		t.Fatalf("the last two messages: %v", all[len(all)-2:])
	}
	if got := conns[1].room().DeadlineMs; got != w.clock.Now().Add(15*time.Minute).UnixMilli() {
		t.Fatalf("deadline %d", got)
	}
}

func TestPlayerWhoDoesNotLoadInTimeSitsOut(t *testing.T) {
	w := newWorld(t, nil)
	room, conns := w.private(2)
	w.must(room.LockMembers(uid(1)))
	tm := w.m.Timings()
	w.advance(tm.SongSelect + tm.SongDraw + tm.Course)
	w.must(room.Loaded(uid(1), true))
	w.advance(tm.Loading - time.Millisecond)
	w.phase(room, pb.Phase_LOADING)
	w.advance(time.Millisecond)
	w.phase(room, pb.Phase_PLAYING)
	if len(conns[1].starts()) != 1 || len(conns[2].starts()) != 0 {
		t.Fatal("start_play went to the wrong players")
	}
	if p2 := conns[2].member(uid(2)); p2.Playing || !conns[2].member(uid(1)).Playing {
		t.Fatalf("who plays: %v", conns[2].room().Members)
	}
	// The one who sits out is not in the result, and stays in the room.
	w.must(room.Status(uid(2), &pb.PlayerStatus{Score: 99}))
	w.must(room.Finish(uid(1), &pb.PlayerStatus{Score: 500, Good: 5}, true))
	w.phase(room, pb.Phase_RESULT)
	result := conns[2].room().Result
	if len(result.Players) != 1 || result.Players[0].UserId != uid(1) || result.Players[0].Award != pb.Award_NO_AWARD || result.TeamScore != 500 {
		t.Fatalf("result: %v", result)
	}
	if !room.Has(uid(2)) {
		t.Fatal("the player who sat out was removed")
	}
}

func TestPlayerWhoCannotLoadSitsOut(t *testing.T) {
	w := newWorld(t, nil)
	room, conns := w.private(3)
	w.must(room.LockMembers(uid(1)))
	tm := w.m.Timings()
	w.advance(tm.SongSelect + tm.SongDraw + tm.Course)
	w.must(room.Loaded(uid(1), true))
	w.must(room.Loaded(uid(2), false))
	w.phase(room, pb.Phase_LOADING)
	w.must(room.Loaded(uid(3), true))
	w.phase(room, pb.Phase_PLAYING)
	if len(conns[2].starts()) != 0 || conns[1].member(uid(2)).Playing {
		t.Fatal("the player who could not load plays")
	}
}

func TestNobodyLoadedGoesBackToTheLobby(t *testing.T) {
	w := newWorld(t, nil)
	room, conns := w.private(2)
	w.must(room.LockMembers(uid(1)))
	tm := w.m.Timings()
	w.advance(tm.SongSelect + tm.SongDraw + tm.Course)
	w.must(room.Loaded(uid(1), false))
	w.must(room.Loaded(uid(2), false))
	w.phase(room, pb.Phase_LOBBY)
	if got := conns[1].notices(); !reflect.DeepEqual(got, []string{"LOAD_FAILED"}) {
		t.Fatalf("notices: %v", got)
	}
	// And when the time runs out with nobody loaded.
	w.must(room.LockMembers(uid(1)))
	w.advance(tm.SongSelect + tm.SongDraw + tm.Course + tm.Loading)
	w.phase(room, pb.Phase_LOBBY)
	if got := conns[1].notices(); !reflect.DeepEqual(got, []string{"LOAD_FAILED", "LOAD_FAILED"}) {
		t.Fatalf("notices: %v", got)
	}
}

func TestTooFewPlayersGoBackToTheLobby(t *testing.T) {
	for name, reach := range map[string]func(w *world, room *Room){
		"SONG_SELECT": func(w *world, room *Room) {},
		"SONG_DRAW":   func(w *world, room *Room) { w.advance(w.m.Timings().SongSelect) },
		"COURSE":      func(w *world, room *Room) { w.advance(w.m.Timings().SongSelect + w.m.Timings().SongDraw) },
	} {
		t.Run(name, func(t *testing.T) {
			w := newWorld(t, nil)
			room, conns := w.private(2)
			w.must(room.LockMembers(uid(1)))
			reach(w, room)
			if got := room.Phase().String(); got != name {
				t.Fatalf("in %s", got)
			}
			w.m.Leave(uid(2))
			w.phase(room, pb.Phase_LOBBY)
			if got := conns[1].notices(); !reflect.DeepEqual(got, []string{"NOT_ENOUGH_PLAYERS"}) {
				t.Fatalf("notices: %v", got)
			}
			if snap := conns[1].room(); snap.DeadlineMs != 0 || len(snap.Members) != 1 || snap.Members[0].Playing || snap.Members[0].Ready {
				t.Fatalf("snapshot: %v", snap)
			}
			// Nothing of the round is left to fire.
			w.advance(time.Hour)
			w.phase(room, pb.Phase_LOBBY)
		})
	}
}

func TestDroppingOutOfARoundBeforeThePlay(t *testing.T) {
	w := newWorld(t, nil)
	room, conns := w.private(3)
	w.must(room.LockMembers(uid(1)))
	// Two of three still make a round; the phase no longer waits for the one who dropped.
	w.must(room.PickSong(uid(1), songA.Source, songA.ChartID, "Oni"))
	w.must(room.PickSong(uid(2), songA.Source, songA.ChartID, "Oni"))
	w.phase(room, pb.Phase_SONG_SELECT)
	room.Disconnected(uid(3), conns[3])
	w.phase(room, pb.Phase_SONG_DRAW)
	w.advance(w.m.Timings().SongDraw)
	w.must(room.SetReady(uid(1), true))
	w.must(room.SetReady(uid(2), true))
	w.phase(room, pb.Phase_LOADING)
	if conns[1].member(uid(3)).Playing {
		t.Fatal("a disconnected member was put in the play")
	}
	w.must(room.Loaded(uid(1), true))
	w.must(room.Loaded(uid(2), true))
	w.phase(room, pb.Phase_PLAYING)
}

func TestFullRoundOfTwo(t *testing.T) {
	w := newWorld(t, nil)
	room, conns := w.playing()
	tm := w.m.Timings()

	// Statuses go to everyone in the play, every 250 ms while something changed.
	w.must(room.Status(uid(1), &pb.PlayerStatus{Score: 1000, Combo: 10, MaxCombo: 10, Good: 8, Ok: 2, Gauge: 1200, MusicTimeMs: 5000}))
	if len(conns[2].statuses()) != 0 {
		t.Fatal("a status was sent on at once")
	}
	w.advance(tm.StatusInterval)
	sent := conns[2].statuses()
	if len(sent) != 1 || len(sent[0].Members) != 2 {
		t.Fatalf("statuses: %v", sent)
	}
	first := sent[0].Members[0]
	if first.UserId != uid(1) || !first.Connected || first.Finished || first.Status.Score != 1000 || first.Status.Combo != 10 || first.Status.Gauge != 1200 || first.Status.MusicTimeMs != 5000 {
		t.Fatalf("status: %v", first)
	}
	if sent[0].Members[1].Status.Score != 0 {
		t.Fatalf("player 2's status: %v", sent[0].Members[1])
	}
	// Nothing changed: nothing is sent.
	w.advance(4 * tm.StatusInterval)
	if len(conns[2].statuses()) != 1 || len(conns[1].statuses()) != 1 {
		t.Fatal("statuses were sent with nothing changed")
	}
	// Several statuses within one interval go out as one.
	w.must(room.Status(uid(2), &pb.PlayerStatus{Score: 300}))
	w.must(room.Status(uid(2), &pb.PlayerStatus{Score: 400}))
	w.advance(tm.StatusInterval)
	if sent = conns[1].statuses(); len(sent) != 2 || sent[1].Members[1].Status.Score != 400 {
		t.Fatalf("statuses: %v", sent)
	}

	// Player 1: all 良, cleared. Player 2: some 可 and one 不可, not cleared.
	w.must(room.Finish(uid(1), &pb.PlayerStatus{Score: 900_000, MaxCombo: 500, Good: 500, Drumroll: 40, Gauge: 10000}, true))
	w.phase(room, pb.Phase_PLAYING)
	if !conns[2].member(uid(1)).Finished {
		t.Fatal("finished is not shown")
	}
	// A status after finishing changes nothing.
	w.must(room.Status(uid(1), &pb.PlayerStatus{Score: 1}))
	w.must(room.Finish(uid(1), &pb.PlayerStatus{Score: 2}, false))
	w.must(room.Finish(uid(2), &pb.PlayerStatus{Score: 600_000, MaxCombo: 100, Good: 300, Ok: 50, Bad: 1, Gauge: 5500}, false))
	w.phase(room, pb.Phase_RESULT)

	// The last statuses went out with the result.
	if sent = conns[1].statuses(); !sent[len(sent)-1].Members[0].Finished || sent[len(sent)-1].Members[1].Status.Score != 600_000 {
		t.Fatalf("last statuses: %v", sent[len(sent)-1])
	}
	snap := conns[2].room()
	if snap.DeadlineMs != w.clock.Now().Add(10*time.Second).UnixMilli() {
		t.Fatalf("deadline %d", snap.DeadlineMs)
	}
	result := snap.Result
	if result.Round != 1 || result.TeamScore != 1_500_000 || result.Source != "fanmade" || result.ChartId != songA.ChartID || result.Title != "Alpha" || len(result.Players) != 2 {
		t.Fatalf("result: %v", result)
	}
	p1, p2 := result.Players[0], result.Players[1]
	if p1.UserId != uid(1) || p1.Nickname != "P1" || p1.Rank != 1 || p1.Award != pb.Award_SUPER_STAR || p1.Course != "Oni" || p1.Level != 9 || p1.Slot != 0 ||
		!p1.Cleared || p1.Disconnected || !p1.FullCombo || !p1.AllGood || p1.Status.Score != 900_000 || p1.Status.Drumroll != 40 {
		t.Fatalf("first: %v", p1)
	}
	if p2.UserId != uid(2) || p2.Rank != 2 || p2.Award != pb.Award_NO_AWARD || p2.Course != "Hard" || p2.Level != 7 || p2.Slot != 1 ||
		p2.Cleared || p2.Disconnected || p2.FullCombo || p2.AllGood || p2.Status.Bad != 1 {
		t.Fatalf("second: %v", p2)
	}
	for _, m := range snap.Members {
		if m.Playing || m.Ready || m.Loaded {
			t.Fatalf("member after the round: %v", m)
		}
	}

	// Back in the lobby with the same members, the result still shown.
	w.advance(tm.Result)
	w.phase(room, pb.Phase_LOBBY)
	snap = conns[1].room()
	if snap.Result.GetRound() != 1 || len(snap.Members) != 2 || snap.DeadlineMs != 0 || snap.HostId != uid(1) {
		t.Fatalf("lobby after the round: %v", snap)
	}
	// What was picked is hidden again; the next round begins clean.
	if snap.Members[1].PickedChartId != "" {
		t.Fatal("picks show in the lobby")
	}
	w.must(room.LockMembers(uid(1)))
	snap = conns[1].room()
	if snap.Round != 2 || snap.Result != nil || snap.Song != nil || snap.SongPickedBy != "" || snap.Members[0].Picked || snap.Members[0].Finished || snap.Members[0].Course != "" {
		t.Fatalf("round 2: %v", snap)
	}
	// No timer of round 1 is left: round 2 runs its own course.
	w.advance(tm.SongSelect - time.Millisecond)
	w.phase(room, pb.Phase_SONG_SELECT)
}

func TestStatusesAreClamped(t *testing.T) {
	huge := &pb.PlayerStatus{Score: 2_000_000_000, Combo: 1 << 30, MaxCombo: 1 << 30, Good: 1 << 30, Ok: 1 << 30, Bad: 1 << 30, Drumroll: 1 << 30, Gauge: 20_000, MusicTimeMs: 1 << 30}
	if got := sanitizeStatus(huge); got.Score != 100_000_000 || got.Combo != 100_000 || got.MaxCombo != 100_000 || got.Good != 100_000 || got.Ok != 100_000 || got.Bad != 100_000 || got.Drumroll != 100_000 || got.Gauge != 10_000 || got.MusicTimeMs != 3_600_000 {
		t.Fatalf("too large: %v", got)
	}
	negative := &pb.PlayerStatus{Score: -5, Combo: -1, MaxCombo: -1, Good: -1, Ok: -1, Bad: -1, Drumroll: -1, Gauge: -1, MusicTimeMs: -1}
	if got := sanitizeStatus(negative); got.Score != 0 || got.Combo != 0 || got.MaxCombo != 0 || got.Good != 0 || got.Ok != 0 || got.Bad != 0 || got.Drumroll != 0 || got.Gauge != 0 || got.MusicTimeMs != 0 {
		t.Fatalf("negative: %v", got)
	}
	if got := sanitizeStatus(nil); got == nil || got.Score != 0 {
		t.Fatalf("nil: %v", got)
	}
	inRange := &pb.PlayerStatus{Score: 123456, Combo: 7, MaxCombo: 70, Good: 60, Ok: 9, Bad: 1, Drumroll: 33, Gauge: 9999, MusicTimeMs: 61000}
	if got := sanitizeStatus(inRange); got.Score != 123456 || got.Combo != 7 || got.MaxCombo != 70 || got.Good != 60 || got.Ok != 9 || got.Bad != 1 || got.Drumroll != 33 || got.Gauge != 9999 || got.MusicTimeMs != 61000 {
		t.Fatalf("in range: %v", got)
	}

	// And in a play: what the others see and what the result counts.
	w := newWorld(t, nil)
	room, conns := w.playing()
	w.must(room.Status(uid(1), huge))
	w.advance(w.m.Timings().StatusInterval)
	if got := conns[2].statuses()[0].Members[0].Status; got.Score != 100_000_000 || got.Gauge != 10_000 {
		t.Fatalf("status sent on: %v", got)
	}
	w.must(room.Finish(uid(1), huge, true))
	w.must(room.Finish(uid(2), negative, true))
	result := conns[1].room().Result
	if result.TeamScore != 100_000_000 || result.Players[0].Status.Good != 100_000 || result.Players[1].Status.Score != 0 {
		t.Fatalf("result: %v", result)
	}
}

func TestPlayerWhoDropsDuringThePlayIsListedAsDisconnected(t *testing.T) {
	w := newWorld(t, nil)
	room, conns := w.playing()
	w.must(room.Status(uid(2), &pb.PlayerStatus{Score: 5000, Good: 5}))
	room.Disconnected(uid(2), conns[2])
	w.advance(w.m.Timings().StatusInterval)
	if got := conns[1].statuses(); len(got) != 1 || got[0].Members[1].Connected {
		t.Fatalf("statuses: %v", got)
	}
	// Still listed while the play runs, also after the grace period.
	w.advance(w.m.Timings().ReconnectGrace)
	w.phase(room, pb.Phase_PLAYING)
	if !room.Has(uid(2)) || conns[1].member(uid(2)).Connected || !conns[1].member(uid(2)).Playing {
		t.Fatalf("the dropped player during the play: %v", conns[1].member(uid(2)))
	}
	w.must(room.Finish(uid(1), &pb.PlayerStatus{Score: 800, Good: 1}, true))
	w.phase(room, pb.Phase_RESULT)
	result := conns[1].room().Result
	if len(result.Players) != 2 || result.TeamScore != 5800 {
		t.Fatalf("result: %v", result)
	}
	// The one who finished comes first, whatever the scores; one finisher earns no award.
	p1, p2 := result.Players[0], result.Players[1]
	if p1.UserId != uid(1) || p1.Disconnected || p1.Rank != 1 || p1.Award != pb.Award_NO_AWARD {
		t.Fatalf("first: %v", p1)
	}
	if p2.UserId != uid(2) || !p2.Disconnected || p2.Rank != 2 || p2.Status.Score != 5000 || p2.Cleared || p2.FullCombo || p2.AllGood {
		t.Fatalf("second: %v", p2)
	}
	if room.Has(uid(2)) || len(conns[1].room().Members) != 1 {
		t.Fatal("the dropped player is still a member after the round")
	}
}

func TestPlayerWhoComesBackDuringThePlayGoesOn(t *testing.T) {
	w := newWorld(t, nil)
	room, conns := w.playing()
	room.Disconnected(uid(2), conns[2])
	w.advance(w.m.Timings().ReconnectGrace - time.Second)
	_, again := w.join(2, byCode(room.Code()))
	if snap := again.room(); snap.Phase != pb.Phase_PLAYING || !again.member(uid(2)).Playing || !again.member(uid(2)).Connected {
		t.Fatalf("snapshot on coming back: %v", snap)
	}
	w.must(room.Finish(uid(1), &pb.PlayerStatus{Score: 10, Good: 1}, true))
	w.phase(room, pb.Phase_PLAYING)
	w.must(room.Finish(uid(2), &pb.PlayerStatus{Score: 20, Good: 1}, true))
	w.phase(room, pb.Phase_RESULT)
	if p := again.room().Result.Players[0]; p.UserId != uid(2) || p.Disconnected || p.Award != pb.Award_SUPER_STAR {
		t.Fatalf("first: %v", p)
	}
}

func TestLeavingDuringThePlay(t *testing.T) {
	w := newWorld(t, nil)
	room, conns := w.playing()
	room.Leave(uid(2))
	if conns[2].closedCode() != "LEFT" {
		t.Fatalf("closed with %q", conns[2].closedCode())
	}
	w.phase(room, pb.Phase_PLAYING)
	w.must(room.Finish(uid(1), &pb.PlayerStatus{Score: 1}, false))
	w.phase(room, pb.Phase_RESULT)
	if got := conns[1].room().Result.Players; len(got) != 2 || !got[1].Disconnected {
		t.Fatalf("result: %v", got)
	}
	// When everyone has gone, the room closes with its round.
	w2 := newWorld(t, nil)
	room2, _ := w2.playing()
	room2.Leave(uid(1))
	room2.Leave(uid(2))
	if !room2.Closed() || w2.m.RoomCount() != 0 {
		t.Fatal("a room nobody is in stayed open")
	}
}

func TestPlayEndsAWhileAfterTheFirstFinish(t *testing.T) {
	w := newWorld(t, nil)
	room, conns := w.playing()
	w.must(room.Status(uid(2), &pb.PlayerStatus{Score: 700, Good: 3}))
	w.must(room.Finish(uid(1), &pb.PlayerStatus{Score: 1}, false))
	w.advance(w.m.Timings().FinishGrace - time.Millisecond)
	w.phase(room, pb.Phase_PLAYING)
	w.advance(time.Millisecond)
	w.phase(room, pb.Phase_RESULT)
	// The one still playing is listed with its last status, as one who did not finish.
	if p := conns[2].room().Result.Players[1]; p.UserId != uid(2) || !p.Disconnected || p.Status.Score != 700 {
		t.Fatalf("second: %v", p)
	}
	if !room.Has(uid(2)) {
		t.Fatal("a connected player was removed")
	}
}

func TestNoPlayIsLongerThanTheLimit(t *testing.T) {
	w := newWorld(t, nil)
	room, conns := w.playing()
	w.advance(w.m.Timings().MaxPlay)
	w.phase(room, pb.Phase_RESULT)
	if got := conns[1].room().Result.Players; len(got) != 2 || !got[0].Disconnected || !got[1].Disconnected {
		t.Fatalf("result: %v", got)
	}
	w.advance(w.m.Timings().Result)
	w.phase(room, pb.Phase_LOBBY)
}

func TestStamps(t *testing.T) {
	w := newWorld(t, nil)
	room, conns := w.private(2)
	w.must(room.Stamp(uid(1), 3))
	// At most one a second per player.
	w.must(room.Stamp(uid(1), 4))
	w.advance(w.m.Timings().StampInterval - time.Millisecond)
	w.must(room.Stamp(uid(1), 5))
	w.must(room.Stamp(uid(2), 15))
	w.advance(time.Millisecond)
	w.must(room.Stamp(uid(1), 0))
	want := []*pb.StampSent{{UserId: uid(1), StampId: 3}, {UserId: uid(2), StampId: 15}, {UserId: uid(1), StampId: 0}}
	for i := 1; i <= 2; i++ {
		got := conns[i].stamps()
		if len(got) != len(want) {
			t.Fatalf("player %d got %v", i, got)
		}
		for j := range want {
			if got[j].UserId != want[j].UserId || got[j].StampId != want[j].StampId {
				t.Fatalf("player %d got %v", i, got)
			}
		}
	}
	w.refused(room.Stamp(uid(1), 16), "BAD_REQUEST")
	w.refused(room.Stamp(uid(1), -1), "BAD_REQUEST")
	w.refused(room.Stamp(uid(9), 1), "NOT_IN_ROOM")
}

func TestNoStampsFromPlayersInThePlay(t *testing.T) {
	w := newWorld(t, func(tm *Timings) { tm.Loading = time.Minute })
	room, conns := w.private(3)
	w.must(room.LockMembers(uid(1)))
	tm := w.m.Timings()
	w.advance(tm.SongSelect + tm.SongDraw + tm.Course)
	w.must(room.Loaded(uid(1), true))
	w.must(room.Loaded(uid(2), true))
	w.must(room.Loaded(uid(3), false))
	w.phase(room, pb.Phase_PLAYING)
	w.must(room.Stamp(uid(1), 1))
	if len(conns[2].stamps()) != 0 {
		t.Fatal("a player in the play sent a stamp")
	}
	// The one sitting out may.
	w.must(room.Stamp(uid(3), 2))
	if got := conns[1].stamps(); len(got) != 1 || got[0].UserId != uid(3) {
		t.Fatalf("stamps: %v", got)
	}
}

/* ---------- ranking ---------- */

func TestRanking(t *testing.T) {
	p := func(id string, status *pb.PlayerStatus) played {
		return played{userID: id, nickname: id, course: "Oni", level: 8, status: sanitizeStatus(status)}
	}
	order := func(result *pb.PlayResult) (ids string) {
		for _, x := range result.Players {
			ids += x.UserId
		}
		return ids
	}
	score := func(n int32) *pb.PlayerStatus { return &pb.PlayerStatus{Score: n, Good: 1} }

	// MVP, unless the top score is 30 % ahead of the second finisher's.
	for _, c := range []struct {
		scores []int32
		want   pb.Award
	}{
		{[]int32{100, 90}, pb.Award_MVP},
		{[]int32{129, 100}, pb.Award_MVP},
		{[]int32{130, 100}, pb.Award_SUPER_STAR},
		{[]int32{13, 10, 12}, pb.Award_MVP},
		{[]int32{100_000_000, 76_923_076}, pb.Award_SUPER_STAR},
		{[]int32{100_000_000, 76_923_077}, pb.Award_MVP},
		{[]int32{100, 100}, pb.Award_MVP},
		{[]int32{130}, pb.Award_NO_AWARD},
		{[]int32{0, 0}, pb.Award_NO_AWARD},
	} {
		var players []played
		for i, s := range c.scores {
			players = append(players, p(string(rune('a'+i)), score(s)))
		}
		result := rankResults(players, 1, songA)
		if got := result.Players[0].Award; got != c.want {
			t.Errorf("scores %v: the top got %v, want %v", c.scores, got, c.want)
		}
		for i, x := range result.Players {
			if x.Rank != int32(i+1) || i > 0 && x.Award != pb.Award_NO_AWARD {
				t.Errorf("scores %v: place %d: %v", c.scores, i, x)
			}
		}
	}

	// Ties in score go by 良, then max combo, then the gauge; equal players keep their order.
	result := rankResults([]played{
		p("a", &pb.PlayerStatus{Score: 100, Good: 1, MaxCombo: 9, Gauge: 9}),
		p("b", &pb.PlayerStatus{Score: 100, Good: 2, MaxCombo: 1, Gauge: 1}),
		p("c", &pb.PlayerStatus{Score: 100, Good: 2, MaxCombo: 5, Gauge: 1}),
		p("d", &pb.PlayerStatus{Score: 100, Good: 2, MaxCombo: 5, Gauge: 7}),
		p("e", &pb.PlayerStatus{Score: 100, Good: 2, MaxCombo: 5, Gauge: 7}),
		p("f", &pb.PlayerStatus{Score: 101}),
	}, 3, songB)
	if got := order(result); got != "fdecba" {
		t.Fatalf("order %s", got)
	}
	if result.Round != 3 || result.TeamScore != 601 || result.Source != "official" || result.ChartId != songB.ChartID || result.Title != "Beta" {
		t.Fatalf("result: %v", result)
	}

	// Players who did not finish come last and earn nothing, however high their score; the
	// award goes by the finishers.
	gone := p("x", score(999))
	gone.disconnected, gone.cleared = true, true
	result = rankResults([]played{gone, p("a", score(10)), p("b", score(20))}, 1, songA)
	if got := order(result); got != "bax" {
		t.Fatalf("order %s", got)
	}
	if result.Players[0].Award != pb.Award_SUPER_STAR || result.Players[2].Award != pb.Award_NO_AWARD || result.Players[2].Cleared || result.Players[2].FullCombo || !result.Players[2].Disconnected || result.TeamScore != 1029 {
		t.Fatalf("result: %v", result)
	}
	result = rankResults([]played{gone, p("a", score(10))}, 1, songA)
	if result.Players[0].Award != pb.Award_NO_AWARD {
		t.Fatalf("one finisher got %v", result.Players[0].Award)
	}

	// Full combo: no 不可 and something hit. All good: full combo with no 可.
	for _, c := range []struct {
		status             *pb.PlayerStatus
		fullCombo, allGood bool
	}{
		{&pb.PlayerStatus{Good: 10}, true, true},
		{&pb.PlayerStatus{Good: 10, Ok: 1}, true, false},
		{&pb.PlayerStatus{Ok: 3}, true, false},
		{&pb.PlayerStatus{Good: 10, Bad: 1}, false, false},
		{&pb.PlayerStatus{Drumroll: 50, Score: 500}, false, false},
		{&pb.PlayerStatus{}, false, false},
	} {
		got := rankResults([]played{p("a", c.status)}, 1, songA).Players[0]
		if got.FullCombo != c.fullCombo || got.AllGood != c.allGood {
			t.Errorf("%v: full combo %v, all good %v", c.status, got.FullCombo, got.AllGood)
		}
	}
}
