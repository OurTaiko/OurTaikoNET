package rules

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"ourtaiko.dev/net/internal/donitems"
	"ourtaiko.dev/net/internal/web"
)

// Source is a song source whose rules may name its categories: each category's ID with its bit in
// charts.category_flags.
type Source struct {
	ID         string
	Categories map[string]int
}

// TxDB is a pool: what the routes read with, and begin their transactions on.
type TxDB interface {
	DB
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Begin(ctx context.Context) (pgx.Tx, error)
}

// API is the rules as the website has them: listed with their charts and categories by name, and
// made, changed and deleted by the title managers.
type API struct {
	DB      TxDB
	Sources []Source
	// Manager says who a request is from when they may manage rules (TITLE_MANAGER), answering
	// it itself when not.
	Manager func(w http.ResponseWriter, r *http.Request) (user string, ok bool)
	// DonItem says whether a part of the 3D Don is in the catalog: a problem to answer when not
	// (status and code set), or an error.
	DonItem func(ctx context.Context, part string, id int) (status int, code, message string, err error)
}

// Listed is a rule as the website has it: the category by its ID in the rule's source, and the
// chart with its name.
type Listed struct {
	Rule
	Category *string `json:"category"`
	Chart    *Chart  `json:"chart"`
}

// Chart names the chart of a chart_clear rule. Gone is a chart that is no longer published:
// nobody can meet the rule any more.
type Chart struct {
	ID                string          `json:"id"`
	Title             string          `json:"title"`
	TitleTranslations json.RawMessage `json:"titleTranslations"`
	Gone              bool            `json:"gone"`
}

var answers = web.Problems{}

func (a *API) source(id string) (Source, bool) {
	for _, s := range a.Sources {
		if s.ID == id {
			return s, true
		}
	}
	return Source{}, false
}

// Categories are each source's category IDs, in the order of their bits: what a category rule
// may name.
func (a *API) Categories() map[string][]string {
	out := map[string][]string{}
	for _, s := range a.Sources {
		ids := make([]string, 0, len(s.Categories))
		for id := range s.Categories {
			ids = append(ids, id)
		}
		slices.SortFunc(ids, func(x, y string) int { return s.Categories[x] - s.Categories[y] })
		out[s.ID] = ids
	}
	return out
}

func (a *API) categoryID(source string, flag int) *string {
	s, _ := a.source(source)
	for id, f := range s.Categories {
		if f == flag {
			return &id
		}
	}
	return nil
}

const listedSelect = `SELECT ` + Columns + `,COALESCE(fc.title,oc.title),COALESCE(fc.title_translations,oc.title_translations),COALESCE(fc.status,oc.status,'')
 FROM net.reward_rules r
 LEFT JOIN fanmade.charts fc ON r.source='fanmade' AND fc.id=r.chart_id
 LEFT JOIN official.charts oc ON r.source='official' AND oc.id=r.chart_id`

func (a *API) scanListed(row interface{ Scan(...any) error }) (Listed, error) {
	var title, status *string
	var translations []byte
	r, err := Scan(row, &title, &translations, &status)
	out := Listed{Rule: r}
	if err != nil {
		return out, err
	}
	if r.Source != nil && r.CategoryFlag != nil {
		out.Category = a.categoryID(*r.Source, *r.CategoryFlag)
	}
	if r.ChartID != nil {
		out.Chart = &Chart{ID: *r.ChartID, TitleTranslations: json.RawMessage(`{}`), Gone: title == nil || *status != "published"}
		if title != nil {
			out.Chart.Title, out.Chart.TitleTranslations = *title, translations
		}
	}
	return out, nil
}

