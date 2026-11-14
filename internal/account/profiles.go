package account

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Players' nicknames and avatars are the account center's: that is where a player changes
// them, and it tells nobody when one does. Asking it each time a name is shown made every
// page that shows one wait for it, and fail to show the name whenever it could not be
// reached. So the site keeps a copy in net.users and shows that. The copy is brought up to
// date
//
//   - when the player does anything signed in: the account center describes them with every
//     session it confirms (Identity), so a player who changes their name sees it at once;
//   - by RunProfileRefresh, which asks about every player the site knows every few minutes,
//     a hundred to a request, outside any request of a player's;
//   - the first time a player without a copy is asked about (Profiles), from the account
//     center directly.
//
// Others see a changed name after the next refresh at the latest.

// ProfileRefreshEvery is how often every known player's copy is brought up to date.
const ProfileRefreshEvery = 10 * time.Minute

// KeepProfiles makes the client keep its copy of players' profiles in net.users, through db.
// Without it (as in tests of one part) every question goes to the account center.
func (c *Client) KeepProfiles(db *pgxpool.Pool) { c.kept = db }

// lookup asks the account center about players. Those it does not know are left out.
func (c *Client) lookup(ctx context.Context, ids []string) (map[string]PublicProfile, error) {
	names := map[string]PublicProfile{}
	for len(ids) > 0 {
		n := min(len(ids), 100)
		var res struct {
			Users []User `json:"users"`
		}
		if e := c.callAgain(ctx, "users/lookup", map[string]any{"userIds": ids[:n]}, &res); e != nil {
			return nil, e
		}
		for _, u := range res.Users {
			names[u.ID] = PublicProfile{Nickname: u.Nickname, AvatarURL: c.AvatarURL(u)}
		}
		ids = ids[n:]
	}
	return names, nil
}

// callAgain is Call for a question that changes nothing, asked once more when the account
// center could not be reached: it replaces its workers now and then, and a connection that
// was open to one of them is cut.
func (c *Client) callAgain(ctx context.Context, path string, in, out any) error {
	e := c.Call(ctx, path, in, out)
	if errors.Is(e, errUnreachable) && ctx.Err() == nil {
		e = c.Call(ctx, path, in, out)
	}
	return e
}

// Profiles is what the account center publishes about each of ids; a player it does not know
// is left out. With a kept copy the answer is the copy, and only players never asked about
// are asked about now.
func (c *Client) Profiles(ctx context.Context, ids []string) (map[string]PublicProfile, error) {
	if c.kept == nil {
		return c.lookup(ctx, ids)
	}
	names := map[string]PublicProfile{}
	checked := map[string]bool{}
	rows, err := c.kept.Query(ctx, `SELECT id,nickname,avatar_url FROM net.users WHERE id=ANY($1) AND profile_checked_at IS NOT NULL`, ids)
	if err != nil {
		return c.lookup(ctx, ids)
	}
	var id string
	var p PublicProfile
	if _, err = pgx.ForEachRow(rows, []any{&id, &p.Nickname, &p.AvatarURL}, func() error {
		checked[id] = true
		// Asked about, and the account center had no such player.
		if p.Nickname != "" || p.AvatarURL != "" {
			names[id] = p
		}
		return nil
	}); err != nil {
		return c.lookup(ctx, ids)
	}
	var unasked []string
	for _, id := range ids {
		if !checked[id] {
			unasked = append(unasked, id)
			checked[id] = true
		}
	}
	if len(unasked) == 0 {
		return names, nil
	}
	fresh, err := c.lookup(ctx, unasked)
	if err != nil {
		// The players there is a copy of are shown; the rest have no name this time.
		log.Printf("player profiles: %d not asked about yet: %v", len(unasked), err)
		return names, nil
	}
	for id, p := range fresh {
		names[id] = p
	}
	if err = c.keep(ctx, unasked, fresh); err != nil {
		log.Printf("player profiles: keeping a copy: %v", err)
	}
	return names, nil
}

// keep writes what the account center said about asked: the profile of each it knows, and
// nothing for one it does not. A player the site has no record of is not given one here.
func (c *Client) keep(ctx context.Context, asked []string, known map[string]PublicProfile) error {
	nicknames, avatars := make([]string, len(asked)), make([]string, len(asked))
	for i, id := range asked {
		nicknames[i], avatars[i] = known[id].Nickname, known[id].AvatarURL
	}
	_, err := c.kept.Exec(ctx, `UPDATE net.users u SET nickname=v.nickname, avatar_url=v.avatar_url, profile_checked_at=now()
 FROM unnest($1::text[],$2::text[],$3::text[]) AS v(id,nickname,avatar_url) WHERE u.id=v.id`, asked, nicknames, avatars)
	return err
}

// remember keeps the profile the account center just described a signed-in player with. It
// writes only when the copy differs.
func (c *Client) remember(ctx context.Context, u User) {
	if c.kept == nil {
		return
	}
	if _, err := c.kept.Exec(ctx, `UPDATE net.users SET nickname=$2, avatar_url=$3, profile_checked_at=now()
 WHERE id=$1 AND (nickname<>$2 OR avatar_url<>$3 OR profile_checked_at IS NULL)`, u.ID, u.Nickname, u.AvatarURL); err != nil && ctx.Err() == nil {
		log.Printf("player profiles: keeping %s: %v", u.ID, err)
	}
}

// RefreshProfiles asks the account center about every player the site knows and keeps the
// answers. It reports how many players it asked about.
func (c *Client) RefreshProfiles(ctx context.Context) (int, error) {
	if c.kept == nil {
		return 0, nil
	}
	rows, err := c.kept.Query(ctx, `SELECT id FROM net.users ORDER BY id`)
	if err != nil {
		return 0, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return 0, err
	}
	asked := 0
	for len(ids) > 0 {
		n := min(len(ids), 100)
		known, err := c.lookup(ctx, ids[:n])
		if err != nil {
			return asked, err
		}
		if err = c.keep(ctx, ids[:n], known); err != nil {
			return asked, err
		}
		asked += n
		ids = ids[n:]
	}
	return asked, nil
}

// RunProfileRefresh runs RefreshProfiles at start-up and every ProfileRefreshEvery until ctx
// ends. A refresh that fails leaves the copies as they were until the next.
func (c *Client) RunProfileRefresh(ctx context.Context) {
	ticker := time.NewTicker(ProfileRefreshEvery)
	defer ticker.Stop()
	for {
		if _, err := c.RefreshProfiles(ctx); err != nil && ctx.Err() == nil {
			log.Printf("player profiles: refresh pending: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
