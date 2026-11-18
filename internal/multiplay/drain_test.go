package multiplay

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	pb "ourtaiko.dev/net/internal/multiplay/multiplaypb"
)

// A server about to be replaced lets the round under way end, and begins nothing new.
func TestDrainingLetsTheRoundEndAndBeginsNoOther(t *testing.T) {
	w := newWorld(t, nil)
	room, conns := w.playing()
	waiting, _ := w.join(3, private)

	w.m.Drain(true)
	if s := w.m.DrainState(); !s.Draining || s.Rooms != 2 || s.Busy != 1 {
		t.Fatalf("while a round is under way: %+v", s)
	}
	for _, target := range []JoinTarget{quick, private, byCode(waiting.Code())} {
		_, err := w.m.Join(player(4), target, &fakeConn{})
		w.refused(err, "SERVER_FULL")
	}

	// A member who dropped comes back.
	room.Disconnected(uid(2), conns[2])
	w.join(2, byCode(room.Code()))

	w.must(room.Finish(uid(1), nil, true))
	w.must(room.Finish(uid(2), nil, true))
	w.phase(room, pb.Phase_RESULT)
	w.advance(w.m.Timings().Result)
	w.phase(room, pb.Phase_LOBBY)
	if s := w.m.DrainState(); s.Busy != 0 {
		t.Fatalf("after the round: %+v", s)
	}
	w.refused(room.LockMembers(uid(1)), "BAD_PHASE")
	w.phase(room, pb.Phase_LOBBY)
}

// A full room does not begin its next round while draining, and does once that is called off.
func TestAFullRoomWaitsForTheDrainToBeCalledOff(t *testing.T) {
	w := newWorld(t, nil)
	room, _ := w.fullRoundOfFive()
	w.advance(w.m.Timings().Result)
	w.m.Drain(true)
	for n := 1; n <= 5; n++ {
		w.must(room.Returned(uid(n)))
	}
	w.phase(room, pb.Phase_LOBBY)
	w.m.Drain(false)
	w.done()
	w.phase(room, pb.Phase_SONG_SELECT)
}

func TestTheAdminAddressDrainsAndCallsItOff(t *testing.T) {
	s := NewServer(ServerConfig{Catalog: staticCatalog{}, Authenticate: testAccounts, Logf: t.Logf})
	t.Cleanup(s.Stop)
	admin := httptest.NewServer(s.AdminHandler())
	t.Cleanup(admin.Close)
	ask := func(method, path string, want int) (state DrainState) {
		t.Helper()
		request, _ := http.NewRequest(method, admin.URL+path, nil)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != want {
			t.Fatalf("%s %s: %s", method, path, response.Status)
		}
		if want == http.StatusOK {
			if err = json.NewDecoder(response.Body).Decode(&state); err != nil {
				t.Fatal(err)
			}
		}
		return state
	}
	ask("GET", "/healthz", http.StatusNoContent)
	ask("POST", "/drain", http.StatusBadRequest)
	if ask("GET", "/drain", http.StatusOK).Draining {
		t.Fatal("draining from the start")
	}
	if !ask("POST", "/drain?for=1h", http.StatusOK).Draining || !s.Manager().DrainState().Draining {
		t.Fatal("not draining after being asked")
	}
	if ask("DELETE", "/drain", http.StatusOK).Draining {
		t.Fatal("still draining after being called off")
	}
}
