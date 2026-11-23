package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"ourtaiko.dev/net/internal/kit/tja"
	"ourtaiko.dev/net/internal/web"
)

// A chart's scene preset is what its TJA says in SCENEPRESET when it is stored, a name the
// site does not take counting as none; changing it on the site rewrites the header in the
// stored file, which is what the game reads; and a chart stored before the column gets it
// from its file without counting as changed.
func TestChartScenePreset(t *testing.T) {
	bothSources(t, testChartScenePreset)
}

func testChartScenePreset(t *testing.T, w interactionWorld) {
	if !tja.Available() {
		t.Skip("tjaparser not found: set TJAPARSER_BIN or put it on PATH")
	}
	s := w.s
	ctx := context.Background()
	audio, err := os.ReadFile("../../kit/audio/testdata/vorbis.ogg")
	if err != nil {
		t.Fatal(err)
	}
	send := func(method, path, text string) Chart {
		t.Helper()
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		out, _ := form.CreateFormFile("tja", "chart.tja")
		_, _ = out.Write([]byte(text))
		if method == "POST" {
			out, _ = form.CreateFormFile("audio", "music.ogg")
			_, _ = out.Write(audio)
		} else {
			_ = form.WriteField("confirmReset", "true")
		}
		_ = form.Close()
		r := httptest.NewRequest(method, path, &body)
		r.Header.Set("Content-Type", form.FormDataContentType())
		r.Header.Set("Origin", "http://localhost")
		r.Header.Set("X-CSRF-Token", "csrf")
		r.Header.Set("Idempotency-Key", web.NewID())
		r.RemoteAddr = fmt.Sprintf("10.9.%d.1:1234", w.c.n.Add(1)&255)
		r.AddCookie(&http.Cookie{Name: w.c.cookie, Value: w.c.tokens["admin"]})
		rec := httptest.NewRecorder()
		w.c.handler.ServeHTTP(rec, r)
		var c Chart
		if rec.Code >= 300 || json.Unmarshal(rec.Body.Bytes(), &c) != nil {
			t.Fatalf("%s %s: %d %s", method, path, rec.Code, rec.Body.String())
		}
		return c
	}
	chart := func(title, headers string) string {
		return "TITLE:" + title + "\r\nBPM:120\r\nWAVE:music.ogg\r\n" + headers + "COURSE:Oni\r\nLEVEL:5\r\n#START\r\n1000,\r\n#END\r\n"
	}
	stored := func(id string) (text string, tjaFile int64, updated time.Time) {
		t.Helper()
		var key string
		if err := s.DB.QueryRow(ctx, `SELECT r.storage_key,r.id,c.update_at FROM charts c JOIN chart_resources r ON r.id=c.tja_file WHERE c.id=$1`, id).Scan(&key, &tjaFile, &updated); err != nil {
			t.Fatal(err)
		}
		f, err := s.Config.Objects.Open(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		data, _ := io.ReadAll(f)
		return string(data), tjaFile, updated
	}
	patch := func(id, body string, want int) Chart {
		t.Helper()
		var c Chart
		if code := w.c.do("admin", "PATCH", w.chart(id), body, &c); code != want {
			t.Fatalf("PATCH %s: %d, want %d", body, code, want)
		}
		return c
	}

	// Read from the file as the game reads it; an unusable name is none.
	named := send("POST", "/api/charts", chart("A", "SCENEPRESET:TOHO\r\n"))
	if named.ScenePreset != "TOHO" {
		t.Fatalf("preset from the file: %q", named.ScenePreset)
	}
	if odd := send("POST", "/api/charts", chart("B", "SCENEPRESET:not a name\r\n")); odd.ScenePreset != "" {
		t.Fatalf("unusable name kept: %q", odd.ScenePreset)
	}
	plain := send("POST", "/api/charts", chart("C", ""))
	if plain.ScenePreset != "" {
		t.Fatalf("preset of a chart naming none: %q", plain.ScenePreset)
	}

	// Changing it rewrites the header in place and keeps the rest of the chart.
	before, beforeFile, _ := stored(named.ID)
	patch(named.ID, `{"scenePreset":"no good"}`, 422)
	patch(named.ID, `{"scenePreset":null}`, 422)
	if same := patch(named.ID, `{"scenePreset":"TOHO"}`, 200); same.ScenePreset != "TOHO" {
		t.Fatal(same.ScenePreset)
	}
	if _, file, _ := stored(named.ID); file != beforeFile {
		t.Fatal("an unchanged preset replaced the stored TJA")
	}
	changed := patch(named.ID, `{"scenePreset":"MIKU"}`, 200)
	text, _, _ := stored(named.ID)
	if changed.ScenePreset != "MIKU" || text != strings.Replace(before, "SCENEPRESET:TOHO", "SCENEPRESET:MIKU", 1) ||
		changed.Title != "A" || len(changed.Difficulties) != 1 || changed.Difficulties[0].Hash != named.Difficulties[0].Hash {
		t.Fatalf("preset not changed in the file: %+v\n%s", changed, text)
	}
	// A chart naming none gains the header after WAVE; empty clears it.
	gained := patch(plain.ID, `{"scenePreset":"ANIMAL"}`, 200)
	if text, _, _ = stored(plain.ID); gained.ScenePreset != "ANIMAL" || !strings.Contains(text, ".ogg\r\nSCENEPRESET:ANIMAL\r\nCOURSE") {
		t.Fatalf("header not added: %q\n%s", gained.ScenePreset, text)
	}
	cleared := patch(plain.ID, `{"scenePreset":""}`, 200)
	if text, _, _ = stored(plain.ID); cleared.ScenePreset != "" || tja.SongHeader([]byte(text), "SCENEPRESET") != "" {
		t.Fatalf("preset not cleared: %q\n%s", cleared.ScenePreset, text)
	}
	var read Chart
	if code := w.c.do("", "GET", w.chart(named.ID), "", &read); code != 200 || read.ScenePreset != "MIKU" {
		t.Fatal("the chart does not answer its preset", code, read.ScenePreset)
	}
	// A replacement brings its own.
	if replaced := send("PUT", w.chart(named.ID)+"/files", chart("A", "SCENEPRESET:DOJO\r\n")); replaced.ScenePreset != "DOJO" {
		t.Fatalf("preset after a replacement: %q", replaced.ScenePreset)
	}

	// A chart from before the column is filled from its file, once, and is not touched by it.
	if _, err = s.DB.Exec(ctx, `UPDATE charts SET scene_preset=NULL WHERE id=ANY($1)`, []string{named.ID, plain.ID}); err != nil {
		t.Fatal(err)
	}
	_, _, at := stored(named.ID)
	if code := w.c.do("", "GET", w.chart(named.ID), "", &read); code != 200 || read.ScenePreset != "" {
		t.Fatal("a preset not read yet", code, read.ScenePreset)
	}
	// Song and Other on Fanmade point at files the store does not hold; they fail and stay.
	_ = s.backfillScenePresets(ctx)
	var filled, empty *string
	if err = s.DB.QueryRow(ctx, `SELECT (SELECT scene_preset FROM charts WHERE id=$1),(SELECT scene_preset FROM charts WHERE id=$2)`, named.ID, plain.ID).Scan(&filled, &empty); err != nil {
		t.Fatal(err)
	}
	if _, _, after := stored(named.ID); filled == nil || *filled != "DOJO" || empty == nil || *empty != "" || !after.Equal(at) {
		t.Fatalf("backfill: %v %v, update_at %v → %v", filled, empty, at, after)
	}

	// A song without files has no TJA to say it in.
	if s.source().Official {
		patch(w.song, `{"scenePreset":"TOHO"}`, 422)
		patch(w.song, `{"scenePreset":""}`, 200)
	}
}
