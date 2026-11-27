package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"ourtaiko.dev/net/internal/account"
	"ourtaiko.dev/net/internal/groups"
	"ourtaiko.dev/net/internal/netdb/netdbtest"
)

const (
	origin = "http://127.0.0.1:5175"
	player = "0000000000000000000000000000abcd"
	other  = "0000000000000000000000000000beef"
)

// testDB is an ourtaiko_net of the test's own, opened for what belongs to no source.
func testDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return netdbtest.NewShared(t)
}

type client struct {
	t       *testing.T
	handler http.Handler
	cookie  string
	csrf    string
}

func (c *client) do(method, path, body string, header map[string]string) *httptest.ResponseRecorder {
	c.t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	if method != "GET" && !strings.HasPrefix(path, "/internal/") {
		r.Header.Set("Origin", origin)
		r.Header.Set("X-CSRF-Token", c.csrf)
	}
	if c.cookie != "" {
		r.AddCookie(&http.Cookie{Name: defaultSessionCookie, Value: c.cookie})
	}
	for k, v := range header {
		if v == "" {
			r.Header.Del(k)
		} else {
			r.Header.Set(k, v)
		}
	}
	w := httptest.NewRecorder()
	c.handler.ServeHTTP(w, r)
	return w
}

func (c *client) json(method, path, body string, status int, out any, header ...map[string]string) {
	c.t.Helper()
	var h map[string]string
	if len(header) > 0 {
		h = header[0]
	}
	w := c.do(method, path, body, h)
	if w.Code != status {
		c.t.Fatalf("%s %s: want %d got %d %s", method, path, status, w.Code, w.Body.String())
	}
	if out != nil {
		if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
			c.t.Fatal(err)
		}
	}
}

// signIn opens a development session for the player.
// part is the number GET /api/title/items gives the part of that type and name.
func (c *client) part(kind, name string) int {
	c.t.Helper()
	var listed struct{ Items []TitleItem }
	c.json("GET", "/api/title/items", "", 200, &listed)
	for _, item := range listed.Items {
		if item.Type == kind && item.Name == name {
			return item.ID
		}
	}
	c.t.Fatal("no part ", kind, "/", name)
	return 0
}

// look is how a request picks a title's layers: the parts of these names by their numbers ("" for
// no effect or icon).
func (c *client) look(band, effect, icon string, overflow bool) string {
	c.t.Helper()
	number := func(kind, name string) string {
		if name == "" {
			return "null"
		}
		return strconv.Itoa(c.part(kind, name))
	}
	return fmt.Sprintf(`"layers":{"ids":{"band":%d,"effect":%s,"icon":%s},"effectOverflow":%t}`, c.part("band", band), number("effect", effect), number("icon", icon), overflow)
}

func (c *client) signIn() {
	c.t.Helper()
	w := c.do("GET", "/api/auth/sso/login?returnTo=/me", "", nil)
	if w.Code != 303 || len(w.Result().Cookies()) != 1 {
		c.t.Fatal("dev sign-in", w.Code)
	}
	c.cookie = w.Result().Cookies()[0].Value
	var me struct {
		User      *User
		CSRFToken string
	}
	c.json("GET", "/api/me", "", 200, &me)
	if me.User == nil || me.User.ID != player {
		c.t.Fatal("not signed in", me)
	}
	c.csrf = me.CSRFToken
}

