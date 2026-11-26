// Package rules gives items by rule (net.reward_rules): a player who clears a certain chart,
// clears enough charts of a category, passes a fixed 段位 course, is seen signed in on the website
// or in the game, or has uploaded enough charts gets the item the rule names. An item is a title or a part of the 3D
// Don (Rule.Item); each kind of item says how it is given and how it is held (target), and
// nothing else here knows it. What a player is given is then told to them as a reward by the
// database's triggers (package rewards); this package does not tell anyone.
//
// A rule gives a player its item once. What it gave is an award of the service "rule" whose
// request_key is "<rule id>:<user id>", so asking again gives nothing, and an item an
// administrator took back is not given again by the same rule. A player who holds the item
// already (given by hand, by another rule) is not given it and the rule is not
// used up for them: if they lose it, the rule gives it when they meet it again.
//
// Rules only give. Nothing here takes an item back: not when a player stops meeting a rule, nor
// when a rule is changed or deleted. Only a manager does, by hand (the grants routes), or by
// deleting the title.
//
// Rules are judged when something a player does can meet them (Played, DanAttempted, Seen), when charts change
// (ChartsChanged), and when a rule is made or changed (Backfill). Nothing goes over them on a
// clock: what meets a rule without the program seeing it (a change made in the database's
// console) is given when one of these next judges that rule.
//
// Every statement names its schemas: the callers' connections differ in search_path.
package rules

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
	"ourtaiko.dev/net/internal/account"
)

// The kinds of rule.
const (
	ChartClear     = "chart_clear"
	CategoryClear  = "category_clear"
	DanClear       = "dan_clear"
	SiteLogin      = "site_login"
	GameLogin      = "game_login"
	ChartsUploaded = "charts_uploaded"
)

// The kinds of item a rule gives (net.reward_rules.item).
const (
	ItemTitle   = "title"
	ItemDonItem = "don_item"
)

// UploadSource is the source players upload charts to, the one a ChartsUploaded rule counts (the
// official source's charts are the built-in account's).
const UploadSource = "fanmade"

// Service is the awards.service of what rules give.
const Service = "rule"

// The verdicts of a 段位 attempt a DanClear rule may take (its Clear): 1–3 red, 4–6 gold, each
// pass, with full combo, with all good.
const DanClearMax = 6

// Courses from the lowest: a rule's course is the lowest that counts.
var Courses = []string{"Easy", "Normal", "Hard", "Oni", "Edit"}

// Rule is one row of net.reward_rules. Which fields a kind has is the table's reward_rules_shape,
// which fields an item has its reward_rules_item.
type Rule struct {
	ID int64 `json:"id"`
	// Item is which kind of item: ItemTitle (TitleID) or ItemDonItem (DonPart and DonItem).
	Item    string  `json:"item"`
	TitleID *string `json:"titleId"`
	DonPart *string `json:"donPart"`
	DonItem *int    `json:"donItem"`
	Kind    string  `json:"kind"`
	Source  *string `json:"source"`
	ChartID *string `json:"chartId"`
	// DanID is a DanClear rule's 段位 course in Source.
	DanID        *string `json:"danId"`
	CategoryFlag *int    `json:"-"`
	Course       *string `json:"course"`
	Clear        *int    `json:"clear"`
	Count        *int    `json:"count"`
}

// Columns are a rule's columns as Scan reads them, of a table named r.
const Columns = `r.id,r.item,r.title_id,r.don_part,r.don_item,r.kind,r.source,r.chart_id,r.dan_id,r.category_flag,r.course,r.clear,r.count`

// Scan reads a rule selected by Columns; more receives what was selected after them.
func Scan(row interface{ Scan(...any) error }, more ...any) (Rule, error) {
	var r Rule
	err := row.Scan(append([]any{&r.ID, &r.Item, &r.TitleID, &r.DonPart, &r.DonItem, &r.Kind, &r.Source, &r.ChartID, &r.DanID, &r.CategoryFlag, &r.Course, &r.Clear, &r.Count}, more...)...)
	return r, err
}

// DB is a pool or a transaction.
type DB interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func schema(source string) (string, bool) {
	return source, source == "fanmade" || source == "official"
}

// target is how a rule's item is given: the insert into the item's own table of the players in
// CTE a (user_id and the award's id), with the item's arguments from $first; the condition that
// the player of row m already holds it; and what the award says it granted. A new kind of item
// is a new case here.
type target struct {
	insert  func(first int) string
	holds   func(first int) string
	args    []any
	granted func(first int) string
	key     string
}

