package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"ourtaiko.dev/net/internal/kit/constants"
	"ourtaiko.dev/net/internal/kit/tja"
	"ourtaiko.dev/net/internal/web"
)

// Difficulty constants from storing to rating: every route stored with the blocks of an
// upload, rated by the first calibration, not recalibrated by the same data, refilled by the
// startup backfill; none of it touches the chart's update_at, which block changes still do.
func TestDifficultyConstants(t *testing.T) { bothSources(t, testDifficultyConstants) }

func testDifficultyConstants(t *testing.T, w interactionWorld) {
	if !tja.Available() {
		t.Skip("tjaparser not found: set TJAPARSER_BIN or put it on PATH")
	}
	s := w.s
	ctx := context.Background()
	// The chart comes in as an upload does, by someone who may add charts on either source.
	source := []byte("TITLE:Backfill\nBPM:120\nWAVE:cbr.mp3\nCOURSE:Hard\nLEVEL:5\n#START\n1020,\n1122,\n#END\n" +
		"COURSE:Oni\nLEVEL:8\n#START\n1000,\n#BRANCHSTART p,80,95\n#N\n1000,\n#E\n2020,\n#M\n3434,\n#BRANCHEND\n1,\n#END\n")
	audio, err := os.ReadFile("../../kit/audio/testdata/cbr.mp3")
	if err != nil {
		t.Fatal(err)
	}
	var form bytes.Buffer
	parts := multipart.NewWriter(&form)
	tf, _ := parts.CreateFormFile("tja", "chart.tja")
	tf.Write(source)
	af, _ := parts.CreateFormFile("audio", "cbr.mp3")
	af.Write(audio)
	parts.Close()
	r := httptest.NewRequest("POST", "/api/charts", &form)
	r.Header.Set("Content-Type", parts.FormDataContentType())
	r.Header.Set("Origin", "http://localhost")
	r.Header.Set("X-CSRF-Token", "csrf")
	r.Header.Set("Idempotency-Key", web.NewID())
	r.AddCookie(&http.Cookie{Name: w.c.cookie, Value: w.c.tokens["admin"]})
	rec := httptest.NewRecorder()
	w.c.handler.ServeHTTP(rec, r)
	var chart Chart
	if rec.Code != 201 || json.Unmarshal(rec.Body.Bytes(), &chart) != nil {
		t.Fatal(rec.Code, rec.Body.String())
	}
	meta, issue := tja.Parse(source, "")
	if issue != nil {
		t.Fatal(issue)
	}
	setDifficulties := func() {
		t.Helper()
		tx, err := s.DB.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if err = setDifficulties(ctx, tx, chart.ID, meta.Difficulties); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	setDifficulties()

	// Every route of each block, under its content hash: note count, main constant (nil until
	// rated), calibration; and the chart's update_at.
	type row struct {
		notes       int32
		main        *float64
		calibration *int32
	}
	stored := func() (map[string]row, time.Time) {
		t.Helper()
		rows, err := s.DB.Query(ctx, `SELECT d.course||'/'||k.route,k.note_count,k.main_constant,k.calibration_version
 FROM chart_difficulties d JOIN difficulty_constants k ON k.content_hash=d.content_hash WHERE d.chart_id=$1`, chart.ID)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		result := map[string]row{}
		for rows.Next() {
			var key string
			var r row
			if err = rows.Scan(&key, &r.notes, &r.main, &r.calibration); err != nil {
				t.Fatal(err)
			}
			result[key] = r
		}
		var updated time.Time
		if err = s.DB.QueryRow(ctx, `SELECT update_at FROM charts WHERE id=$1`, chart.ID).Scan(&updated); err != nil {
			t.Fatal(err)
		}
		return result, updated
	}
	uploaded, updated := stored()
	// Uploaded before any calibration. Oni: 1 common note and 1 after the branch, with 1, 2 or 4 on the routes; not rated yet.
	notes := map[string]int32{"Hard/unbranched": 6, "Oni/normal": 3, "Oni/expert": 4, "Oni/master": 6}
	for key, n := range notes {
		if r, ok := uploaded[key]; !ok || r.notes != n || r.main != nil {
			t.Fatalf("stored %s: %+v", key, uploaded[key])
		}
	}
	if len(uploaded) != len(notes) {
		t.Fatalf("stored %v", uploaded)
	}

	// The first calibration rates every row, and touches no chart.
	result, err := constants.Recalibrate(ctx, s.DB)
	if err != nil || !result.New || result.Calibration.Version != 1 || result.Rated != 4 || result.Calibration.Routes != 4 {
		t.Fatalf("first calibration: %+v %v", result, err)
	}
	rated, at := stored()
	for key, r := range rated {
		if r.main == nil || *r.calibration != 1 {
			t.Fatalf("%s not rated: %+v", key, r)
		}
	}
	if !at.Equal(updated) {
		t.Fatalf("calibrating touched the chart: update_at %v → %v", updated, at)
	}
	// Each row is rated from its own raw values, by rating.py's formula.
	raws, err := s.DB.Query(ctx, `SELECT d.course||'/'||k.route,k.constants_version,k.note_count,k.raw_stamina,k.raw_handspeed,k.raw_burst,k.raw_complex,k.raw_complex_ratio,k.raw_rhythm,k.raw_rhythm_ratio,k.main_constant,k.rhythm
 FROM chart_difficulties d JOIN difficulty_constants k ON k.content_hash=d.content_hash WHERE d.chart_id=$1`, chart.ID)
	if err != nil {
		t.Fatal(err)
	}
	for raws.Next() {
		var key string
		var version int16
		var notes int32
		var raw constants.Raw
		var main, rhythm float64
		if err = raws.Scan(&key, &version, &notes, &raw.Stamina, &raw.Handspeed, &raw.Burst, &raw.Complex, &raw.ComplexRatio, &raw.Rhythm, &raw.RhythmRatio, &main, &rhythm); err != nil {
			t.Fatal(err)
		}
		raw.NoteCount = int(notes)
		if want := constants.Rate(raw, result.Calibration.Params); version != int16(constants.Version) || main != want.MainConstant || rhythm != want.Rhythm {
			t.Fatalf("%s rated %v %v by version %d, want %+v", key, main, rhythm, version, want)
		}
	}
	raws.Close()
	// The chart page gets every rated route of each block; lists go without.
	var detail Chart
	if code := w.c.do("", "GET", w.chart(chart.ID)+"?by=id", "", &detail); code != 200 {
		t.Fatal("detail", code)
	}
	for _, d := range detail.Difficulties {
		var routes []string
		for _, c := range d.Constants {
			routes = append(routes, c.Route)
		}
		want := map[string]string{"Hard": "unbranched", "Oni": "normal expert master"}[d.Course]
		last := d.Course + "/" + routes[len(routes)-1]
		if strings.Join(routes, " ") != want || d.Constants[len(d.Constants)-1].MainConstant != *rated[last].main {
			t.Fatalf("%s: constants %+v", d.Course, d.Constants)
		}
	}
	if code, body := w.c.raw("", "/api/charts"); code != 200 || strings.Contains(string(body), `"constants"`) {
		t.Fatalf("the list carries constants: %d", code)
	}
	if again, err := constants.Recalibrate(ctx, s.DB); err != nil || again.New || again.Rated != 0 {
		t.Fatalf("second calibration: %+v %v", again, err)
	}

	// A block stored without its constants, as charts from before there were any, gets them
	// back from the stored file, rated by the latest calibration; update_at is left alone.
	if _, err = s.DB.Exec(ctx, `DELETE FROM difficulty_constants`); err != nil {
		t.Fatal(err)
	}
	// The titles have been edited since the upload; the stored file still says "Backfill".
	if _, err = s.DB.Exec(ctx, `UPDATE charts SET title_translations=title_translations||'{"zh":"改过的标题"}'::jsonb - 'en',
 subtitle_translations='{"ja":"改过的副标题"}'::jsonb WHERE id=$1`, chart.ID); err != nil {
		t.Fatal(err)
	}
	titles := func() (title, subtitle string) {
		t.Helper()
		if err = s.DB.QueryRow(ctx, `SELECT title_translations::text,subtitle_translations::text FROM charts WHERE id=$1`, chart.ID).Scan(&title, &subtitle); err != nil {
			t.Fatal(err)
		}
		return
	}
	editedTitle, editedSubtitle := titles()
	_, updated = stored()
	var prefix string
	if err = s.DB.QueryRow(ctx, `SELECT split_part(content_hash,':',1)||':' FROM chart_difficulties WHERE chart_id=$1 LIMIT 1`, chart.ID).Scan(&prefix); err != nil {
		t.Fatal(err)
	}
	if err = s.backfillContentHashes(ctx, prefix, constants.Version); err != nil {
		t.Fatal(err)
	}
	backfilled, at := stored()
	for key, r := range rated {
		if b := backfilled[key]; b.notes != r.notes || b.main == nil || *b.main != *r.main || !at.Equal(updated) {
			t.Fatalf("backfill %s: %+v, want %+v; update_at %v → %v", key, b, r, updated, at)
		}
	}
	if title, subtitle := titles(); title != editedTitle || subtitle != editedSubtitle {
		t.Fatalf("the backfill rewrote edited titles: %s %s, want %s %s", title, subtitle, editedTitle, editedSubtitle)
	}

	// A block's scores and 段位 songs go with its hash, whatever changed it: another hash of
	// the same algorithm, or another algorithm; and any update of a block is a change of the
	// chart.
	current := map[string]string{}
	for _, course := range []string{"Hard", "Oni"} {
		var value string
		if err = s.DB.QueryRow(ctx, `SELECT content_hash FROM chart_difficulties WHERE chart_id=$1 AND course=$2`, chart.ID, course).Scan(&value); err != nil {
			t.Fatal(err)
		}
		current[course] = value
	}
	if _, err = s.DB.Exec(ctx, `INSERT INTO scores(id,user_id,difficulty_id,good,ok,bad,score,drumroll,max_combo,payload_digest)
 SELECT 'kept-'||course,$2,id,1,0,0,1000,0,1,repeat('a',64) FROM chart_difficulties WHERE chart_id=$1`, chart.ID, testUsers["owner"].ID); err != nil {
		t.Fatal(err)
	}
	danID := web.NewID()
	if _, err = s.DB.Exec(ctx, `INSERT INTO dan_courses(id,owner_id,title,code,exams) VALUES($2,$1,'{"en":"Kept"}','SHODAN','[]')`, testUsers["owner"].ID, danID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(ctx, `INSERT INTO dan_course_songs(course_id,position,chart_id,course,side) VALUES($2,0,$1,'Hard','single'),($2,1,$1,'Oni','single')`, chart.ID, danID); err != nil {
		t.Fatal(err)
	}
	kept := func() (latest, valid int) {
		t.Helper()
		if err = s.DB.QueryRow(ctx, `SELECT (SELECT count(*) FROM scores WHERE id LIKE 'kept-%' AND is_latest),(SELECT count(*) FROM dan_course_songs WHERE chart_id=$1 AND valid)`, chart.ID).Scan(&latest, &valid); err != nil {
			t.Fatal(err)
		}
		return
	}
	_, updated = stored()
	if _, err = s.DB.Exec(ctx, `UPDATE chart_difficulties SET content_hash=$2||repeat('0',64) WHERE chart_id=$1 AND course='Hard'`, chart.ID, prefix); err != nil {
		t.Fatal(err)
	}
	_, at = stored()
	if latest, valid := kept(); latest != 1 || valid != 1 || !at.After(updated) {
		t.Fatalf("after another hash: %d latest scores, %d valid songs, update_at %v → %v", latest, valid, updated, at)
	}
	if _, err = s.DB.Exec(ctx, `UPDATE chart_difficulties SET content_hash='v0:'||split_part(content_hash,':',2) WHERE chart_id=$1 AND course='Oni'`, chart.ID); err != nil {
		t.Fatal(err)
	}
	if latest, valid := kept(); latest != 0 || valid != 0 {
		t.Fatalf("after another algorithm: %d latest scores, %d valid songs", latest, valid)
	}
	for course, value := range current {
		if _, err = s.DB.Exec(ctx, `UPDATE chart_difficulties SET content_hash=$3 WHERE chart_id=$1 AND course=$2`, chart.ID, course, value); err != nil {
			t.Fatal(err)
		}
	}
	_, updated = stored()
	// Storing the same reading again changes nothing.
	setDifficulties()
	if _, at := stored(); !at.Equal(updated) {
		t.Fatalf("same file again: update_at %v → %v", updated, at)
	}
	// A block's columns count as a change of the chart.
	if _, err := s.DB.Exec(ctx, `UPDATE chart_difficulties SET maker='Someone' WHERE chart_id=$1 AND course='Hard'`, chart.ID); err != nil {
		t.Fatal(err)
	}
	if _, at := stored(); !at.After(updated) {
		t.Fatal("a maker change no longer touches the chart")
	}
}