func TestAwardsTitlesAndNameplates(t *testing.T) {
	pool := testDB(t)
	ctx := context.Background()
	app := New(pool, Config{Origin: origin, DevUserID: player})
	c := &client{t: t, handler: app.Handler()}
	key, err := RegisterService(ctx, pool, "fanmade", "OurTaiko Fanmade")
	if err != nil {
		t.Fatal(err)
	}
	auth := map[string]string{"X-Service-ID": "fanmade", "X-Service-Key": key}

	// A title manager (the development user is one) makes the site's own titles: public, with no
	// maker, and the words for how each is obtained.
	c.json("PUT", "/api/admin/titles/kidou", `{"name":"太鼓の達人","nameTranslations":{"en":"Taiko Master"},"obtain":" 十段に合格 ",`+c.look("lumen-title-gold", "lumen-title-gold", "", false)+`}`, 401, nil)
	c.signIn()
	var kidou Title
	c.json("PUT", "/api/admin/titles/kidou", `{"name":"太鼓の達人","nameTranslations":{"en":"Taiko Master"},"obtain":" 十段に合格 ",`+c.look("lumen-title-gold", "lumen-title-gold", "", false)+`}`, 200, &kidou)
	if kidou.Visibility != public || kidou.OwnerID != nil || kidou.Obtain != "十段に合格" {
		t.Fatal("a manager's title", kidou)
	}
	c.json("PUT", "/api/admin/titles/fan", `{"name":"ファン",`+c.look("lumen-title-wood", "", "", false)+`}`, 200, nil)
	c.json("PUT", "/api/admin/titles/Bad", `{"name":"x",`+c.look("lumen-title-wood", "", "", false)+`}`, 422, nil)
	c.json("PUT", "/api/admin/titles/odd", `{"name":"x","layers":{"band":"gold"}}`, 422, nil)
	c.json("PUT", "/api/admin/titles/odd", `{"name":"x","obtain":"`+strings.Repeat("長", 501)+`",`+c.look("lumen-title-wood", "", "", false)+`}`, 422, nil)
	c.json("PUT", "/api/admin/titles/odd", `{"name":"x","style":"gold"}`, 400, nil)
	var catalog struct{ Titles []ListedTitle }
	c.json("GET", "/api/titles", "", 200, &catalog)
	if len(catalog.Titles) != 2 || catalog.Titles[0].ID != "kidou" || string(catalog.Titles[0].NameTranslations) != `{"en":"Taiko Master"}` || catalog.Titles[0].Holders != 0 {
		t.Fatal("catalog", catalog)
	}

	// Services prove themselves; browsers may not call them.
	award := func(body string, status int, out any, header map[string]string) {
		t.Helper()
		c.json("POST", "/internal/v1/awards", body, status, out, header)
	}
	award(`{"key":"x","userId":"`+player+`","verdict":1}`, 401, nil, map[string]string{"X-Service-ID": "fanmade", "X-Service-Key": strings.Repeat("0", 64)})
	award(`{"key":"x","userId":"`+player+`","verdict":1}`, 403, nil, map[string]string{"X-Service-ID": "fanmade", "X-Service-Key": key, "Origin": origin})

	type reply struct {
		Granted   Granted
		Nameplate Nameplate
	}
	var first reply
	award(`{"key":"attempt-1","userId":"`+player+`","courseId":"10","courseTitle":"十段","official":true,"verdict":2,"dan":20}`, 200, &first, auth)
	if first.Granted.Dan == nil || !first.Granted.Dan.Upgraded || first.Granted.Dan.Previous != nil || first.Granted.Titles == nil || len(first.Granted.Titles) != 0 {
		t.Fatalf("first pass %+v", first.Granted)
	}
	if first.Nameplate.Dan == nil || first.Nameplate.Dan.Index != 20 || first.Nameplate.Dan.Gold || first.Nameplate.Title != nil {
		t.Fatalf("plate after first pass %+v", first.Nameplate)
	}
	var again reply
	award(`{"key":"attempt-1","userId":"`+player+`","courseId":"10","courseTitle":"十段","official":true,"verdict":2,"dan":20}`, 200, &again, auth)
	if !again.Granted.Dan.Upgraded {
		t.Fatal("the same attempt answers what it granted the first time", again.Granted)
	}
	award(`{"key":"attempt-1","userId":"`+other+`","verdict":2}`, 409, nil, auth)
	var lower reply
	award(`{"key":"attempt-2","userId":"`+player+`","official":true,"verdict":1,"dan":20}`, 200, &lower, auth)
	if lower.Granted.Dan.Upgraded || lower.Granted.Dan.Verdict != 2 || *lower.Granted.Dan.Previous != 2 {
		t.Fatalf("a badge only goes up %+v", lower.Granted)
	}
	var gold reply
	award(`{"key":"attempt-3","userId":"`+player+`","official":true,"verdict":5,"dan":20}`, 200, &gold, auth)
	if !gold.Granted.Dan.Upgraded || !gold.Nameplate.Dan.Gold || gold.Nameplate.Dan.Combo != 1 {
		t.Fatalf("gold full combo %+v", gold)
	}
	var failed reply
	award(`{"key":"attempt-4","userId":"`+player+`","official":true,"verdict":0,"dan":24}`, 200, &failed, auth)
	if failed.Granted.Dan != nil || failed.Nameplate.Dan.Index != 20 {
		t.Fatalf("a failed attempt gives nothing %+v", failed)
	}
	// A course that is not official gives no badge, and is recorded all the same.
	var unofficial reply
	award(`{"key":"attempt-5","userId":"`+player+`","official":false,"verdict":3}`, 200, &unofficial, auth)
	if unofficial.Granted.Dan != nil {
		t.Fatalf("unofficial %+v", unofficial.Granted)
	}
	award(`{"key":"attempt-6","userId":"`+player+`","official":false,"verdict":3,"dan":8}`, 422, nil, auth)
	award(`{"key":"attempt-6b","userId":"`+player+`","official":true,"verdict":3,"dan":5}`, 422, nil, auth)
	var higher reply
	award(`{"key":"attempt-7","userId":"`+player+`","official":true,"verdict":4,"dan":21}`, 200, &higher, auth)
	if higher.Granted.Dan == nil || !higher.Granted.Dan.Upgraded {
		t.Fatalf("another badge %+v", higher.Granted)
	}
	award(`{"key":"attempt-8","userId":"`+player+`","courseTitle":"みんなの段位","official":true,"verdict":3}`, 200, nil, auth)
	// Titles are not an award's: a course gives them by rule (reward_rules_test.go), a manager by
	// hand.
	c.json("POST", "/api/admin/grants", `{"userId":"`+player+`","titleId":"kidou"}`, 200, nil)
	c.json("POST", "/api/admin/grants", `{"userId":"`+player+`","titleId":"fan"}`, 200, nil)

	// The player wears one of their titles; the plate is public.
	c.json("PUT", "/api/me/nameplate", `{"titleId":"nope","showDan":true}`, 422, nil)
	var worn Nameplate
	c.json("PUT", "/api/me/nameplate", `{"titleId":"kidou","showDan":true}`, 200, &worn)
	if worn.Title == nil || worn.Title.Style != "gold" || worn.Dan == nil || worn.Dan.Index != 21 ||
		worn.Title.Layers.Band != "band/lumen-title-gold" || worn.Title.Layers.Effect == nil || *worn.Title.Layers.Effect != "effect/lumen-title-gold" || worn.Title.Layers.Icon != nil {
		t.Fatal("worn", worn)
	}
	public := &client{t: t, handler: app.Handler()}
	var plate Nameplate
	public.json("GET", "/api/users/"+player+"/nameplate", "", 200, &plate)
	if plate.Title == nil || plate.Title.ID != "kidou" || !plate.Dan.Gold || plate.Dan.Index != 21 || plate.Nickname != "本地开发用户" {
		t.Fatal("public plate", plate)
	}
	var batch struct{ Nameplates map[string]Nameplate }
	w := public.do("GET", "/api/nameplates?ids="+player+","+other+","+player, "", nil)
	if w.Code != 200 || w.Header().Get("Access-Control-Allow-Origin") != "*" || json.Unmarshal(w.Body.Bytes(), &batch) != nil ||
		len(batch.Nameplates) != 2 || batch.Nameplates[player].Title.ID != "kidou" || batch.Nameplates[other].Title != nil {
		t.Fatal("batch plates", w.Code, w.Body.String())
	}
	public.json("GET", "/api/nameplates?ids=nope", "", 400, nil)
	c.json("PUT", "/api/me/nameplate", `{"titleId":null,"showDan":false}`, 200, &worn)
	public.json("GET", "/api/users/"+player+"/nameplate", "", 200, &plate)
	if plate.Title != nil || plate.Dan != nil || plate.ShowDan {
		t.Fatal("hidden", plate)
	}
	var held struct{ Titles []HeldTitle }
	public.json("GET", "/api/users/"+player+"/titles", "", 200, &held)
	if len(held.Titles) != 2 || held.Titles[0].Title.ID != "fan" || held.Titles[1].Service != "admin" {
		t.Fatal("held titles", held)
	}
	var dans struct {
		Badges  []DanBadge
		History []AwardRecord
	}
	public.json("GET", "/api/users/"+player+"/dans", "", 200, &dans)
	if len(dans.Badges) != 2 || dans.Badges[1].Verdict != 5 || len(dans.History) != 7 {
		t.Fatalf("dans %+v", dans)
	}

	// A title manager grants and takes back a title, and the catalog counts who holds each; a
	// revoked key stops working.
	c.json("POST", "/api/admin/grants", `{"userId":"`+other+`","titleId":"fan"}`, 200, nil)
	public.json("GET", "/api/users/"+other+"/titles", "", 200, &held)
	if len(held.Titles) != 1 || held.Titles[0].Service != "admin" {
		t.Fatal("granted", held)
	}
	c.json("GET", "/api/titles", "", 200, &catalog)
	if catalog.Titles[0].Holders != 1 || catalog.Titles[1].ID != "fan" || catalog.Titles[1].Holders != 2 {
		t.Fatal("holders", catalog)
	}
	c.json("DELETE", "/api/admin/grants", `{"userId":"`+other+`","titleId":"fan"}`, 200, nil)
	c.json("DELETE", "/api/admin/grants", `{"userId":"`+other+`","titleId":"fan"}`, 404, nil)
	if err = DisableService(ctx, pool, "fanmade"); err != nil {
		t.Fatal(err)
	}
	award(`{"key":"attempt-9","userId":"`+player+`","verdict":1}`, 401, nil, auth)
	c.json("PUT", "/api/me/nameplate", `{"titleId":null,"showDan":true}`, 403, nil, map[string]string{"Origin": "https://evil.test"})
}

