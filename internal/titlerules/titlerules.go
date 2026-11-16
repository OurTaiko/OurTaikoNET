// Package titlerules gives titles by rule: a player who clears a certain chart, clears enough
// charts of a category, is seen signed in on the website or in the game, or has uploaded
// enough charts gets the title the rule names (net.title_rules).
//
// A rule gives a player its title once. What it gave is an award of the service "rule" whose
// request_key is "<rule id>:<user id>", so asking again gives nothing, and a title an
// administrator took back is not given again by the same rule.
//
// Every statement names its schemas: the callers' connections differ in search_path.
package titlerules

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
)

// The kinds of rule.
const (
	ChartClear     = "chart_clear"
	CategoryClear  = "category_clear"
	SiteLogin      = "site_login"
	GameLogin      = "game_login"
	ChartsUploaded = "charts_uploaded"
)

// Service is the awards.service of what rules give.
const Service = "rule"

// Courses from the lowest: a rule's course is the lowest that counts.
var Courses = []string{"Easy", "Normal", "Hard", "Oni", "Edit"}

// Rule is one row of net.title_rules. Which fields a kind has is the table's title_rules_shape.
type Rule struct {
	ID           int64   `json:"id"`
	TitleID      string  `json:"titleId"`
	Kind         string  `json:"kind"`
	Source       *string `json:"source"`
	ChartID      *string `json:"chartId"`
	CategoryFlag *int    `json:"-"`
	Course       *string `json:"course"`
	Clear        *int    `json:"clear"`
	Count        *int    `json:"count"`
}

// Columns are a rule's columns as Scan reads them, of a table named r.
const Columns = `r.id,r.title_id,r.kind,r.source,r.chart_id,r.category_flag,r.course,r.clear,r.count`

// Scan reads a rule selected by Columns; more receives what was selected after them.
func Scan(row interface{ Scan(...any) error }, more ...any) (Rule, error) {
	var r Rule
	err := row.Scan(append([]any{&r.ID, &r.TitleID, &r.Kind, &r.Source, &r.ChartID, &r.CategoryFlag, &r.Course, &r.Clear, &r.Count}, more...)...)
	return r, err
}

// DB is a pool or a transaction.
type DB interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func schema(source string) (string, bool) {
	return source, source == "fanmade" || source == "official"
}

// Valid says whether r is a rule the table takes (title_rules_shape) and this package can
// judge. It does not look at whether the title or the chart exist.
func (r Rule) Valid() bool {
	source := r.Source != nil
	if source {
		if _, ok := schema(*r.Source); !ok {
			return false
		}
	}
	course := r.Course == nil
	for _, c := range Courses {
		course = course || *r.Course == c
	}
	clear := r.Clear != nil && *r.Clear >= 1 && *r.Clear <= 3
	count := r.Count != nil && *r.Count >= 1 && *r.Count <= 100000
	flag := r.CategoryFlag != nil && *r.CategoryFlag > 0 && *r.CategoryFlag&(*r.CategoryFlag-1) == 0
	switch r.Kind {
	case ChartClear:
		return source && r.ChartID != nil && *r.ChartID != "" && r.CategoryFlag == nil && course && clear && r.Count == nil
	case CategoryClear:
		return source && r.ChartID == nil && flag && course && clear && (r.Count == nil || count)
	case ChartsUploaded:
		return source && r.ChartID == nil && r.CategoryFlag == nil && r.Course == nil && r.Clear == nil && count
	case SiteLogin, GameLogin:
		return !source && r.ChartID == nil && r.CategoryFlag == nil && r.Course == nil && r.Clear == nil && r.Count == nil
	}
	return false
}

// courseRank is SQL for a course's place in Courses, from 1.
func courseRank(column string) string {
	return `array_position(ARRAY['Easy','Normal','Hard','Oni','Edit'],` + column + `)`
}

