package multiplay

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	pb "ourtaiko.dev/net/internal/multiplay/multiplaypb"
)

// member is one player's place in a room.
type member struct {
	Player
	// slot is the joined order; it is never given to another member.
	slot int32
	// conn is nil while the player is disconnected.
	conn Conn
	// dropTimer runs while a disconnected player's place is kept.
	dropTimer *timer
	// SONG_SELECT: picked says the player has chosen; pick is the song, nil for おまかせ.
	picked bool
	pick   *Song
	// preferred is the course asked for with this round's pick, lastCourse the one played in
	// the last round; both may be empty.
	preferred  string
	lastCourse string
	// course is the course of the drawn song this player plays.
	course   string
	ready    bool
	loaded   bool
	playing  bool
	finished bool
	cleared  bool
	status   *pb.PlayerStatus
	// lastStamp is when the player last sent a stamp; stamped is false until the first.
	lastStamp time.Time
	stamped   bool
	// protocol is the version the player's game speaks; away, from a play's start, that the
	// player has not come back to the room's screen (never set below protocol 2, whose games
	// do not say when they are back).
	protocol int32
	away     bool
}

// Room is one room. Its exported methods are what players ask of it; each takes the manager's
// mutex. The others expect it held.
type Room struct {
	m      *Manager
	code   string
	public bool
	phase  pb.Phase
	hostID string
	round  int32
	closed bool
	// members are in joined order.
	members  []*member
	nextSlot int32

	// deadline is when phaseTimer fires; zero when the phase has no deadline.
	deadline    time.Time
	phaseTimer  *timer
	statusTimer *timer
	finishTimer *timer

	// The drawn song, its message (made once, sent with every snapshot) and who picked it.
	song         *Song
	songMessage  *pb.Song
	songPickedBy string
	result       *pb.PlayResult
	// drawing: the round's song is being drawn; the catalog may be being asked.
	drawing     bool
	statusDirty bool
	// dirty: a snapshot is owed to the members (Manager.unlock).
	dirty bool
}

func (r *Room) t() Timings { return r.m.deps.Timings }

// Code is the room's 5-digit number.
func (r *Room) Code() string { return r.code }

// Phase is the phase the room is in.
func (r *Room) Phase() pb.Phase {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	return r.phase
}

// Closed reports whether the room is gone.
func (r *Room) Closed() bool {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	return r.closed
}

// Has reports whether the player has a place in the room, connected or not.
func (r *Room) Has(userID string) bool {
	r.m.mu.Lock()
	defer r.m.mu.Unlock()
	return r.find(userID) != nil
}

func (r *Room) find(userID string) *member {
	for _, m := range r.members {
		if m.UserID == userID {
			return m
		}
	}
	return nil
}

// member is the player's place, or the error every request of a stranger gets.
func (r *Room) member(userID string) (*member, error) {
	if m := r.find(userID); m != nil {
		return m, nil
	}
	return nil, errNotInRoom
}

func (r *Room) full() bool { return len(r.members) >= r.t().MaxMembers }

// present is how many members are connected.
func (r *Room) present() int {
	n := 0
	for _, m := range r.members {
		if m.conn != nil {
			n++
		}
	}
	return n
}

// acceptsQuickMatch: a quick match may put a new player here.
func (r *Room) acceptsQuickMatch() bool {
	return r.public && !r.closed && r.phase == pb.Phase_LOBBY && !r.full()
}

/* ---------- joining and leaving ---------- */

// join adds a player, or gives a member that dropped (or connects again from elsewhere) its
// place back.
func (r *Room) join(player Player, conn Conn, protocol int32) error {
	if r.closed {
		return refuse("ROOM_CLOSED", "The room was closed")
	}
	if existing := r.find(player.UserID); existing != nil {
		if existing.conn != nil && existing.conn != conn {
			existing.conn.Close("REPLACED", "This account entered the room from elsewhere")
		}
		existing.dropTimer.cancel()
		existing.dropTimer = nil
		existing.conn = conn
		// Where the player is has not changed with the connection: away stays as it was,
		// unless the game that came back cannot say when it returns.
		existing.protocol = protocol
		existing.away = existing.away && protocol >= 2
		r.maybeStartFull()
		existing.Nickname, existing.AvatarURL = player.Nickname, player.AvatarURL
		r.changed()
		return nil
	}
	if r.phase != pb.Phase_LOBBY {
		return refuse("ROOM_BUSY", "The room is in the middle of a round; try again later")
	}
	if r.full() {
		return refuse("ROOM_FULL", "The room is full")
	}
	r.members = append(r.members, &member{Player: player, slot: r.nextSlot, conn: conn, status: &pb.PlayerStatus{}, protocol: protocol})
	r.nextSlot++
	if r.hostID == "" {
		r.hostID = player.UserID
	}
	r.maybeStartFull()
	r.changed()
	return nil
}

