package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5"
	"ourtaiko.dev/net/internal/kit/charthash"
	"ourtaiko.dev/net/internal/kit/constants"
	"ourtaiko.dev/net/internal/kit/tja"
)

// RunContentHashBackfill fills chart_difficulties.content_hash for charts stored before
// migration 010, or hashed by an older algorithm than the tool now reports, and the raw
// difficulty constants (difficulty_constants, every route) for blocks without them in the
// current constants.Version; a chart that has no titles from its file yet gets the ones
// TJAParser reads (origin and every locale), never removing a key. It runs after start-up, resumes on the next
// start, and leaves a block the tool cannot play NULL with a log line; the retire trigger
// ignores a hash set for the first time, and the constants touch neither scores nor the
// chart's update_at.
func (s *Server) RunContentHashBackfill(ctx context.Context) {
	cli, ok := tja.Inspector.(charthash.CLI)
	if !ok {
		return
	}
	version, err := cli.Version(ctx)
	if err != nil {
		log.Printf("content hash backfill skipped: %v", err)
		return
	}
	if err := s.backfillContentHashes(ctx, version+":", constants.Version); err != nil && ctx.Err() == nil {
		log.Printf("content hash backfill: %v", err)
	}
	if err := s.backfillScenePresets(ctx); err != nil && ctx.Err() == nil {
		log.Printf("scene preset backfill: %v", err)
	}
}

func (s *Server) backfillContentHashes(ctx context.Context, prefix string, constantsVersion int) error {
	rows, err := s.DB.Query(ctx, `SELECT DISTINCT c.id FROM charts c JOIN chart_difficulties d ON d.chart_id=c.id
 WHERE c.status<>'deleted' AND c.tja_file IS NOT NULL AND (d.content_hash IS NULL OR left(d.content_hash,length($1))<>$1
  OR NOT EXISTS (SELECT 1 FROM difficulty_constants k WHERE k.content_hash=d.content_hash AND k.constants_version=$2)) ORDER BY c.id`, prefix, constantsVersion)
	if err != nil {
		return err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	log.Printf("content hash backfill: %d charts", len(ids))
	var failures []error
	done := 0
	for _, id := range ids {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := s.backfillChartHashes(ctx, id); err != nil {
			log.Printf("content hash backfill for %s failed: %v", id, err)
			failures = append(failures, err)
			continue
		}
		done++
	}
	log.Printf("content hash backfill: %d charts done, %d failed", done, len(failures))
	return errors.Join(failures...)
}

func (s *Server) backfillChartHashes(parent context.Context, id string) error {
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	result, found, err := s.inspectStored(ctx, tx, id)
	if err != nil || !found {
		return err
	}
	for _, b := range result.Blocks {
		if b.Error != "" {
			log.Printf("content hash backfill: chart %s %s/%s cannot be played: %s", id, b.Name, b.Side, b.Error)
			continue
		}
		if b.Hash == "" {
			continue
		}
		if _, err = tx.Exec(ctx, `UPDATE chart_difficulties SET content_hash=$4 WHERE chart_id=$1 AND course=$2 AND side=$3 AND content_hash IS DISTINCT FROM $4`, id, b.Name, b.Side, b.Hash); err != nil {
			return err
		}
		if err = constants.Store(ctx, tx, b.Hash, constants.OfRoutes(b.Notes)); err != nil {
			return err
		}
	}
	// Titles: for a chart that has not had them from its file yet (no origin), add what the file
	// says (origin and every locale the game maps), keeping every key already stored, since an
	// "en" written by the uploader cannot be told from a defaulted one. Once it has them the
	// titles are the site's to edit, and the stored file keeps the old ones: reading it again
	// (a new hash algorithm, new constants) must not bring them back.
	titles, _ := json.Marshal(result.Titles)
	subtitles, _ := json.Marshal(result.Subtitles)
	if _, err = tx.Exec(ctx, `UPDATE charts SET title_translations=COALESCE(title_translations,'{}'::jsonb)||$2::jsonb,
 subtitle_translations=COALESCE(subtitle_translations,'{}'::jsonb)||$3::jsonb
 WHERE id=$1 AND NOT (COALESCE(title_translations,'{}'::jsonb) ? 'origin')`, id, titles, subtitles); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// inspectStored locks the chart's row and reads its stored TJA with TJAParser; found is false
// for a chart that is gone, has no TJA, or is locked by someone else (left for the next start).
func (s *Server) inspectStored(ctx context.Context, tx pgx.Tx, id string) (result charthash.Inspection, found bool, err error) {
	var key string
	var size int64
	err = tx.QueryRow(ctx, `SELECT r.storage_key,r.byte_size FROM charts c JOIN chart_resources r ON r.id=c.tja_file WHERE c.id=$1 FOR UPDATE OF c SKIP LOCKED`, id).Scan(&key, &size)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, false, nil
	}
	if err != nil {
		return result, false, err
	}
	dir, err := os.MkdirTemp(s.Config.Storage, "hash-")
	if err != nil {
		return result, false, err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "stored.tja")
	in, err := s.Config.Objects.Open(ctx, key)
	if err != nil {
		return result, false, err
	}
	data, err := io.ReadAll(io.LimitReader(in, size+1))
	in.Close()
	if err != nil {
		return result, false, err
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		return result, false, err
	}
	result, err = tja.Inspector.Inspect(ctx, path)
	return result, err == nil, err
}

// backfillScenePresets fills scene_preset of the charts stored before the column, or by a tool
// that did not report it, from what TJAParser reads in their files. A chart's preset is NULL
// only until then, so each chart is read once.
func (s *Server) backfillScenePresets(ctx context.Context) error {
	rows, err := s.DB.Query(ctx, `SELECT id FROM charts WHERE status<>'deleted' AND tja_file IS NOT NULL AND scene_preset IS NULL ORDER BY id`)
	if err != nil {
		return err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil || len(ids) == 0 {
		return err
	}
	log.Printf("scene preset backfill: %d charts", len(ids))
	var failures []error
	done := 0
	for _, id := range ids {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		reported, err := s.backfillScenePreset(ctx, id)
		if err != nil {
			log.Printf("scene preset backfill for %s failed: %v", id, err)
			failures = append(failures, err)
			continue
		}
		if !reported {
			return errors.New("this tjaparser does not report scenePreset (v0.16.0 or newer does)")
		}
		done++
	}
	log.Printf("scene preset backfill: %d charts done, %d failed", done, len(failures))
	return errors.Join(failures...)
}

// backfillScenePreset fills one chart; reported is false when the tool does not say the preset.
func (s *Server) backfillScenePreset(parent context.Context, id string) (reported bool, err error) {
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	result, found, err := s.inspectStored(ctx, tx, id)
	if err != nil || !found {
		return true, err
	}
	preset := tja.ScenePresetName(result.ScenePreset)
	if preset == nil {
		return false, nil
	}
	// Reading the file is not a change to the chart: update_at stays.
	if _, err = tx.Exec(ctx, `UPDATE charts SET scene_preset=$2 WHERE id=$1 AND scene_preset IS NULL`, id, *preset); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}