func (r Rule) lowestCourse() int {
	if r.Course != nil {
		for i, c := range Courses {
			if c == *r.Course {
				return i + 1
			}
		}
	}
	return 1
}

// met is a query for the players who meet r, one column user_id: all of them when $1 is ”,
// else that player or nobody. A score counts while it stands (is_latest) on a published chart.
//
// The two sign-in rules have nothing kept to judge by: here they are met by players with a
// website session, and by players with a score (only the game sends scores). Seen gives them
// to the player it is told of without asking this.
func (r Rule) met() (string, []any, error) {
	args := []any{}
	switch r.Kind {
	case SiteLogin:
		return `SELECT DISTINCT s.user_id FROM net.sessions s WHERE ($1='' OR s.user_id=$1)`, args, nil
	case GameLogin:
		return `SELECT DISTINCT sc.user_id FROM net.scores sc WHERE ($1='' OR sc.user_id=$1)`, args, nil
	}
	if !r.Valid() {
		return "", nil, errors.New("titlerules: invalid rule " + strconv.FormatInt(r.ID, 10))
	}
	s, _ := schema(*r.Source)
	plays := ` FROM ` + s + `.scores sc JOIN ` + s + `.chart_difficulties d ON d.id=sc.difficulty_id JOIN ` + s + `.charts c ON c.id=d.chart_id
 WHERE c.status='published' AND c.tja_file IS NOT NULL AND sc.is_latest AND ($1='' OR sc.user_id=$1) AND sc.clear_status>=$2 AND ` + courseRank("d.course") + `>=$3`
	switch r.Kind {
	case ChartClear:
		return `SELECT DISTINCT sc.user_id` + plays + ` AND c.id=$4`, append(args, *r.Clear, r.lowestCourse(), *r.ChartID), nil
	case CategoryClear:
		// Without a count it is every chart of the category that has a course high enough,
		// as many as there are now (and at least one).
		return `SELECT sc.user_id` + plays + ` AND (c.category_flags & $4)<>0 GROUP BY sc.user_id
 HAVING count(DISTINCT c.id)>=COALESCE($5::int,(SELECT GREATEST(count(*),1) FROM ` + s + `.charts c2
  WHERE c2.status='published' AND c2.tja_file IS NOT NULL AND (c2.category_flags & $4)<>0
  AND EXISTS(SELECT 1 FROM ` + s + `.chart_difficulties d2 WHERE d2.chart_id=c2.id AND ` + courseRank("d2.course") + `>=$3)))`,
			append(args, *r.Clear, r.lowestCourse(), *r.CategoryFlag, r.Count), nil
	default: // ChartsUploaded: the uploader is the first owner.
		return `SELECT c.owner_ids[1] AS user_id FROM ` + s + `.charts c
 WHERE c.status='published' AND c.tja_file IS NOT NULL AND ($1='' OR c.owner_ids[1]=$1) GROUP BY c.owner_ids[1] HAVING count(*)>=$2`, append(args, *r.Count), nil
	}
}

