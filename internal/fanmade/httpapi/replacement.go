package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5"
)

func (s *Server) beginUpload(w http.ResponseWriter, r *http.Request, user, key, digest string) (pgx.Tx, bool) {
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		internal(w, err)
		return nil, false
	}
	proceed := false
	defer func() {
		if !proceed {
			tx.Rollback(r.Context())
		}
	}()
	if _, err = tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, user+":"+key); err != nil {
		internal(w, err)
		return nil, false
	}
	var chartID, previousDigest, tjaHash, audioHash, status string
	err = tx.QueryRow(r.Context(), `SELECT COALESCE(chart_id,''),payload_digest,tja_sha256,audio_sha256,status FROM upload_requests WHERE user_id=$1 AND idempotency_key=$2`, user, key).Scan(&chartID, &previousDigest, &tjaHash, &audioHash, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		proceed = true
		return tx, true
	}
	if err != nil {
		internal(w, err)
		return nil, false
	}
	if previousDigest != digest {
		problem(w, 409, "IDEMPOTENCY_CONFLICT", "此请求标识已用于不同的内容，请重新选择后提交")
		return nil, false
	}
	if status == "processing" {
		respond(w, 202, map[string]string{"status": "processing"})
		return nil, false
	}
	c, err := s.readChart(tx.QueryRow(r.Context(), chartSelect+` WHERE c.id=$1 AND `+publishedChart, chartID))
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, 409, "CHART_REMOVED", "该次上传的作品已删除，请重新选择文件")
		return nil, false
	}
	if err != nil {
		internal(w, err)
		return nil, false
	}
	if tjaHash != c.TJAHash || audioHash != c.AudioHash {
		problem(w, 409, "CHART_FILES_CHANGED", "该次上传已被后续更新替换，请重新打开歌曲页面")
		return nil, false
	}
	c.setOwners(s.publicProfiles(r.Context(), c.OwnerIDs))
	respond(w, 200, c)
	return nil, false
}

// Stage retained audio for validation and preview generation; the replacement
// keeps referring to the same stored file. The caller has no lock yet and
// verifies the stored hash here and again before committing.
func (s *Server) copyAudio(ctx context.Context, c *Chart, dir string) (*stagedFile, error) {
	in, err := s.Config.Objects.Open(ctx, c.AudioKey)
	if err != nil {
		return nil, err
	}
	defer in.Close()
	f := &stagedFile{name: c.AudioName, path: filepath.Join(dir, "audio"), sha: c.AudioHash, size: c.AudioSize}
	out, err := os.OpenFile(f.path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(out, h), io.LimitReader(in, c.AudioSize+1))
	closeErr := out.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		return nil, err
	}
	if n != c.AudioSize || hex.EncodeToString(h.Sum(nil)) != c.AudioHash {
		return nil, errors.New("stored audio integrity mismatch")
	}
	return f, nil
}

// retireChart keeps a replaced chart's scores. When the audio changed they all
// stop being scores of the current files; a changed TJA retires only the
// blocks whose content hash changed (the chart_difficulties trigger, from
// setDifficulties). Which older files a score belongs to is not recorded.
func retireChart(ctx context.Context, tx pgx.Tx, chartID, description string, audioChanged bool) error {
	if audioChanged {
		if _, err := tx.Exec(ctx, `UPDATE scores SET is_latest=false WHERE is_latest AND difficulty_id IN (SELECT id FROM chart_difficulties WHERE chart_id=$1)`, chartID); err != nil {
			return err
		}
	}
	_, err := tx.Exec(ctx, `UPDATE charts SET description=$2 WHERE id=$1`, chartID, description)
	return err
}