var donParts = map[string]bool{"cos": true, "head": true, "body": true, "puchi": true, "paint": true}

func (r Rule) target() (target, bool) {
	switch {
	case r.Item == ItemTitle && r.TitleID != nil && *r.TitleID != "" && r.DonPart == nil && r.DonItem == nil:
		return target{
			insert: func(first int) string {
				return fmt.Sprintf(`INSERT INTO net.user_titles(user_id,title_id,award_id) SELECT a.user_id,$%d::text,a.id FROM a`, first)
			},
			holds: func(first int) string {
				return fmt.Sprintf(`EXISTS(SELECT 1 FROM net.user_titles h WHERE h.user_id=m.user_id AND h.title_id=$%d::text)`, first)
			},
			args: []any{*r.TitleID}, key: "title:" + *r.TitleID,
			granted: func(first int) string {
				return fmt.Sprintf(`jsonb_build_object('titles',jsonb_build_array($%d::text))`, first)
			},
		}, true
	case r.Item == ItemDonItem && r.TitleID == nil && r.DonPart != nil && donParts[*r.DonPart] && r.DonItem != nil && *r.DonItem > 0:
		return target{
			insert: func(first int) string {
				return fmt.Sprintf(`INSERT INTO net.user_don_items(user_id,part,item_id,award_id) SELECT a.user_id,$%d::text,$%d::int,a.id FROM a`, first, first+1)
			},
			holds: func(first int) string {
				return fmt.Sprintf(`EXISTS(SELECT 1 FROM net.user_don_items h WHERE h.user_id=m.user_id AND h.part=$%d::text AND h.item_id=$%d::int)`, first, first+1)
			},
			args: []any{*r.DonPart, *r.DonItem},
			granted: func(first int) string {
				return fmt.Sprintf(`jsonb_build_object('donItems',jsonb_build_array(jsonb_build_object('part',$%d::text,'id',$%d::int)))`, first, first+1)
			},
			key: "don:" + *r.DonPart + ":" + strconv.Itoa(*r.DonItem),
		}, true
	}
	return target{}, false
}

// Key names what the rule gives, as "title:<id>" or "don:<part>:<number>".
func (r Rule) Key() string {
	t, _ := r.target()
	return t.key
}

