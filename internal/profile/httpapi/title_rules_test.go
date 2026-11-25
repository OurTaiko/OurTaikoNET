package httpapi

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"ourtaiko.dev/net/internal/account"
)

// The administrators of the titles say what gives a title by itself; everyone can read it.
func TestAdministratorsKeepTheRulesOfTitles(t *testing.T) {
	pool := testDB(t)
	ctx := context.Background()
	app := New(pool, Config{Origin: origin, DevUserID: player, Sources: []Source{{ID: "fanmade", Categories: map[string]int{"game": 1, "pop": 4}}, {ID: "official", Categories: map[string]int{"namco": 1}}}})
	c := &client{t: t, handler: app.Handler()}
	c.json("POST", "/api/admin/titles/first/rules", `{"kind":"site_login"}`, 401, nil)
	c.signIn()
	c.json("PUT", "/api/admin/titles/first", `{"name":"はじめまして","layers":{"band":"band/lumen-title-wood"}}`, 200, nil)
	c.json("POST", "/api/admin/titles/none/rules", `{"kind":"site_login"}`, 404, nil)

	// A rule gives its title at once to those who meet it: the administrator is signed in.
	var made struct {
		Rule    TitleRule
		Granted int
	}
	c.json("POST", "/api/admin/titles/first/rules", `{"kind":"site_login"}`, 201, &made)
	if made.Granted != 1 || made.Rule.ID == 0 || made.Rule.Kind != "site_login" || *made.Rule.TitleID != "first" {
		t.Fatal(made)
	}
	var held struct{ Titles []HeldTitle }
	c.json("GET", "/api/users/"+player+"/titles", "", 200, &held)
	if len(held.Titles) != 1 || held.Titles[0].Title.ID != "first" || held.Titles[0].Service != "rule" {
		t.Fatal(held)
	}
	// What a rule gave is not a 段位 attempt.
	var dans struct{ History []AwardRecord }
	c.json("GET", "/api/users/"+player+"/dans", "", 200, &dans)
	if len(dans.History) != 0 {
		t.Fatal(dans)
	}

	// Each kind has its own fields, and nothing else.
	for _, body := range []string{
		`{"kind":"nothing"}`,
		`{"kind":"site_login","count":2}`,
		`{"kind":"game_login","source":"fanmade"}`,
		`{"kind":"chart_clear","source":"fanmade","clear":1}`,
		`{"kind":"chart_clear","source":"fanmade","chartId":"x","clear":4}`,
		`{"kind":"chart_clear","source":"elsewhere","chartId":"x","clear":1}`,
		`{"kind":"category_clear","source":"fanmade","clear":1}`,
		`{"kind":"category_clear","source":"fanmade","category":"namco","clear":1}`,
		`{"kind":"category_clear","source":"fanmade","category":"game","clear":1,"course":"Ura"}`,
		`{"kind":"category_clear","source":"fanmade","category":"game","clear":1,"count":0}`,
		`{"kind":"charts_uploaded","source":"fanmade"}`,
		`{"kind":"charts_uploaded","source":"fanmade","count":3,"clear":1}`,
	} {
		c.json("POST", "/api/admin/titles/first/rules", body, 422, nil)
	}
	// A chart has to be there, published.
	c.json("POST", "/api/admin/titles/first/rules", `{"kind":"chart_clear","source":"fanmade","chartId":"missing","clear":1}`, 422, nil)
	const chart = "cccccccccccccccccccccccccccccccc"
	if _, err := pool.Exec(ctx, `INSERT INTO fanmade.chart_resources(storage_key,original_filename,sha256,byte_size,extension) VALUES('k1','a.tja',repeat('a',64),1,'tja'),('k2','a.ogg',repeat('b',64),1,'ogg')`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO fanmade.charts(id,owner_ids,title,title_translations,bpm,duration,tja_file,audio_file)
 SELECT $1,ARRAY[$2::text],'さいたま2000','{"en":"Saitama 2000"}',120,10,(SELECT id FROM fanmade.chart_resources WHERE storage_key='k1'),(SELECT id FROM fanmade.chart_resources WHERE storage_key='k2')`, chart, player); err != nil {
		t.Fatal(err)
	}
	c.json("POST", "/api/admin/titles/first/rules", fmt.Sprintf(`{"kind":"chart_clear","source":"fanmade","chartId":%q,"course":"Oni","clear":2}`, chart), 201, &made)
	if made.Granted != 0 || made.Rule.Chart == nil || made.Rule.Chart.Title != "さいたま2000" || made.Rule.Chart.Gone || *made.Rule.Course != "Oni" || *made.Rule.Clear != 2 {
		t.Fatal(made)
	}
	c.json("POST", "/api/admin/titles/first/rules", `{"kind":"category_clear","source":"fanmade","category":"pop","clear":1,"count":10}`, 201, &made)
	if made.Rule.Category == nil || *made.Rule.Category != "pop" || *made.Rule.Count != 10 {
		t.Fatal(made)
	}
	// The player has one chart there already.
	c.json("POST", "/api/admin/titles/first/rules", `{"kind":"charts_uploaded","source":"fanmade","count":1}`, 201, &made)
	if made.Granted != 0 {
		t.Fatal("the title was held already", made)
	}
	last := made.Rule.ID

	// A private title, its maker's alone, takes no rules.
	var own Title
	c.json("POST", "/api/me/titles", `{"name":"わたしの","layers":{"band":"band/lumen-title-wood"}}`, 201, &own)
	c.json("POST", "/api/admin/titles/"+own.ID+"/rules", `{"kind":"site_login"}`, 422, nil)

	// Anyone reads the rules with the catalog.
	guest := &client{t: t, handler: app.Handler()}
	var catalog struct {
		Titles []Title
		Rules  []TitleRule
	}
	guest.json("GET", "/api/titles", "", 200, &catalog)
	if len(catalog.Rules) != 4 || catalog.Rules[1].Chart.ID != chart || !strings.Contains(string(catalog.Rules[1].Chart.TitleTranslations), "Saitama 2000") {
		t.Fatal(catalog.Rules)
	}

	// Removing a rule leaves what it gave; removing the title removes its rules.
	guest.json("DELETE", fmt.Sprintf("/api/admin/rules/%d", last), "", 401, nil)
	c.json("DELETE", fmt.Sprintf("/api/admin/rules/%d", last), "", 200, nil)
	c.json("DELETE", fmt.Sprintf("/api/admin/rules/%d", last), "", 404, nil)
	c.json("DELETE", "/api/admin/rules/x", "", 404, nil)
	c.json("GET", "/api/users/"+player+"/titles", "", 200, &held)
	if len(held.Titles) != 2 || held.Titles[0].Title.ID != own.ID || held.Titles[1].Title.ID != "first" {
		t.Fatal(held)
	}
	c.json("DELETE", "/api/admin/titles/first", "", 200, nil)
	guest.json("GET", "/api/titles", "", 200, &catalog)
	if len(catalog.Rules) != 0 {
		t.Fatal(catalog.Rules)
	}
}

// The built-in account owns the official charts but has uploaded nothing: no rule gives it a title.
func TestTheBuiltInAccountEarnsNoUploadTitles(t *testing.T) {
	pool := testDB(t)
	ctx := context.Background()
	app := New(pool, Config{Origin: origin, DevUserID: player, Sources: []Source{{ID: "official", Categories: map[string]int{"namco": 1}}}})
	c := &client{t: t, handler: app.Handler()}
	c.signIn()
	if _, err := pool.Exec(ctx, `INSERT INTO official.chart_resources(storage_key,original_filename,sha256,byte_size,extension) VALUES(repeat('a',64)||'.tja','a.tja',repeat('a',64),1,'tja'),(repeat('b',64)||'.ogg','a.ogg',repeat('b',64),1,'ogg')`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO official.charts(id,owner_ids,title,bpm,duration,demo_end,tja_file,audio_file,category_flags)
 SELECT repeat('c',32),ARRAY[$1::text],'Song',120,10,10,(SELECT id FROM official.chart_resources WHERE extension='tja'),(SELECT id FROM official.chart_resources WHERE extension='ogg'),1`, account.SystemUserID); err != nil {
		t.Fatal(err)
	}
	c.json("PUT", "/api/admin/titles/first", `{"name":"はじめまして","layers":{"band":"band/lumen-title-wood"}}`, 200, nil)
	var made struct{ Granted int }
	c.json("POST", "/api/admin/titles/first/rules", `{"kind":"charts_uploaded","source":"official","count":1}`, 201, &made)
	if made.Granted != 0 {
		t.Fatal(made)
	}
}