// List is every rule of item ("" for every kind), in the order they were made.
func (a *API) List(ctx context.Context, item string) ([]Listed, error) {
	rows, err := a.DB.Query(ctx, listedSelect+` WHERE ($1='' OR r.item=$1) ORDER BY r.id`, item)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rules := []Listed{}
	for rows.Next() {
		rule, err := a.scanListed(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

// Routes are the managers' routes:
//
//	POST   /api/admin/titles/{id}/rules            a rule that gives a public title
//	POST   /api/admin/don/items/{part}/{id}/rules  a rule that gives a part of the Don
//	PUT    /api/admin/rules/{id}                   the conditions of a rule changed (not its item)
//	DELETE /api/admin/rules/{id}
//
// The first three take {kind, source, chartId, category, course, clear, count} and answer
// {rule, granted}: the rule and how many players got its item at once.
func (a *API) Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/admin/titles/{id}/rules", a.createTitleRule)
	mux.HandleFunc("POST /api/admin/don/items/{part}/{id}/rules", a.createDonItemRule)
	mux.HandleFunc("PUT /api/admin/rules/{id}", a.updateRule)
	mux.HandleFunc("DELETE /api/admin/rules/{id}", a.deleteRule)
}

type body struct {
	Kind     string  `json:"kind"`
	Source   *string `json:"source"`
	ChartID  *string `json:"chartId"`
	Category *string `json:"category"`
	Course   *string `json:"course"`
	Clear    *int    `json:"clear"`
	Count    *int    `json:"count"`
}

// problem is a problem to answer: a status and a code.
type problem struct {
	status        int
	code, message string
}

// itemCheck checks, in a new rule's transaction, that the item it gives may be given by rule.
type itemCheck func(ctx context.Context, tx pgx.Tx) (*problem, error)

// createTitleRule: only a public title takes rules, a private one is its maker's alone.
func (a *API) createTitleRule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	a.create(w, r, Rule{Item: ItemTitle, TitleID: &id}, func(ctx context.Context, tx pgx.Tx) (*problem, error) {
		var visibility string
		err := tx.QueryRow(ctx, `SELECT visibility FROM net.titles WHERE id=$1 FOR SHARE`, id).Scan(&visibility)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return &problem{404, "TITLE_NOT_FOUND", "称号不存在"}, nil
		case err != nil:
			return nil, err
		case visibility != "public":
			return &problem{422, "RULE_TITLE_PRIVATE", "私有称号不能设置发放条件"}, nil
		}
		return nil, nil
	})
}

// createDonItemRule: a part of the 3D Don of the catalog (not 0, none).
func (a *API) createDonItemRule(w http.ResponseWriter, r *http.Request) {
	part := r.PathValue("part")
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || !(donitems.Item{Part: part, ID: id}).Valid() {
		answers.Problem(w, 404, "DON_ITEM_NOT_FOUND", "没有这件咚酱装扮")
		return
	}
	a.create(w, r, Rule{Item: ItemDonItem, DonPart: &part, DonItem: &id}, func(ctx context.Context, tx pgx.Tx) (*problem, error) {
		status, code, message, err := a.DonItem(ctx, part, id)
		if err != nil || code == "" {
			return nil, err
		}
		return &problem{status, code, message}, nil
	})
}

// conditions is the rule of item with the conditions of the request's body, or false once it
// has answered that they are not valid.
func (a *API) conditions(w http.ResponseWriter, r *http.Request, item Rule) (Rule, bool) {
	var b body
	if !answers.Decode(w, r, &b) {
		return item, false
	}
	rule := Rule{ID: item.ID, Item: item.Item, TitleID: item.TitleID, DonPart: item.DonPart, DonItem: item.DonItem,
		Kind: b.Kind, Source: b.Source, ChartID: b.ChartID, Course: b.Course, Clear: b.Clear, Count: b.Count}
	if b.Category != nil {
		var flag int
		known := false
		if b.Source != nil {
			if s, ok := a.source(*b.Source); ok {
				flag, known = s.Categories[*b.Category]
			}
		}
		if !known {
			answers.Problem(w, 422, "RULE_INVALID", "该曲源没有这个分类")
			return rule, false
		}
		rule.CategoryFlag = &flag
	}
	if !rule.Valid() {
		answers.Problem(w, 422, "RULE_INVALID", "发放条件无效")
		return rule, false
	}
	return rule, true
}

// create makes a rule that gives item (a Rule with only its item set), once check has allowed
// the item, and gives it at once to those who meet the rule already.
func (a *API) create(w http.ResponseWriter, r *http.Request, item Rule, check itemCheck) {
	user, ok := a.Manager(w, r)
	if !ok {
		return
	}
	rule, ok := a.conditions(w, r, item)
	if !ok {
		return
	}
	a.save(w, r, 201, check, func(ctx context.Context, tx pgx.Tx) (int64, error) {
		var id int64
		err := tx.QueryRow(ctx, `INSERT INTO net.reward_rules(item,title_id,don_part,don_item,kind,source,chart_id,category_flag,course,clear,count,created_by)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING id`,
			rule.Item, rule.TitleID, rule.DonPart, rule.DonItem, rule.Kind, rule.Source, rule.ChartID, rule.CategoryFlag, rule.Course, rule.Clear, rule.Count, user).Scan(&id)
		return id, err
	}, rule)
}