// Disconnected is called when a member's stream ended without `leave`: the place is kept for
// a while. conn is the stream that ended; a member that has another by now is left alone.
func (r *Room) Disconnected(userID string, conn Conn) {
	r.m.mu.Lock()
	defer r.m.unlock()
	m := r.find(userID)
	if m == nil || m.conn != conn {
		return
	}
	m.conn = nil
	m.dropTimer = r.m.after(r.t().ReconnectGrace, func() {
		m.dropTimer = nil
		if m.conn == nil {
			r.drop(m)
		}
	})
	r.changed()
	r.afterPresenceChange()
}

// Leave takes the player out of the room and closes their connection (LEFT).
func (r *Room) Leave(userID string) {
	r.m.mu.Lock()
	defer r.m.unlock()
	r.leave(userID)
}

func (r *Room) leave(userID string) {
	m := r.find(userID)
	if m == nil {
		return
	}
	conn := m.conn
	m.conn = nil
	if r.m.userRoom[userID] == r {
		delete(r.m.userRoom, userID)
	}
	r.drop(m)
	if conn != nil {
		conn.Close("LEFT", "Left the room")
	}
}

// Kick removes a member. Host only, LOBBY only.
func (r *Room) Kick(byUserID, userID string) error {
	r.m.mu.Lock()
	defer r.m.unlock()
	if err := r.requireHost(byUserID); err != nil {
		return err
	}
	if r.phase != pb.Phase_LOBBY {
		return refuse("BAD_PHASE", "Members can only be removed in the lobby")
	}
	if userID == byUserID {
		return refuse("BAD_REQUEST", "The host cannot remove themselves")
	}
	m := r.find(userID)
	if m == nil {
		return refuse("BAD_REQUEST", "No such member")
	}
	conn := m.conn
	m.conn = nil
	r.drop(m)
	if conn != nil {
		conn.Close("KICKED", "The host removed you from the room")
	}
	return nil
}

// drop removes a member for good, or, during a play they are in, keeps them in the round as
// disconnected until the result.
func (r *Room) drop(m *member) {
	m.dropTimer.cancel()
	m.dropTimer = nil
	if r.phase == pb.Phase_PLAYING && m.playing {
		r.changed()
		r.afterPresenceChange()
		return
	}
	r.removeMember(m)
}

func (r *Room) removeMember(m *member) {
	i := -1
	for j, x := range r.members {
		if x == m {
			i = j
		}
	}
	if i < 0 {
		return
	}
	r.members = append(r.members[:i:i], r.members[i+1:]...)
	if r.m.userRoom[m.UserID] == r {
		delete(r.m.userRoom, m.UserID)
	}
	if len(r.members) == 0 {
		r.close()
		return
	}
	if r.hostID == m.UserID {
		// The longest-standing member who is connected; anyone when nobody is.
		next := r.members[0]
		for _, x := range r.members {
			if x.conn != nil {
				next = x
				break
			}
		}
		r.hostID = next.UserID
	}
	r.changed()
	r.afterPresenceChange()
}

// removeGone removes the members that are neither connected nor in their grace period: the
// ones a play kept listed as disconnected.
func (r *Room) removeGone() {
	for _, m := range append([]*member(nil), r.members...) {
		if m.conn == nil && m.dropTimer == nil {
			r.removeMember(m)
		}
	}
}

// afterPresenceChange: someone left or dropped, so a phase that waits for everyone may be
// done now, or lack players.
func (r *Room) afterPresenceChange() {
	switch r.phase {
	case pb.Phase_SONG_SELECT, pb.Phase_SONG_DRAW, pb.Phase_COURSE:
		if r.present() < r.t().MinPlayers {
			r.backToLobby("NOT_ENOUGH_PLAYERS", "Too few players remain; back to the lobby")
			return
		}
		if r.phase == pb.Phase_SONG_SELECT {
			r.maybeEndSongSelect()
		}
		if r.phase == pb.Phase_COURSE {
			r.maybeEndCourse()
		}
	case pb.Phase_LOADING:
		r.maybeStartPlay(false)
	case pb.Phase_PLAYING:
		r.maybeFinishRound()
	}
}