func TestPlayersMakeTheirOwnTitles(t *testing.T) {
	pool := testDB(t)
	ctx := context.Background()
	app := New(pool, Config{Origin: origin, DevUserID: player})
	c := &client{t: t, handler: app.Handler()}
	c.json("POST", "/api/me/titles", `{"name":"x",`+c.look("lumen-title-wood", "", "", false)+`}`, 401, nil)
	c.json("GET", "/api/me/titles", "", 401, nil)
	c.signIn()

	// Up to five private titles, looking any way their maker likes: each layer an item of its
	// own slot. The old style alone is not a look.
	for _, body := range []string{
		`{"name":" ",` + c.look("lumen-title-wood", "", "", false) + `}`,
		`{"name":"x"}`,
		// A part is picked by its number: its name picks nothing, and there is no look without a band.
		`{"name":"x","layers":{"band":"band/lumen-title-wood"}}`,
		`{"name":"x","layers":{"ids":{"effect":1}}}`,
		`{"name":"x","layers":{"ids":{"band":0}}}`,
	} {
		c.json("POST", "/api/me/titles", body, 422, nil)
	}
	// A part has to be one there is, of the layer it is picked for, and one cut for a player's
	// plate: the AI plate's bands have another shape.
	wood, gold := c.part("band", "lumen-title-wood"), c.part("band", "lumen-title-gold")
	for _, ids := range []string{
		`{"band":999999}`,
		fmt.Sprintf(`{"band":%d,"effect":%d}`, wood, gold),
		fmt.Sprintf(`{"band":%d,"icon":999999}`, wood),
		fmt.Sprintf(`{"band":%d}`, c.part("icon", "lumen-title-wec")),
		fmt.Sprintf(`{"band":%d}`, c.part("band", "lumen-ai-title-gold")),
		fmt.Sprintf(`{"band":%d,"effect":%d}`, gold, c.part("effect", "lumen-ai-title-gold")),
	} {
		body := `{"name":"x","layers":{"ids":` + ids + `}}`
		var refused struct{ Code string }
		c.json("POST", "/api/me/titles", body, 422, &refused)
		if refused.Code != "TITLE_PART_UNKNOWN" {
			t.Fatal("unknown part", body, refused)
		}
	}
	c.json("POST", "/api/me/titles", `{"name":"x","style":"gold"}`, 400, nil)
	made := []Title{}
	// Whether the effect overflows the band is its maker's to say, and means nothing without one.
	for _, look := range []struct {
		body, style, band, effect, icon string
		overflow                        bool
	}{
		{`` + c.look("lumen-title-rainbow", "", "", true) + ``, "rainbow", "band/lumen-title-rainbow", "", "", false},
		{`` + c.look("lumen-title-toho_y22", "lumen-title-gold", "lumen-title-crown_gold", true) + ``, "wood", "band/lumen-title-toho_y22", "effect/lumen-title-gold", "icon/lumen-title-crown_gold", true},
		{`` + c.look("lumen-title-wood", "", "lumen-title-wec", false) + ``, "wood", "band/lumen-title-wood", "", "icon/lumen-title-wec", false},
		{`` + c.look("lumen-title-gold", "lumen-title-gold", "", false) + ``, "gold", "band/lumen-title-gold", "effect/lumen-title-gold", "", false},
		{`` + c.look("lumen-title-purple", "", "", false) + ``, "purple", "band/lumen-title-purple", "", "", false},
	} {
		var title Title
		c.json("POST", "/api/me/titles", `{"name":"称号","nameTranslations":{"en":"Title"},`+look.body+`}`, 201, &title)
		if title.Visibility != private || title.OwnerID == nil || *title.OwnerID != player || !account.ValidSubject(title.ID) ||
			title.Style != look.style || title.Layers.Band != look.band || str(title.Layers.Effect) != look.effect || str(title.Layers.Icon) != look.icon ||
			title.Layers.EffectOverflow != look.overflow ||
			title.Layers.IDs.Band != c.part("band", strings.TrimPrefix(look.band, "band/")) || (look.effect == "") != (title.Layers.IDs.Effect == nil) || (look.icon == "") != (title.Layers.IDs.Icon == nil) ||
			len(title.Layers.Hashes.Band) != 64 || (look.effect == "") != (title.Layers.Hashes.Effect == nil) || (look.icon == "") != (title.Layers.Hashes.Icon == nil) {
			t.Fatal("made", look.body, title)
		}
		made = append(made, title)
	}
	c.json("POST", "/api/me/titles", `{"name":"六",`+c.look("lumen-title-wood", "", "", false)+`}`, 409, nil)

	// A private title is its maker's alone. They hold it from the start, which is no reward, and
	// wear it; it is not in the catalog, and nobody else can be given it, by a course, a rule or
	// a manager.
	var mine struct {
		Titles      []OwnTitle
		Limit, Used int
	}
	c.json("GET", "/api/me/titles", "", 200, &mine)
	if len(mine.Titles) != 5 || mine.Limit != 5 || mine.Used != 5 || mine.Titles[4].ID != made[0].ID || mine.Titles[0].Holders != 1 || mine.Titles[0].Review != nil {
		t.Fatalf("my titles %+v", mine)
	}
	var held struct{ Titles []HeldTitle }
	c.json("GET", "/api/users/"+player+"/titles", "", 200, &held)
	if len(held.Titles) != 5 || held.Titles[0].Service != "" {
		t.Fatal("a maker holds what they made", held)
	}
	var rewards int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM rewards WHERE user_id=$1`, player).Scan(&rewards); err != nil || rewards != 0 {
		t.Fatal("making a title is no reward", rewards, err)
	}
	var plate Nameplate
	c.json("PUT", "/api/me/nameplate", `{"titleId":"`+made[0].ID+`","showDan":true}`, 200, &plate)
	if plate.Title == nil || plate.Title.ID != made[0].ID {
		t.Fatal("wearing one's own", plate)
	}
	w := c.do("GET", "/api/titles", "", nil)
	var catalog struct{ Titles []ListedTitle }
	if w.Code != 200 || w.Header().Get("Access-Control-Allow-Origin") != "*" || json.Unmarshal(w.Body.Bytes(), &catalog) != nil || len(catalog.Titles) != 0 {
		t.Fatal("catalog", w.Code, w.Body.String())
	}
	c.json("POST", "/api/admin/grants", `{"userId":"`+other+`","titleId":"`+made[2].ID+`"}`, 422, nil)
	c.json("POST", "/api/admin/titles/"+made[2].ID+"/rules", `{"kind":"site_login"}`, 422, nil)
	c.json("PUT", "/api/admin/titles/"+made[2].ID, `{"name":"管理员改",`+c.look("lumen-title-wood", "", "", false)+`}`, 409, nil)

	// The maker changes their private title as they like; another player's is not theirs.
	var changed Title
	c.json("PUT", "/api/me/titles/"+made[0].ID, `{"name":"改名","description":"说明",`+c.look("lumen-title-purple", "lumen-title-purple", "", false)+`}`, 200, &changed)
	if changed.Name != "改名" || changed.Style != "purple" || changed.Description != "说明" || string(changed.NameTranslations) != `{}` || str(changed.Layers.Effect) != "effect/lumen-title-purple" {
		t.Fatal("changed", changed)
	}
	c.json("PUT", "/api/me/titles/"+made[0].ID, `{"name":"改名","layers":{"band":"band/"}}`, 422, nil)
	if _, err := pool.Exec(ctx, `INSERT INTO titles(id,name,style,visibility,owner_id) VALUES('theirs','他人','wood','private',$1)`, other); err != nil {
		t.Fatal(err)
	}
	c.json("PUT", "/api/me/titles/theirs", `{"name":"x",`+c.look("lumen-title-wood", "", "", false)+`}`, 404, nil)
	c.json("DELETE", "/api/me/titles/theirs", "", 404, nil)
	c.json("POST", "/api/me/titles/theirs/reviews", `{"request":"x"}`, 404, nil)

	// Deleting a private title takes it off its maker's nameplate, and frees its place.
	c.json("DELETE", "/api/me/titles/"+made[0].ID, "", 200, nil)
	c.json("DELETE", "/api/me/titles/"+made[0].ID, "", 404, nil)
	c.json("GET", "/api/users/"+player+"/nameplate", "", 200, &plate)
	if plate.Title != nil {
		t.Fatal("still worn", plate)
	}
	c.json("POST", "/api/me/titles", `{"name":"七",`+c.look("lumen-title-wood", "", "", false)+`}`, 201, nil)
	c.json("POST", "/api/me/titles", `{"name":"八",`+c.look("lumen-title-wood", "", "", false)+`}`, 409, nil)
}

// reviewWorld is a site with user groups: boss, who signs in, is a title manager, and so is
// second; maker makes titles. It gives each a session.
func reviewWorld(t *testing.T) (pool *pgxpool.Pool, maker, boss *client) {
	t.Helper()
	pool = testDB(t)
	ctx := context.Background()
	const second = "0000000000000000000000000000cafe"
	if _, err := pool.Exec(ctx, `INSERT INTO users(id,groups,nickname) VALUES($1,0,'Maker'),($2,$4,'Boss'),($3,$4,'Second')`, player, other, second, int64(groups.TitleManager)); err != nil {
		t.Fatal(err)
	}
	// The account center: an access token is its player's ID.
	sso := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			AccessToken string `json:"accessToken"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/internal/v1/web/introspect" || !account.ValidSubject(body.AccessToken) {
			w.WriteHeader(401)
			w.Write([]byte(`{"code":"UNAUTHORIZED","message":"no"}`))
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"user": map[string]any{"id": body.AccessToken, "username": "player", "nickname": "N", "emailVerified": true}, "expiresAt": time.Now().Add(time.Hour)})
	}))
	t.Cleanup(sso.Close)
	accounts, err := account.New(account.Config{Issuer: sso.URL, ServiceID: "net", ServiceKey: strings.Repeat("k", 64), ClientID: "net", ClientSecret: "secret",
		RedirectURL: origin + "/api/auth/sso/callback", EncryptionKey: strings.Repeat("b", 64)})
	if err != nil {
		t.Fatal(err)
	}
	app := New(pool, Config{Origin: origin, SSO: accounts, Groups: true})
	open := func(user string) *client {
		t.Helper()
		c := &client{t: t, handler: app.Handler()}
		cookie, _, err := app.OpenSession(ctx, user, user, "refresh", time.Now().Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		c.cookie = cookie
		var me struct{ CSRFToken string }
		c.json("GET", "/api/me", "", 200, &me)
		c.csrf = me.CSRFToken
		return c
	}
	return pool, open(player), open(other)
}

