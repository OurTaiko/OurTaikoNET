package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The parts of the Don that must be earned: an administrator locks one (whoever wears it keeps
// it), gives and takes it back; a rule gives one; wearing a locked part needs owning it.
func TestLockedPartsOfTheDonAreEarned(t *testing.T) {
	bucket := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(testDonCatalog)) }))
	defer bucket.Close()
	pool := testDB(t)
	ctx := context.Background()
	app := New(pool, Config{Origin: origin, DevUserID: player, DonCatalogURL: bucket.URL + "/3dchara/catalog.json", DonClient: bucket.Client()})
	c := &client{t: t, handler: app.Handler()}
	c.signIn()

	// The player wears costume 30, and the other player has worn nothing.
	c.json("PUT", "/api/v1/me/don", `{"costume":true,"cos":30,"head":0,"body":0,"puchi":1}`, 200, nil)
	if _, err := pool.Exec(ctx, `INSERT INTO net.users(id) VALUES ($1) ON CONFLICT DO NOTHING`, other); err != nil {
		t.Fatal(err)
	}

	// Only parts of the catalog, and never none or taken-off, can be locked.
	c.json("PUT", "/api/v1/admin/don/locks/cos/31", `{"locked":true}`, 404, nil)
	c.json("PUT", "/api/v1/admin/don/locks/cos/0", `{"locked":true}`, 404, nil)
	c.json("PUT", "/api/v1/admin/don/locks/hat/1", `{"locked":true}`, 404, nil)
	// Locking costume 30 gives it to the one wearing it.
	var lock struct{ Given int }
	c.json("PUT", "/api/v1/admin/don/locks/cos/30", `{"locked":true}`, 200, &lock)
	if lock.Given != 1 {
		t.Fatal(lock)
	}
	c.json("PUT", "/api/v1/admin/don/locks/paint/2", `{"locked":true}`, 200, &lock)
	var items struct {
		Locked []struct {
			Part string
			ID   int
		}
	}
	guest := &client{t: t, handler: app.Handler()}
	guest.json("GET", "/api/v1/don/items", "", 200, &items)
	if len(items.Locked) != 2 {
		t.Fatal(items)
	}
	var mine struct {
		Owned []struct {
			Part string
			ID   int
		}
	}
	c.json("GET", "/api/v1/me/don/items", "", 200, &mine)
	if len(mine.Owned) != 1 || mine.Owned[0].Part != "cos" || mine.Owned[0].ID != 30 {
		t.Fatal(mine)
	}
	guest.json("GET", "/api/v1/me/don/items", "", 401, nil)

	// The player keeps wearing it; a locked part they do not own cannot be put on.
	c.json("PUT", "/api/v1/me/don", `{"costume":true,"cos":30,"head":0,"body":0,"puchi":1}`, 200, nil)
	w := c.do("PUT", "/api/v1/me/don", `{"costume":true,"cos":30,"head":0,"body":0,"puchi":1,"paint":2}`, nil)
	if w.Code != 403 {
		t.Fatal(w.Code, w.Body.String())
	}
	// A locked part only stored (a costume while wearing a head and a body) is not worn.
	c.json("PUT", "/api/v1/admin/don/locks/head/3", `{"locked":true}`, 200, nil)
	c.json("PUT", "/api/v1/me/don", `{"costume":true,"cos":30,"head":3,"body":4,"puchi":1}`, 200, nil)

	// Given by an administrator: owned, worn, and told as a reward.
	c.json("POST", "/api/v1/admin/don/grants", `{"userId":"`+player+`","part":"paint","id":2}`, 200, nil)
	c.json("PUT", "/api/v1/me/don", `{"costume":true,"cos":30,"head":0,"body":0,"puchi":1,"paint":2}`, 200, nil)
	var reward string
	if err := pool.QueryRow(ctx, `SELECT kind FROM net.rewards WHERE user_id=$1 AND data->>'part'='paint'`, player).Scan(&reward); err != nil || reward != "don_item" {
		t.Fatal(reward, err)
	}
	// Described for a client that does not read the catalog: its name and picture.
	ask := httptest.NewRequest("GET", "/api/v1/game/rewards", nil)
	raw := string(app.DescribeReward(ask, "don_item", []byte(`{"part":"cos","itemId":30,"service":"admin"}`)))
	// Costume 30 has no icon in this catalog: a name and no picture.
	if !strings.Contains(raw, `"鏡もち"`) || !strings.Contains(raw, `"service":"admin"`) || strings.Contains(raw, `"picture"`) {
		t.Fatal(raw)
	}
	// A puchichara is its own picture.
	raw = string(app.DescribeReward(ask, "don_item", []byte(`{"part":"puchi","itemId":1}`)))
	if !strings.Contains(raw, `"picture":"`+bucket.URL+`/3dchara/puchi/1.png?v=p1p1p1p1p1p1"`) {
		t.Fatal(raw)
	}
	// A part the catalog does not have is answered as it is.
	if raw = string(app.DescribeReward(ask, "don_item", []byte(`{"part":"cos","itemId":99}`))); raw != `{"part":"cos","itemId":99}` {
		t.Fatal(raw)
	}
	if got := app.DescribeReward(httptest.NewRequest("GET", "/", nil), "title", []byte(`{"x":1}`)); string(got) != `{"x":1}` {
		t.Fatal("another kind changed", string(got))
	}

	// Taken back: off them too.
	c.json("DELETE", "/api/v1/admin/don/grants", `{"userId":"`+player+`","part":"paint","id":2}`, 200, nil)
	c.json("DELETE", "/api/v1/admin/don/grants", `{"userId":"`+player+`","part":"paint","id":2}`, 404, nil)
	var answer DonAnswer
	c.json("GET", "/api/v1/me/don", "", 200, &answer)
	if answer.Look.Paint != 0 || answer.Look.Cos != 30 {
		t.Fatal(answer.Look)
	}

	// A rule gives a part: here to whoever has signed in on the website, the player at once.
	var made struct {
		Rule    TitleRule
		Granted int
	}
	c.json("POST", "/api/v1/admin/don/items/paint/2/rules", `{"kind":"site_login"}`, 201, &made)
	if made.Granted != 1 || made.Rule.TitleID != nil || made.Rule.DonPart == nil || *made.Rule.DonPart != "paint" || *made.Rule.DonItem != 2 {
		t.Fatal(made)
	}
	c.json("POST", "/api/v1/admin/don/items/paint/9/rules", `{"kind":"site_login"}`, 404, nil)
	c.json("PUT", "/api/v1/me/don", `{"costume":true,"cos":30,"head":0,"body":0,"puchi":1,"paint":2}`, 200, nil)
	guest.json("GET", "/api/v1/don/items", "", 200, &struct{}{})
	var listed struct{ Rules []TitleRule }
	guest.json("GET", "/api/v1/don/items", "", 200, &listed)
	if len(listed.Rules) != 1 || *listed.Rules[0].DonItem != 2 {
		t.Fatal(listed)
	}

	// Unlocked: the other player may put it on without owning it.
	c.json("PUT", "/api/v1/admin/don/locks/paint/2", `{"locked":false}`, 200, nil)
	c.json("PUT", "/api/v1/admin/don/locks/paint/2", `{}`, 400, nil)
	guest.json("PUT", "/api/v1/admin/don/locks/paint/2", `{"locked":true}`, 401, nil)
}
