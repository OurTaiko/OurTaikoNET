package rules

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"ourtaiko.dev/net/internal/netdb/netdbtest"
)

var alice, bob = strings.Repeat("a", 32), strings.Repeat("b", 32)

// world is a shared database with two players and a public title "t", and add makes fanmade
// charts: each of the given category bits, with an Oni block, first owned by owner.
func world(t *testing.T) (*pgxpool.Pool, func(id string, flags int, owners ...string) int64) {
	t.Helper()
	pool := netdbtest.NewShared(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO net.users(id) VALUES($1),($2)`, alice, bob); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO net.titles(id,name,style,visibility) VALUES('t','T','wood','public')`); err != nil {
		t.Fatal(err)
	}
	add := func(id string, flags int, owners ...string) int64 {
		t.Helper()
		var slot int64
		err := pool.QueryRow(ctx, `WITH f AS (INSERT INTO fanmade.chart_resources(storage_key,original_filename,sha256,byte_size,extension)
 VALUES($1||'.tja','a.tja',repeat('1',64),1,'tja'),($1||'.ogg','a.ogg',repeat('2',64),1,'ogg') RETURNING id),
 c AS (INSERT INTO fanmade.charts(id,owner_ids,title,bpm,duration,category_flags,tja_file,audio_file)
  SELECT $1,$3::text[],'Song',120,10,$2,min(id),max(id) FROM f RETURNING id)
 INSERT INTO fanmade.chart_difficulties(chart_id,course,side,level,content_hash,good_points,ok_points,long_hit_points,kiwami_threshold,scoring_version)
 SELECT id,'Oni','single',8,'v2:'||encode(sha256(id::bytea),'hex'),1000,500,100,1000000,1 FROM c RETURNING id`, id, flags, owners).Scan(&slot)
		if err != nil {
			t.Fatal(err)
		}
		return slot
	}
	return pool, add
}

// judgeAll judges every rule for every player, as making each of them again would.
func judgeAll(ctx context.Context, db DB) (int, error) {
	rules, err := read(ctx, db, `true`)
	if err != nil {
		return 0, err
	}
	return backfillAll(ctx, db, rules)
}

func clear(t *testing.T, pool *pgxpool.Pool, user string, slot int64) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `INSERT INTO fanmade.scores(id,user_id,difficulty_id,good,ok,bad,score,drumroll,max_combo,payload_digest,clear_status)
 VALUES(md5(random()::text),$1,$2,10,0,0,0,0,10,encode(sha256(random()::text::bytea),'hex'),1)`, user, slot); err != nil {
		t.Fatal(err)
	}
}