// close ends the room: every member is told ROOM_CLOSED and the number is free again.
func (r *Room) close() {
	if r.closed {
		return
	}
	r.closed = true
	r.clearPhaseTimer()
	r.stopPlayTimers()
	for _, m := range r.members {
		m.dropTimer.cancel()
		m.dropTimer = nil
		if r.m.userRoom[m.UserID] == r {
			delete(r.m.userRoom, m.UserID)
		}
		if m.conn != nil {
			m.conn.Close("ROOM_CLOSED", "The room was closed")
			m.conn = nil
		}
	}
	r.members = nil
	if r.m.rooms[r.code] == r {
		delete(r.m.rooms, r.code)
	}
}

/* ---------- host actions ---------- */

func (r *Room) requireHost(userID string) error {
	if _, err := r.member(userID); err != nil {
		return err
	}
	if r.hostID != userID {
		return refuse("NOT_HOST", "Only the host can do this")
	}
	return nil
}

// OpenRoom turns a private room into a public one. Host only; it cannot be undone.
func (r *Room) OpenRoom(userID string) error {
	r.m.mu.Lock()
	defer r.m.unlock()
	if err := r.requireHost(userID); err != nil {
		return err
	}
	if r.public {
		return nil
	}
	r.public = true
	r.changed()
	return nil
}

// LockMembers begins a round with the members who are in the room. Host only, LOBBY only.
// Those still at their own result screen after the last round are left behind: they are
// taken out of the room (LEFT_BEHIND).
func (r *Room) LockMembers(userID string) error {
	r.m.mu.Lock()
	defer r.m.unlock()
	if err := r.requireHost(userID); err != nil {
		return err
	}
	if r.phase != pb.Phase_LOBBY {
		return refuse("BAD_PHASE", "A round cannot begin now")
	}
	if r.m.draining {
		return refuse("BAD_PHASE", "The server is about to restart")
	}
	// Whoever presses start is at the room's screen, whether or not `returned` has come yet.
	if host := r.find(userID); host.away {
		host.away = false
		r.changed()
	}
	if r.inRoom() < r.t().MinPlayers {
		return refuse("NOT_ENOUGH_PLAYERS", fmt.Sprintf("A round needs at least %d players in the room", r.t().MinPlayers))
	}
	for _, m := range append([]*member(nil), r.members...) {
		if !m.away {
			continue
		}
		conn := m.conn
		m.conn = nil
		r.drop(m)
		if conn != nil {
			conn.Close("LEFT_BEHIND", "The next round began before you were back in the room")
		}
	}
	r.startSongSelect()
	return nil
}

// inRoom counts the members who are at the room's screen: connected, and not away at a play
// or its result.
func (r *Room) inRoom() int {
	n := 0
	for _, m := range r.members {
		if m.conn != nil && !m.away {
			n++
		}
	}
	return n
}

// maybeStartFull begins a round by itself once the room is full and every member is in it.
// Besides the host's start this is the only way a round begins.
func (r *Room) maybeStartFull() {
	if r.phase == pb.Phase_LOBBY && !r.closed && !r.m.draining && r.full() && r.inRoom() == len(r.members) {
		r.startSongSelect()
	}
}

// Returned says the player is back at the room's screen after a play.
func (r *Room) Returned(userID string) error {
	r.m.mu.Lock()
	defer r.m.unlock()
	m, err := r.member(userID)
	if err != nil {
		return err
	}
	if !m.away {
		return nil
	}
	m.away = false
	r.changed()
	r.maybeStartFull()
	return nil
}

/* ---------- phases ---------- */

func (r *Room) setPhaseTimer(d time.Duration, f func()) {
	r.clearPhaseTimer()
	r.deadline = r.m.deps.Now().Add(d)
	r.phaseTimer = r.m.after(d, func() {
		r.phaseTimer = nil
		r.deadline = time.Time{}
		if !r.closed {
			f()
		}
	})
}

