package account_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"ourtaiko.dev/net/internal/account"
	"ourtaiko.dev/net/internal/netdb/netdbtest"
)

// center stands in for the account center: who it knows, how often it was asked, and whether
// it can be reached.
type center struct {
	*httptest.Server
	mu     sync.Mutex
	names  map[string]string
	avatar map[string]string
	asked  int
	cut    int // connections to cut before answering again
	signed string
}

func newCenter(t *testing.T) *center {
	c := &center{names: map[string]string{}, avatar: map[string]string{}}
	c.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.asked++
		if c.cut > 0 {
			c.cut--
			panic(http.ErrAbortHandler)
		}
		user := func(id string) map[string]any {
			return map[string]any{"id": id, "nickname": c.names[id], "avatarUrl": c.avatar[id]}
		}
		switch r.URL.Path {
		case "/internal/v1/users/lookup":
			var in struct {
				IDs []string `json:"userIds"`
			}
			json.NewDecoder(r.Body).Decode(&in)
			users := []map[string]any{}
			for _, id := range in.IDs {
				if _, known := c.names[id]; known {
					users = append(users, user(id))
				}
			}
			json.NewEncoder(w).Encode(map[string]any{"users": users})
		case "/internal/v1/web/introspect":
			json.NewEncoder(w).Encode(map[string]any{"user": user(c.signed)})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(c.Close)
	return c
}

func (c *center) set(id, name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.names[id] = name
}

func (c *center) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.asked
}

// A name is shown from the site's copy: the account center is asked about a player once, is
// not needed afterwards, and a changed name arrives with the player's own next request or the
// next refresh.
func TestPlayersNamesAreShownFromTheSitesCopy(t *testing.T) {
	ctx := context.Background()
	pool := netdbtest.New(t, "")
	don, katsu, gone, stranger := strings.Repeat("a", 32), strings.Repeat("b", 32), strings.Repeat("c", 32), strings.Repeat("d", 32)
	if _, err := pool.Exec(ctx, `INSERT INTO net.users(id) VALUES ($1),($2),($3)`, don, katsu, gone); err != nil {
		t.Fatal(err)
	}
	sso := newCenter(t)
	sso.set(don, "Don")
	sso.set(katsu, "Katsu")
	sso.set(stranger, "Stranger")
	picture := sso.URL + "/avatars/" + don + "/" + strings.Repeat("0", 32) + ".webp"
	sso.avatar[don] = picture
	client := &account.Client{Config: account.Config{Issuer: sso.URL, ServiceID: "net"}, HTTP: sso.Client()}
	client.KeepProfiles(pool)
	all := []string{don, katsu, gone}
	ask := func(ids ...string) map[string]account.PublicProfile {
		t.Helper()
		names, err := client.Profiles(ctx, ids)
		if err != nil {
			t.Fatal(err)
		}
		return names
	}

	// The first time, the account center is asked, once for all of them.
	names := ask(all...)
	if len(names) != 2 || names[don] != (account.PublicProfile{Nickname: "Don", AvatarURL: picture}) || names[katsu].Nickname != "Katsu" {
		t.Fatal(names)
	}
	if sso.count() != 1 {
		t.Fatal("asked", sso.count())
	}
	// After that it is not asked, not even about the player it did not know.
	if names = ask(all...); len(names) != 2 || names[don].Nickname != "Don" || sso.count() != 1 {
		t.Fatal(names, sso.count())
	}
	// Nor is it needed: unreachable, the names are still there.
	sso.mu.Lock()
	sso.cut = 100
	sso.mu.Unlock()
	if names = ask(all...); names[don].Nickname != "Don" || names[katsu].Nickname != "Katsu" || sso.count() != 1 {
		t.Fatal(names, sso.count())
	}
	// A player the site has no record of is asked about each time, and with the account
	// center unreachable has no name; the others keep theirs.
	if names = ask(don, stranger); names[don].Nickname != "Don" || names[stranger].Nickname != "" {
		t.Fatal(names)
	}
	if sso.count() != 3 {
		t.Fatal("a question that could not be asked is asked once more, and no more:", sso.count())
	}
	if _, err := client.RefreshProfiles(ctx); err == nil {
		t.Fatal("a refresh that reached nobody reported success")
	}
	if names = ask(all...); names[don].Nickname != "Don" {
		t.Fatal("a failed refresh changed the copy:", names)
	}

	// One cut connection is not a failure: the question is asked again.
	sso.mu.Lock()
	sso.cut = 1
	sso.mu.Unlock()
	if names = ask(stranger); names[stranger].Nickname != "Stranger" {
		t.Fatal(names)
	}
	sso.mu.Lock()
	sso.cut = 0
	sso.mu.Unlock()

	// A name changed at the account center is the old one here until the next refresh.
	sso.set(don, "Don-chan")
	sso.set(gone, "Back")
	sso.mu.Lock()
	delete(sso.names, katsu)
	sso.mu.Unlock()
	if names = ask(all...); names[don].Nickname != "Don" {
		t.Fatal(names)
	}
	if n, err := client.RefreshProfiles(ctx); err != nil || n != 3 {
		t.Fatal(n, err)
	}
	names = ask(all...)
	if names[don].Nickname != "Don-chan" || names[gone].Nickname != "Back" {
		t.Fatal(names)
	}
	if _, shown := names[katsu]; shown {
		t.Fatal("a player the account center no longer knows still has a name:", names)
	}

	// The player's own request brings their new name at once.
	sso.set(don, "Don the Third")
	sso.mu.Lock()
	sso.signed = don
	sso.mu.Unlock()
	if u, err := client.Identity(ctx, "web", "token"); err != nil || u.Nickname != "Don the Third" {
		t.Fatal(u, err)
	}
	before := sso.count()
	if names = ask(don); names[don].Nickname != "Don the Third" || sso.count() != before {
		t.Fatal(names, sso.count()-before)
	}
}

// Without a kept copy, as in the tests of one part, every question goes to the account center.
func TestWithoutACopyTheAccountCenterIsAsked(t *testing.T) {
	sso := newCenter(t)
	don := strings.Repeat("a", 32)
	sso.set(don, "Don")
	client := &account.Client{Config: account.Config{Issuer: sso.URL, ServiceID: "net"}, HTTP: sso.Client()}
	for i := 1; i <= 2; i++ {
		names, err := client.Profiles(context.Background(), []string{don})
		if err != nil || names[don].Nickname != "Don" || sso.count() != i {
			t.Fatal(names, err, sso.count())
		}
	}
}
