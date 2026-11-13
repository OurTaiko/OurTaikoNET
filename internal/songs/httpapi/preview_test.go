package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ourtaiko.dev/net/internal/kit/objectstore"
	"ourtaiko.dev/net/internal/songs/teststore"
	"ourtaiko.dev/net/internal/web"
)

func TestPreviewLifecycle(t *testing.T) {
	for _, remote := range []bool{false, true} {
		t.Run(fmt.Sprint("s3=", remote), func(t *testing.T) {
			ctx := context.Background()
			pool := testDB(t, Fanmade)
			const owner = "d46774d30dd13b92d9e536808da468a4"
			cookie := strings.Repeat("1", 64)
			if _, err := pool.Exec(ctx, `INSERT INTO test_sso_users(id,username,password_hash) VALUES($1,'author','unused')`, owner); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `INSERT INTO test_sessions(token_hash,user_id,csrf_token,expires_at) VALUES($1,$2,'csrf',now()+interval '1 day')`, web.Hash(cookie), owner); err != nil {
				t.Fatal(err)
			}
			cfg := Config{Storage: t.TempDir(), Origin: "http://localhost"}
			if remote {
				cfg.Objects = teststore.New(t)
			}
			app := testServer(t, pool, cfg)
			source, err := os.ReadFile("../../kit/audio/testdata/vorbis.ogg")
			if err != nil {
				t.Fatal(err)
			}
			file, err := os.Open("../../kit/audio/testdata/vorbis.ogg")
			if err != nil {
				t.Fatal(err)
			}
			err = app.Config.Objects.Put(ctx, "audio", file, int64(len(source)), "audio/ogg", web.Hash(string(source)))
			file.Close()
			if err != nil {
				t.Fatal(err)
			}
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			if _, err = tx.Exec(ctx, `INSERT INTO charts(id,owner_ids,title,bpm,duration,audio_file,tja_file,demo_start) VALUES('song',ARRAY[$1],'Test',120,0.5,test_file('audio','audio.ogg',$2,$3),test_file('tja','song.tja',repeat('a',64),1),0.1)`, owner, web.Hash(string(source)), len(source)); err != nil {
				t.Fatal(err)
			}
			if err = tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			if err = app.BackfillPreviews(ctx); err != nil {
				t.Fatal(err)
			}
			var key string
			if err = pool.QueryRow(ctx, `SELECT r.storage_key FROM charts c JOIN chart_resources r ON r.id=c.preview_file WHERE c.id='song'`).Scan(&key); err != nil {
				t.Fatal(err)
			}
			c, err := app.chart(ctx, "song")
			if err != nil || c.DemoEnd != 15.1 || !strings.HasSuffix(c.PreviewPath, ".ogg") {
				t.Fatalf("chart=%+v err=%v", c, err)
			}
			if remote && c.PreviewPath != cfg.Objects.(*objectstore.S3).Prefix+key {
				t.Fatal("incorrect bucket path", c.PreviewPath)
			}
			if err = app.BackfillPreviews(ctx); err != nil {
				t.Fatal(err)
			}
			var unchanged string
			pool.QueryRow(ctx, `SELECT r.storage_key FROM charts c JOIN chart_resources r ON r.id=c.preview_file WHERE c.id='song'`).Scan(&unchanged)
			if unchanged != key {
				t.Fatal("backfill replaced an existing preview")
			}
			call := func(body, token, csrf string) *httptest.ResponseRecorder {
				r := httptest.NewRequest("PATCH", "/api/v1/charts/song", strings.NewReader(body))
				r.Header.Set("Content-Type", "application/json")
				r.Header.Set("Origin", "http://localhost")
				r.Header.Set("X-CSRF-Token", csrf)
				if token != "" {
					r.AddCookie(&http.Cookie{Name: "ourtaiko_session", Value: token})
				}
				w := httptest.NewRecorder()
				app.Handler().ServeHTTP(w, r)
				return w
			}
			if w := call(`{"demoEnd":0.3}`, "", "csrf"); w.Code != 401 {
				t.Fatal(w.Code, w.Body.String())
			}
			if w := call(`{"demoEnd":0.3}`, cookie, ""); w.Code != 403 {
				t.Fatal(w.Code, w.Body.String())
			}
			for _, body := range []string{`{"demoStart":-1}`, `{"demoEnd":0.1}`, `{"demoStart":0.5}`, `{"demoEnd":null}`, `{"demoStart":"0.2"}`} {
				if w := call(body, cookie, "csrf"); w.Code != 422 {
					t.Fatal(w.Code, w.Body.String())
				}
			}
			w := call(`{"demoStart":0.2,"demoEnd":0.4}`, cookie, "csrf")
			var updated Chart
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &updated) != nil || updated.DemoEnd != 0.4 || updated.DemoStart != 0.2 || updated.PreviewPath == c.PreviewPath {
				t.Fatal(w.Code, w.Body.String())
			}
			if updated.AudioHash != c.AudioHash {
				t.Fatal("source resources changed")
			}
			app.cleanupReplacedFiles()
			if _, err = app.Config.Objects.Open(ctx, key); !os.IsNotExist(err) {
				t.Fatal("old preview still exists", err)
			}
			// Lists expose the current preview location; the full audio endpoint stays separate.
			w = httptest.NewRecorder()
			app.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/charts", nil))
			if w.Code != 200 || !strings.Contains(w.Body.String(), `"previewPath"`) {
				t.Fatal(w.Code, w.Body.String())
			}
			var currentKey string
			pool.QueryRow(ctx, `SELECT r.storage_key FROM charts c JOIN chart_resources r ON r.id=c.preview_file WHERE c.id='song'`).Scan(&currentKey)
			object, err := app.Config.Objects.Open(ctx, currentKey)
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(object)
			object.Close()
			if err != nil || string(data[:4]) != "OggS" {
				t.Fatal("invalid stored preview", err)
			}
			// A failure while retrieving the source does not undo the edit: the range stands, and
			// the clip of the old range is dropped for the backfill to cut a new one.
			if err = app.Config.Objects.Delete(ctx, "audio"); err != nil {
				t.Fatal(err)
			}
			if w = call(`{"demoEnd":0.45}`, cookie, "csrf"); w.Code != 200 {
				t.Fatal(w.Code, w.Body.String())
			}
			after, err := app.chart(ctx, "song")
			if err != nil || after.DemoEnd != 0.45 || after.PreviewPath != "" {
				t.Fatal("a failed clip left the old one in place", err, after.DemoEnd, after.PreviewPath)
			}
		})
	}
}

