package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ourtaiko.dev/net/internal/songs/teststore"
	"ourtaiko.dev/net/internal/web"
)

// A playlist's cover: one WebP, stored, served, linked for the game as it is, and deleted
// once nothing refers to it.
func TestPlaylistCover(t *testing.T) { bothSources(t, testPlaylistCover) }

func testPlaylistCover(t *testing.T, w interactionWorld) {
	c, s, ctx := w.c, w.s, context.Background()
	owner := testUsers["owner"].ID
	var p Playlist
	if code := c.do("owner", "POST", "/api/playlists", `{"name":"Covered","public":true}`, &p); code != 201 {
		t.Fatal(code)
	}
	path := "/api/playlists/" + p.ID + "/cover"
	var picture bytes.Buffer
	if e := png.Encode(&picture, image.NewNRGBA(image.Rect(0, 0, 40, 30))); e != nil {
		t.Fatal(e)
	}
	upload := func(user string, data []byte) *httptest.ResponseRecorder {
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		out, _ := form.CreateFormFile("cover", "cover.png")
		_, _ = out.Write(data)
		_ = form.Close()
		r := httptest.NewRequest("PUT", path, &body)
		r.Header.Set("Origin", "http://localhost")
		r.Header.Set("Content-Type", form.FormDataContentType())
		r.Header.Set("X-CSRF-Token", "csrf")
		r.AddCookie(&http.Cookie{Name: c.cookie, Value: c.tokens[user]})
		rec := httptest.NewRecorder()
		c.handler.ServeHTTP(rec, r)
		return rec
	}
	if rec := upload("alice", picture.Bytes()); rec.Code != 403 {
		t.Fatal("foreign cover", rec.Code)
	}
	if rec := upload("owner", []byte("not an image")); rec.Code != 422 {
		t.Fatal("invalid cover", rec.Code)
	}
	rec := upload("owner", picture.Bytes())
	var result struct{ CoverHash string }
	if e := json.Unmarshal(rec.Body.Bytes(), &result); e != nil || rec.Code != 200 || len(result.CoverHash) != 64 {
		t.Fatal("cover", rec.Code, rec.Body.String())
	}
	if code := c.do("", "GET", "/api/playlists/"+p.ID, "", &p); code != 200 || p.CoverHash != result.CoverHash {
		t.Fatal("cover hash", code, p.CoverHash)
	}
	// Covers are read from the bucket only: there is no API route to read one.
	rec = httptest.NewRecorder()
	c.handler.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	if rec.Code != 404 || web.Hash(string(storedCover(t, s, "playlists", p.ID))) != result.CoverHash {
		t.Fatal("stored cover", rec.Code)
	}

	// Only the WebP is stored: the game is given the same file.
	var coverKey string
	var coverSize, files int64
	if e := s.DB.QueryRow(ctx, `SELECT r.storage_key,r.byte_size FROM playlists p JOIN chart_resources r ON r.id=p.cover_file WHERE p.id=$1`, p.ID).Scan(&coverKey, &coverSize); e != nil || coverKey != result.CoverHash+".webp" {
		t.Fatal("stored cover", coverKey, e)
	}
	if e := s.DB.QueryRow(ctx, `SELECT count(*) FROM chart_resources WHERE extension IN ('webp','jpg')`).Scan(&files); e != nil || files != 1 {
		t.Fatal("cover files", files, e)
	}

	// Its folder in the game lists songs of the catalog: with files and in a category. Song is
	// one; Other is not, for want of files on the official source and of a category on Fanmade.
	var e2 error
	if s.source().Official {
		var tja, audio int64
		digest := web.NewID() + web.NewID()
		e2 = s.DB.QueryRow(ctx, `INSERT INTO chart_resources(storage_key,original_filename,sha256,byte_size,extension) VALUES($1||'.tja','a.tja',$1,1,'tja') RETURNING id`, digest).Scan(&tja)
		if e2 == nil {
			e2 = s.DB.QueryRow(ctx, `INSERT INTO chart_resources(storage_key,original_filename,sha256,byte_size,extension) VALUES($1||'.ogg','a.ogg',$1,1,'ogg') RETURNING id`, digest).Scan(&audio)
		}
		if e2 == nil {
			_, e2 = s.DB.Exec(ctx, `UPDATE charts SET tja_file=$1,audio_file=$2,duration=10 WHERE id=$3`, tja, audio, w.song)
		}
	}
	if e2 == nil {
		_, e2 = s.DB.Exec(ctx, `UPDATE charts SET category_flags=CASE WHEN id=$1 THEN 1 ELSE 0 END WHERE id IN ($1,$2)`, w.song, w.other)
	}
	if e2 != nil {
		t.Fatal(e2)
	}
	if code := c.do("owner", "PUT", "/api/playlists/"+p.ID+"/items/"+w.song, `{}`, nil); code != 201 {
		t.Fatal("add", code)
	}
	if code := c.do("owner", "PUT", "/api/playlists/"+p.ID+"/items/"+w.other, `{}`, nil); code != 201 {
		t.Fatal("add a song outside the catalog", code)
	}
	folders := func(app *Server) []GamePlaylist {
		t.Helper()
		tx, e := s.DB.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer tx.Rollback(ctx)
		items, e := s.gamePlaylists(ctx, tx, &User{ID: owner})
		if e == nil {
			e = app.gamePlaylistDetails(ctx, items)
		}
		if e != nil || len(items) == 0 || items[0].ID != p.ID || items[0].Kind != "owned" {
			t.Fatal("game folders", e, items)
		}
		return items
	}
	// The song outside the catalog is not in the game's folder.
	if own := folders(s)[0]; own.Cover != nil || strings.Join(own.ChartIDs, ",") != w.song || own.Owner == "" {
		t.Fatalf("folder without direct downloads %+v", own)
	}
	personal := personalGamePlaylists(folders(s))
	if len(personal) != 1 || personal[0].ID != p.ID {
		t.Fatal("the account's own folders", personal)
	}
	// A bucket without a public address gets signed links.
	direct := &Server{DB: s.DB, Config: Config{Source: s.source(), Objects: teststore.New(t), SSO: s.Config.SSO}}
	if link := folders(direct)[0].Cover; link == nil || !strings.Contains(link.URL, coverKey) || link.HeadURL == "" ||
		link.SHA256 != result.CoverHash || link.Size != coverSize || link.ContentType != "image/webp" {
		t.Fatalf("cover link %+v", link)
	}

	covers := func() int {
		var n int
		if e := s.DB.QueryRow(ctx, `SELECT count(*) FROM chart_resources WHERE extension IN ('webp','jpg')`).Scan(&n); e != nil {
			t.Fatal(e)
		}
		return n
	}
	// A song sharing the image keeps the file when the playlist releases it.
	if _, e := s.DB.Exec(ctx, `UPDATE charts SET cover_file=(SELECT id FROM chart_resources WHERE sha256=$1) WHERE id=$2`, result.CoverHash, w.song); e != nil {
		t.Fatal(e)
	}
	if code := c.do("owner", "DELETE", path, `{}`, nil); code != 200 || covers() != 1 {
		t.Fatal("removing a cover a song shares", code, covers())
	}
	if storedCover(t, s, "playlists", p.ID) != nil {
		t.Fatal("removed cover still named")
	}
	if rec := upload("owner", picture.Bytes()); rec.Code != 200 || covers() != 1 {
		t.Fatal("cover again", rec.Code, covers())
	}
	if _, e := s.DB.Exec(ctx, `UPDATE charts SET cover_file=NULL WHERE id=$1`, w.song); e != nil {
		t.Fatal(e)
	}
	// The queue of replaced files leaves a cover alone while its playlist refers to it.
	s.cleanupReplacedFiles()
	if covers() != 1 {
		t.Fatal("playlist cover deleted while referenced", covers())
	}
	if code := c.do("owner", "DELETE", "/api/playlists/"+p.ID, `{}`, nil); code != 200 || covers() != 0 {
		t.Fatal("cover kept after its playlist", code, covers())
	}
	if _, e := s.Config.Objects.Open(ctx, coverKey); e == nil {
		t.Fatal("cover object kept", coverKey)
	}
}
