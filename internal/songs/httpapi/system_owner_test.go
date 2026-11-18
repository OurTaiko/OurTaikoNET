package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"ourtaiko.dev/net/db"
	"ourtaiko.dev/net/internal/account"
	"ourtaiko.dev/net/internal/web"
)

// registerSong sends the official source's form for a song without files, as admin.
func registerSong(world interactionWorld, title, collaborators string) *httptest.ResponseRecorder {
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	_ = form.WriteField("title", title)
	if collaborators != "" {
		_ = form.WriteField("collaboratorIds", collaborators)
	}
	_ = form.Close()
	r := httptest.NewRequest("POST", "/api/charts", &body)
	r.Header.Set("Content-Type", form.FormDataContentType())
	r.Header.Set("Origin", "http://localhost")
	r.Header.Set("X-CSRF-Token", "csrf")
	r.Header.Set("Idempotency-Key", web.NewID())
	r.AddCookie(&http.Cookie{Name: world.c.cookie, Value: world.c.tokens["admin"]})
	w := httptest.NewRecorder()
	world.c.handler.ServeHTTP(w, r)
	return w
}

// A song added to the official source belongs to the built-in account, which shows no name,
// hears of nothing and is not a player.
func TestOfficialChartsBelongToTheBuiltInAccount(t *testing.T) {
	world := interactionWorldOf(t, Official)
	ctx := context.Background()
	alice := testUsers["alice"].ID

	if w := registerSong(world, "Together", `["`+alice+`"]`); w.Code != 409 || !strings.Contains(w.Body.String(), "OFFICIAL_CHART") {
		t.Fatal(w.Code, w.Body.String())
	}
	w := registerSong(world, "Song only", "")
	var song Chart
	if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &song) != nil {
		t.Fatal(w.Code, w.Body.String())
	}
	if !slices.Equal(song.OwnerIDs, []string{account.SystemUserID}) || song.OwnerID != account.SystemUserID || song.Author != "" {
		t.Fatalf("%+v %q %q", song.OwnerIDs, song.OwnerID, song.Author)
	}
	if len(song.Owners) != 1 || song.Owners[0].Nickname != "" {
		t.Fatalf("%+v", song.Owners)
	}

	// Nobody can be invited to it: it has no owner to send the invitation.
	if code := world.c.do("admin", "POST", world.chart(song.ID)+"/collaborations", `{"userId":"`+alice+`"}`, nil); code != 403 {
		t.Fatal(code)
	}

	if err := notify(ctx, world.s.DB, Notice{Recipient: account.SystemUserID, Kind: "chart_comment", ChartID: song.ID}); err != nil {
		t.Fatal(err)
	}
	var notices int
	if err := world.s.DB.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE recipient_id=$1`, account.SystemUserID).Scan(&notices); err != nil || notices != 0 {
		t.Fatal(notices, err)
	}

	if code := world.c.do("alice", "GET", "/api/users/"+account.SystemUserID, "", nil); code != 404 {
		t.Fatal(code)
	}
	var users UserDirectory
	if code := world.c.do("alice", "GET", "/api/users", "", &users); code != 200 {
		t.Fatal(code)
	}
	if slices.ContainsFunc(users.Items, func(u PublicUser) bool { return u.ID == account.SystemUserID }) {
		t.Fatal("the built-in account is listed as a player")
	}
}

// Nobody can be asked to co-create a chart with the built-in account.
func TestTheBuiltInAccountCannotBeInvited(t *testing.T) {
	world := interactionWorldOf(t, Fanmade)
	if code := world.c.do("owner", "POST", world.chart(world.song)+"/collaborations", `{"userId":"`+account.SystemUserID+`"}`, nil); code != 400 {
		t.Fatal(code)
	}
}

// The migration moves every official chart to the built-in account, withdraws open invitations
// and leaves the chart's update_at alone.
func TestMigrationGivesOfficialChartsToTheBuiltInAccount(t *testing.T) {
	world := interactionWorldOf(t, Official)
	ctx := context.Background()
	pool := world.s.DB
	owner, alice := testUsers["owner"].ID, testUsers["alice"].ID
	if _, err := pool.Exec(ctx, `UPDATE charts SET owner_ids=ARRAY[$1::text,$2::text] WHERE id=$3`, owner, alice, world.song); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO chart_collaborations(id,chart_id,inviter_id,recipient_id) VALUES('open',$1,$2,$3)`, world.other, owner, alice); err != nil {
		t.Fatal(err)
	}
	var before time.Time
	if err := pool.QueryRow(ctx, `SELECT update_at FROM charts WHERE id=$1`, world.song).Scan(&before); err != nil {
		t.Fatal(err)
	}

	migrations, err := db.Migrations()
	if err != nil {
		t.Fatal(err)
	}
	found := slices.IndexFunc(migrations, func(m db.Migration) bool { return m.Name == "official_chart_owner" })
	if found < 0 {
		t.Fatal("migration not found")
	}
	if _, err = pool.Exec(ctx, migrations[found].SQL); err != nil {
		t.Fatal(err)
	}

	var owners []string
	var after time.Time
	if err = pool.QueryRow(ctx, `SELECT owner_ids,update_at FROM charts WHERE id=$1`, world.song).Scan(&owners, &after); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(owners, []string{account.SystemUserID}) || !after.Equal(before) {
		t.Fatal(owners, before, after)
	}
	var status string
	if err = pool.QueryRow(ctx, `SELECT status FROM chart_collaborations WHERE id='open'`).Scan(&status); err != nil || status != "revoked" {
		t.Fatal(status, err)
	}
	// The trigger is back on for every later change.
	if _, err = pool.Exec(ctx, `UPDATE charts SET description='Changed' WHERE id=$1`, world.song); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT update_at FROM charts WHERE id=$1`, world.song).Scan(&after); err != nil || !after.After(before) {
		t.Fatal(before, after, err)
	}
}