// The game's preview clip: the backfill cuts one for a chart without it and leaves update_at
// alone, the resource manifest lists it, and a new preview range replaces it.
func TestPreviewClips(t *testing.T) {
	w := interactionWorldOf(t, Official)
	s := w.s
	ctx := context.Background()
	source := filepath.Join(t.TempDir(), "a.ogg")
	if out, err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "sine=frequency=440:duration=20", "-c:a", "libvorbis", source).CombinedOutput(); err != nil {
		t.Skip("ffmpeg with libvorbis is needed:", err, string(out))
	}
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	if err = os.WriteFile(filepath.Join(s.Config.Storage, digest+".ogg"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	var audioID int64
	if err = s.DB.QueryRow(ctx, `INSERT INTO chart_resources(storage_key,original_filename,sha256,byte_size,extension) VALUES($1,'a.ogg',$2,$3,'ogg') RETURNING id`,
		digest+".ogg", digest, len(data)).Scan(&audioID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(ctx, `UPDATE charts SET tja_file=$2,audio_file=$2,duration=20,demo_start=2,demo_end=8 WHERE id=$1`, w.song, audioID); err != nil {
		t.Fatal(err)
	}
	clip := func() (key string, updated time.Time) {
		t.Helper()
		if err := s.DB.QueryRow(ctx, `SELECT COALESCE(r.storage_key,''),c.update_at FROM charts c LEFT JOIN chart_resources r ON r.id=c.preview_file WHERE c.id=$1`, w.song).Scan(&key, &updated); err != nil {
			t.Fatal(err)
		}
		return
	}
	_, before := clip()
	if err = s.BackfillPreviews(ctx); err != nil {
		t.Fatal(err)
	}
	first, after := clip()
	if first == "" || !after.Equal(before) {
		t.Fatal("backfill made no clip or changed update_at", first, before, after)
	}
	var manifest struct {
		Resources map[string]struct {
			URL         string
			ContentType string
		}
	}
	if code := w.c.do("", "GET", w.chart(w.song)+"/resources", "", &manifest); code != 200 || manifest.Resources["preview"].ContentType != "audio/ogg" {
		t.Fatal("manifest lacks the clip", code, manifest)
	}
	if code, body := w.c.raw("", w.chart(w.song)+"/preview"); code != 200 || len(body) == 0 {
		t.Fatal("clip not served", code)
	}
	if err = s.BackfillPreviews(ctx); err != nil {
		t.Fatal(err)
	}
	if again, _ := clip(); again != first {
		t.Fatal("backfill redid an existing clip")
	}
	if code := w.c.do("owner", "PATCH", w.chart(w.song), `{"demoStart":4,"demoEnd":10}`, nil); code != 200 {
		t.Fatal("range edit", code)
	}
	second, _ := clip()
	var old int
	if err = s.DB.QueryRow(ctx, `SELECT count(*) FROM chart_resources WHERE storage_key=$1`, first).Scan(&old); err != nil {
		t.Fatal(err)
	}
	if second == "" || second == first || old != 0 {
		t.Fatal("a new range kept the old clip", first, second, old)
	}
	if _, err = os.Stat(filepath.Join(s.Config.Storage, first)); !os.IsNotExist(err) {
		t.Fatal("old clip object left behind", err)
	}
}