func (r *Room) clearPhaseTimer() {
	r.phaseTimer.cancel()
	r.phaseTimer = nil
	r.deadline = time.Time{}
}

func (r *Room) setPhase(phase pb.Phase) {
	r.phase = phase
	r.changed()
}

// backToLobby gives a round up: the members stay, and are told why when there is a reason.
func (r *Room) backToLobby(code, message string) {
	r.clearPhaseTimer()
	r.stopPlayTimers()
	r.drawing = false
	for _, m := range r.members {
		m.playing, m.ready, m.loaded = false, false, false
	}
	// The phase first: removing members below must not come back to this round's phases.
	r.setPhase(pb.Phase_LOBBY)
	r.removeGone()
	if r.closed {
		return
	}
	if code != "" {
		r.notice(code, message)
	}
}

func (r *Room) startSongSelect() {
	if r.closed {
		return
	}
	r.round++
	r.result = nil
	r.song, r.songMessage, r.songPickedBy = nil, nil, ""
	r.drawing = false
	for _, m := range r.members {
		m.picked, m.pick, m.preferred, m.course = false, nil, "", ""
		m.ready, m.loaded, m.playing, m.finished, m.cleared = false, false, false, false, false
		m.status = &pb.PlayerStatus{}
	}
	r.setPhase(pb.Phase_SONG_SELECT)
	r.setPhaseTimer(r.t().SongSelect, r.drawSong)
}

// PickSong is a player's choice in SONG_SELECT: a song (source and chartID) or, with both
// empty, おまかせ; course is the course the player would play, empty for no wish. A later
// pick replaces an earlier one. The catalog is asked without the mutex held.
func (r *Room) PickSong(userID, source, chartID, course string) error {
	mgr := r.m
	source, chartID, course = strings.TrimSpace(source), strings.TrimSpace(chartID), strings.TrimSpace(course)
	mgr.mu.Lock()
	_, err := r.member(userID)
	switch {
	case err != nil:
	case r.phase != pb.Phase_SONG_SELECT:
		err = refuse("BAD_PHASE", "Songs cannot be picked now")
	case (source == "") != (chartID == ""):
		err = refuse("BAD_REQUEST", "A song is a source and a chart, or neither")
	case course != "" && courseIndex(course) < 0:
		err = refuse("BAD_REQUEST", "No such course")
	}
	// Once the draw has begun the picks are settled; a late one is dropped.
	round, late := r.round, r.drawing
	mgr.unlock()
	if err != nil || late {
		return err
	}
	var pick *Song
	if chartID != "" {
		ctx, cancel := context.WithTimeout(mgr.ctx, lookupTimeout)
		pick, err = mgr.deps.Catalog.Get(ctx, source, chartID)
		cancel()
		if errors.Is(err, ErrChartNotFound) || err == nil && (pick == nil || len(pick.Courses) == 0) {
			return refuse("CHART_NOT_FOUND", "No such playable chart")
		}
		if err != nil {
			mgr.deps.Logf("multiplay: room %s: chart lookup failed: %v", r.code, err)
			return refuse("SERVER_ERROR", "The chart could not be looked up; try again")
		}
	}
	mgr.mu.Lock()
	defer mgr.unlock()
	// The room may have moved on while the catalog was asked.
	m := r.find(userID)
	if m == nil || r.closed || r.round != round || r.phase != pb.Phase_SONG_SELECT || r.drawing {
		return nil
	}
	m.picked, m.pick, m.preferred = true, pick, course
	r.changed()
	r.maybeEndSongSelect()
	return nil
}

func (r *Room) maybeEndSongSelect() {
	present := 0
	for _, m := range r.members {
		if m.conn == nil {
			continue
		}
		if !m.picked {
			return
		}
		present++
	}
	if present > 0 {
		r.drawSong()
	}
}

