package donitems

import (
	"context"
	"testing"

	"ourtaiko.dev/net/internal/netdb/netdbtest"
)

const (
	wearer  = "0000000000000000000000000000aaaa"
	other   = "0000000000000000000000000000bbbb"
	nothing = "0000000000000000000000000000cccc"
)

// Locking, owning, giving and taking back a part of the Don, and what that does to rewards and
// to what players wear.
func TestPartsAreLockedOwnedGivenAndTakenBack(t *testing.T) {
	pool := netdbtest.NewShared(t)
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO net.users(id) VALUES ($1),($2),($3)`, wearer, other, nothing)
	// The wearer has costume 7 on and the puchichara 3; the other has costume 7 stored but wears a
	// head and a body.
	exec(`INSERT INTO net.don_looks(user_id,costume,cos_id,head_id,body_id,puchi_id) VALUES ($1,true,7,0,0,3),($2,false,7,4,5,0)`, wearer, other)
	cos7, puchi3 := Item{"cos", 7}, Item{"puchi", 3}

	// Nothing is locked: anyone may wear anything, and none and taken-off are never items.
	if barred, err := Barred(ctx, pool, nothing, []Item{cos7, puchi3, {"cos", 0}, {"paint", 0}}); err != nil || len(barred) != 0 {
		t.Fatal(barred, err)
	}
	if (Item{"cos", 0}).Valid() || (Item{"hat", 1}).Valid() || !cos7.Valid() {
		t.Fatal("Valid")
	}

	// Locking gives the part to whoever wears it now, not to whoever only stored it; again does nothing.
	if n, err := Lock(ctx, pool, cos7); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	if n, err := Lock(ctx, pool, cos7); err != nil || n != 0 {
		t.Fatal("locked twice", n, err)
	}
	if locked, err := Locked(ctx, pool); err != nil || len(locked) != 1 || locked[0] != cos7 {
		t.Fatal(locked, err)
	}
	for user, barred := range map[string]int{wearer: 0, other: 1, nothing: 1} {
		got, err := Barred(ctx, pool, user, []Item{cos7})
		if err != nil || len(got) != barred {
			t.Fatal(user, got, err)
		}
	}
	// Given on locking: nothing new to tell.
	var rewards int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM net.rewards`).Scan(&rewards); err != nil || rewards != 0 {
		t.Fatal("a reward for wearing it already", rewards, err)
	}

	// Given by an administrator: an award, and a reward that names the part.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	given, err := Give(ctx, tx, other, cos7, "admin", "grant-1", map[string]any{"by": wearer})
	if err != nil || !given {
		t.Fatal(given, err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var kind, part, service string
	var item int
	if err = pool.QueryRow(ctx, `SELECT kind,data->>'part',(data->>'itemId')::int,data->>'service' FROM net.rewards WHERE user_id=$1`, other).Scan(&kind, &part, &item, &service); err != nil {
		t.Fatal(err)
	}
	if kind != "don_item" || part != "cos" || item != 7 || service != "admin" {
		t.Fatal(kind, part, item, service)
	}
	if owned, err := Of(ctx, pool, other); err != nil || len(owned) != 1 || owned[0].Item != cos7 {
		t.Fatal(owned, err)
	}

	// Taken back before it was seen: the reward goes; taken back from the wearer of a locked part:
	// off them too. A part they do not have is not taken.
	if had, err := Take(ctx, pool, other, cos7); err != nil || !had {
		t.Fatal(had, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM net.rewards`).Scan(&rewards); err != nil || rewards != 0 {
		t.Fatal("the unseen reward stayed", rewards, err)
	}
	if had, err := Take(ctx, pool, wearer, cos7); err != nil || !had {
		t.Fatal(had, err)
	}
	var cos, puchi int
	if err = pool.QueryRow(ctx, `SELECT cos_id,puchi_id FROM net.don_looks WHERE user_id=$1`, wearer).Scan(&cos, &puchi); err != nil || cos != 0 || puchi != 3 {
		t.Fatal("not taken off", cos, puchi, err)
	}
	if had, err := Take(ctx, pool, nothing, cos7); err != nil || had {
		t.Fatal(had, err)
	}

	// Unlocked, it is anyone's again.
	if err = Unlock(ctx, pool, cos7); err != nil {
		t.Fatal(err)
	}
	if barred, err := Barred(ctx, pool, nothing, []Item{cos7}); err != nil || len(barred) != 0 {
		t.Fatal(barred, err)
	}
}
