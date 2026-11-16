package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"
	"ourtaiko.dev/net/internal/titlerules"
	"ourtaiko.dev/net/internal/web"
)

// TitleRule is a rule that gives a title by itself (package titlerules) as the website has
// it: the category by its ID in the rule's source, and the chart with its name.
type TitleRule struct {
	titlerules.Rule
	Category *string    `json:"category"`
	Chart    *RuleChart `json:"chart"`
}

// RuleChart names the chart of a chart_clear rule. Gone is a chart that is no longer
// published: nobody can meet the rule any more.
type RuleChart struct {
	ID                string          `json:"id"`
	Title             string          `json:"title"`
	TitleTranslations json.RawMessage `json:"titleTranslations"`
	Gone              bool            `json:"gone"`
}

// categoryFlag is a category of a source by its ID, and categoryID the other way.
func (s *Server) categoryFlag(source, id string) (int, bool) {
	for _, src := range s.Config.Sources {
		if src.ID == source {
			flag, ok := src.Categories[id]
			return flag, ok
		}
	}
	return 0, false
}

func (s *Server) categoryID(source string, flag int) *string {
	for _, src := range s.Config.Sources {
		if src.ID == source {
			for id, f := range src.Categories {
				if f == flag {
					return &id
				}
			}
		}
	}
	return nil
}

const ruleSelect = `SELECT ` + titlerules.Columns + `,COALESCE(fc.title,oc.title),COALESCE(fc.title_translations,oc.title_translations),COALESCE(fc.status,oc.status,'')
 FROM net.title_rules r
 LEFT JOIN fanmade.charts fc ON r.source='fanmade' AND fc.id=r.chart_id
 LEFT JOIN official.charts oc ON r.source='official' AND oc.id=r.chart_id`

func (s *Server) readRule(row rowScanner) (TitleRule, error) {
	var title, status *string
	var translations []byte
	r, err := titlerules.Scan(row, &title, &translations, &status)
	out := TitleRule{Rule: r}
	if err != nil {
		return out, err
	}
	if r.Source != nil && r.CategoryFlag != nil {
		out.Category = s.categoryID(*r.Source, *r.CategoryFlag)
	}
	if r.ChartID != nil {
		out.Chart = &RuleChart{ID: *r.ChartID, TitleTranslations: json.RawMessage(`{}`), Gone: title == nil || *status != "published"}
		if title != nil {
			out.Chart.Title, out.Chart.TitleTranslations = *title, translations
		}
	}
	return out, nil
}

// titleRules reads every rule, in the order they were made.
func (s *Server) titleRules(ctx context.Context) ([]TitleRule, error) {
	rows, err := s.DB.Query(ctx, ruleSelect+` ORDER BY r.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rules := []TitleRule{}
	for rows.Next() {
		rule, err := s.readRule(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

type ruleBody struct {
	Kind     string  `json:"kind"`
	Source   *string `json:"source"`
	ChartID  *string `json:"chartId"`
	Category *string `json:"category"`
	Course   *string `json:"course"`
	Clear    *int    `json:"clear"`
	Count    *int    `json:"count"`
}

// createTitleRule is POST /api/v1/admin/titles/{id}/rules {kind, source, chartId, category,
// course, clear, count}: from now on the title is given to whoever meets the rule, and at once
// to everyone who meets it already. Only a title of the catalog's own takes rules, not one a
// player made. It answers {rule, granted}: the rule and how many players got the title now.
func (s *Server) createTitleRule(w http.ResponseWriter, r *http.Request) {
	v, ok := s.admin(w, r)
	if !ok {
		return
	}
	var body ruleBody
	if !decode(w, r, &body) {
		return
	}
	rule := titlerules.Rule{TitleID: r.PathValue("id"), Kind: body.Kind, Source: body.Source, ChartID: body.ChartID, Course: body.Course, Clear: body.Clear, Count: body.Count}
	if body.Category != nil {
		flag, known := 0, false
		if body.Source != nil {
			flag, known = s.categoryFlag(*body.Source, *body.Category)
		}
		if !known {
			problem(w, 422, "RULE_INVALID", "该曲源没有这个分类")
			return
		}
		rule.CategoryFlag = &flag
	}
	if !rule.Valid() {
		problem(w, 422, "RULE_INVALID", "发放条件无效")
		return
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var owned bool
	err = tx.QueryRow(r.Context(), `SELECT owner_id IS NOT NULL FROM net.titles WHERE id=$1 FOR SHARE`, rule.TitleID).Scan(&owned)
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, 404, "TITLE_NOT_FOUND", "称号不存在")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	if owned {
		problem(w, 422, "RULE_TITLE_OWNED", "玩家自制的称号不能设置发放条件")
		return
	}
	if rule.ChartID != nil {
		var published bool
		// The source is one of the two (Valid).
		if err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM `+*rule.Source+`.charts c WHERE c.id=$1 AND c.status='published' AND c.tja_file IS NOT NULL)`, *rule.ChartID).Scan(&published); err != nil {
			internal(w, err)
			return
		}
		if !published {
			problem(w, 422, "RULE_CHART_NOT_FOUND", "该曲源没有这首歌曲")
			return
		}
	}
	if err = tx.QueryRow(r.Context(), `INSERT INTO net.title_rules(title_id,kind,source,chart_id,category_flag,course,clear,count,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`,
		rule.TitleID, rule.Kind, rule.Source, rule.ChartID, rule.CategoryFlag, rule.Course, rule.Clear, rule.Count, v.User.ID).Scan(&rule.ID); err != nil {
		internal(w, err)
		return
	}
	granted, err := titlerules.Backfill(r.Context(), tx, rule)
	if err != nil {
		internal(w, err)
		return
	}
	saved, err := s.readRule(tx.QueryRow(r.Context(), ruleSelect+` WHERE r.id=$1`, rule.ID))
	if err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		internal(w, err)
		return
	}
	web.Respond(w, 201, map[string]any{"rule": saved, "granted": granted})
}

// deleteTitleRule is DELETE /api/v1/admin/rules/{id}: the rule gives nothing from now on.
// Players keep what it gave.
func (s *Server) deleteTitleRule(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.admin(w, r); !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		problem(w, 404, "RULE_NOT_FOUND", "发放条件不存在")
		return
	}
	tag, err := s.DB.Exec(r.Context(), `DELETE FROM net.title_rules WHERE id=$1`, id)
	if err != nil {
		internal(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		problem(w, 404, "RULE_NOT_FOUND", "发放条件不存在")
		return
	}
	web.Respond(w, 200, map[string]bool{"ok": true})
}
