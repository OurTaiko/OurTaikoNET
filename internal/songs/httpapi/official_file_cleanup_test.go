package httpapi

import (
	"context"
	"strings"
	"testing"
)

// A file a chart stops referring to is queued by the database in that same change, so the
// cleaner deletes it even when the program never got to release it itself. A file something
// still refers to is taken off the queue and kept.
func TestQueuedFilesAreCleanedUp(t *testing.T) {
	w := officialInteractionFixture(t)
	s, ctx := w.s, context.Background()
	store := func(extension string) (id int64, key string) {
		digest := ID() + ID()
		key = digest + "." + extension
		if e := s.Config.Objects.Put(ctx, key, strings.NewReader("x"), 1, "application/octet-stream", digest); e != nil {
			t.Fatal(e)
		}
		if e := s.DB.QueryRow(ctx, `INSERT INTO chart_resources(storage_key,original_filename,sha256,byte_size,extension) VALUES($1,'a.'||$3,$2,1,$3) RETURNING id`, key, digest, extension).Scan(&id); e != nil {
			t.Fatal(e)
		}
		return id, key
	}
	lone, loneKey := store("webp")
	shared, sharedKey := store("webp")
	if _, e := s.DB.Exec(ctx, `UPDATE charts SET cover_file=CASE WHEN id=$3 THEN $1::bigint ELSE $2::bigint END,bgimage_file=$2 WHERE id IN ($3,$4)`, lone, shared, w.song, w.other); e != nil {
		t.Fatal(e)
	}
	// As if the program stopped right after the change: nothing releases the files.
	if _, e := s.DB.Exec(ctx, `UPDATE charts SET cover_file=NULL,bgimage_file=NULL WHERE id=$1`, w.song); e != nil {
		t.Fatal(e)
	}
	count := func(query string, args ...any) (n int) {
		t.Helper()
		if e := s.DB.QueryRow(ctx, query, args...).Scan(&n); e != nil {
			t.Fatal(e)
		}
		return n
	}
	if count(`SELECT count(*) FROM retired_files WHERE storage_key=ANY($1)`, []string{loneKey, sharedKey}) != 2 {
		t.Fatal("the database did not queue the files")
	}
	s.cleanupReplacedFiles()
	if count(`SELECT count(*) FROM retired_files`) != 0 {
		t.Fatal("queue not drained")
	}
	if count(`SELECT count(*) FROM chart_resources WHERE id=$1`, lone) != 0 || count(`SELECT count(*) FROM chart_resources WHERE id=$1`, shared) != 1 {
		t.Fatal("file records", count(`SELECT count(*) FROM chart_resources`))
	}
	if f, e := s.Config.Objects.Open(ctx, loneKey); e == nil {
		f.Close()
		t.Fatal("object of an unused file kept")
	}
	f, e := s.Config.Objects.Open(ctx, sharedKey)
	if e != nil {
		t.Fatal("object another chart uses deleted", e)
	}
	f.Close()
}