// drawSong settles the round's song. A song somebody picked always goes before chance: one
// of the connected members' songs is drawn, each with the same chance, and おまかせ only
// says "whatever the others pick". Only when nobody picked a song is the catalog asked for
// any song, without the mutex held.
func (r *Room) drawSong() {
	if r.phase != pb.Phase_SONG_SELECT || r.drawing {
		return
	}
	r.drawing = true
	r.clearPhaseTimer()
	r.changed()
	var songs []*member
	for _, m := range r.members {
		if m.conn == nil {
			continue
		}
		// Whoever picked nothing leaves it to chance.
		m.picked = true
		if m.pick != nil {
			songs = append(songs, m)
		}
	}
	if len(songs) > 0 {
		winner := songs[r.m.deps.Intn(len(songs))]
		r.drawn(winner.pick, winner.UserID)
		return
	}
	mgr, round := r.m, r.round
	mgr.deps.Go(func() {
		ctx, cancel := context.WithTimeout(mgr.ctx, lookupTimeout)
		song, err := mgr.deps.Catalog.Random(ctx)
		cancel()
		mgr.mu.Lock()
		defer mgr.unlock()
		// The room may have closed, gone back to the lobby or begun another round meanwhile.
		if r.closed || r.round != round || r.phase != pb.Phase_SONG_SELECT || !r.drawing {
			return
		}
		switch {
		case err != nil:
			mgr.deps.Logf("multiplay: room %s: song draw failed: %v", r.code, err)
			r.backToLobby("SERVER_ERROR", "The song could not be drawn; try again")
		case song == nil || len(song.Courses) == 0:
			r.backToLobby("NO_CHARTS", "The site has no playable charts yet")
		default:
			r.drawn(song, "")
		}
	})
}

// drawn makes song the round's song and gives every member the course they play unless they
// pick another: the one they asked for with their pick, else the one they played last round,
// else Oni, when the song has it; otherwise the nearest the song has (nearestCourse).
func (r *Room) drawn(song *Song, pickedBy string) {
	r.drawing = false
	r.song, r.songMessage, r.songPickedBy = song, song.message(), pickedBy
	for _, m := range r.members {
		preferred := m.preferred
		if preferred == "" {
			preferred = m.lastCourse
		}
		m.course = nearestCourse(song.Courses, preferred)
	}
	r.setPhase(pb.Phase_SONG_DRAW)
	r.setPhaseTimer(r.t().SongDraw, r.startCourse)
}

func (r *Room) startCourse() {
	r.setPhase(pb.Phase_COURSE)
	r.setPhaseTimer(r.t().Course, r.startLoading)
	r.maybeEndCourse()
}

// PickCourse changes the course of the drawn song the player plays, until they are ready.
func (r *Room) PickCourse(userID, course string) error {
	r.m.mu.Lock()
	defer r.m.unlock()
	m, err := r.member(userID)
	if err != nil {
		return err
	}
	if r.phase != pb.Phase_SONG_DRAW && r.phase != pb.Phase_COURSE {
		return refuse("BAD_PHASE", "Courses cannot be picked now")
	}
	if r.song == nil || !r.song.hasCourse(course) {
		return refuse("BAD_REQUEST", "The song has no such course")
	}
	if m.ready {
		return refuse("BAD_PHASE", "Already ready")
	}
	m.course = course
	r.changed()
	return nil
}

// SetReady says the player has settled on a course. Once every connected member is ready the
// round goes on without waiting for the countdown.
func (r *Room) SetReady(userID string, ready bool) error {
	r.m.mu.Lock()
	defer r.m.unlock()
	m, err := r.member(userID)
	if err != nil {
		return err
	}
	if r.phase != pb.Phase_SONG_DRAW && r.phase != pb.Phase_COURSE {
		return refuse("BAD_PHASE", "Nobody is asked to be ready now")
	}
	m.ready = ready
	r.changed()
	r.maybeEndCourse()
	return nil
}

func (r *Room) maybeEndCourse() {
	if r.phase != pb.Phase_COURSE {
		return
	}
	present := 0
	for _, m := range r.members {
		if m.conn == nil {
			continue
		}
		if !m.ready {
			return
		}
		present++
	}
	if present > 0 {
		r.startLoading()
	}
}

func (r *Room) startLoading() {
	if r.phase != pb.Phase_COURSE {
		return
	}
	for _, m := range r.members {
		m.playing = m.conn != nil
		m.ready = m.playing
		m.loaded = false
	}
	r.setPhase(pb.Phase_LOADING)
	r.setPhaseTimer(r.t().Loading, func() {
		r.maybeStartPlay(true)
	})
}