func TestATitleBecomesPublicByReview(t *testing.T) {
	pool, maker, boss := reviewWorld(t)
	ctx := context.Background()
	notices := func(c *client) (items []TitleNotice, unread int) {
		t.Helper()
		var page struct {
			Items  []TitleNotice
			Unread int
		}
		c.json("GET", "/api/me/title-notices", "", 200, &page)
		return page.Items, page.Unread
	}
	type told struct {
		Review   Review
		Title    Title
		UserID   string
		UserName string
	}
	read := func(n TitleNotice) (data told) {
		t.Helper()
		if err := json.Unmarshal(n.Data, &data); err != nil {
			t.Fatal(err)
		}
		return data
	}

	var title Title
	maker.json("POST", "/api/me/titles", `{"name":"写谱人","nameTranslations":{"en":"Charter"},"description":"说明",`+maker.look("lumen-title-gold", "lumen-title-gold", "", false)+`}`, 201, &title)

	// The maker asks for a review, saying how the title should be obtained. It goes as it is.
	for _, body := range []string{`{}`, `{"request":"  "}`, `{"request":"` + strings.Repeat("长", 501) + `"}`, `{"request":"x","name":"另一个"}`, `{"request":"x",` + maker.look("lumen-title-wood", "", "", false) + `}`} {
		maker.json("POST", "/api/me/titles/"+title.ID+"/reviews", body, 422, nil)
	}
	var asked Review
	maker.json("POST", "/api/me/titles/"+title.ID+"/reviews", `{"request":" 上传 10 张谱面的人 "}`, 201, &asked)
	if asked.Kind != "publish" || asked.Status != "pending" || asked.Request != "上传 10 张谱面的人" || asked.Name != "写谱人" || string(asked.NameTranslations) != `{"en":"Charter"}` ||
		asked.Layers.Band != "band/lumen-title-gold" || asked.DecidedAt != nil || asked.TitleID != title.ID || asked.UserID != player {
		t.Fatalf("asked %+v", asked)
	}
	maker.json("POST", "/api/me/titles/"+title.ID+"/reviews", `{"request":"再来"}`, 409, nil)
	// While it waits the title stays as it was asked to be reviewed.
	maker.json("PUT", "/api/me/titles/"+title.ID, `{"name":"偷改",`+maker.look("lumen-title-wood", "", "", false)+`}`, 409, nil)
	var mine struct {
		Titles []OwnTitle
		Used   int
	}
	maker.json("GET", "/api/me/titles", "", 200, &mine)
	if len(mine.Titles) != 1 || !mine.Titles[0].Pending || mine.Titles[0].Review == nil || mine.Titles[0].Review.ID != asked.ID || mine.Used != 1 {
		t.Fatalf("my titles while waiting %+v", mine)
	}

	// Every title manager is told, and sees the queue; a player does neither.
	told1, unread := notices(boss)
	if len(told1) != 1 || unread != 1 || told1[0].Kind != "review_requested" || told1[0].Read {
		t.Fatalf("the managers are told %+v", told1)
	}
	if data := read(told1[0]); data.Review.ID != asked.ID || data.Title.ID != title.ID || data.UserID != player || data.UserName != "N" {
		t.Fatalf("what they are told %+v", data)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM title_notices WHERE kind='review_requested'`).Scan(&count); err != nil || count != 2 {
		t.Fatal("one notice a manager", count, err)
	}
	if items, _ := notices(maker); len(items) != 0 {
		t.Fatal("the maker is not told of their own request", items)
	}
	var unreadCount struct{ Count int }
	boss.json("GET", "/api/me/title-notices/unread", "", 200, &unreadCount)
	boss.json("POST", "/api/me/title-notices/read", "", 200, nil)
	if _, after := notices(boss); unreadCount.Count != 1 || after != 0 {
		t.Fatal("read", unreadCount, after)
	}
	maker.json("GET", "/api/admin/title-reviews", "", 403, nil)
	maker.json("POST", "/api/admin/title-reviews/"+asked.ID, `{"approve":true}`, 403, nil)
	var queue struct {
		Items []QueuedReview
		Total int
	}
	boss.json("GET", "/api/admin/title-reviews", "", 200, &queue)
	if queue.Total != 1 || len(queue.Items) != 1 || queue.Items[0].ID != asked.ID || queue.Items[0].UserName != "Maker" || queue.Items[0].Title.Visibility != private {
		t.Fatalf("queue %+v", queue)
	}
	boss.json("GET", "/api/admin/title-reviews?status=other", "", 400, nil)

	// Returned with a reply: the title stays private and can be changed and asked for again.
	boss.json("POST", "/api/admin/title-reviews/"+asked.ID, `{"reply":"x"}`, 422, nil)
	boss.json("POST", "/api/admin/title-reviews/nope", `{"approve":false}`, 404, nil)
	var decided struct {
		Review Review
		Title  Title
	}
	boss.json("POST", "/api/admin/title-reviews/"+asked.ID, `{"approve":false,"reply":" 名字太泛了 "}`, 200, &decided)
	if decided.Review.Status != "returned" || decided.Review.Reply != "名字太泛了" || decided.Review.DecidedAt == nil || decided.Title.Visibility != private {
		t.Fatalf("returned %+v", decided)
	}
	boss.json("POST", "/api/admin/title-reviews/"+asked.ID, `{"approve":true}`, 409, nil)
	told2, _ := notices(maker)
	if len(told2) != 1 || told2[0].Kind != "review_returned" || read(told2[0]).Review.Reply != "名字太泛了" {
		t.Fatalf("the maker is told %+v", told2)
	}
	maker.json("PUT", "/api/me/titles/"+title.ID, `{"name":"十谱达人","description":"说明",`+maker.look("lumen-title-gold", "lumen-title-gold", "", false)+`}`, 200, nil)

	// A request taken back is no longer in the queue.
	var again Review
	maker.json("POST", "/api/me/titles/"+title.ID+"/reviews", `{"request":"上传 10 张谱面的人"}`, 201, &again)
	boss.json("DELETE", "/api/me/title-reviews/"+again.ID, "", 404, nil)
	maker.json("DELETE", "/api/me/title-reviews/"+again.ID, "", 200, nil)
	maker.json("DELETE", "/api/me/title-reviews/"+again.ID, "", 404, nil)
	boss.json("POST", "/api/admin/title-reviews/"+again.ID, `{"approve":true}`, 409, nil)
	boss.json("GET", "/api/admin/title-reviews", "", 200, &queue)
	if queue.Total != 0 {
		t.Fatal("a withdrawn request waits no more", queue)
	}

	// Approved: the title is public with the manager's words for how it is obtained, in the
	// catalog, no longer one of its maker's five, and theirs to change only by review.
	maker.json("POST", "/api/me/titles/"+title.ID+"/reviews", `{"request":"上传 10 张谱面的人"}`, 201, &again)
	boss.json("POST", "/api/admin/title-reviews/"+again.ID, `{"approve":true,"reply":"欢迎","obtain":"在自制曲源上传 10 张谱面"}`, 200, &decided)
	if decided.Review.Status != "approved" || decided.Title.Visibility != public || decided.Title.Obtain != "在自制曲源上传 10 张谱面" || decided.Title.Name != "十谱达人" || *decided.Title.OwnerID != player {
		t.Fatalf("approved %+v", decided)
	}
	var catalog struct{ Titles []ListedTitle }
	maker.json("GET", "/api/titles", "", 200, &catalog)
	if len(catalog.Titles) != 1 || catalog.Titles[0].ID != title.ID || catalog.Titles[0].OwnerName != "Maker" || catalog.Titles[0].Holders != 1 || catalog.Titles[0].Obtain == "" {
		t.Fatalf("catalog %+v", catalog)
	}
	maker.json("GET", "/api/me/titles", "", 200, &mine)
	if mine.Used != 0 || mine.Titles[0].Pending || mine.Titles[0].Review.Status != "approved" || mine.Titles[0].Review.Reply != "欢迎" {
		t.Fatalf("my titles after %+v", mine)
	}
	if told3, _ := notices(maker); len(told3) != 2 || told3[0].Kind != "review_approved" || read(told3[0]).Title.Visibility != public {
		t.Fatalf("told of the approval %+v", told3)
	}
	maker.json("PUT", "/api/me/titles/"+title.ID, `{"name":"偷改",`+maker.look("lumen-title-wood", "", "", false)+`}`, 409, nil)
	maker.json("DELETE", "/api/me/titles/"+title.ID, "", 403, nil)
	boss.json("GET", "/api/admin/title-reviews?status=decided", "", 200, &queue)
	if queue.Total != 3 || queue.Items[0].ID != again.ID || queue.Items[0].ReviewerName != "Boss" {
		t.Fatalf("decided %+v", queue)
	}

	// Now the managers give it: by hand, by a rule, by a course they made official.
	boss.json("POST", "/api/admin/grants", `{"userId":"`+other+`","titleId":"`+title.ID+`"}`, 200, nil)
	boss.json("POST", "/api/admin/titles/"+title.ID+"/rules", `{"kind":"site_login"}`, 201, nil)

	// A change to a public title is asked for whole, and the title changes when it is approved.
	maker.json("POST", "/api/me/titles/"+title.ID+"/reviews", `{"request":"换个样子"}`, 422, nil)
	var change Review
	maker.json("POST", "/api/me/titles/"+title.ID+"/reviews", `{"request":"换个样子","name":"百谱达人","nameTranslations":{"en":"Hundred"},"description":"新说明",`+maker.look("lumen-title-rainbow", "lumen-title-purple", "lumen-title-crown_gold", true)+`}`, 201, &change)
	if change.Kind != "change" || change.Name != "百谱达人" || !change.Layers.EffectOverflow {
		t.Fatalf("a change %+v", change)
	}
	maker.json("GET", "/api/titles", "", 200, &catalog)
	if catalog.Titles[0].Name != "十谱达人" || !catalog.Titles[0].Pending {
		t.Fatal("the title waits for the review", catalog)
	}
	boss.json("GET", "/api/admin/title-reviews", "", 200, &queue)
	if queue.Total != 1 || queue.Items[0].Title.Name != "十谱达人" || queue.Items[0].Name != "百谱达人" {
		t.Fatalf("a change against the title as it is %+v", queue)
	}
	boss.json("POST", "/api/admin/title-reviews/"+change.ID, `{"approve":true,"obtain":"在自制曲源上传 100 张谱面"}`, 200, &decided)
	if decided.Title.Name != "百谱达人" || decided.Title.Style != "rainbow" || decided.Title.Layers.Band != "band/lumen-title-rainbow" || str(decided.Title.Layers.Effect) != "effect/lumen-title-purple" || !decided.Title.Layers.EffectOverflow ||
		str(decided.Title.Layers.Icon) != "icon/lumen-title-crown_gold" || decided.Title.Description != "新说明" || decided.Title.Obtain != "在自制曲源上传 100 张谱面" || string(decided.Title.NameTranslations) != `{"en":"Hundred"}` {
		t.Fatalf("changed %+v", decided.Title)
	}

	// A manager changes a public title directly, sees every title, and removes one: its maker is
	// told, and the players who held it hold it no more.
	var direct Title
	boss.json("PUT", "/api/admin/titles/"+title.ID, `{"name":"千谱达人","obtain":"活动发放",`+boss.look("lumen-title-wood", "", "", false)+`}`, 200, &direct)
	if direct.Name != "千谱达人" || direct.Visibility != public || direct.OwnerID == nil || *direct.OwnerID != player {
		t.Fatal("a manager's change keeps the maker", direct)
	}
	var kept Title
	maker.json("POST", "/api/me/titles", `{"name":"自用",`+maker.look("lumen-title-wood", "", "", false)+`}`, 201, &kept)
	var all struct{ Titles []ListedTitle }
	maker.json("GET", "/api/admin/titles", "", 403, nil)
	boss.json("GET", "/api/admin/titles", "", 200, &all)
	if len(all.Titles) != 2 || all.Titles[0].ID != kept.ID || all.Titles[0].Visibility != private || all.Titles[1].Holders != 2 {
		t.Fatalf("every title %+v", all)
	}
	boss.json("DELETE", "/api/admin/titles/"+kept.ID, "", 200, nil)
	boss.json("DELETE", "/api/admin/titles/"+kept.ID, "", 404, nil)
	if told4, _ := notices(maker); told4[0].Kind != "title_removed" || read(told4[0]).Title.Name != "自用" {
		t.Fatalf("told of the removal %+v", told4)
	}
	boss.json("DELETE", "/api/admin/titles/"+title.ID, "", 200, nil)
	var held struct{ Titles []HeldTitle }
	boss.json("GET", "/api/users/"+other+"/titles", "", 200, &held)
	if len(held.Titles) != 0 {
		t.Fatal("still held", held)
	}
}

func TestScoresOfEveryServiceInOneList(t *testing.T) {
	pool := testDB(t)
	ctx := context.Background()
	// Each play on a chart block of its own, in the source's tables.
	play := func(source, id, chart string, score int64, level int, at string, recording, latest bool) {
		t.Helper()
		var replay any
		if recording {
			replay = `{"version":1}`
		}
		_, err := pool.Exec(ctx, fmt.Sprintf(`
WITH player AS (INSERT INTO net.users(id) VALUES ($1) ON CONFLICT DO NOTHING),
 tja AS (INSERT INTO %[1]s.chart_resources(storage_key,original_filename,sha256,byte_size,extension) VALUES ($3||$3||'.tja','a.tja',$3||$3,1,'tja') RETURNING id),
 audio AS (INSERT INTO %[1]s.chart_resources(storage_key,original_filename,sha256,byte_size,extension) VALUES (repeat('a',32)||$3||'.ogg','a.ogg',repeat('a',32)||$3,1,'ogg') RETURNING id),
 chart AS (INSERT INTO %[1]s.charts(id,owner_ids,title,bpm,duration,demo_end,tja_file,audio_file) SELECT $3,ARRAY[$1],'Song '||$2,120,60,10,tja.id,audio.id FROM tja,audio RETURNING id),
 block AS (INSERT INTO %[1]s.chart_difficulties(chart_id,course,side,level,content_hash) SELECT id,'Oni','single',$5,'v2:'||repeat('0',64) FROM chart RETURNING id)
INSERT INTO net.scores(source,id,user_id,good,ok,bad,score,drumroll,max_combo,payload_digest,difficulty_id,clear_status,submitted_at,replay_data,is_latest)
 SELECT $8,$2,$1,1,0,0,$4,0,1,repeat('d',64),block.id,1,$6::timestamptz,$7::jsonb,$9 FROM block`, source),
			player, id, chart, score, level, at, replay, source, latest)
		if err != nil {
			t.Fatal(source, id, err)
		}
	}
	// The player must exist before a chart can name them as its owner.
	if _, err := pool.Exec(ctx, `INSERT INTO net.users(id) VALUES ($1)`, player); err != nil {
		t.Fatal(err)
	}
	// f2 and d1 kept their recordings; f3's is of files replaced since.
	f1, f2, f3, d1 := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32), strings.Repeat("d", 32)
	play("fanmade", "f1", f1, 900000, 8, "2026-01-03T00:00:00Z", false, true)
	play("fanmade", "f2", f2, 700000, 10, "2026-01-01T00:00:00Z", true, true)
	play("fanmade", "f3", f3, 990000, 3, "2025-12-01T00:00:00Z", true, false)
	play("official", "d1", d1, 950000, 9, "2026-01-02T00:00:00Z", true, true)
	sources := []Source{
		{ID: "fanmade", Name: "Fanmade", Site: "https://fanmade.test"},
		{ID: "official", Name: "Database", Site: "https://database.test"},
		// A source this database does not have: reported, and the others still show.
		{ID: "down", Name: "Down", Site: "https://down.test"},
	}
	c := &client{t: t, handler: New(pool, Config{Origin: origin, Sources: sources}).Handler()}
	type page struct {
		Items   []SourceScore
		Total   int
		Sort    string
		Sources []struct {
			ID    string
			Count int
			Error string
		}
	}
	ids := func(p page) string {
		out := []string{}
		for _, item := range p.Items {
			out = append(out, item.Source+":"+item.ID)
		}
		return strings.Join(out, ",")
	}
	var merged page
	c.json("GET", "/api/users/"+player+"/scores", "", 200, &merged)
	if merged.Sort != "level" || merged.Total != 4 || ids(merged) != "fanmade:f2,official:d1,fanmade:f1,fanmade:f3" {
		t.Fatalf("merged by level: %s %+v", ids(merged), merged)
	}
	if merged.Items[1].ChartURL != "https://database.test/charts/"+d1 || merged.Items[0].ChartURL != "https://fanmade.test/charts/"+f2 {
		t.Fatal("chart links", merged.Items[0].ChartURL, merged.Items[1].ChartURL)
	}
	// A play with a recording links to it on its chart's page; one of replaced files does not.
	if merged.Items[0].ReplayURL != "https://fanmade.test/charts/"+f2+"?replay=f2" || merged.Items[1].ReplayURL != "https://database.test/charts/"+d1+"?replay=d1" ||
		merged.Items[2].ReplayURL != "" || merged.Items[3].ReplayURL != "" || !merged.Items[3].HasReplay {
		t.Fatal("replay links", merged.Items[0].ReplayURL, merged.Items[1].ReplayURL, merged.Items[2].ReplayURL, merged.Items[3].ReplayURL)
	}
	if merged.Sources[2].Error == "" || merged.Sources[0].Count != 3 || merged.Sources[1].Count != 1 {
		t.Fatalf("a source that cannot be read is reported %+v", merged.Sources)
	}
	c.json("GET", "/api/users/"+player+"/scores?sort=recent", "", 200, &merged)
	if ids(merged) != "fanmade:f1,official:d1,fanmade:f2,fanmade:f3" {
		t.Fatal("merged by date", ids(merged))
	}
	c.json("GET", "/api/users/"+player+"/scores?source=official&sort=score", "", 200, &merged)
	if ids(merged) != "official:d1" || len(merged.Sources) != 1 {
		t.Fatal("one service", ids(merged))
	}
	// Every play, with a second one on a block: the best view shows one row for it, with the
	// better clear and the count.
	if _, err := pool.Exec(ctx, `INSERT INTO net.scores(source,id,user_id,good,ok,bad,score,drumroll,max_combo,payload_digest,difficulty_id,clear_status,submitted_at)
 SELECT 'official','d2',user_id,1,0,0,100,0,1,repeat('d',64),difficulty_id,3,'2026-01-05' FROM net.scores WHERE id='d1'`); err != nil {
		t.Fatal(err)
	}
	c.json("GET", "/api/users/"+player+"/scores?source=official&view=all", "", 200, &merged)
	if ids(merged) != "official:d2,official:d1" {
		t.Fatal("every play, newest first", ids(merged))
	}
	c.json("GET", "/api/users/"+player+"/scores?source=official", "", 200, &merged)
	if ids(merged) != "official:d1" || merged.Items[0].BestClearStatus == nil || *merged.Items[0].BestClearStatus != 3 || *merged.Items[0].PlayCount != 2 {
		t.Fatalf("best of a block: %s %+v", ids(merged), merged.Items)
	}
	for _, query := range []string{"?view=x", "?sort=x", "?course=Ura", "?source=nope", "?page=0"} {
		c.json("GET", "/api/users/"+player+"/scores"+query, "", 400, nil)
	}
	c.json("GET", "/api/users/not-a-user/scores", "", 404, nil)
}

func TestTheWebsiteIsServedWithItsPageRoutes(t *testing.T) {
	pool := testDB(t)
	site := fstest.MapFS{
		"index.html":         {Data: []byte("<!doctype html><title>Profile</title>")},
		"assets/app-1234.js": {Data: []byte("console.log(1)")},
	}
	c := &client{t: t, handler: New(pool, Config{Origin: origin, Site: site}).Handler()}
	for _, path := range []string{"/", "/users/" + player, "/me"} {
		w := c.do("GET", path, "", nil)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "<title>Profile</title>") || w.Header().Get("Cache-Control") != "no-cache" {
			t.Fatal(path, w.Code, w.Header())
		}
	}
	w := c.do("GET", "/assets/app-1234.js", "", nil)
	if w.Code != 200 || !strings.Contains(w.Header().Get("Cache-Control"), "immutable") {
		t.Fatal("asset", w.Code, w.Header())
	}
	if w = c.do("GET", "/assets/missing.js", "", nil); w.Code != 404 {
		t.Fatal("missing asset", w.Code)
	}
	if w = c.do("GET", "/api/nothing", "", nil); w.Code != 404 || !strings.Contains(w.Body.String(), "NOT_FOUND") {
		t.Fatal("unknown API path", w.Code)
	}
}

// str is an optional layer as written in a test: "" for none.
func str(item *string) string {
	if item == nil {
		return ""
	}
	return *item
}

// The parts titles are made of and the looks to start from are the site's own rows: what the
// arcade's set had when they were first written, and what was made since.
func TestPartsAndPresetsAreListed(t *testing.T) {
	pool := testDB(t)
	c := &client{t: t, handler: New(pool, Config{Origin: origin}).Handler()}
	var listed struct {
		Items   []TitleItem
		Presets []TitlePreset
	}
	c.json("GET", "/api/title/items", "", 200, &listed)
	parts, hashes := map[string]TitleItem{}, map[string]bool{}
	count := map[string]int{}
	for n, item := range listed.Items {
		// Numbered in the order they were added.
		if item.ID != n+1 || item.Name == "" || len(item.Hash) != 64 || string(item.NameTranslations) != `{}` {
			t.Fatal("part", item)
		}
		parts[item.Type+"/"+item.Name], hashes[item.Hash] = item, true
		count[item.Type+" "+item.Kind+" "+item.Source]++
	}
	if len(parts) != len(listed.Items) || len(hashes) != len(listed.Items) {
		t.Fatal("parts repeat", len(listed.Items), len(parts), len(hashes))
	}
	for group, want := range map[string]int{"band player arcade": 43, "effect player arcade": 22, "icon player arcade": 69,
		"band ai arcade": 8, "effect ai arcade": 5, "band player custom": 1, "effect player custom": 1, "icon player custom": 1} {
		if count[group] != want {
			t.Fatal(group, count[group], "of", want, count)
		}
	}
	if first := listed.Items[0]; first.Type != "band" || first.Kind != "player" || first.Source != "arcade" {
		t.Fatal("first part", first)
	}
	// A part stays what it is: its type and its name are what titles are made of it by.
	for _, change := range []string{`type='icon'`, `name='other'`} {
		if _, err := pool.Exec(context.Background(), `UPDATE net.title_items SET `+change+` WHERE id=1`); err == nil {
			t.Fatal("a part changed its", change)
		}
	}
	if _, err := pool.Exec(context.Background(), `UPDATE net.title_items SET hash=repeat('a',64),name_translations='{"en":"Wood"}',name=name WHERE id=1`); err != nil {
		t.Fatal("a part's picture and translations change:", err)
	}
	// Every preset wears parts of its own kind, and the two kinds have the same names where the AI
	// plate has its own cut. The arcade's first, then what was added.
	kinds := map[string][]TitlePreset{}
	for _, p := range listed.Presets {
		kinds[p.Kind] = append(kinds[p.Kind], p)
		for _, id := range []*string{&p.Layers.Band, p.Layers.Effect, p.Layers.Icon} {
			if id != nil && parts[*id].Kind != p.Kind {
				t.Fatal("preset", p, "wears", *id)
			}
		}
		if p.Layers.Hashes.Band != parts[p.Layers.Band].Hash || (p.Layers.Effect != nil && str(p.Layers.Hashes.Effect) != parts[*p.Layers.Effect].Hash) {
			t.Fatal("preset", p, "does not say where its parts are")
		}
		if p.Layers.EffectOverflow && p.Layers.Effect == nil {
			t.Fatal("overflow without an effect", p)
		}
	}
	player, ai := kinds["player"], kinds["ai"]
	if len(player) != 89 || len(ai) != 8 || len(listed.Presets) != 97 || listed.Presets[0].Name != "wood" || listed.Presets[0].ID != 1 || listed.Presets[0].Kind != "player" ||
		player[2].Name != "gold" || !player[2].Layers.EffectOverflow || str(player[2].Layers.Effect) != "effect/lumen-title-gold" ||
		player[88].Name != "jojo_1" || player[88].ID != 89 || player[88].Layers.Band != "band/jojo-dio-mansion" || !player[88].Layers.EffectOverflow ||
		ai[2].Name != "gold" || ai[2].Layers.Band != "band/lumen-ai-title-gold" || ai[7].Name != "ai_4" || ai[7].ID != 97 {
		t.Fatal("presets", len(player), len(ai), listed.Presets[0])
	}
}
