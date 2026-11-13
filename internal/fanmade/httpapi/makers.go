package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"ourtaiko.dev/net/internal/kit/constants"
	"ourtaiko.dev/net/internal/kit/tja"
)

// Makers typed on the website, one per block: an upload, a replacement or an edit may set
// each block's maker, which otherwise comes from the TJA.
// The client identifies blocks, but cannot change their server-parsed difficulty.
func applyDifficultyMakers(raw string, difficulties []tja.Difficulty) error {
	if raw == "" {
		return nil
	}
	var entries []struct {
		Course string  `json:"course"`
		Side   string  `json:"side"`
		Maker  *string `json:"maker"`
	}
	if !utf8.ValidString(raw) || json.Unmarshal([]byte(raw), &entries) != nil || len(entries) != len(difficulties) {
		return fmt.Errorf("请为每个谱面块提供对应的制作者")
	}
	names := make(map[[2]string]string, len(entries))
	for _, entry := range entries {
		if entry.Course == "" || entry.Side == "" || entry.Maker == nil {
			return fmt.Errorf("制作者对应的谱面块无效")
		}
		slot := [2]string{entry.Course, entry.Side}
		if _, exists := names[slot]; exists {
			return fmt.Errorf("制作者对应的谱面块重复")
		}
		name := strings.TrimSpace(*entry.Maker)
		if len(name) > 500 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
			return fmt.Errorf("制作者名不能超过 500 字节或包含控制字符")
		}
		names[slot] = name
	}
	for i := range difficulties {
		name, exists := names[[2]string{difficulties[i].Course, difficulties[i].Side}]
		if !exists {
			return fmt.Errorf("制作者对应的难度无效")
		}
		difficulties[i].Maker = name
	}
	return nil
}

// Keep the first occurrence in block order, so credits never shuffle between reads.
func difficultyMakers(difficulties []tja.Difficulty) string {
	seen := map[string]bool{}
	names := []string{}
	for _, d := range difficulties {
		name := strings.TrimSpace(d.Maker)
		if name != "" && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return strings.Join(names, " | ")
}

// setDifficulties makes chart_difficulties hold exactly the given blocks of a
// chart. Rows are rewritten only when something differs, so storing the same
// file again neither changes their IDs nor touches the chart's update_at.
func setDifficulties(ctx context.Context, tx pgx.Tx, chartID string, blocks []tja.Difficulty) error {
	rows, err := tx.Query(ctx, `SELECT course,side,level,maker,branching,COALESCE(content_hash,'') FROM chart_difficulties WHERE chart_id=$1`, chartID)
	if err != nil {
		return err
	}
	stored, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (tja.Difficulty, error) {
		var d tja.Difficulty
		return d, row.Scan(&d.Course, &d.Side, &d.Level, &d.Maker, &d.Branching, &d.Hash)
	})
	if err != nil {
		return err
	}
	// The constants are kept by content hash (difficulty_constants), whatever the rows do.
	for _, d := range blocks {
		if err = constants.Store(ctx, tx, d.Hash, d.Routes); err != nil {
			return err
		}
	}
	wanted := map[[2]string]tja.Difficulty{}
	for _, d := range blocks {
		d.ID, d.Line, d.Routes = 0, 0, nil
		wanted[[2]string{d.Course, d.Side}] = d
	}
	same := len(stored) == len(wanted) && !slices.ContainsFunc(stored, func(d tja.Difficulty) bool {
		return !reflect.DeepEqual(wanted[[2]string{d.Course, d.Side}], d)
	})
	if same {
		return nil
	}
	// Keep the rows, and with them the IDs scores refer to, of slots that remain. A slot the
	// new file no longer has goes with its scores, whose request keys are kept: a retried
	// submission of such a score is answered SCORE_REMOVED and not stored again.
	for _, d := range stored {
		if _, ok := wanted[[2]string{d.Course, d.Side}]; ok {
			continue
		}
		if _, err = tx.Exec(ctx, `INSERT INTO retired_score_requests(user_id,idempotency_key)
 SELECT user_id,idempotency_key FROM scores WHERE idempotency_key IS NOT NULL AND difficulty_id IN (SELECT id FROM chart_difficulties WHERE chart_id=$1 AND course=$2 AND side=$3) ON CONFLICT DO NOTHING`, chartID, d.Course, d.Side); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `DELETE FROM scores WHERE difficulty_id IN (SELECT id FROM chart_difficulties WHERE chart_id=$1 AND course=$2 AND side=$3)`, chartID, d.Course, d.Side); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `DELETE FROM chart_difficulties WHERE chart_id=$1 AND course=$2 AND side=$3`, chartID, d.Course, d.Side); err != nil {
			return err
		}
	}
	for _, d := range blocks {
		var hash *string
		if d.Hash != "" {
			hash = &d.Hash
		}
		// A changed content_hash retires the row's scores and invalidates 段位 songs (trigger).
		_, err = tx.Exec(ctx, `INSERT INTO chart_difficulties(chart_id,course,side,level,maker,branching,content_hash) VALUES($1,$2,$3,$4,$5,$6,$7)
 ON CONFLICT(chart_id,course,side) DO UPDATE SET level=EXCLUDED.level,maker=EXCLUDED.maker,branching=EXCLUDED.branching,content_hash=COALESCE(EXCLUDED.content_hash,chart_difficulties.content_hash)
 WHERE (chart_difficulties.level,chart_difficulties.maker,chart_difficulties.branching,chart_difficulties.content_hash) IS DISTINCT FROM (EXCLUDED.level,EXCLUDED.maker,EXCLUDED.branching,COALESCE(EXCLUDED.content_hash,chart_difficulties.content_hash))`,
			chartID, d.Course, d.Side, d.Level, d.Maker, d.Branching, hash)
		if err != nil {
			return err
		}
	}
	return nil
}
