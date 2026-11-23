package httpapi

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ourtaiko.dev/net/internal/web"
)

// One change may store the same content twice: an upload whose cover and background image
// are the same picture. Both get the one file, and the second does not wait for the change
// it is part of.
func TestStoreFileTwiceInOneChange(t *testing.T) {
	bothSources(t, testStoreFileTwiceInOneChange)
}

func testStoreFileTwiceInOneChange(t *testing.T, w interactionWorld) {
	s := w.s
	// Waiting for its own transaction never ends by itself: only this deadline would.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	data := []byte("picture " + web.NewID())
	digest := web.Hash(string(data))
	path := filepath.Join(t.TempDir(), "picture.webp")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	store := func(name string) int64 {
		t.Helper()
		id, err := s.storeFile(ctx, tx, path, name, "webp", "image/webp", digest, int64(len(data)))
		if err != nil {
			t.Fatalf("storing %s: %v", name, err)
		}
		return id
	}
	first := store("cover.webp")
	if second := store("bgimage.webp"); second != first {
		t.Fatalf("the same content got files %d and %d", first, second)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var files, pending int
	if err := s.DB.QueryRow(ctx, `SELECT (SELECT count(*) FROM chart_resources WHERE sha256=$1),(SELECT count(*) FROM pending_objects WHERE storage_key=$2)`,
		digest, digest+".webp").Scan(&files, &pending); err != nil {
		t.Fatal(err)
	}
	if files != 1 || pending != 0 {
		t.Fatalf("%d file records and %d pending writes, want 1 and 0", files, pending)
	}
}