func rule(t *testing.T, pool *pgxpool.Pool, columns, values string) Rule {
	t.Helper()
	r, err := Scan(pool.QueryRow(context.Background(), `INSERT INTO net.reward_rules AS r(item,title_id,`+columns+`) VALUES('title','t',`+values+`) RETURNING `+Columns))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func holders(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT user_id FROM net.user_titles WHERE title_id='t' ORDER BY user_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return ids
}

func expect(t *testing.T, pool *pgxpool.Pool, want ...string) {
	t.Helper()
	if got := holders(t, pool); !slices.Equal(got, append([]string{}, want...)) {
		t.Fatalf("held by %v, want %v", got, want)
	}
}

// A player who holds the item already does not use the rule up: once they lose it, the rule
// gives it again. A rule that gave it once does not.
func TestAHolderDoesNotUseARuleUp(t *testing.T) {
	pool, _ := world(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE net.users SET seen_on_site_at=now() WHERE id IN ($1,$2)`, alice, bob); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `WITH a AS (INSERT INTO net.awards(service,request_key,user_id,granted) VALUES('admin','hand',$1,'{}') RETURNING id)
 INSERT INTO net.user_titles(user_id,title_id,award_id) SELECT $1,'t',id FROM a`, alice); err != nil {
		t.Fatal(err)
	}
	r := rule(t, pool, `kind`, `'site_login'`)
	if n, err := Backfill(ctx, pool, r); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	var awards int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM net.awards WHERE service='rule' AND user_id=$1`, alice).Scan(&awards); err != nil || awards != 0 {
		t.Fatal("the rule was used up on a holder", awards, err)
	}
	// Taken back by hand: the rule gives it now, once.
	if _, err := pool.Exec(ctx, `DELETE FROM net.user_titles WHERE user_id=$1`, alice); err != nil {
		t.Fatal(err)
	}
	if got, err := Seen(ctx, pool, SiteLogin, alice); err != nil || !slices.Equal(got, []string{"title:t"}) {
		t.Fatal(got, err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM net.user_titles WHERE user_id=$1`, alice); err != nil {
		t.Fatal(err)
	}
	if n, err := judgeAll(ctx, pool); err != nil || n != 0 {
		t.Fatal("given again", n, err)
	}
	expect(t, pool, bob)
}

// The sign-in rules go by where each player was seen.
func TestSignInRulesGoByWhereAPlayerWasSeen(t *testing.T) {
	pool, _ := world(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE net.users SET seen_in_game_at=now() WHERE id=$1`, bob); err != nil {
		t.Fatal(err)
	}
	site := rule(t, pool, `kind`, `'site_login'`)
	if n, err := Backfill(ctx, pool, site); err != nil || n != 0 {
		t.Fatal(n, err)
	}
	game := rule(t, pool, `kind`, `'game_login'`)
	if n, err := Backfill(ctx, pool, game); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	expect(t, pool, bob)
	// Seen only judges a player seen there.
	if got, err := Seen(ctx, pool, SiteLogin, alice); err != nil || len(got) != 0 {
		t.Fatal(got, err)
	}
}

// What changes in a source's charts is judged for everyone: a chart that leaves a category, a
// collaborator joining a chart (every owner of a chart counts it as an upload, not only the first).
func TestChangesOfChartsAreJudgedForEveryone(t *testing.T) {
	pool, add := world(t)
	ctx := context.Background()
	a := add(strings.Repeat("1", 32), 1, bob)
	add(strings.Repeat("2", 32), 1, bob)
	add(strings.Repeat("3", 32), 4, alice)
	clear(t, pool, alice, a)
	// Every chart of category 1; two charts of the source.
	all := rule(t, pool, `kind,source,category_flag,clear`, `'category_clear','fanmade',1,1`)
	if n, err := Backfill(ctx, pool, all); err != nil || n != 0 {
		t.Fatal(n, err)
	}
	// The second chart of the category goes to another: alice has cleared all of it.
	if _, err := pool.Exec(ctx, `UPDATE fanmade.charts SET category_flags=4 WHERE id=$1`, strings.Repeat("2", 32)); err != nil {
		t.Fatal(err)
	}
	if n, err := ChartsChanged(ctx, pool, "fanmade"); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	expect(t, pool, alice)

	if _, err := pool.Exec(ctx, `DELETE FROM net.user_titles`); err != nil {
		t.Fatal(err)
	}
	uploads := rule(t, pool, `kind,count`, `'charts_uploaded',3`)
	if n, err := Backfill(ctx, pool, uploads); err != nil || n != 0 {
		t.Fatal(n, err)
	}
	// Bob joins alice's chart as a collaborator: three charts are his.
	if _, err := pool.Exec(ctx, `UPDATE fanmade.charts SET owner_ids=array_append(owner_ids,$1) WHERE id=$2`, bob, strings.Repeat("3", 32)); err != nil {
		t.Fatal(err)
	}
	if n, err := ChartsChanged(ctx, pool, "fanmade"); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	expect(t, pool, bob)
	if _, err := ChartsChanged(ctx, pool, "elsewhere"); err == nil {
		t.Fatal("an unknown source was judged")
	}
}

// Valid takes exactly the rules the table takes, so a rule the API lets through is never refused
// by the database, and one it refuses would have been.
func TestValidIsWhatTheTableTakes(t *testing.T) {
	pool, add := world(t)
	ctx := context.Background()
	add(strings.Repeat("1", 32), 1, bob)
	str := func(s string) *string { return &s }
	num := func(n int) *int { return &n }
	title, part := Rule{Item: ItemTitle, TitleID: str("t")}, Rule{Item: ItemDonItem, DonPart: str("paint"), DonItem: num(2)}
	conditions := []Rule{
		{Kind: SiteLogin}, {Kind: GameLogin}, {Kind: SiteLogin, Count: num(1)}, {Kind: GameLogin, Source: str("fanmade")},
		{Kind: ChartClear, Source: str("fanmade"), ChartID: str(strings.Repeat("1", 32)), Clear: num(1)},
		{Kind: ChartClear, Source: str("fanmade"), ChartID: str(strings.Repeat("1", 32)), Clear: num(3), Course: str("Oni")},
		{Kind: ChartClear, Source: str("fanmade"), Clear: num(1)},
		{Kind: ChartClear, Source: str("fanmade"), ChartID: str(strings.Repeat("1", 32)), Clear: num(1), Count: num(2)},
		{Kind: CategoryClear, Source: str("official"), CategoryFlag: num(4), Clear: num(2)},
		{Kind: CategoryClear, Source: str("fanmade"), CategoryFlag: num(1), Clear: num(1), Count: num(3), Course: str("Edit")},
		{Kind: CategoryClear, Source: str("fanmade"), CategoryFlag: num(3), Clear: num(1)},
		{Kind: CategoryClear, Source: str("fanmade"), CategoryFlag: num(1)},
		{Kind: CategoryClear, Source: str("fanmade"), CategoryFlag: num(1), Clear: num(4)},
		{Kind: ChartsUploaded, Count: num(1)},
		{Kind: ChartsUploaded},
		{Kind: ChartsUploaded, Count: num(1), Clear: num(1)},
		{Kind: ChartsUploaded, Source: str("fanmade"), Count: num(1)},
	}
	items := []Rule{title, part,
		{Item: ItemTitle, DonPart: str("paint"), DonItem: num(2)},
		{Item: ItemDonItem, TitleID: str("t")},
		{Item: ItemDonItem, TitleID: str("t"), DonPart: str("paint"), DonItem: num(2)},
	}
	for i, item := range items {
		for j, c := range conditions {
			r := c
			r.Item, r.TitleID, r.DonPart, r.DonItem = item.Item, item.TitleID, item.DonPart, item.DonItem
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			_, err = tx.Exec(ctx, `INSERT INTO net.reward_rules(item,title_id,don_part,don_item,kind,source,chart_id,category_flag,course,clear,count)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, r.Item, r.TitleID, r.DonPart, r.DonItem, r.Kind, r.Source, r.ChartID, r.CategoryFlag, r.Course, r.Clear, r.Count)
			tx.Rollback(ctx)
			if taken := err == nil; taken != r.Valid() {
				t.Errorf("item %d, conditions %d: table takes it %v, Valid %v (%v)", i, j, taken, r.Valid(), err)
			}
		}
	}
}

// The same rule twice is refused by the table.
func TestARuleIsMadeOnce(t *testing.T) {
	pool, _ := world(t)
	rule(t, pool, `kind,source,category_flag,clear`, `'category_clear','fanmade',1,1`)
	_, err := pool.Exec(context.Background(), `INSERT INTO net.reward_rules(item,title_id,kind,source,category_flag,clear) VALUES('title','t','category_clear','fanmade',1,1)`)
	if err == nil || !strings.Contains(fmt.Sprint(err), "reward_rules_once") {
		t.Fatal(err)
	}
}

// What a rule gave stays given: no longer meeting the rule (a chart added to the category, the
// score retired, the chart deleted), the rule made harder or deleted, and judging the rules again
// take nothing back. Only a manager takes an item back, by hand.
func TestWhatARuleGaveStaysGiven(t *testing.T) {
	pool, add := world(t)
	ctx := context.Background()
	a := add(strings.Repeat("1", 32), 1, bob)
	clear(t, pool, alice, a)
	all := rule(t, pool, `kind,source,category_flag,clear`, `'category_clear','fanmade',1,1`)
	if n, err := Backfill(ctx, pool, all); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	expect(t, pool, alice)
	pass := func() {
		t.Helper()
		if _, err := ChartsChanged(ctx, pool, "fanmade"); err != nil {
			t.Fatal(err)
		}
		if _, err := judgeAll(ctx, pool); err != nil {
			t.Fatal(err)
		}
		expect(t, pool, alice)
	}
	// Another chart in the category: alice no longer has all of it.
	add(strings.Repeat("2", 32), 1, bob)
	pass()
	// Her score retired, then the chart deleted.
	if _, err := pool.Exec(ctx, `UPDATE fanmade.scores SET is_latest=false`); err != nil {
		t.Fatal(err)
	}
	pass()
	if _, err := pool.Exec(ctx, `DELETE FROM fanmade.scores; DELETE FROM fanmade.charts WHERE id='`+strings.Repeat("1", 32)+`'`); err != nil {
		t.Fatal(err)
	}
	pass()
	// The rule made harder, then deleted.
	if _, err := pool.Exec(ctx, `UPDATE net.reward_rules SET clear=3,count=50 WHERE id=$1`, all.ID); err != nil {
		t.Fatal(err)
	}
	pass()
	if _, err := pool.Exec(ctx, `DELETE FROM net.reward_rules`); err != nil {
		t.Fatal(err)
	}
	pass()
}
