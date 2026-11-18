// Package groups is what a player may do beyond what every player may.
//
// A player is in any number of user groups. Each group is one bit of net.users.groups, and a
// route that only some may use names the group it needs. Nobody is in a group by default.
//
// The account center knows one thing about the site: who its administrator is. That
// permission is for managing the site, which includes giving and taking groups in the manage
// panel. It puts its holder in no group: an administrator who is to upload official charts
// gives themself OFFICIAL_MANAGER like anyone else.
package groups

import (
	"context"
	"errors"
	"fmt"
	"ourtaiko.dev/net/internal/files"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Set is a player's groups as kept in the database.
type Set uint32

// The groups. A bit, once given a meaning, keeps it: the numbers are in the database.
const (
	// OfficialManager adds, replaces, edits and removes charts of the official source, keeps
	// its dan courses and playlists, and moderates its comments.
	OfficialManager Set = 1 << iota
	// FanmadeManager edits or removes any chart, dan course or playlist of the fanmade source,
	// marks its dan courses official, and moderates its comments.
	FanmadeManager
	// TitleManager keeps the title catalog and gives or takes titles by hand.
	TitleManager
)

// Group describes one group for the manage panel.
type Group struct {
	Name  string `json:"name"`
	Bit   Set    `json:"bit"`
	About string `json:"about"`
}

var all = []Group{
	{"OFFICIAL_MANAGER", OfficialManager, "Adds, replaces, edits and removes charts of the official source, keeps its dan courses and playlists, and moderates its comments."},
	{"FANMADE_MANAGER", FanmadeManager, "Edits or removes any chart, dan course or playlist of the fanmade source, marks its dan courses official, and moderates its comments."},
	{"TITLE_MANAGER", TitleManager, "Keeps the title catalog and gives or takes titles by hand."},
}

// All lists the groups in the order of their bits.
func All() []Group { return append([]Group(nil), all...) }

// Known is every bit that has a meaning.
func Known() Set {
	var known Set
	for _, g := range all {
		known |= g.Bit
	}
	return known
}

// Has reports whether the set contains every group of want.
func (s Set) Has(want Set) bool { return s&want == want }

// Names lists the set's groups by name, in the order of their bits.
func (s Set) Names() []string {
	names := []string{}
	for _, g := range all {
		if s.Has(g.Bit) {
			names = append(names, g.Name)
		}
	}
	return names
}

// Parse turns group names into a set. A name that is not a group is an error.
func Parse(names []string) (Set, error) {
	var set Set
	for _, name := range names {
		found := false
		for _, g := range all {
			if g.Name == name {
				set |= g.Bit
				found = true
			}
		}
		if !found {
			return 0, fmt.Errorf("no such group %q", name)
		}
	}
	return set, nil
}

// Querier is a pool, a connection or a transaction.
type Querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Of reads a player's groups. A player the site has not heard of is in none.
func Of(ctx context.Context, db Querier, user string) (Set, error) {
	var bits int64
	err := db.QueryRow(ctx, `SELECT groups FROM net.users WHERE id=$1`, user).Scan(&bits)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return Set(bits) & Known(), err
}

// Member is a player and their groups.
// Member is a player and their groups, with the name and avatar the site keeps of them so
// that the manage panel shows people and not ids. Username is the account center's login
// name; the site keeps no copy of it, so it is there only when the account center was asked
// for a player by that name.
type Member struct {
	ID         string   `json:"id"`
	Username   string   `json:"username,omitempty"`
	Nickname   string   `json:"nickname"`
	AvatarPath string   `json:"avatarPath"`
	Groups     []string `json:"groups"`
	Bits       Set      `json:"bits"`
}

// Give sets a player's groups to exactly set, on behalf of by, and records the change. The
// player need not have been heard of before: an administrator may prepare someone's groups
// ahead of their first sign-in. It reports the player's groups before.
func Give(ctx context.Context, tx pgx.Tx, user string, set Set, by string) (before Set, err error) {
	if set&^Known() != 0 {
		return 0, errors.New("unknown group bits")
	}
	if _, err = tx.Exec(ctx, `INSERT INTO net.users(id) VALUES($1) ON CONFLICT (id) DO NOTHING`, user); err != nil {
		return 0, err
	}
	var bits int64
	if err = tx.QueryRow(ctx, `SELECT groups FROM net.users WHERE id=$1 FOR UPDATE`, user).Scan(&bits); err != nil {
		return 0, err
	}
	before = Set(bits)
	if before == set {
		return before, nil
	}
	if _, err = tx.Exec(ctx, `UPDATE net.users SET groups=$2 WHERE id=$1`, user, int64(set)); err != nil {
		return before, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO net.group_changes(user_id,groups_before,groups_after,changed_by) VALUES($1,$2,$3,$4)`, user, int64(before), int64(set), by)
	return before, err
}

// Members lists the players who are in any group, or, when ids are given, those players
// whatever their groups (one who is not known has none).
func Members(ctx context.Context, db interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}, ids []string) ([]Member, error) {
	sql, args := `SELECT id,groups,nickname,avatar_url FROM net.users WHERE groups<>0 ORDER BY id`, []any{}
	if len(ids) > 0 {
		sql, args = `SELECT id,groups,nickname,avatar_url FROM net.users WHERE id=ANY($1) ORDER BY id`, []any{ids}
	}
	members, err := scanMembers(db.Query(ctx, sql, args...))
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, m := range members {
		seen[m.ID] = true
	}
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			members = append(members, Member{ID: id, Groups: []string{}})
		}
	}
	sort.Slice(members, func(i, j int) bool { return members[i].ID < members[j].ID })
	return members, nil
}

// Find lists the players whose nickname, as the site keeps it, contains text, whatever their
// case: one whose nickname is exactly text first, then whoever was active last. At most limit.
func Find(ctx context.Context, db interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}, text string, limit int) ([]Member, error) {
	pattern := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(text) + "%"
	return scanMembers(db.Query(ctx, `SELECT id,groups,nickname,avatar_url FROM net.users
 WHERE nickname ILIKE $1 ESCAPE '\'
 ORDER BY lower(nickname)=lower($2) DESC, last_active_at DESC NULLS LAST, id LIMIT $3`, pattern, text, limit))
}

func scanMembers(rows pgx.Rows, err error) ([]Member, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	members := []Member{}
	for rows.Next() {
		var m Member
		var bits int64
		var avatar string
		if err = rows.Scan(&m.ID, &bits, &m.Nickname, &avatar); err != nil {
			return nil, err
		}
		m.AvatarPath = files.Path(avatar)
		m.Groups, m.Bits = Set(bits).Names(), Set(bits)&Known()
		members = append(members, m)
	}
	return members, rows.Err()
}