// Loaded says the player is ready to play the song, or (ok false) could not load it and sits
// the round out.
func (r *Room) Loaded(userID string, ok bool) error {
	r.m.mu.Lock()
	defer r.m.unlock()
	m, err := r.member(userID)
	if err != nil {
		return err
	}
	if r.phase != pb.Phase_LOADING || !m.playing {
		return nil
	}
	if ok {
		m.loaded = true
	} else {
		m.playing = false
	}
	r.changed()
	r.maybeStartPlay(false)
	return nil
}

// maybeStartPlay starts the play once every connected player has loaded, or, when the time
// to load is up, with those who have: the others sit the round out.
func (r *Room) maybeStartPlay(timedOut bool) {
	if r.phase != pb.Phase_LOADING {
		return
	}
	loaded := 0
	for _, m := range r.members {
		if !m.playing || m.conn == nil {
			continue
		}
		if m.loaded {
			loaded++
		} else if !timedOut {
			return
		}
	}
	if loaded == 0 {
		r.backToLobby("LOAD_FAILED", "Nobody finished loading; back to the lobby")
		return
	}
	for _, m := range r.members {
		if m.playing && !m.loaded {
			m.playing = false
		}
	}
	r.startPlay()
}

func (r *Room) startPlay() {
	r.setPhase(pb.Phase_PLAYING)
	r.setPhaseTimer(r.t().MaxPlay, r.finishRound)
	start := &pb.ServerMessage{Kind: &pb.ServerMessage_StartPlay{StartPlay: &pb.StartPlay{Round: r.round}}}
	for _, m := range r.members {
		if m.playing {
			m.lastCourse = m.course
			// At the play, and at their own result screen after it, until the game says so.
			m.away = m.protocol >= 2
			if m.conn != nil {
				m.conn.Send(start)
			}
		}
	}
	r.armStatusTimer()
}

func (r *Room) armStatusTimer() {
	r.statusTimer = r.m.after(r.t().StatusInterval, func() {
		r.statusTimer = nil
		if r.closed || r.phase != pb.Phase_PLAYING {
			return
		}
		r.flushStatuses()
		r.armStatusTimer()
	})
}

// stopPlayTimers stops what only runs during a play.
func (r *Room) stopPlayTimers() {
	r.statusTimer.cancel()
	r.statusTimer = nil
	r.finishTimer.cancel()
	r.finishTimer = nil
}

/* ---------- the play ---------- */

// Status is what a player shows the others during the play. It is sent on with everyone's,
// every Timings.StatusInterval.
func (r *Room) Status(userID string, status *pb.PlayerStatus) error {
	r.m.mu.Lock()
	defer r.m.unlock()
	m, err := r.member(userID)
	if err != nil {
		return err
	}
	if r.phase != pb.Phase_PLAYING || !m.playing || m.finished {
		return nil
	}
	m.status = sanitizeStatus(status)
	r.statusDirty = true
	return nil
}

func (r *Room) flushStatuses() {
	if !r.statusDirty {
		return
	}
	r.statusDirty = false
	statuses := &pb.PlayStatuses{}
	for _, m := range r.members {
		if m.playing {
			statuses.Members = append(statuses.Members, &pb.MemberStatus{UserId: m.UserID, Status: m.status, Connected: m.conn != nil, Finished: m.finished})
		}
	}
	r.broadcast(&pb.ServerMessage{Kind: &pb.ServerMessage_Statuses{Statuses: statuses}}, func(m *member) bool { return m.playing })
}

// Finish is a player's final status. The round ends once every connected player has
// finished, or Timings.FinishGrace after the first one did.
func (r *Room) Finish(userID string, final *pb.PlayerStatus, cleared bool) error {
	r.m.mu.Lock()
	defer r.m.unlock()
	m, err := r.member(userID)
	if err != nil {
		return err
	}
	if r.phase != pb.Phase_PLAYING || !m.playing || m.finished {
		return nil
	}
	m.status = sanitizeStatus(final)
	m.cleared = cleared
	m.finished = true
	r.statusDirty = true
	if r.finishTimer == nil {
		r.finishTimer = r.m.after(r.t().FinishGrace, func() {
			r.finishTimer = nil
			r.finishRound()
		})
	}
	r.changed()
	r.maybeFinishRound()
	return nil
}

func (r *Room) maybeFinishRound() {
	if r.phase != pb.Phase_PLAYING {
		return
	}
	for _, m := range r.members {
		if m.playing && m.conn != nil && !m.finished {
			return
		}
	}
	r.finishRound()
}