// updateRule changes a rule's conditions; its item stays. What it gave stays given, and it
// gives its item at once to those who meet the new conditions; once it gave a player the item
// it does not give it them again.
func (a *API) updateRule(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.Manager(w, r); !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		answers.Problem(w, 404, "RULE_NOT_FOUND", "发放条件不存在")
		return
	}
	item, err := Scan(a.DB.QueryRow(r.Context(), `SELECT `+Columns+` FROM net.reward_rules r WHERE r.id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		answers.Problem(w, 404, "RULE_NOT_FOUND", "发放条件不存在")
		return
	}
	if err != nil {
		answers.Internal(w, err)
		return
	}
	rule, ok := a.conditions(w, r, item)
	if !ok {
		return
	}
	a.save(w, r, 200, nil, func(ctx context.Context, tx pgx.Tx) (int64, error) {
		tag, err := tx.Exec(ctx, `UPDATE net.reward_rules SET kind=$2,source=$3,chart_id=$4,category_flag=$5,course=$6,clear=$7,count=$8 WHERE id=$1`,
			id, rule.Kind, rule.Source, rule.ChartID, rule.CategoryFlag, rule.Course, rule.Clear, rule.Count)
		if err == nil && tag.RowsAffected() == 0 {
			err = pgx.ErrNoRows
		}
		return id, err
	}, rule)
}

// save writes a rule in a transaction (write, which returns its id) once check, when there is
// one, has allowed its item and its chart is published, gives its item to those who meet it,
// and answers {rule, granted}.
func (a *API) save(w http.ResponseWriter, r *http.Request, status int, check itemCheck, write func(context.Context, pgx.Tx) (int64, error), rule Rule) {
	ctx := r.Context()
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		answers.Internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	if check != nil {
		p, err := check(ctx, tx)
		if err != nil {
			answers.Internal(w, err)
			return
		}
		if p != nil {
			answers.Problem(w, p.status, p.code, p.message)
			return
		}
	}
	if rule.ChartID != nil {
		var published bool
		// The source is one of the two (Valid).
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM `+*rule.Source+`.charts c WHERE c.id=$1 AND c.status='published' AND c.tja_file IS NOT NULL)`, *rule.ChartID).Scan(&published); err != nil {
			answers.Internal(w, err)
			return
		}
		if !published {
			answers.Problem(w, 422, "RULE_CHART_NOT_FOUND", "该曲源没有这首歌曲")
			return
		}
	}
	rule.ID, err = write(ctx, tx)
	var pgErr *pgconn.PgError
	switch {
	case errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "reward_rules_once":
		answers.Problem(w, 409, "RULE_EXISTS", "已有相同的发放条件")
		return
	case errors.Is(err, pgx.ErrNoRows):
		answers.Problem(w, 404, "RULE_NOT_FOUND", "发放条件不存在")
		return
	case err != nil:
		answers.Internal(w, err)
		return
	}
	granted, err := Backfill(ctx, tx, rule)
	if err != nil {
		answers.Internal(w, err)
		return
	}
	saved, err := a.scanListed(tx.QueryRow(ctx, listedSelect+` WHERE r.id=$1`, rule.ID))
	if err != nil {
		answers.Internal(w, err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		answers.Internal(w, err)
		return
	}
	web.Respond(w, status, map[string]any{"rule": saved, "granted": granted})
}

// deleteRule: the rule gives nothing from now on. Players keep what it gave.
func (a *API) deleteRule(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.Manager(w, r); !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		answers.Problem(w, 404, "RULE_NOT_FOUND", "发放条件不存在")
		return
	}
	tag, err := a.DB.Exec(r.Context(), `DELETE FROM net.reward_rules WHERE id=$1`, id)
	if err != nil {
		answers.Internal(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		answers.Problem(w, 404, "RULE_NOT_FOUND", "发放条件不存在")
		return
	}
	web.Respond(w, 200, map[string]bool{"ok": true})
}
