package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"ourtaiko.dev/net/internal/kit/objectstore"
)

// Matches a chart_resources row r that some chart or playlist still refers to.
const referencedFile = `(EXISTS(SELECT 1 FROM charts c WHERE c.tja_file=r.id OR c.audio_file=r.id OR c.cover_file=r.id OR c.preview_file=r.id OR c.movie_file=r.id OR c.bgimage_file=r.id)
 OR EXISTS(SELECT 1 FROM playlists p WHERE p.cover_file=r.id OR p.game_cover_file=r.id))`

func (s *Server) remoteStorage() bool { _, ok := s.Config.Objects.(*objectstore.S3); return ok }

// fileExtension is the stored form: lowercase, without the dot.
func fileExtension(name string) string {
	return strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
}

// Identical files share one object, so writers and the cleaner serialize on
// its key until tx ends. Without this the cleaner could delete an object that
// an uncommitted upload is about to reference.
func lockObject(ctx context.Context, tx pgx.Tx, key string) error {
	_, e := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "object:"+key)
	return e
}

// storeFile stores path as {sha256}.{extension} and returns its file ID,
// reusing the record and object of identical content. The first registered
// name is kept. The caller references the ID in tx.
func (s *Server) storeFile(ctx context.Context, tx pgx.Tx, path, name, extension, media, digest string, size int64) (int64, error) {
	key := digest + "." + extension
	if e := lockObject(ctx, tx, key); e != nil {
		return 0, e
	}
	// Records the write before it happens, so an object left by a failed
	// upload can be found again.
	if _, e := s.DB.Exec(ctx, `INSERT INTO pending_objects(storage_key) VALUES($1) ON CONFLICT DO NOTHING`, key); e != nil {
		return 0, e
	}
	stored, e := s.objectStored(ctx, key, size)
	if e != nil {
		return 0, e
	}
	if !stored {
		f, e := os.Open(path)
		if e != nil {
			return 0, e
		}
		e = s.Config.Objects.Put(ctx, key, f, size, media, digest)
		f.Close()
		if e != nil && !objectstore.AlreadyStored(e) {
			return 0, e
		}
	}
	var id int64
	e = tx.QueryRow(ctx, `INSERT INTO chart_resources(storage_key,original_filename,sha256,byte_size,extension) VALUES($1,$2,$3,$4,$5)
 ON CONFLICT(storage_key) DO UPDATE SET storage_key=EXCLUDED.storage_key RETURNING id`, key, name, digest, size, extension).Scan(&id)
	if e == nil {
		_, e = tx.Exec(ctx, `DELETE FROM pending_objects WHERE storage_key=$1`, key)
	}
	return id, e
}

// fileDigest returns the SHA-256 and size of a staged file.
func fileDigest(path string) (string, int64, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", 0, e
	}
	defer f.Close()
	h := sha256.New()
	n, e := io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), n, e
}

// objectStored reports whether key already holds the complete object. The
// caller holds its lock; an interrupted local write is removed for rewriting.
func (s *Server) objectStored(ctx context.Context, key string, size int64) (bool, error) {
	f, e := s.Config.Objects.Open(ctx, key)
	if errors.Is(e, os.ErrNotExist) {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	complete := f.Size() == size
	f.Close()
	if complete {
		return true, nil
	}
	return false, s.Config.Objects.Delete(ctx, key)
}

// reconcilePending queues the objects of writes that never became a file record. storeFile
// forgets a pending write in the transaction that records the file, so what is left after a
// day was abandoned.
func (s *Server) reconcilePending(ctx context.Context) error {
	// Only age out abandoned writes after a full day. A commit with an unknown
	// outcome is resolved against live references before any object is retired.
	_, e := s.DB.Exec(ctx, `WITH settled AS (
 DELETE FROM pending_objects p WHERE p.created_at<now()-interval '1 day' RETURNING storage_key
 ) INSERT INTO retired_files(storage_key)
 SELECT storage_key FROM settled p WHERE
 NOT EXISTS(SELECT 1 FROM chart_resources r WHERE r.storage_key=p.storage_key)
 ON CONFLICT DO NOTHING`)
	return e
}

// RunFileCleanup retries committed file deletions on startup and every 30 seconds.
func (s *Server) RunFileCleanup(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		s.cleanupReplacedFiles()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Server) cleanupReplacedFiles() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.reconcilePending(ctx); err != nil {
		log.Printf("pending object cleanup failed: %v", err)
		return
	}
	if err := s.deleteRetiredFiles(ctx); err != nil {
		log.Printf("retired file cleanup pending: %v", err)
	}
}

func (s *Server) deleteRetiredFiles(ctx context.Context) error {
	rows, err := s.DB.Query(ctx, `SELECT storage_key FROM retired_files ORDER BY created_at,storage_key LIMIT 100`)
	if err != nil {
		return err
	}
	var keys []string
	for rows.Next() {
		var key string
		if err = rows.Scan(&key); err != nil {
			rows.Close()
			return err
		}
		keys = append(keys, key)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	var failures []error
	for _, key := range keys {
		if err = s.deleteRetiredFile(ctx, key); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

// releaseFiles deletes the given files and their objects now, unless a chart or playlist
// still refers to them. The database queued them (retired_files) when the row that referred
// to them went, so one that fails here is deleted by the periodic cleanup.
func (s *Server) releaseFiles(ctx context.Context, ids []int64) error {
	rows, err := s.DB.Query(ctx, `SELECT storage_key FROM chart_resources WHERE id=ANY($1)`, ids)
	if err != nil {
		return err
	}
	keys, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	var failures []error
	for _, key := range keys {
		failures = append(failures, s.deleteRetiredFile(ctx, key))
	}
	return errors.Join(failures...)
}

// deleteRetiredFile deletes one queued file and its object unless a chart still
// refers to it, then removes it from the queue. A failure leaves it queued for
// retry. The key lock excludes an upload that is about to reuse the object.
func (s *Server) deleteRetiredFile(ctx context.Context, key string) error {
	if !filepath.IsLocal(key) {
		return errors.New("invalid retired storage key")
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = lockObject(ctx, tx, key); err != nil {
		return err
	}
	var live bool
	err = tx.QueryRow(ctx, `SELECT `+referencedFile+` FROM chart_resources r WHERE r.storage_key=$1`, key).Scan(&live)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if !live {
		if _, err = tx.Exec(ctx, `DELETE FROM chart_resources WHERE storage_key=$1`, key); err != nil {
			return err
		}
		if err = s.Config.Objects.Delete(ctx, key); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `DELETE FROM retired_files WHERE storage_key=$1`, key); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