// finishRound ranks the round and shows the result, then goes back to the lobby with the
// members who are still there.
func (r *Room) finishRound() {
	if r.phase != pb.Phase_PLAYING {
		return
	}
	r.flushStatuses()
	r.stopPlayTimers()
	var players []played
	for _, m := range r.members {
		if m.playing {
			players = append(players, played{
				userID: m.UserID, nickname: m.Nickname, avatarURL: m.AvatarURL, slot: m.slot, course: m.course, level: r.song.level(m.course),
				status: m.status, cleared: m.cleared, disconnected: !m.finished,
			})
		}
	}
	r.result = rankResults(players, r.round, r.song)
	for _, m := range r.members {
		m.playing, m.ready, m.loaded = false, false, false
	}
	r.setPhase(pb.Phase_RESULT)
	// Members that dropped during the play leave now; those still in their grace period stay.
	r.removeGone()
	if r.closed {
		return
	}
	r.setPhaseTimer(r.t().Result, func() {
		r.setPhase(pb.Phase_LOBBY)
		r.maybeStartFull()
	})
}

/* ---------- stamps ---------- */

// Stamp sends a stamp to everyone in the room. A stamp sooner than Timings.StampInterval
// after the player's last, or from a player in the middle of a play, is dropped silently.
func (r *Room) Stamp(userID string, stampID int32) error {
	r.m.mu.Lock()
	defer r.m.unlock()
	m, err := r.member(userID)
	if err != nil {
		return err
	}
	if stampID < 0 || stampID >= StampCount {
		return refuse("BAD_REQUEST", "No such stamp")
	}
	if r.phase == pb.Phase_PLAYING && m.playing {
		return nil
	}
	now := r.m.deps.Now()
	if m.stamped && now.Sub(m.lastStamp) < r.t().StampInterval {
		return nil
	}
	m.stamped, m.lastStamp = true, now
	r.broadcast(&pb.ServerMessage{Kind: &pb.ServerMessage_Stamp{Stamp: &pb.StampSent{UserId: userID, StampId: stampID}}}, nil)
	return nil
}

/* ---------- messages ---------- */

func (r *Room) notice(code, message string) {
	r.broadcast(&pb.ServerMessage{Kind: &pb.ServerMessage_Notice{Notice: &pb.Notice{Code: code, Message: message}}}, nil)
}

// broadcast sends a message to the connected members, or to those of them filter keeps.
func (r *Room) broadcast(message *pb.ServerMessage, filter func(*member) bool) {
	for _, m := range r.members {
		if m.conn != nil && (filter == nil || filter(m)) {
			m.conn.Send(message)
		}
	}
}

// changed notes that the members are owed a snapshot. However many changes one operation
// makes, they get one, when the manager's mutex is released.
func (r *Room) changed() {
	if r.dirty || r.closed {
		return
	}
	r.dirty = true
	r.m.dirty = append(r.m.dirty, r)
}

func (r *Room) snapshot() *pb.Room {
	// What each player picked stays hidden until the draw.
	reveal := r.phase != pb.Phase_SONG_SELECT && r.phase != pb.Phase_LOBBY
	room := &pb.Room{
		Code: r.code, Public: r.public, Phase: r.phase, ServerTimeMs: r.m.deps.Now().UnixMilli(),
		HostId: r.hostID, MaxMembers: int32(r.t().MaxMembers), MinPlayers: int32(r.t().MinPlayers), Round: r.round,
		Song: r.songMessage, SongPickedBy: r.songPickedBy, Result: r.result,
	}
	if !r.deadline.IsZero() {
		room.DeadlineMs = r.deadline.UnixMilli()
	}
	for _, m := range r.members {
		out := &pb.Member{
			UserId: m.UserID, Nickname: m.Nickname, AvatarUrl: m.AvatarURL, Slot: m.slot, Connected: m.conn != nil,
			Picked: m.picked, Course: m.course, Ready: m.ready, Loaded: m.loaded, Playing: m.playing, Finished: m.finished,
			Away: m.away,
		}
		if reveal && m.pick != nil {
			out.PickedSource, out.PickedChartId = m.pick.Source, m.pick.ChartID
		}
		room.Members = append(room.Members, out)
	}
	return room
}
