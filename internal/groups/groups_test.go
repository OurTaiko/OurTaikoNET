package groups

import (
	"context"
	"testing"

	"ourtaiko.dev/net/internal/netdb/netdbtest"
)

func TestNamesAndBits(t *testing.T) {
	// The numbers are in the database: a group's bit never changes.
	if OfficialManager != 1 || FanmadeManager != 2 || TitleManager != 4 || TagManager != 8 {
		t.Fatal("a group changed its bit", OfficialManager, FanmadeManager, TitleManager, TagManager)
	}
	set, err := Parse([]string{"TITLE_MANAGER", "OFFICIAL_MANAGER"})
	if err != nil || set != 5 || !set.Has(OfficialManager) || set.Has(FanmadeManager) || set.Has(OfficialManager|FanmadeManager) {
		t.Fatal(set, err)
	}
	if names := set.Names(); len(names) != 2 || names[0] != "OFFICIAL_MANAGER" || names[1] != "TITLE_MANAGER" {
		t.Fatal(names)
	}
	if _, err = Parse([]string{"ADMIN"}); err == nil {
		t.Fatal("a name that is no group was accepted")
	}
	if len(Set(0).Names()) != 0 || Known() != 15 {
		t.Fatal(Set(0).Names(), Known())
	}
}

func TestGiveAndRead(t *testing.T) {
	pool := netdbtest.NewShared(t)
	ctx := context.Background()
	const admin, player = "0000000000000000000000000000ad01", "00000000000000000000000000000b0b"
	if _, err := pool.Exec(ctx, `INSERT INTO net.users(id) VALUES ($1)`, admin); err != nil {
		t.Fatal(err)
	}
	if set, err := Of(ctx, pool, player); err != nil || set != 0 {
		t.Fatal("someone unheard of is in a group", set, err)
	}
	give := func(set Set) Set {
		t.Helper()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		before, err := Give(ctx, tx, player, set, admin)
		if err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		return before
	}
	if before := give(OfficialManager | TitleManager); before != 0 {
		t.Fatal(before)
	}
	if set, err := Of(ctx, pool, player); err != nil || set != OfficialManager|TitleManager {
		t.Fatal(set, err)
	}
	give(OfficialManager | TitleManager) // the same again changes and records nothing
	if before := give(FanmadeManager); before != OfficialManager|TitleManager {
		t.Fatal(before)
	}
	var changes int
	var last string
	if err := pool.QueryRow(ctx, `SELECT count(*), (SELECT groups_before||'>'||groups_after||' by '||changed_by FROM net.group_changes ORDER BY id DESC LIMIT 1) FROM net.group_changes`).Scan(&changes, &last); err != nil || changes != 2 || last != "5>2 by "+admin {
		t.Fatal("recorded changes:", changes, last, err)
	}
	tx, _ := pool.Begin(ctx)
	if _, err := Give(ctx, tx, player, 1<<20, admin); err == nil {
		t.Fatal("a bit with no meaning was given")
	}
	tx.Rollback(ctx)
	members, err := Members(ctx, pool, nil)
	if err != nil || len(members) != 1 || members[0].ID != player || members[0].Groups[0] != "FANMADE_MANAGER" {
		t.Fatal(members, err)
	}
	members, err = Members(ctx, pool, []string{admin, "00000000000000000000000000000000"})
	if err != nil || len(members) != 2 || len(members[0].Groups) != 0 || len(members[1].Groups) != 0 {
		t.Fatal(members, err)
	}
}
