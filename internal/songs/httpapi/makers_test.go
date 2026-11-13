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

	"ourtaiko.dev/net/internal/kit/tja"
	"ourtaiko.dev/net/internal/web"
)

func TestDifficultyMakersFlow(t *testing.T) {
	pool := testDB(t, Fanmade)
	ctx := context.Background()
	token := strings.Repeat("d", 64)
	if _, err := pool.Exec(ctx, `INSERT INTO test_sso_users(id,username,password_hash) VALUES('037c0ccac142bf8602cbbe373b9a6f16','makertest','unused')`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO test_sessions(token_hash,user_id,csrf_token,expires_at) VALUES($1,'037c0ccac142bf8602cbbe373b9a6f16','csrf',now()+interval '1 day'),($2,'037c0ccac142bf8602cbbe373b9a6f16','csrf',now()+interval '1 day')`, web.Hash(token), web.Hash("game:"+token)); err != nil {
		t.Fatal(err)
	}
	handler := testServer(t, pool, Config{Origin: "http://localhost", Storage: t.TempDir()}).Handler()
	audio, err := os.ReadFile("../../kit/audio/testdata/cbr.mp3")
	if err != nil {
		t.Fatal(err)
	}
	source := "TITLE:Maker Test\nMAKER:Original\nBPM:120\nWAVE:cbr.mp3\n"
	for _, course := range []string{"Hard", "Oni", "Edit"} {
		source += "COURSE:" + course + "\nLEVEL:5\n#START\n1000,\n#END\n"
	}
	post := func(makers, key string, extra ...map[string]string) *httptest.ResponseRecorder {
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		tf, _ := form.CreateFormFile("tja", "chart.tja")
		tf.Write([]byte(source))
		af, _ := form.CreateFormFile("audio", "cbr.mp3")
		af.Write(audio)
		if makers != "" {
			form.WriteField("difficultyMakers", makers)
		}
		for _, fields := range extra {
			for name, value := range fields {
				form.WriteField(name, value)
			}
		}
		form.Close()
		r := httptest.NewRequest("POST", "/api/v1/charts", &body)
		r.Header.Set("Content-Type", form.FormDataContentType())
		r.Header.Set("Origin", "http://localhost")
		r.Header.Set("X-CSRF-Token", "csrf")
		r.Header.Set("Idempotency-Key", key)
		r.AddCookie(&http.Cookie{Name: "ourtaiko_session", Value: token})
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	decode := func(w *httptest.ResponseRecorder, status int) Chart {
		t.Helper()
		var c Chart
		if w.Code != status || json.Unmarshal(w.Body.Bytes(), &c) != nil {
			t.Fatalf("%d: %s", w.Code, w.Body.String())
		}
		return c
	}
	original := decode(post("", web.NewID()), 201)
	if original.Maker != "Original" {
		t.Fatal(original.Maker)
	}
	for _, d := range original.Difficulties {
		if d.Maker != "Original" {
			t.Fatal(d)
		}
	}
	previewKey := web.NewID()
	custom := map[string]string{"demoStart": "0.1", "demoEnd": "0.3"}
	preview := decode(post("", previewKey, custom), 201)
	if preview.DemoStart != 0.1 || preview.DemoEnd != 0.3 || preview.PreviewPath == "" {
		t.Fatalf("range not saved: %+v", preview)
	}
	if retry := decode(post("", previewKey, custom), 200); retry.ID != preview.ID {
		t.Fatal("range retry created new chart")
	}
	if w := post("", previewKey, map[string]string{"demoStart": "0.1", "demoEnd": "0.4"}); w.Code != 409 {
		t.Fatalf("range conflict: %d %s", w.Code, w.Body.String())
	}
	for _, fields := range []map[string]string{{"demoStart": "-1"}, {"demoStart": "NaN"}, {"demoEnd": "Inf"}, {"demoEnd": ""}, {"demoStart": "0.3", "demoEnd": "0.1"}, {"demoStart": "1200"}, {"demoEnd": "1216"}} {
		if w := post("", web.NewID(), fields); w.Code != 422 {
			t.Fatalf("bad range accepted: %d %s", w.Code, w.Body.String())
		}
	}
	makers := `[{"course":"Edit","side":"single","maker":"A"},{"course":"Hard","side":"single","maker":" A "},{"course":"Oni","side":"single","maker":"B"}]`
	key := web.NewID()
	c := decode(post(makers, key), 201)
	if c.Maker != "A | B" {
		t.Fatal(c.Maker)
	}
	for i, name := range []string{"A", "B", "A"} {
		if c.Difficulties[i].Maker != name {
			t.Fatal(c.Difficulties)
		}
	}
	retry := decode(post(`[{"course":"Hard","side":"single","maker":"A"},{"course":"Oni","side":"single","maker":"B"},{"course":"Edit","side":"single","maker":"A"}]`, key), 200)
	if retry.ID != c.ID {
		t.Fatal("retry created new chart")
	}
	if w := post(strings.ReplaceAll(makers, `"B"`, `"C"`), key); w.Code != 409 {
		t.Fatal("maker change must conflict", w.Code, w.Body.String())
	}
	for _, raw := range []string{`null`, `[]`, `{}`, `[{"course":"Hard","side":"single","maker":"A"}]`, `[{"course":"Hard","side":"single","maker":"A"},{"course":"Hard","side":"single","maker":"B"},{"course":"Edit","side":"single","maker":"C"}]`, `[{"course":"Hard","side":"single","maker":"A"},{"course":"Oni","side":"single","maker":"B"},{"course":"Missing","side":"single","maker":"C"}]`, `[{"course":"Hard","side":"single","maker":null},{"course":"Oni","side":"single","maker":"B"},{"course":"Edit","side":"single","maker":"C"}]`, strings.Replace(makers, `"B"`, `"B\nC"`, 1), strings.Replace(makers, `"B"`, `"`+strings.Repeat("界", 167)+`"`, 1)} {
		if w := post(raw, web.NewID()); w.Code != 422 {
			t.Fatalf("invalid %s: %d %s", raw, w.Code, w.Body.String())
		}
	}
	blank := decode(post(`[{"course":"Hard","side":"single","maker":""},{"course":"Oni","side":"single","maker":" "},{"course":"Edit","side":"single","maker":""}]`, web.NewID()), 201)
	if blank.Maker != "" {
		t.Fatal("blank should not fall back to original", blank.Maker)
	}
	for _, path := range []string{"/api/v1/charts/" + c.ID + "?by=id", "/api/v1/charts?q=B", "/api/v1/me/charts", "/api/v1/game/catalog"} {
		r := httptest.NewRequest("GET", path, nil)
		r.AddCookie(&http.Cookie{Name: "ourtaiko_session", Value: token})
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"maker":"A | B"`) || !strings.Contains(w.Body.String(), `"maker":"B"`) {
			t.Fatal(path, w.Code, w.Body.String())
		}
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM chart_difficulties d WHERE d.chart_id=$1 AND d.maker IN ('A','B')`, c.ID).Scan(&count); err != nil || count != 3 {
		t.Fatal(count, err)
	}
}

// Makers typed on the website, one per block, on the official source: an upload, an edit and
// a replacement set them; without them a block's maker is the TJA's.
func TestDifficultyMakers(t *testing.T) {
	if !tja.Available() {
		t.Skip("tjaparser not found: set TJAPARSER_BIN or put it on PATH")
	}
	w := interactionWorldOf(t, Official)
	audio, err := os.ReadFile("../../kit/audio/testdata/vorbis.ogg")
	if err != nil {
		t.Fatal(err)
	}
	source := "TITLE:Maker Test\nMAKER:Original\nBPM:120\nWAVE:vorbis.ogg\n"
	for _, course := range []string{"Hard", "Oni"} {
		source += "COURSE:" + course + "\nLEVEL:5\n#START\n1000,\n#END\n"
	}
	send := func(method, path string, files bool, fields map[string]string) *httptest.ResponseRecorder {
		t.Helper()
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		part, _ := form.CreateFormFile("tja", "chart.tja")
		part.Write([]byte(source))
		if files {
			part, _ = form.CreateFormFile("audio", "vorbis.ogg")
			part.Write(audio)
		}
		for name, value := range fields {
			form.WriteField(name, value)
		}
		form.Close()
		r := httptest.NewRequest(method, path, &body)
		r.Header.Set("Content-Type", form.FormDataContentType())
		r.Header.Set("Origin", "http://localhost")
		r.Header.Set("X-CSRF-Token", "csrf")
		r.Header.Set("Idempotency-Key", web.NewID())
		r.RemoteAddr = "10.8.200.1:1234"
		r.AddCookie(&http.Cookie{Name: w.c.cookie, Value: w.c.tokens["admin"]})
		rec := httptest.NewRecorder()
		w.c.handler.ServeHTTP(rec, r)
		return rec
	}
	makers := func(c Chart) map[string]string {
		names := map[string]string{}
		for _, d := range c.Difficulties {
			names[d.Course] = d.Maker
		}
		return names
	}
	typed := `[{"course":"Hard","side":"single","maker":"A"},{"course":"Oni","side":"single","maker":" B "}]`
	rec := send("POST", "/api/v1/charts", true, map[string]string{"difficultyMakers": typed})
	var c Chart
	if rec.Code != 201 || json.Unmarshal(rec.Body.Bytes(), &c) != nil {
		t.Fatal("upload", rec.Code, rec.Body.String())
	}
	if got := makers(c); got["Hard"] != "A" || got["Oni"] != "B" || c.Maker != "A | B" {
		t.Fatal("typed makers", got, c.Maker)
	}
	for _, bad := range []string{`[{"course":"Hard","side":"single","maker":"A"}]`, `{}`, `[{"course":"Tower","side":"single","maker":"A"},{"course":"Oni","side":"single","maker":"B"}]`} {
		if rec = send("POST", "/api/v1/charts", true, map[string]string{"difficultyMakers": bad}); rec.Code != 422 || !bytes.Contains(rec.Body.Bytes(), []byte("MAKERS_INVALID")) {
			t.Fatal("bad makers accepted", bad, rec.Code, rec.Body.String())
		}
	}
	// An edit changes only the makers it is given.
	var edited Chart
	if code := w.c.do("admin", "PATCH", w.chart(c.ID), `{"difficultyMakers":[{"course":"Hard","side":"single","maker":"C"},{"course":"Oni","side":"single","maker":"B"}]}`, &edited); code != 200 || makers(edited)["Hard"] != "C" || makers(edited)["Oni"] != "B" {
		t.Fatal("edit", code, makers(edited))
	}
	if code := w.c.do("admin", "PATCH", w.chart(c.ID), `{"difficultyMakers":[{"course":"Hard","side":"single","maker":"C"}]}`, nil); code != 422 {
		t.Fatal("partial makers accepted", code)
	}
	// A replacement without makers takes the TJA's; with them, the typed ones.
	if rec = send("PUT", w.chart(c.ID)+"/files", false, map[string]string{"confirmReset": "true"}); rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &c) != nil || makers(c)["Oni"] != "Original" {
		t.Fatal("replacement", rec.Code, rec.Body.String())
	}
	if rec = send("PUT", w.chart(c.ID)+"/files", false, map[string]string{"confirmReset": "true", "difficultyMakers": typed}); rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &c) != nil || makers(c)["Hard"] != "A" {
		t.Fatal("replacement with makers", rec.Code, rec.Body.String())
	}
}
