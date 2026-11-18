package httpapi

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ourtaiko.dev/net/internal/kit/tja"
	"ourtaiko.dev/net/internal/songs/teststore"
	"ourtaiko.dev/net/internal/web"
)

func TestBackgroundMovie(t *testing.T) {
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip(tool + " required")
		}
	}
	for _, remote := range []bool{false, true} {
		t.Run(fmt.Sprint("s3=", remote), func(t *testing.T) { testBackgroundMovie(t, remote) })
	}
}
func testBackgroundMovie(t *testing.T, remote bool) {
	pool := testDB(t, Fanmade)
	ctx := context.Background()
	const owner = "89b6ef3a5cb57b6e04f74711d15a8a5f"
	cookie := strings.Repeat("a", 64)
	if _, err := pool.Exec(ctx, `INSERT INTO test_sso_users(id,username,password_hash) VALUES($1,'tester','unused')`, owner); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO test_sessions(token_hash,user_id,csrf_token,expires_at) VALUES($1,$2,'csrf',now()+interval '1 day')`, web.Hash(cookie), owner); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Origin: "http://localhost", Storage: t.TempDir()}
	if remote {
		cfg.Objects = teststore.New(t)
	}
	app := testServer(t, pool, cfg)
	handler := app.Handler()
	audio, err := os.ReadFile("../../kit/audio/testdata/vorbis.ogg")
	if err != nil {
		t.Fatal(err)
	}
	// Larger than the output limits and with a sound track of its own.
	source := filepath.Join(t.TempDir(), "clip.mp4")
	if out, err := exec.Command("ffmpeg", "-nostdin", "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=2400x1350:rate=120:duration=0.5", "-f", "lavfi", "-i", "sine=duration=0.5",
		"-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac", "-shortest", source).CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}
	clip, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	type part struct {
		field, name string
		data        []byte
	}
	send := func(method, path, key string, parts ...part) *httptest.ResponseRecorder {
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		for _, p := range parts {
			if p.name == "" {
				_ = form.WriteField(p.field, string(p.data))
				continue
			}
			out, _ := form.CreateFormFile(p.field, p.name)
			_, _ = out.Write(p.data)
		}
		_ = form.Close()
		r := httptest.NewRequest(method, path, &body)
		r.Header.Set("Content-Type", form.FormDataContentType())
		r.Header.Set("Origin", "http://localhost")
		r.Header.Set("X-CSRF-Token", "csrf")
		r.Header.Set("Idempotency-Key", key)
		r.AddCookie(&http.Cookie{Name: "ourtaiko_session", Value: cookie})
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		return w
	}
	decode := func(w *httptest.ResponseRecorder, code int) Chart {
		t.Helper()
		var c Chart
		if w.Code != code || json.Unmarshal(w.Body.Bytes(), &c) != nil {
			t.Fatalf("want %d got %d: %s", code, w.Code, w.Body.String())
		}
		return c
	}
	rejected := func(w *httptest.ResponseRecorder, status int, code string) {
		t.Helper()
		if w.Code != status || !strings.Contains(w.Body.String(), code) {
			t.Fatalf("want %d %s, got %d: %s", status, code, w.Code, w.Body.String())
		}
	}
	read := func(key string) ([]byte, error) {
		f, err := app.Config.Objects.Open(ctx, key)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		return io.ReadAll(f)
	}
	tja := func(title, headers string) part {
		return part{"tja", "chart.tja", []byte("TITLE:" + title + "\nBPM:120\nWAVE:vorbis.ogg\n" + headers + "COURSE:Oni\nLEVEL:5\n#START\n1000,\n#END\n")}
	}
	sound, movie := part{"audio", "vorbis.ogg", audio}, part{"movie", "clip.mp4", clip}
	withMovie := "BGMOVIE:clip.mp4\nMOVIEOFFSET:-0.5\n"

	rejected(send("POST", "/api/charts", web.NewID(), tja("A", withMovie), sound, part{"movie", "other.mp4", clip}), 422, "TJA_MOVIE_MISMATCH")
	rejected(send("POST", "/api/charts", web.NewID(), tja("A", ""), sound, movie), 400, "UPLOAD_FILES_INVALID")
	rejected(send("POST", "/api/charts", web.NewID(), tja("A", "BGMOVIE:clip.avi\n"), sound, part{"movie", "clip.avi", clip}), 400, "UPLOAD_FILES_INVALID")
	app.cleanupReplacedFiles()
	var n int
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM charts)+(SELECT count(*) FROM chart_resources)+(SELECT count(*) FROM retired_files)`).Scan(&n); err != nil || n != 0 {
		t.Fatal("rejected uploads left data", n, err)
	}

	// A movie that cannot be decoded is left out; the chart is still published
	// and its stored TJA no longer declares the movie.
	partial := decode(send("POST", "/api/charts", web.NewID(), tja("Partial", withMovie), sound, part{"movie", "clip.mp4", []byte("not a video")}), 201)
	if partial.MovieHash != "" || len(partial.FailedFiles) != 1 || partial.FailedFiles[0] != (fileFailure{"movie", "MOVIE_INVALID"}) {
		t.Fatalf("undecodable movie: %+v", partial)
	}
	var partialKey string
	if err = pool.QueryRow(ctx, `SELECT r.storage_key FROM charts c JOIN chart_resources r ON r.id=c.tja_file WHERE c.id=$1`, partial.ID).Scan(&partialKey); err != nil {
		t.Fatal(err)
	}
	if data, err := read(partialKey); err != nil || !strings.Contains(string(data), "\nBGMOVIE:\nMOVIEOFFSET:-0.5\n") {
		t.Fatal("stored TJA still declares the rejected movie", err)
	}
	remove := func(id string) int {
		r := httptest.NewRequest("DELETE", "/api/charts/"+id, strings.NewReader("{}"))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", "http://localhost")
		r.Header.Set("X-CSRF-Token", "csrf")
		r.AddCookie(&http.Cookie{Name: "ourtaiko_session", Value: cookie})
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w.Code
	}
	if code := remove(partial.ID); code != 200 {
		t.Fatal("delete", code)
	}
	app.cleanupReplacedFiles()

	key := web.NewID()
	chart := decode(send("POST", "/api/charts", key, tja("A", withMovie), sound, movie), 201)
	if len(chart.MovieHash) != 64 || chart.MovieSize <= 0 || chart.MovieOffset != -0.5 || chart.MovieHash == web.Hash(string(clip)) {
		t.Fatalf("movie not recorded: %+v", chart)
	}
	if retry := decode(send("POST", "/api/charts", key, tja("A", withMovie), sound, movie), 200); retry.ID != chart.ID || retry.MovieHash != chart.MovieHash {
		t.Fatal("retry did not return the stored chart")
	}
	stored, err := read(chart.MovieHash + ".mp4")
	if err != nil || web.Hash(string(stored)) != chart.MovieHash || int64(len(stored)) != chart.MovieSize {
		t.Fatal("stored movie", err)
	}
	// The stored movie is the silent, bounded re-encoding, not the upload.
	file := filepath.Join(t.TempDir(), "stored.mp4")
	if err = os.WriteFile(file, stored, 0600); err != nil {
		t.Fatal(err)
	}
	probe, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "stream=codec_type,codec_name,width,height,r_frame_rate", "-of", "csv=p=0", file).Output()
	if err != nil || strings.TrimSpace(string(probe)) != "h264,video,1280,720,60/1" {
		t.Fatalf("stored movie format %q %v", probe, err)
	}
	// The API no longer tells the stored TJA's hash; its storage key comes from the database.
	tjaKey := func(id string) string {
		var key string
		if err := pool.QueryRow(ctx, `SELECT r.storage_key FROM charts c JOIN chart_resources r ON r.id=c.tja_file WHERE c.id=$1`, id).Scan(&key); err != nil {
			t.Fatal(err)
		}
		return key
	}
	text := get("/api/charts/" + chart.ID + "/tja")
	if remote {
		data, err := read(tjaKey(chart.ID))
		if err != nil {
			t.Fatal(err)
		}
		text.Body = bytes.NewBuffer(data)
	}
	if !strings.Contains(text.Body.String(), "\nBGMOVIE:"+chart.MovieHash+".mp4\nMOVIEOFFSET:-0.5\n") || !strings.Contains(text.Body.String(), "\nWAVE:"+chart.AudioHash+".ogg\n") {
		t.Fatal("stored TJA does not name the stored movie", text.Body.String())
	}
	// The movie alone: redirected to the object host, or served from local storage.
	if w := get("/api/charts/" + chart.ID + "/movie"); !remote && (w.Code != 200 || w.Header().Get("Content-Type") != "video/mp4" || !bytes.Equal(w.Body.Bytes(), stored)) {
		t.Fatal("movie route", w.Code)
	}
	archive := get("/api/charts/" + chart.ID + "/download")
	z, err := zip.NewReader(bytes.NewReader(archive.Body.Bytes()), int64(archive.Body.Len()))
	if err != nil || len(z.File) != 3 || z.File[2].Name != chart.MovieHash+".mp4" || z.File[2].UncompressedSize64 != uint64(chart.MovieSize) {
		t.Fatal("ZIP lacks the movie", err)
	}
	if remote {
		var manifest struct {
			Resources map[string]struct {
				SHA         string `json:"sha256"`
				Size        int64  `json:"size"`
				ContentType string `json:"contentType"`
			}
		}
		w := get("/api/charts/" + chart.ID + "/resources")
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &manifest) != nil {
			t.Fatal(w.Code, w.Body.String())
		}
		if m := manifest.Resources["movie"]; m.SHA != chart.MovieHash || m.Size != chart.MovieSize || m.ContentType != "video/mp4" {
			t.Fatalf("manifest movie %+v", m)
		}
	}

	// The offset alone: the stored TJA is rewritten, its chart and scores kept.
	patch := func(body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("PATCH", "/api/charts/"+chart.ID, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", "http://localhost")
		r.Header.Set("X-CSRF-Token", "csrf")
		r.AddCookie(&http.Cookie{Name: "ourtaiko_session", Value: cookie})
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	declares := func(id, headers string) {
		t.Helper()
		if data, err := read(tjaKey(id)); err != nil || !strings.Contains(string(data), headers) {
			t.Fatalf("stored TJA lacks %q: %s %v", headers, data, err)
		}
	}
	beforeOffset := tjaKey(chart.ID)
	rejected(patch(`{"movieOffset":1300}`), 422, "TJA_MOVIE_OFFSET_INVALID")
	rejected(patch(`{"movieOffset":null}`), 422, "TJA_MOVIE_OFFSET_INVALID")
	shifted := decode(patch(`{"movieOffset":2.5}`), 200)
	if shifted.MovieOffset != 2.5 || shifted.MovieHash != chart.MovieHash || shifted.Title != "A" || len(shifted.Difficulties) != 1 || tjaKey(chart.ID) == beforeOffset {
		t.Fatalf("offset not set: %+v", shifted)
	}
	declares(chart.ID, "\nBGMOVIE:"+chart.MovieHash+".mp4\nMOVIEOFFSET:2.5\n")
	// The movie alone, with or without a new offset.
	other := filepath.Join(t.TempDir(), "other.mp4")
	if out, err := exec.Command("ffmpeg", "-nostdin", "-v", "error", "-f", "lavfi", "-i", "testsrc=size=320x240:rate=30:duration=0.5", "-c:v", "libx264", "-preset", "ultrafast", other).CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}
	otherClip, err := os.ReadFile(other)
	if err != nil {
		t.Fatal(err)
	}
	moviePath := "/api/charts/" + chart.ID + "/movie"
	rejected(send("PUT", moviePath, web.NewID()), 400, "UPLOAD_FILES_INVALID")
	rejected(send("PUT", moviePath, web.NewID(), part{"movie", "clip.avi", otherClip}), 400, "UPLOAD_FILES_INVALID")
	rejected(send("PUT", moviePath, web.NewID(), part{"movieOffset", "", []byte("x")}, part{"movie", "o.mp4", otherClip}), 422, "TJA_MOVIE_OFFSET_INVALID")
	rejected(send("PUT", moviePath, web.NewID(), part{"movie", "o.mp4", []byte("not a video")}), 422, "MOVIE_INVALID")
	swapped := decode(send("PUT", moviePath, web.NewID(), part{"movie", "any name.webm", otherClip}), 200)
	if len(swapped.MovieHash) != 64 || swapped.MovieHash == chart.MovieHash || swapped.MovieOffset != 2.5 || swapped.Title != "A" {
		t.Fatalf("movie not replaced: %+v", swapped)
	}
	declares(chart.ID, "\nBGMOVIE:"+swapped.MovieHash+".mp4\nMOVIEOFFSET:2.5\n")
	app.cleanupReplacedFiles()
	if _, err = read(chart.MovieHash + ".mp4"); !os.IsNotExist(err) {
		t.Fatal("replaced movie still stored", err)
	}
	if _, err = read(beforeOffset); !os.IsNotExist(err) {
		t.Fatal("replaced TJA still stored", err)
	}
	// Back to the first movie, so the checks below see it.
	chart = decode(send("PUT", moviePath, web.NewID(), part{"movie", "clip.mp4", clip}, part{"movieOffset", "", []byte("-0.5")}), 200)
	if chart.MovieOffset != -0.5 {
		t.Fatalf("offset not sent with the movie: %+v", chart)
	}
	declares(chart.ID, "\nBGMOVIE:"+chart.MovieHash+".mp4\nMOVIEOFFSET:-0.5\n")

	// A replacement that still declares BGMOVIE keeps the stored movie without a new upload.
	files := "/api/charts/" + chart.ID + "/files"
	confirm := part{"confirmReset", "", []byte("true")}
	originalKey := tjaKey(chart.ID)
	kept := decode(send("PUT", files, web.NewID(), tja("B", "BGMOVIE:renamed.webm\nMOVIEOFFSET:1.25\n"), confirm), 200)
	if kept.MovieHash != chart.MovieHash || kept.MovieOffset != 1.25 || kept.Title != "B" || tjaKey(kept.ID) == originalKey {
		t.Fatalf("movie not kept: %+v", kept)
	}
	if data, err := read(tjaKey(kept.ID)); err != nil || !strings.Contains(string(data), "\nBGMOVIE:"+chart.MovieHash+".mp4\n") {
		t.Fatal("replacement TJA does not name the kept movie", err)
	}
	// Dropping BGMOVIE removes the movie; the offset falls back to zero.
	plain := decode(send("PUT", files, web.NewID(), tja("C", ""), confirm), 200)
	if plain.MovieHash != "" || plain.MovieSize != 0 || plain.MovieOffset != 0 {
		t.Fatalf("movie not removed: %+v", plain)
	}
	app.cleanupReplacedFiles()
	if _, err = read(chart.MovieHash + ".mp4"); !os.IsNotExist(err) {
		t.Fatal("removed movie still stored", err)
	}
	// A chart without BGMOVIE gains the headers after WAVE.
	rejected(patch(`{"movieOffset":1}`), 422, "MOVIE_NOT_FOUND")
	added := decode(send("PUT", moviePath, web.NewID(), part{"movieOffset", "", []byte("0.75")}, part{"movie", "o.mp4", otherClip}), 200)
	if added.MovieHash != swapped.MovieHash || added.MovieOffset != 0.75 || added.Title != "C" {
		t.Fatalf("movie not added: %+v", added)
	}
	declares(added.ID, "\nWAVE:"+added.AudioHash+".ogg\nBGMOVIE:"+added.MovieHash+".mp4\nMOVIEOFFSET:0.75\n")
	// Adding it back and deleting the chart removes it again.
	again := decode(send("PUT", files, web.NewID(), tja("D", withMovie), confirm, movie), 200)
	if len(again.MovieHash) != 64 {
		t.Fatal("movie not added by replacement")
	}
	code := remove(chart.ID)
	app.cleanupReplacedFiles()
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM charts)+(SELECT count(*) FROM chart_resources)+(SELECT count(*) FROM retired_files)`).Scan(&n); code != 200 || err != nil || n != 0 {
		t.Fatal("delete left data", code, n, err)
	}
	if _, err = read(again.MovieHash + ".mp4"); !os.IsNotExist(err) {
		t.Fatal("deleted chart's movie still stored", err)
	}
}

// Background movies on the official source: a replacement stores the movie transcoded under its hash
// and points BGMOVIE at it, keeps it while the TJA still declares one, drops it when the TJA
// no longer does, leaves out a movie that is not a video, and an upload that asks not to wait
// is answered 202 and reported by an upload_result notification.
func TestBackgroundMovies(t *testing.T) {
	if !tja.Available() {
		t.Skip("tjaparser not found: set TJAPARSER_BIN or put it on PATH")
	}
	dir := t.TempDir()
	movie := filepath.Join(dir, "a.mp4")
	if out, err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "testsrc=duration=2:size=320x240:rate=30", "-c:v", "libx264", "-pix_fmt", "yuv420p", movie).CombinedOutput(); err != nil {
		t.Skip("ffmpeg with libx264 is needed:", err, string(out))
	}
	movieData, err := os.ReadFile(movie)
	if err != nil {
		t.Fatal(err)
	}
	w := interactionWorldOf(t, Official)
	s := w.s
	ctx := context.Background()
	// A chart with files whose audio stays: the movie needs no audio decoding.
	source := []byte("TITLE:Song\nBPM:120\nWAVE:a.ogg\nCOURSE:Oni\nLEVEL:8\n#START\n1100,\n#END\n")
	sum := sha256.Sum256(source)
	digest := hex.EncodeToString(sum[:])
	if err = os.WriteFile(filepath.Join(s.Config.Storage, digest+".tja"), source, 0o600); err != nil {
		t.Fatal(err)
	}
	var tjaID, audioID int64
	err = s.DB.QueryRow(ctx, `INSERT INTO chart_resources(storage_key,original_filename,sha256,byte_size,extension) VALUES($1,'a.tja',$2,$3,'tja') RETURNING id`, digest+".tja", digest, len(source)).Scan(&tjaID)
	if err == nil {
		err = s.DB.QueryRow(ctx, `INSERT INTO chart_resources(storage_key,original_filename,sha256,byte_size,extension) VALUES($1,'a.ogg',$2,1,'ogg') RETURNING id`, digest+".ogg", digest).Scan(&audioID)
	}
	if err == nil {
		_, err = s.DB.Exec(ctx, `UPDATE charts SET title='Song',tja_file=$2,audio_file=$3,duration=10 WHERE id=$1`, w.song, tjaID, audioID)
	}
	if err != nil {
		t.Fatal(err)
	}
	if meta, issue := tja.Parse(source, ""); issue != nil {
		t.Fatal(issue)
	} else if tx, err := s.DB.Begin(ctx); err != nil {
		t.Fatal(err)
	} else {
		if err = setDifficulties(ctx, tx, w.song, meta.Difficulties); err == nil {
			err = tx.Commit(ctx)
		}
		if err != nil {
			t.Fatal(err)
		}
	}

	send := func(chart, movieName string, movie []byte, async bool) *httptest.ResponseRecorder {
		t.Helper()
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		part, _ := form.CreateFormFile("tja", "new.tja")
		part.Write([]byte(chart))
		if movie != nil {
			part, _ = form.CreateFormFile("movie", movieName)
			part.Write(movie)
		}
		form.WriteField("confirmReset", "true")
		form.Close()
		r := httptest.NewRequest("PUT", w.chart(w.song)+"/files", &body)
		r.Header.Set("Content-Type", form.FormDataContentType())
		r.Header.Set("Origin", "http://localhost")
		r.Header.Set("X-CSRF-Token", "csrf")
		r.Header.Set("Idempotency-Key", web.NewID())
		if async {
			r.Header.Set("Prefer", "respond-async")
		}
		r.RemoteAddr = fmt.Sprintf("10.8.%d.1:1234", w.c.n.Add(1)&255)
		r.AddCookie(&http.Cookie{Name: w.c.cookie, Value: w.c.tokens["owner"]})
		rec := httptest.NewRecorder()
		w.c.handler.ServeHTTP(rec, r)
		return rec
	}
	decode := func(rec *httptest.ResponseRecorder) Chart {
		t.Helper()
		if rec.Code != 200 {
			t.Fatal(rec.Code, rec.Body.String())
		}
		var c Chart
		if err := json.Unmarshal(rec.Body.Bytes(), &c); err != nil {
			t.Fatal(err)
		}
		return c
	}
	storedTJA := func() string {
		t.Helper()
		c, err := s.chart(ctx, w.song)
		if err != nil {
			t.Fatal(err)
		}
		f, err := s.Config.Objects.Open(ctx, c.TJAKey)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		data, _ := io.ReadAll(f)
		return string(data)
	}
	withMovie := strings.Replace(string(source), "WAVE:a.ogg", "WAVE:a.ogg\nBGMOVIE:a.mp4\nMOVIEOFFSET:1.5", 1)

	if code := send(string(source), "a.mp4", movieData, false).Code; code != 400 {
		t.Fatal("a movie the TJA does not declare", code)
	}
	// Declared but not sent, with no stored movie to keep: updated without one.
	if bare := decode(send(withMovie, "", nil, false)); bare.MovieHash != "" || !strings.Contains(storedTJA(), "BGMOVIE:\n") {
		t.Fatal("a declared movie that was not sent", bare.MovieHash, storedTJA())
	}
	c := decode(send(withMovie, "a.mp4", movieData, false))
	if c.MovieHash == "" || c.MovieOffset != 1.5 || !strings.Contains(storedTJA(), "BGMOVIE:"+c.MovieHash+".mp4\n") {
		t.Fatal("movie not stored", c.MovieHash, c.MovieOffset, storedTJA())
	}
	var manifest struct {
		Resources map[string]struct{ ContentType string }
	}
	if code := w.c.do("", "GET", w.chart(w.song)+"/resources", "", &manifest); code != 200 || manifest.Resources["movie"].ContentType != "video/mp4" {
		t.Fatal("manifest lacks the movie", code, manifest)
	}
	if code, body := w.c.raw("", w.chart(w.song)+"/movie"); code != 200 || len(body) == 0 {
		t.Fatal("movie not served", code)
	}
	first := c.MovieHash

	// The stored text declares the stored movie: saving it again keeps the movie.
	if kept := decode(send(storedTJA(), "", nil, false)); kept.MovieHash != first {
		t.Fatal("movie not kept", kept.MovieHash)
	}
	// Not a video: the chart is updated and the movie left out.
	broken := decode(send(strings.Replace(withMovie, "a.mp4", "b.mp4", 1), "b.mp4", []byte("not a video"), false))
	if broken.MovieHash != "" || len(broken.FailedFiles) != 1 || broken.FailedFiles[0].Code != "MOVIE_INVALID" || !strings.Contains(storedTJA(), "BGMOVIE:\n") {
		t.Fatal("bad movie not left out", broken.MovieHash, broken.FailedFiles, storedTJA())
	}
	var left int
	if err = s.DB.QueryRow(ctx, `SELECT count(*) FROM chart_resources WHERE sha256=$1`, first).Scan(&left); err != nil || left != 0 {
		t.Fatal("replaced movie not released", left, err)
	}

	// Asked not to wait: 202 now, the result as a notification once processed.
	if rec := send(withMovie, "a.mp4", movieData, true); rec.Code != 202 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	deadline := time.Now().Add(30 * time.Second)
	var status string
	for time.Now().Before(deadline) {
		if err = s.DB.QueryRow(ctx, `SELECT data->>'status' FROM notifications WHERE recipient_id=$1 AND kind='upload_result' ORDER BY created_at DESC LIMIT 1`, testUsers["owner"].ID).Scan(&status); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if status != "updated" {
		t.Fatal("no upload_result notification", status, err)
	}
	if c, err = s.chart(ctx, w.song); err != nil || c.MovieHash == "" {
		t.Fatal("background upload did not store the movie", err)
	}

	// The offset alone: the stored TJA is rewritten and the old one released.
	released := func(digest string) {
		t.Helper()
		var n int
		if err := s.DB.QueryRow(ctx, `SELECT count(*) FROM chart_resources WHERE sha256=$1`, digest).Scan(&n); err != nil || n != 0 {
			t.Fatal("replaced file not released", digest, n, err)
		}
	}
	if code := w.c.do("owner", "PATCH", w.chart(w.song), `{"movieOffset":1300}`, nil); code != 422 {
		t.Fatal("offset out of range", code)
	}
	beforeTJA := c.TJAHash
	var shifted Chart
	if code := w.c.do("owner", "PATCH", w.chart(w.song), `{"movieOffset":-2.25}`, &shifted); code != 200 || shifted.MovieOffset != -2.25 || shifted.MovieHash != c.MovieHash || len(shifted.Difficulties) != 1 {
		t.Fatal("offset not set", code, shifted.MovieOffset)
	}
	if !strings.Contains(storedTJA(), "\nBGMOVIE:"+c.MovieHash+".mp4\nMOVIEOFFSET:-2.25\n") {
		t.Fatal("stored TJA lacks the offset", storedTJA())
	}
	released(beforeTJA)
	// The movie alone, with its offset; a bad one changes nothing.
	sendMovie := func(offset string, name string, data []byte) *httptest.ResponseRecorder {
		t.Helper()
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		if offset != "" {
			form.WriteField("movieOffset", offset)
		}
		if data != nil {
			part, _ := form.CreateFormFile("movie", name)
			part.Write(data)
		}
		form.Close()
		r := httptest.NewRequest("PUT", w.chart(w.song)+"/movie", &body)
		r.Header.Set("Content-Type", form.FormDataContentType())
		r.Header.Set("Origin", "http://localhost")
		r.Header.Set("X-CSRF-Token", "csrf")
		r.RemoteAddr = fmt.Sprintf("10.9.%d.1:1234", w.c.n.Add(1)&255)
		r.AddCookie(&http.Cookie{Name: w.c.cookie, Value: w.c.tokens["owner"]})
		rec := httptest.NewRecorder()
		w.c.handler.ServeHTTP(rec, r)
		return rec
	}
	if code := sendMovie("", "", nil).Code; code != 400 {
		t.Fatal("no movie", code)
	}
	if code := sendMovie("x", "o.mp4", movieData).Code; code != 422 {
		t.Fatal("bad offset", code)
	}
	if rec := sendMovie("", "o.mp4", []byte("not a video")); rec.Code != 422 || !strings.Contains(rec.Body.String(), "MOVIE_INVALID") {
		t.Fatal("bad movie", rec.Code, rec.Body.String())
	}
	other := filepath.Join(dir, "b.mp4")
	if out, err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "testsrc2=duration=1:size=320x240:rate=30", "-c:v", "libx264", "-pix_fmt", "yuv420p", other).CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}
	otherData, err := os.ReadFile(other)
	if err != nil {
		t.Fatal(err)
	}
	shifted, _ = s.chart(ctx, w.song)
	swapped := decode(sendMovie("0.75", "any name.webm", otherData))
	if swapped.MovieHash == "" || swapped.MovieHash == c.MovieHash || swapped.MovieOffset != 0.75 || swapped.Title != c.Title {
		t.Fatal("movie not replaced", swapped.MovieHash, swapped.MovieOffset)
	}
	if !strings.Contains(storedTJA(), "\nBGMOVIE:"+swapped.MovieHash+".mp4\nMOVIEOFFSET:0.75\n") {
		t.Fatal("stored TJA does not name the new movie", storedTJA())
	}
	released(c.MovieHash)
	released(shifted.TJAHash)
	c = swapped

	// A new chart (only administrators upload) goes through the same job: its audio decoded,
	// its movie stored.
	oggData, err := os.ReadFile("../../kit/audio/testdata/vorbis.ogg")
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	for _, f := range []struct {
		field, name string
		data        []byte
	}{{"tja", "new.tja", []byte(strings.Replace(withMovie, "WAVE:a.ogg", "WAVE:vorbis.ogg", 1))}, {"audio", "vorbis.ogg", oggData}, {"movie", "a.mp4", movieData}} {
		part, _ := form.CreateFormFile(f.field, f.name)
		part.Write(f.data)
	}
	form.Close()
	r := httptest.NewRequest("POST", "/api/charts", &body)
	r.Header.Set("Content-Type", form.FormDataContentType())
	r.Header.Set("Origin", "http://localhost")
	r.Header.Set("X-CSRF-Token", "csrf")
	r.Header.Set("Idempotency-Key", web.NewID())
	r.RemoteAddr = "10.8.200.1:1234"
	r.AddCookie(&http.Cookie{Name: w.c.cookie, Value: w.c.tokens["admin"]})
	rec := httptest.NewRecorder()
	w.c.handler.ServeHTTP(rec, r)
	var published Chart
	if rec.Code != 201 || json.Unmarshal(rec.Body.Bytes(), &published) != nil || published.MovieHash == "" || published.Duration <= 0 || published.ID == w.song {
		t.Fatal("new chart with a movie", rec.Code, rec.Body.String())
	}
}
