package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"ourtaiko.dev/net/internal/web"
)

// awarder stands in for the part that keeps titles and badges in the same database.
type awarder struct {
	fail, refuse bool
	asked        []AwardRequest
	// What the attempt's transaction could see when it asked.
	sawAttempt bool
}

func (a *awarder) Grant(ctx context.Context, tx pgx.Tx, request AwardRequest) (json.RawMessage, error) {
	a.asked = append(a.asked, request)
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM dan_attempts WHERE id=$1)`, request.Key).Scan(&a.sawAttempt); err != nil {
		return nil, err
	}
	if a.fail {
		return nil, errors.New("the award could not be written")
	}
	if a.refuse {
		return nil, &ProfileRejection{Status: 422, Code: "AWARD_INVALID"}
	}
	return json.RawMessage(`{"granted":{"titles":[],"skipped":[]},"nameplate":{"userId":"` + request.UserID + `"}}`), nil
}

// With an Awarder a 段位 attempt, its scores and what it earns are one transaction: when the
// award cannot be recorded nothing is saved and the game's retry starts from nothing.
func TestDanAttemptAndItsAwardAreSavedTogether(t *testing.T) {
	pool := testDB(t, Fanmade)
	ctx := context.Background()
	const origin = "http://127.0.0.1:5173"
	const alice, carol = "0000000000000000000000000000a11c", "0000000000000000000000000000ca01"
	const song, course = "11111111111111111111111111111111", "dddddddddddddddddddddddddddddddd"
	_, err := pool.Exec(ctx, `INSERT INTO test_sso_users(id,username,password_hash,nickname) VALUES ('0000000000000000000000000000a11c','alice','unused','Alice'),('0000000000000000000000000000ca01','carol','unused','Carol');
 INSERT INTO charts(id,owner_ids,title,bpm,duration,tja_file,audio_file) VALUES
 ('11111111111111111111111111111111',ARRAY['0000000000000000000000000000a11c'],'Alpha',120,10,test_file('tja-a','a.tja',repeat('a',64),1),test_file('ogg-a','a.ogg',repeat('b',64),1));
 SELECT test_slot('11111111111111111111111111111111','Oni',9);
 INSERT INTO dan_courses(id,owner_id,title,exams,code) VALUES ('dddddddddddddddddddddddddddddddd','0000000000000000000000000000a11c','{"en":"Course"}','[]','COURSE');
 INSERT INTO dan_course_songs(course_id,position,chart_id,course,side) VALUES ('dddddddddddddddddddddddddddddddd',0,'11111111111111111111111111111111','Oni','single')`)
	if err != nil {
		t.Fatal(err)
	}
	awards := &awarder{}
	s := testServer(t, pool, Config{Origin: origin, Awards: awards})
	handler := s.Handler()
	token := web.NewID() + web.NewID()
	if _, err = pool.Exec(ctx, `INSERT INTO test_sessions(token_hash,user_id,csrf_token,expires_at) VALUES($1,$2,'',now()+interval '1 day')`, web.Hash("game:"+token), carol); err != nil {
		t.Fatal(err)
	}
	var slot int64
	if err = pool.QueryRow(ctx, `SELECT id FROM chart_difficulties WHERE chart_id=$1`, song).Scan(&slot); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"courseId":"%s","verdict":1,"scores":[{"difficultyId":%d,"good":5,"ok":0,"bad":0,"score":2000,"drumroll":0,"max_combo":5}]}`, course, slot)
	requests := 0
	send := func(key string) (int, map[string]json.RawMessage) {
		t.Helper()
		r := httptest.NewRequest("POST", "/api/v1/game/dan-results", strings.NewReader(body))
		requests++
		r.RemoteAddr = fmt.Sprintf("192.0.2.%d:1234", requests)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Idempotency-Key", key)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		var reply map[string]json.RawMessage
		json.Unmarshal(w.Body.Bytes(), &reply)
		return w.Code, reply
	}
	saved := func() (attempts, scores int) {
		t.Helper()
		if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM dan_attempts),(SELECT count(*) FROM scores)`).Scan(&attempts, &scores); err != nil {
			t.Fatal(err)
		}
		return
	}

	// The award fails: the request fails as temporarily unavailable, which the game
	// retries, and neither the attempt nor its score is kept.
	awards.fail = true
	if status, _ := send("attempt-key-0001"); status != 503 {
		t.Fatal("a failed award must fail the request", status)
	}
	if !awards.sawAttempt {
		t.Fatal("the award was not asked for inside the attempt's transaction")
	}
	if attempts, scores := saved(); attempts != 0 || scores != 0 {
		t.Fatalf("a failed award left %d attempts and %d scores", attempts, scores)
	}

	// The same request again, now succeeding: saved once, with what it earned.
	awards.fail = false
	status, reply := send("attempt-key-0001")
	if status != 200 || !strings.Contains(string(reply["profile"]), `"granted"`) || len(reply["profileError"]) != 0 {
		t.Fatal("the retry", status, reply)
	}
	if attempts, scores := saved(); attempts != 1 || scores != 1 {
		t.Fatalf("after the retry: %d attempts, %d scores", attempts, scores)
	}
	var stored []byte
	if err = pool.QueryRow(ctx, `SELECT granted FROM dan_attempts`).Scan(&stored); err != nil || !strings.Contains(string(stored), "granted") {
		t.Fatal("what was granted is kept with the attempt", string(stored), err)
	}
	if request := awards.asked[len(awards.asked)-1]; request.UserID != carol || request.CourseID != course || request.CourseOwner != alice || request.Verdict != 1 {
		t.Fatalf("asked for %+v", request)
	}

	// Sent once more, it is answered from what is kept and nobody is asked.
	asked := len(awards.asked)
	if status, reply = send("attempt-key-0001"); status != 200 || len(awards.asked) != asked || !strings.Contains(string(reply["profile"]), `"granted"`) {
		t.Fatal("a repeat asks again", status, len(awards.asked)-asked)
	}

	// An attempt that will never be taken is saved all the same and the game is told to stop.
	awards.refuse = true
	status, reply = send("attempt-key-0002")
	if status != 200 || string(reply["profile"]) != "null" || string(reply["profileRejected"]) != `"AWARD_INVALID"` || len(reply["profileError"]) != 0 {
		t.Fatal("refused", status, reply)
	}
	if attempts, scores := saved(); attempts != 2 || scores != 2 {
		t.Fatalf("after a refusal: %d attempts, %d scores", attempts, scores)
	}
}
