package objectstore

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// A store that only reads serves what the public address serves, in ranges, and refuses
// every write without asking anything of the address.
func TestAPublicStoreReadsAndNeverWrites(t *testing.T) {
	const content = "0123456789abcdefghij"
	modified := time.Date(2026, 11, 11, 14, 4, 11, 0, time.UTC)
	var requests []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path+" "+r.Header.Get("Range"))
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			t.Errorf("the public address was sent %s", r.Method)
		}
		switch r.URL.Path {
		case "/fanmade/song.ogg":
			if match := r.Header.Get("If-Match"); r.Method == http.MethodGet && match != `"v1"` {
				t.Errorf("If-Match = %q", match)
			}
			w.Header().Set("ETag", `"v1"`)
			http.ServeContent(w, r, "song.ogg", modified, strings.NewReader(content))
		default:
			// What a bucket that may not be listed says of a key it does not hold.
			w.WriteHeader(http.StatusForbidden)
		}
	}))
	defer server.Close()

	store, err := NewPublic("fanmade/", server.URL+"/fanmade")
	if err != nil {
		t.Fatal(err)
	}
	store.http = server.Client()
	if !store.ReadOnly() || store.Prefix != "fanmade/" {
		t.Fatal(store.ReadOnly(), store.Prefix)
	}
	ctx := context.Background()

	f, err := store.Open(ctx, "song.ogg")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if f.Size() != int64(len(content)) || !f.ModTime().Equal(modified) {
		t.Fatal(f.Size(), f.ModTime())
	}
	if all, err := io.ReadAll(f); err != nil || string(all) != content {
		t.Fatal(string(all), err)
	}
	if _, err := f.Seek(10, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	part := make([]byte, 4)
	if _, err := io.ReadFull(f, part); err != nil || string(part) != "abcd" {
		t.Fatal(string(part), err)
	}
	if _, err := f.Seek(-3, io.SeekEnd); err != nil {
		t.Fatal(err)
	}
	if rest, err := io.ReadAll(f); err != nil || string(rest) != "hij" {
		t.Fatal(string(rest), err)
	}

	if _, err := store.Open(ctx, "missing.ogg"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if _, err := store.Open(ctx, "../secret"); !errors.Is(err, ErrKey) {
		t.Fatal(err)
	}

	before := len(requests)
	digest := strings.Repeat("0", 64)
	if err := store.Put(ctx, "new.ogg", strings.NewReader("x"), 1, "audio/ogg", digest); !errors.Is(err, ErrReadOnly) {
		t.Fatal(err)
	}
	if err := store.Delete(ctx, "song.ogg"); !errors.Is(err, ErrReadOnly) {
		t.Fatal(err)
	}
	if _, err := store.Sign(ctx, "song.ogg", "GET", "song.ogg", "audio/ogg", time.Minute); err == nil {
		t.Fatal("signed a link without credentials")
	}
	if len(requests) != before {
		t.Fatal("a refused write reached the address:", requests[before:])
	}
	if link, err := store.PublicURL("song.ogg"); err != nil || link != server.URL+"/fanmade/song.ogg" {
		t.Fatal(link, err)
	}
}

func TestAPublicStoreNeedsItsPrefixAndAddress(t *testing.T) {
	for _, c := range [][2]string{{"", "https://cdn.example/fanmade"}, {"fanmade", ""}, {"../x", "https://cdn.example/x"}, {"fanmade", "http://cdn.example/fanmade"}} {
		if _, err := NewPublic(c[0], c[1]); err == nil {
			t.Fatalf("accepted %q", c)
		}
	}
}
