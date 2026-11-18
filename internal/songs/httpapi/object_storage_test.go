package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ourtaiko.dev/net/internal/songs/teststore"
	"ourtaiko.dev/net/internal/web"
)

func verifyResourceLinks(t *testing.T, h http.Handler, chartID string, audio []byte) {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/charts/"+chartID+"/resources", nil))
	var payload struct {
		ChartID string `json:"chartId"`

		Resources map[string]struct {
			Path string `json:"path"`
			SHA  string `json:"sha256"`
			Size int64  `json:"size"`
		}
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &payload) != nil {
		t.Fatal("resources", w.Code, w.Body.String())
	}
	if payload.ChartID != chartID {
		t.Fatal("resource identity must be chart only")
	}
	// TJA, audio and preview; the ZIP is built on request, not stored.
	if _, stored := payload.Resources["download"]; stored || len(payload.Resources) != 3 {
		t.Fatal("missing direct resources")
	}
	for kind, v := range payload.Resources {
		// Each is a file under the distribution root: in the bucket, under its prefix.
		if !strings.HasPrefix(v.Path, "fanmade/test/") || strings.ContainsAny(v.Path, "?#") {
			t.Fatal("resource path", kind, v.Path)
		}
		// Its bytes are what the manifest says, read here through the API's own route.
		r := httptest.NewRecorder()
		h.ServeHTTP(r, httptest.NewRequest("GET", "/api/charts/"+chartID+"/"+kind, nil))
		data := r.Body.Bytes()
		if r.Code != 200 || int64(len(data)) != v.Size || web.Hash(string(data)) != v.SHA {
			t.Fatal("direct integrity", kind, r.Code)
		}
		if kind == "audio" && !bytes.Equal(data, audio) {
			t.Fatal("direct audio changed")
		}
	}
}
func TestS3PendingRecovery(t *testing.T) {
	pool := testDB(t, Fanmade)
	ctx := context.Background()
	store := teststore.New(t)
	app := testServer(t, pool, Config{Objects: store, Storage: t.TempDir()})
	for _, key := range []string{"abandoned", "recent", "referenced"} {
		teststore.Seed(t, store, key, []byte("test"))
	}
	_, e := pool.Exec(ctx, `INSERT INTO pending_objects(storage_key,created_at) VALUES('abandoned',now()-interval '2 days'),('recent',now()),('referenced',now()-interval '2 days'); INSERT INTO retired_files(storage_key) VALUES('referenced'); BEGIN; INSERT INTO users(id) VALUES('u');
 INSERT INTO charts(id,owner_ids,title,bpm,duration,tja_file,audio_file) VALUES('c',ARRAY['u'],'Recovery',120,10,test_file('referenced','test',repeat('a',64),4),test_file('referenced','test',repeat('a',64),4)); COMMIT;`)
	if e != nil {
		t.Fatal(e)
	}
	if e = app.reconcilePending(ctx); e != nil {
		t.Fatal(e)
	}
	if e = app.deleteRetiredFiles(ctx); e != nil {
		t.Fatal(e)
	}
	if f, e := store.Open(ctx, "abandoned"); e == nil {
		f.Close()
		t.Fatal("orphan retained")
	}
	for _, key := range []string{"recent", "referenced"} {
		f, e := store.Open(ctx, key)
		if e != nil {
			t.Fatal("live/pending deleted", e)
		}
		f.Close()
	}
	w := httptest.NewRecorder()
	app.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/game/catalog", nil))
	if w.Code != 200 {
		t.Fatal("game inaccessible", w.Code)
	}
}
