// Package donitems keeps which parts of the 3D Don each player owns, and which parts must be
// owned to be worn (net.user_don_items, net.don_item_locks; migration 0010).
//
// Every part may be worn by anyone unless it is locked. Locking a part gives it to everyone who
// wears it at that moment, so nobody's Don changes; taking a part back from a player takes it off
// them. The parts themselves are in the public catalog, which this package does not read: the
// caller checks that a part exists. Number 0 of a part (none, or the piece taken off) is never
// locked or owned.
//
// Statements name their schema: callers' connections differ in search_path.
package donitems

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// The parts, as the catalog names them.
var Parts = []string{"cos", "head", "body", "puchi", "paint"}

// Item is one part of the Don by its number.
type Item struct {
	Part string `json:"part"`
	ID   int    `json:"id"`
}

// Valid says whether an item can be locked or owned: a known part and a number above 0.
func (i Item) Valid() bool {
	if i.ID <= 0 {
		return false
	}
	for _, p := range Parts {
		if p == i.Part {
			return true
		}
	}
	return false
}

// Owned is an item a player owns and since when.
type Owned struct {
	Item
	AcquiredAt time.Time `json:"acquiredAt"`
}

// DB is a pool, a connection or a transaction.
type DB interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Locked is every part that must be owned to be worn.
func Locked(ctx context.Context, db DB) ([]Item, error) {
	rows, err := db.Query(ctx, `SELECT part,item_id FROM net.don_item_locks ORDER BY part,item_id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Item])
}

// Of is what a player owns, newest first.
func Of(ctx context.Context, db DB, user string) ([]Owned, error) {
	rows, err := db.Query(ctx, `SELECT part,item_id,acquired_at FROM net.user_don_items WHERE user_id=$1 ORDER BY acquired_at DESC,part,item_id`, user)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Owned, error) {
		var o Owned
		err := row.Scan(&o.Part, &o.ID, &o.AcquiredAt)
		return o, err
	})
}

// Barred is which of the items a player may not wear: locked ones they do not own. Items that are
// not Valid (none, taken off) are never barred.
func Barred(ctx context.Context, db DB, user string, items []Item) ([]Item, error) {
	barred := []Item{}
	for _, it := range items {
		if !it.Valid() {
			continue
		}
		var no bool
		err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM net.don_item_locks WHERE part=$2 AND item_id=$3)
 AND NOT EXISTS(SELECT 1 FROM net.user_don_items WHERE user_id=$1 AND part=$2 AND item_id=$3)`, user, it.Part, it.ID).Scan(&no)
		if err != nil {
			return nil, err
		}
		if no {
			barred = append(barred, it)
		}
	}
	return barred, nil
}

// worn is SQL for the players wearing part $1 number $2 of net.don_looks: a costume is worn when
// costume is on, a head and a body when it is off.
const worn = `SELECT l.user_id FROM net.don_looks l WHERE CASE $1::text
 WHEN 'cos' THEN l.costume AND l.cos_id=$2 WHEN 'head' THEN NOT l.costume AND l.head_id=$2
 WHEN 'body' THEN NOT l.costume AND l.body_id=$2 WHEN 'puchi' THEN l.puchi_id=$2 ELSE l.paint_id=$2 END`

// Lock makes an item one that must be owned, giving it to everyone wearing it now, and returns how
// many were given it. Locking a locked item does nothing.
func Lock(ctx context.Context, db DB, it Item) (int, error) {
	if !it.Valid() {
		return 0, errors.New("donitems: not an item")
	}
	var given int
	err := db.QueryRow(ctx, `WITH l AS (INSERT INTO net.don_item_locks(part,item_id) VALUES($1,$2) ON CONFLICT DO NOTHING RETURNING 1),
 g AS (INSERT INTO net.user_don_items(user_id,part,item_id) SELECT w.user_id,$1,$2 FROM (`+worn+`) w WHERE EXISTS(SELECT 1 FROM l)
  ON CONFLICT DO NOTHING RETURNING 1)
 SELECT count(*) FROM g`, it.Part, it.ID).Scan(&given)
	return given, err
}

// Unlock makes an item anyone's again. Those who own it keep it.
func Unlock(ctx context.Context, db DB, it Item) error {
	_, err := db.Exec(ctx, `DELETE FROM net.don_item_locks WHERE part=$1 AND item_id=$2`, it.Part, it.ID)
	return err
}

// Give gives a player an item, recorded as an award of the service (such as "admin") with what
// the award should say it granted; the reward follows from it (migration 0010). It returns whether
// the player did not have it already. Call it in a transaction.
func Give(ctx context.Context, db DB, user string, it Item, service, key string, granted map[string]any) (bool, error) {
	if !it.Valid() {
		return false, errors.New("donitems: not an item")
	}
	if granted == nil {
		granted = map[string]any{}
	}
	granted["donItems"] = []Item{it}
	data, _ := json.Marshal(granted)
	var award int64
	if err := db.QueryRow(ctx, `INSERT INTO net.awards(service,request_key,user_id,granted) VALUES($1,$2,$3,$4) RETURNING id`, service, key, user, data).Scan(&award); err != nil {
		return false, err
	}
	tag, err := db.Exec(ctx, `INSERT INTO net.user_don_items(user_id,part,item_id,award_id) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, user, it.Part, it.ID, award)
	return err == nil && tag.RowsAffected() == 1, err
}

// Take takes an item back from a player, and off them if they wear it and it is locked. It
// returns whether they had it.
func Take(ctx context.Context, db DB, user string, it Item) (bool, error) {
	tag, err := db.Exec(ctx, `DELETE FROM net.user_don_items WHERE user_id=$1 AND part=$2 AND item_id=$3`, user, it.Part, it.ID)
	if err != nil || tag.RowsAffected() == 0 {
		return false, err
	}
	// Off them: the piece back to number 0 (none, or taken off).
	_, err = db.Exec(ctx, `UPDATE net.don_looks l SET
 cos_id=CASE WHEN $2='cos' AND l.cos_id=$3 THEN 0 ELSE l.cos_id END,
 head_id=CASE WHEN $2='head' AND l.head_id=$3 THEN 0 ELSE l.head_id END,
 body_id=CASE WHEN $2='body' AND l.body_id=$3 THEN 0 ELSE l.body_id END,
 puchi_id=CASE WHEN $2='puchi' AND l.puchi_id=$3 THEN 0 ELSE l.puchi_id END,
 paint_id=CASE WHEN $2='paint' AND l.paint_id=$3 THEN 0 ELSE l.paint_id END,
 updated_at=now()
 WHERE l.user_id=$1 AND EXISTS(SELECT 1 FROM net.don_item_locks k WHERE k.part=$2 AND k.item_id=$3)`, user, it.Part, it.ID)
	return true, err
}
