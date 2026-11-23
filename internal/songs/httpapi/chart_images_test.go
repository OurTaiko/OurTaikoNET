package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ourtaiko.dev/net/internal/kit/tja"
	"ourtaiko.dev/net/internal/web"
)

// The uploaded cover is the image PREIMAGE names and the background image the one BGIMAGE
// names, which it must declare; the stored TJA names the chart's stored images, as its
// package holds them, also after the cover alone changes; and a replacement still declaring
// BGIMAGE keeps the stored image.
func TestChartImagesFollowTheirHeaders(t *testing.T) {
	bothSources(t, testChartImagesFollowTheirHeaders)
}

func testChartImagesFollowTheirHeaders(t *testing.T, w interactionWorld) {
	if !tja.Available() {
		t.Skip("tjaparser not found: set TJAPARSER_BIN or put it on PATH")
	}
	s := w.s
	// Whoever may add charts on either source sends the new uploads.
	sender := "admin"
	ctx := context.Background()
	audio, err := os.ReadFile("../../kit/audio/testdata/vorbis.ogg")
	if err != nil {
		t.Fatal(err)
	}
	picture := func(c color.NRGBA) []byte {
		img := image.NewNRGBA(image.Rect(0, 0, 40, 30))
		for y := 0; y < 30; y++ {
			for x := 0; x < 40; x++ {
				img.SetNRGBA(x, y, c)
			}
		}
		var b bytes.Buffer
		_ = png.Encode(&b, img)
		return b.Bytes()
	}
	// Distinct images: equal ones would share one stored file (TestStoreFileTwiceInOneChange).
	red, blue, green := picture(color.NRGBA{R: 255, A: 255}), picture(color.NRGBA{B: 255, A: 255}), picture(color.NRGBA{G: 255, A: 255})
	type part struct {
		field, name string
		data        []byte
	}
	send := func(method, path string, parts ...part) *httptest.ResponseRecorder {
		t.Helper()
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
		r.Header.Set("Idempotency-Key", web.NewID())
		// A fresh client address per request keeps the per-address write limit out of the way.
		r.RemoteAddr = fmt.Sprintf("10.9.%d.1:1234", w.c.n.Add(1)&255)
		r.AddCookie(&http.Cookie{Name: w.c.cookie, Value: w.c.tokens[sender]})
		rec := httptest.NewRecorder()
		w.c.handler.ServeHTTP(rec, r)
		return rec
	}
	upload := func(want int, parts ...part) Chart {
		t.Helper()
		rec := send("POST", "/api/charts", parts...)
		if rec.Code != want {
			t.Fatalf("status %d want %d: %s", rec.Code, want, rec.Body.String())
		}
		var c Chart
		_ = json.Unmarshal(rec.Body.Bytes(), &c)
		return c
	}
	chart := func(title, headers string) part {
		return part{"tja", "chart.tja", []byte("TITLE:" + title + "\nBPM:120\n" + headers + "WAVE:music.ogg\nCOURSE:Oni\nLEVEL:5\n#START\n1000,\n#END\n")}
	}
	sound := part{"audio", "music.ogg", audio}
	stored := func(id string) string {
		t.Helper()
		var key string
		if err := s.DB.QueryRow(ctx, `SELECT r.storage_key FROM charts c JOIN chart_resources r ON r.id=c.tja_file WHERE c.id=$1`, id).Scan(&key); err != nil {
			t.Fatal(err)
		}
		f, err := s.Config.Objects.Open(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		data, _ := io.ReadAll(f)
		return string(data)
	}
	declares := func(id, want string) {
		t.Helper()
		if text := stored(id); !strings.Contains(text, want) {
			t.Fatalf("stored TJA of %s lacks %q:\n%s", id, want, text)
		}
	}

	// PREIMAGE may give a path; the cover is matched by its name and stored as WebP.
	named := upload(201, chart("A", "PREIMAGE:img\\jacket.png\n"), sound, part{"cover", "jacket.png", red})
	if named.CoverHash == "" {
		t.Fatal("cover not saved")
	}
	declares(named.ID, "\nPREIMAGE:"+named.CoverHash+".webp\n")

	rec := send("POST", "/api/charts", chart("B", "PREIMAGE:jacket.png\n"), sound, part{"cover", "other.png", red})
	if rec.Code != 422 || !strings.Contains(rec.Body.String(), `"TJA_PREIMAGE_MISMATCH"`) {
		t.Fatalf("mismatched cover: %d %s", rec.Code, rec.Body.String())
	}

	// A cover the TJA does not declare is added after WAVE.
	added := upload(201, chart("C", ""), sound, part{"cover", "art.png", blue})
	declares(added.ID, "\nWAVE:"+added.AudioHash+".ogg\nPREIMAGE:"+added.CoverHash+".webp\n")

	// Without a cover the declared image is cleared rather than left pointing at nothing.
	bare := upload(201, chart("D", "PREIMAGE:jacket.png\n"), sound)
	if bare.CoverHash != "" {
		t.Fatal("unexpected cover")
	}
	declares(bare.ID, "\nPREIMAGE:\n")
	plain := upload(201, chart("E", ""), sound)
	if strings.Contains(stored(plain.ID), "PREIMAGE") {
		t.Fatal("a chart without a cover gained PREIMAGE")
	}

	// Changing the cover alone rewrites PREIMAGE, adding it when missing.
	for _, id := range []string{bare.ID, plain.ID} {
		rec = send("PUT", "/api/charts/"+id+"/cover", part{"cover", "new.png", blue})
		if rec.Code != 200 {
			t.Fatalf("cover change: %d %s", rec.Code, rec.Body.String())
		}
		var changed struct{ CoverHash string }
		_ = json.Unmarshal(rec.Body.Bytes(), &changed)
		declares(id, "PREIMAGE:"+changed.CoverHash+".webp\n")
	}

	// A file replacement without a cover keeps the chart's cover, which PREIMAGE then names.
	rec = send("PUT", "/api/charts/"+named.ID+"/files", chart("A2", "PREIMAGE:renamed.jpg\n"), part{"confirmReset", "", []byte("true")})
	if rec.Code != 200 {
		t.Fatalf("replacement: %d %s", rec.Code, rec.Body.String())
	}
	declares(named.ID, "\nPREIMAGE:"+named.CoverHash+".webp\n")
	// ... and one with a cover must name it as PREIMAGE does.
	rec = send("PUT", "/api/charts/"+named.ID+"/files", chart("A3", "PREIMAGE:renamed.jpg\n"), part{"confirmReset", "", []byte("true")}, part{"cover", "jacket.png", blue})
	if rec.Code != 422 || !strings.Contains(rec.Body.String(), `"TJA_PREIMAGE_MISMATCH"`) {
		t.Fatalf("mismatched replacement cover: %d %s", rec.Code, rec.Body.String())
	}

	// BGIMAGE works as BGMOVIE: its file must be the declared one, stored as WebP.
	rec = send("POST", "/api/charts", chart("F", ""), sound, part{"bgimage", "bg.png", red})
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), `"UPLOAD_FILES_INVALID"`) {
		t.Fatalf("undeclared background: %d %s", rec.Code, rec.Body.String())
	}
	rec = send("POST", "/api/charts", chart("F", "BGIMAGE:bg.png\n"), sound, part{"bgimage", "other.png", red})
	if rec.Code != 422 || !strings.Contains(rec.Body.String(), `"TJA_BGIMAGE_MISMATCH"`) {
		t.Fatalf("mismatched background: %d %s", rec.Code, rec.Body.String())
	}
	backdrop := upload(201, chart("F", "BGIMAGE:img/BG.png\n"), sound, part{"bgimage", "BG.png", red}, part{"cover", "art.png", blue})
	if backdrop.BackgroundHash == "" || backdrop.BackgroundHash == backdrop.CoverHash {
		t.Fatalf("background not saved apart from the cover: %+v", backdrop.BackgroundHash)
	}
	declares(backdrop.ID, "\nBGIMAGE:"+backdrop.BackgroundHash+".webp\n")
	get := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		w.c.handler.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		return rec
	}
	// With public links the route redirects to the stored object.
	if rec = get("/api/charts/" + backdrop.ID + "/bgimage"); (rec.Code != 200 && rec.Code != 302) || (rec.Code == 302 && !strings.Contains(rec.Header().Get("Location"), backdrop.BackgroundHash+".webp")) {
		t.Fatalf("background route: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	if rec = get("/api/charts/" + backdrop.ID + "/resources"); !strings.Contains(rec.Body.String(), `"bgimage"`) {
		t.Fatalf("resources lack the background: %s", rec.Body.String())
	}
	// Declared without a file: published without one.
	missing := upload(201, chart("G", "BGIMAGE:bg.png\n"), sound)
	if missing.BackgroundHash != "" {
		t.Fatal("unexpected background")
	}
	declares(missing.ID, "\nBGIMAGE:\n")
	// BGMOVIE likewise: declared without a file, the chart is published without a movie.
	unfilmed := upload(201, chart("H", "BGMOVIE:clip.mp4\nMOVIEOFFSET:1.5\n"), sound)
	if unfilmed.MovieHash != "" {
		t.Fatal("unexpected movie")
	}
	declares(unfilmed.ID, "\nBGMOVIE:\n")
	// A replacement still declaring BGIMAGE keeps the stored image; one without drops it.
	rec = send("PUT", "/api/charts/"+backdrop.ID+"/files", chart("F2", "BGIMAGE:elsewhere.jpg\n"), part{"confirmReset", "", []byte("true")})
	if rec.Code != 200 {
		t.Fatalf("replacement keeping the background: %d %s", rec.Code, rec.Body.String())
	}
	declares(backdrop.ID, "\nBGIMAGE:"+backdrop.BackgroundHash+".webp\n")
	rec = send("PUT", "/api/charts/"+backdrop.ID+"/files", chart("F3", ""), part{"confirmReset", "", []byte("true")})
	var dropped Chart
	_ = json.Unmarshal(rec.Body.Bytes(), &dropped)
	if rec.Code != 200 || dropped.BackgroundHash != "" || strings.Contains(stored(backdrop.ID), "BGIMAGE") {
		t.Fatalf("replacement dropping the background: %d %s", rec.Code, rec.Body.String())
	}
	if rec = get("/api/charts/" + backdrop.ID + "/bgimage"); rec.Code != 404 {
		t.Fatalf("dropped background still served: %d", rec.Code)
	}

	// A replacement whose audio stays, by the chart's owner: nothing is decoded again.
	sender = "owner"
	source := []byte("TITLE:Song\nBPM:120\nWAVE:a.ogg\nCOURSE:Oni\nLEVEL:8\n#START\n1100,\n#END\n")
	sum := sha256.Sum256(source)
	digest := hex.EncodeToString(sum[:])
	if err := os.WriteFile(filepath.Join(s.Config.Storage, digest+".tja"), source, 0o600); err != nil {
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
	files := w.chart(w.song) + "/files"
	again := func(headers string) part {
		return part{"tja", "new.tja", []byte(strings.Replace(string(source), "WAVE:a.ogg\n", "WAVE:a.ogg\n"+headers, 1))}
	}
	confirm := part{"confirmReset", "", []byte("true")}
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
	refused := func(rec *httptest.ResponseRecorder, status int, code string) {
		t.Helper()
		if rec.Code != status || !strings.Contains(rec.Body.String(), `"`+code+`"`) {
			t.Fatalf("want %d %s, got %d %s", status, code, rec.Code, rec.Body.String())
		}
	}

	refused(send("PUT", files, again("PREIMAGE:jacket.png\n"), confirm, part{"cover", "other.png", red}), 422, "TJA_PREIMAGE_MISMATCH")
	refused(send("PUT", files, again(""), confirm, part{"bgimage", "bg.png", red}), 400, "UPLOAD_FILES_INVALID")
	refused(send("PUT", files, again("BGIMAGE:bg.png\n"), confirm, part{"bgimage", "other.png", red}), 422, "TJA_BGIMAGE_MISMATCH")

	both := decode(send("PUT", files, again("PREIMAGE:img\\jacket.png\nBGIMAGE:../art/BG.png\n"), confirm, part{"cover", "jacket.png", red}, part{"bgimage", "BG.png", green}))
	if both.CoverHash == "" || both.BackgroundHash == "" {
		t.Fatal("images not stored", both.CoverHash, both.BackgroundHash)
	}
	declares(w.song, "\nPREIMAGE:"+both.CoverHash+".webp\nBGIMAGE:"+both.BackgroundHash+".webp\n")
	if code, body := w.c.raw("", w.chart(w.song)+"/bgimage"); code != 200 || web.Hash(string(body)) != both.BackgroundHash {
		t.Fatal("background image not served", code)
	}
	var manifest struct {
		Resources map[string]struct{ ContentType string }
	}
	if code := w.c.do("", "GET", w.chart(w.song)+"/resources", "", &manifest); code != 200 || manifest.Resources["bgimage"].ContentType != "image/webp" {
		t.Fatal("manifest lacks the background image", code, manifest)
	}

	// The cover alone: PREIMAGE follows and the old TJA is released.
	before := stored(w.song)
	rec = send("PUT", w.chart(w.song)+"/cover", part{"cover", "new.png", blue})
	var changed struct{ CoverHash string }
	if _ = json.Unmarshal(rec.Body.Bytes(), &changed); rec.Code != 200 || changed.CoverHash == "" {
		t.Fatal("cover change", rec.Code, rec.Body.String())
	}
	declares(w.song, "\nPREIMAGE:"+changed.CoverHash+".webp\n")
	var left int
	beforeSum := sha256.Sum256([]byte(before))
	if err = s.DB.QueryRow(ctx, `SELECT count(*) FROM chart_resources WHERE sha256=$1`, hex.EncodeToString(beforeSum[:])).Scan(&left); err != nil || left != 0 {
		t.Fatal("replaced TJA not released", left, err)
	}

	// Still declaring BGIMAGE without a file keeps the stored image; dropping it releases it.
	kept := decode(send("PUT", files, again("BGIMAGE:renamed.jpg\n"), confirm))
	if kept.BackgroundHash != both.BackgroundHash || kept.CoverHash != changed.CoverHash {
		t.Fatal("images not kept", kept.BackgroundHash, kept.CoverHash)
	}
	declares(w.song, "\nPREIMAGE:"+changed.CoverHash+".webp\nBGIMAGE:"+both.BackgroundHash+".webp\n")
	gone := decode(send("PUT", files, again(""), confirm))
	if gone.BackgroundHash != "" || strings.Contains(stored(w.song), "BGIMAGE") {
		t.Fatal("background image not dropped", gone.BackgroundHash, stored(w.song))
	}
	if err = s.DB.QueryRow(ctx, `SELECT count(*) FROM chart_resources WHERE sha256=$1`, both.BackgroundHash).Scan(&left); err != nil || left != 0 {
		t.Fatal("dropped background image not released", left, err)
	}
}
