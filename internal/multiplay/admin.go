package multiplay

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// AdminHandler is what the publish script asks of a running server, on an address only the
// container itself can reach:
//
//	GET    /healthz          the process is up
//	GET    /drain            the DrainState
//	POST   /drain?for=25m    drain for that long at most, so that a publish that died does not
//	                         leave the server refusing rounds for ever
//	DELETE /drain            stop draining
func (s *Server) AdminHandler() http.Handler {
	var (
		mu   sync.Mutex
		lift *time.Timer
	)
	cancelLift := func() {
		if lift != nil {
			lift.Stop()
			lift = nil
		}
	}
	state := func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(s.manager.DrainState())
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("GET /drain", func(w http.ResponseWriter, r *http.Request) { state(w) })
	mux.HandleFunc("POST /drain", func(w http.ResponseWriter, r *http.Request) {
		limit, err := time.ParseDuration(r.URL.Query().Get("for"))
		if err != nil || limit <= 0 {
			http.Error(w, "for must be a duration, such as 25m", http.StatusBadRequest)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		cancelLift()
		lift = time.AfterFunc(limit, func() {
			s.logf("multiplay: still running %s after draining began: taking new rounds again", limit)
			s.manager.Drain(false)
		})
		s.manager.Drain(true)
		state(w)
	})
	mux.HandleFunc("DELETE /drain", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		cancelLift()
		s.manager.Drain(false)
		state(w)
	})
	return mux
}