// Valid says whether r is a rule the table takes (reward_rules_shape, reward_rules_item) and
// this package can judge. It does not look at whether the item or the chart exist.
func (r Rule) Valid() bool {
	if _, ok := r.target(); !ok {
		return false
	}
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
	if r.Kind != DanClear && r.DanID != nil {
		return false
	}
	switch r.Kind {
	case ChartClear:
		return source && r.ChartID != nil && *r.ChartID != "" && r.CategoryFlag == nil && course && clear && r.Count == nil
	case CategoryClear:
		return source && r.ChartID == nil && flag && course && clear && (r.Count == nil || count)
	case DanClear:
		verdict := r.Clear != nil && *r.Clear >= 1 && *r.Clear <= DanClearMax
		return source && r.DanID != nil && *r.DanID != "" && r.ChartID == nil && r.CategoryFlag == nil && r.Course == nil && verdict && r.Count == nil
	case ChartsUploaded:
		return !source && r.ChartID == nil && r.CategoryFlag == nil && r.Course == nil && r.Clear == nil && count
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
// An attempt at a 段位 course counts while the course is published and fixed, if it was made
// since the course was fixed: before, its maker could change its songs at will. A change the
// managers approved since leaves the time of fixing, and the attempts before it, as they are. The sign-in rules go by
// where a player was seen (net.users, signin.RecordActivity).
func (r Rule) met() (string, []any, error) {
	if !r.Valid() {
		return "", nil, errors.New("rules: invalid rule " + strconv.FormatInt(r.ID, 10))
	}
	args := []any{}
	switch r.Kind {
	case SiteLogin:
		return `SELECT u.id AS user_id FROM net.users u WHERE u.seen_on_site_at IS NOT NULL AND ($1='' OR u.id=$1)`, args, nil
	case GameLogin:
		return `SELECT u.id AS user_id FROM net.users u WHERE u.seen_in_game_at IS NOT NULL AND ($1='' OR u.id=$1)`, args, nil
	case ChartsUploaded:
		// A chart counts for each of its owners (its collaborators too), but the built-in account.
		return `SELECT o.user_id FROM ` + UploadSource + `.charts c CROSS JOIN LATERAL unnest(c.owner_ids) AS o(user_id)
 WHERE c.status='published' AND c.tja_file IS NOT NULL AND o.user_id<>'` + account.SystemUserID + `' AND ($1='' OR o.user_id=$1) GROUP BY o.user_id HAVING count(DISTINCT c.id)>=$2`, append(args, *r.Count), nil
	}
	s, _ := schema(*r.Source)
	if r.Kind == DanClear {
		return `SELECT DISTINCT a.user_id FROM ` + s + `.dan_attempts a JOIN ` + s + `.dan_courses d ON d.id=a.course_id
 WHERE d.id=$3 AND d.status='published' AND d.fixed_at IS NOT NULL AND a.submitted_at>=d.fixed_at AND a.verdict>=$2 AND ($1='' OR a.user_id=$1)`,
			append(args, *r.Clear, *r.DanID), nil
	}
	plays := ` FROM ` + s + `.scores sc JOIN ` + s + `.chart_difficulties d ON d.id=sc.difficulty_id JOIN ` + s + `.charts c ON c.id=d.chart_id
 WHERE c.status='published' AND c.tja_file IS NOT NULL AND sc.is_latest AND ($1='' OR sc.user_id=$1) AND sc.clear_status>=$2 AND ` + courseRank("d.course") + `>=$3`
	if r.Kind == ChartClear {
		return `SELECT DISTINCT sc.user_id` + plays + ` AND c.id=$4`, append(args, *r.Clear, r.lowestCourse(), *r.ChartID), nil
	}
	// CategoryClear. Without a count it is every chart of the category that has a course high
	// enough, as many as there are now (and at least one).
	return `SELECT sc.user_id` + plays + ` AND (c.category_flags & $4)<>0 GROUP BY sc.user_id
 HAVING count(DISTINCT c.id)>=COALESCE($5::int,(SELECT GREATEST(count(*),1) FROM ` + s + `.charts c2
  WHERE c2.status='published' AND c2.tja_file IS NOT NULL AND (c2.category_flags & $4)<>0
  AND EXISTS(SELECT 1 FROM ` + s + `.chart_difficulties d2 WHERE d2.chart_id=c2.id AND ` + courseRank("d2.course") + `>=$3)))`,
		append(args, *r.Clear, r.lowestCourse(), *r.CategoryFlag, r.Count), nil
}

// give gives r's item to the players the query set selects (one column user_id, its arguments
// from $2; all of them when user is "", else that one) who neither hold it nor have had it from
// r, and returns them. One statement: it needs no transaction of its own.
func (r Rule) give(ctx context.Context, db DB, set string, user string, args []any) ([]string, error) {
	t, ok := r.target()
	if !ok {
		return nil, errors.New("rules: rule " + strconv.FormatInt(r.ID, 10) + " gives nothing")
	}
	n := len(args) + 1
	all := append(append(append([]any{user}, args...), r.ID, r.Kind), t.args...)
	rule, kind, item := n+1, n+2, n+3
	title, titleArgs := r.courseTitle(item + len(t.args))
	all = append(all, titleArgs...)
	rows, err := db.Query(ctx, fmt.Sprintf(`WITH met AS (%s),
 a AS (INSERT INTO net.awards(service,request_key,user_id,granted,course_title)
  SELECT '`+Service+`',$%d::bigint||':'||m.user_id,m.user_id,%s||jsonb_build_object('rule',$%d::bigint,'kind',$%d::text),%s
  FROM met m JOIN net.users u ON u.id=m.user_id WHERE NOT %s
  ON CONFLICT(service,request_key) DO NOTHING RETURNING id,user_id),
 t AS (%s ON CONFLICT DO NOTHING RETURNING user_id)
 SELECT user_id FROM t ORDER BY user_id`, set, rule, t.granted(item), rule, kind, title, t.holds(item), t.insert(item)), all...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// courseTitle is SQL for the course_title of what r gives, with its arguments from $first: the
// name of a DanClear rule's course (in the first language it has, as the game's attempts are
// recorded), so that the reward says what earned it; nothing for the other kinds.
func (r Rule) courseTitle(first int) (string, []any) {
	if r.Kind != DanClear {
		return `''`, nil
	}
	s, _ := schema(*r.Source)
	return fmt.Sprintf(`COALESCE((SELECT COALESCE(d.title->>'ja',d.title->>'zh',d.title->>'en',d.title->>'ko',d.title->>'zh_tw') FROM %s.dan_courses d WHERE d.id=$%d::text),'')`, s, first), []any{*r.DanID}
}

// Backfill gives r's item to every player who meets it and returns how many got it.
func Backfill(ctx context.Context, db DB, r Rule) (int, error) {
	set, args, err := r.met()
	if err != nil {
		return 0, err
	}
	users, err := r.give(ctx, db, set, "", args)
	return len(users), err
}

// read reads the rules selected by where (of a table r, arguments from $1).
func read(ctx context.Context, db DB, where string, args ...any) ([]Rule, error) {
	rows, err := db.Query(ctx, `SELECT `+Columns+` FROM net.reward_rules r WHERE `+where+` ORDER BY r.id`, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Rule, error) { return Scan(row) })
}

// backfillAll gives the items of rules to everyone who meets them, and returns how many items
// were given.
func backfillAll(ctx context.Context, db DB, rules []Rule) (int, error) {
	total := 0
	for _, r := range rules {
		n, err := Backfill(ctx, db, r)
		if err != nil {
			return total, fmt.Errorf("rule %d: %w", r.ID, err)
		}
		total += n
	}
	return total, nil
}

// judge gives user the items of those of rules they meet, and returns the items (Rule.Key)
// they got.
func judge(ctx context.Context, db DB, user string, rules []Rule) ([]string, error) {
	got := []string{}
	for _, r := range rules {
		set, args, err := r.met()
		if err != nil {
			return got, err
		}
		users, err := r.give(ctx, db, set, user, args)
		if err != nil {
			return got, err
		}
		if len(users) > 0 {
			got = append(got, r.Key())
		}
	}
	return got, nil
}

// unused is the condition, for a rule r and a player $1, that r has not given them anything.
const unused = ` AND NOT EXISTS(SELECT 1 FROM net.awards a WHERE a.service='` + Service + `' AND a.request_key=r.id||':'||$1)`

// Played judges the rules a score on a chart can have met: those of the chart, and those of
// its categories. Call it once the score is saved, in its transaction or after. It returns
// the items the player got (Rule.Key).
func Played(ctx context.Context, db DB, source, user, chart string) ([]string, error) {
	s, ok := schema(source)
	if !ok || user == "" {
		return nil, errors.New("rules: unknown source or no player")
	}
	rules, err := read(ctx, db, `r.source=$2 AND ((r.kind='`+ChartClear+`' AND r.chart_id=$3)
 OR (r.kind='`+CategoryClear+`' AND (r.category_flag & COALESCE((SELECT c.category_flags FROM `+s+`.charts c WHERE c.id=$3),0))<>0))`+unused, user, source, chart)
	if err != nil || len(rules) == 0 {
		return nil, err
	}
	return judge(ctx, db, user, rules)
}

// DanAttempted judges the rules of a 段位 course a player just made an attempt at. Call it once
// the attempt is saved, in its transaction or after. It returns the items the player got.
func DanAttempted(ctx context.Context, db DB, source, user, course string) ([]string, error) {
	if _, ok := schema(source); !ok || user == "" {
		return nil, errors.New("rules: unknown source or no player")
	}
	rules, err := read(ctx, db, `r.kind='`+DanClear+`' AND r.source=$2 AND r.dan_id=$3`+unused, user, source, course)
	if err != nil || len(rules) == 0 {
		return nil, err
	}
	return judge(ctx, db, user, rules)
}

// Seen gives a player who was just seen signed in the items for that: kind is SiteLogin or
// GameLogin. Where they were seen must be on their row already (signin.RecordActivity).
func Seen(ctx context.Context, db DB, kind, user string) ([]string, error) {
	if (kind != SiteLogin && kind != GameLogin) || user == "" {
		return nil, errors.New("rules: not a sign-in rule")
	}
	rules, err := read(ctx, db, `r.kind=$2`+unused, user, kind)
	if err != nil {
		return nil, err
	}
	return judge(ctx, db, user, rules)
}

// ChartsChanged judges, for every player, the rules that go by a source's charts as a whole:
// those of its categories (a chart added to one or taken out of it, deleted, or with its courses
// changed) and, for UploadSource, those of uploads (a chart published, a collaborator joining
// one). Call it once the change is saved. It returns how many items were given.
func ChartsChanged(ctx context.Context, db DB, source string) (int, error) {
	if _, ok := schema(source); !ok {
		return 0, errors.New("rules: unknown source")
	}
	rules, err := read(ctx, db, `(r.source=$1 AND r.kind='`+CategoryClear+`') OR ($1='`+UploadSource+`' AND r.kind='`+ChartsUploaded+`')`, source)
	if err != nil {
		return 0, err
	}
	return backfillAll(ctx, db, rules)
}