// give gives r's title to the players the query set selects (one column user_id, its
// arguments from $2) who have not had it from r, and returns them. One statement: it needs no
// transaction of its own.
func (r Rule) give(ctx context.Context, db DB, set string, user string, args []any) ([]string, error) {
	n := len(args) + 1
	all := append(append([]any{user}, args...), r.ID, r.TitleID, r.Kind)
	rows, err := db.Query(ctx, fmt.Sprintf(`WITH met AS (%s),
 a AS (INSERT INTO net.awards(service,request_key,user_id,granted)
  SELECT '`+Service+`',$%d::bigint||':'||m.user_id,m.user_id,jsonb_build_object('titles',jsonb_build_array($%d::text),'rule',$%d::bigint,'kind',$%d::text)
  FROM met m JOIN net.users u ON u.id=m.user_id
  ON CONFLICT(service,request_key) DO NOTHING RETURNING id,user_id),
 t AS (INSERT INTO net.user_titles(user_id,title_id,award_id) SELECT a.user_id,$%d::text,a.id FROM a ON CONFLICT DO NOTHING RETURNING user_id)
 SELECT user_id FROM t ORDER BY user_id`, set, n+1, n+2, n+1, n+3, n+2), all...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// Backfill gives r's title to every player who meets it already and returns how many got it.
func Backfill(ctx context.Context, db DB, r Rule) (int, error) {
	set, args, err := r.met()
	if err != nil {
		return 0, err
	}
	users, err := r.give(ctx, db, set, "", args)
	return len(users), err
}

// pending reads the rules selected by where (of a table r, arguments from $2) that have given
// user nothing yet.
func pending(ctx context.Context, db DB, user, where string, args ...any) ([]Rule, error) {
	rows, err := db.Query(ctx, `SELECT `+Columns+` FROM net.title_rules r WHERE `+where+`
 AND NOT EXISTS(SELECT 1 FROM net.awards a WHERE a.service='`+Service+`' AND a.request_key=r.id||':'||$1) ORDER BY r.id`, append([]any{user}, args...)...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Rule, error) { return Scan(row) })
}

// judge gives user the titles of those of rules they meet, and returns the titles they did
// not hold before.
func judge(ctx context.Context, db DB, user string, rules []Rule) ([]string, error) {
	titles := []string{}
	for _, r := range rules {
		set, args, err := r.met()
		if err != nil {
			return titles, err
		}
		got, err := r.give(ctx, db, set, user, args)
		if err != nil {
			return titles, err
		}
		if len(got) > 0 {
			titles = append(titles, r.TitleID)
		}
	}
	return titles, nil
}

// Played judges the rules a score on a chart can have met: those of the chart, and those of
// its categories. Call it once the score is saved, in its transaction or after. It returns
// the titles the player got.
func Played(ctx context.Context, db DB, source, user, chart string) ([]string, error) {
	s, ok := schema(source)
	if !ok || user == "" {
		return nil, errors.New("titlerules: unknown source or no player")
	}
	rules, err := pending(ctx, db, user, `r.source=$2 AND ((r.kind='`+ChartClear+`' AND r.chart_id=$3)
 OR (r.kind='`+CategoryClear+`' AND (r.category_flag & COALESCE((SELECT c.category_flags FROM `+s+`.charts c WHERE c.id=$3),0))<>0))`, source, chart)
	if err != nil || len(rules) == 0 {
		return nil, err
	}
	return judge(ctx, db, user, rules)
}

// Uploaded judges the rules that count a player's charts in a source. Call it once a chart of
// theirs is published, in its transaction or after.
func Uploaded(ctx context.Context, db DB, source, user string) ([]string, error) {
	if _, ok := schema(source); !ok || user == "" {
		return nil, errors.New("titlerules: unknown source or no player")
	}
	rules, err := pending(ctx, db, user, `r.source=$2 AND r.kind='`+ChartsUploaded+`'`, source)
	if err != nil || len(rules) == 0 {
		return nil, err
	}
	return judge(ctx, db, user, rules)
}

// Seen gives a player who was just seen signed in the titles for that: kind is SiteLogin or
// GameLogin. The player must be in net.users already.
func Seen(ctx context.Context, db DB, kind, user string) ([]string, error) {
	if (kind != SiteLogin && kind != GameLogin) || user == "" {
		return nil, errors.New("titlerules: not a sign-in rule")
	}
	rules, err := pending(ctx, db, user, `r.kind=$2`, kind)
	if err != nil {
		return nil, err
	}
	titles := []string{}
	for _, r := range rules {
		got, err := r.give(ctx, db, `SELECT $1::text AS user_id`, user, nil)
		if err != nil {
			return titles, err
		}
		if len(got) > 0 {
			titles = append(titles, r.TitleID)
		}
	}
	return titles, nil
}
