package httpapi

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestTheAdministratorAnnounces(t *testing.T) {
	pool := testDB(t)
	c := &client{t: t, handler: New(pool, Config{Origin: origin, DevUserID: player}).Handler()}

	var page AnnouncementPage
	// The titles shown, in order.
	read := func(query string, want ...string) {
		t.Helper()
		page = AnnouncementPage{}
		c.json("GET", "/api/announcements"+query, "", 200, &page)
		titles := []string{}
		for _, item := range page.Items {
			titles = append(titles, item.Title)
		}
		if page.Items == nil || fmt.Sprint(titles) != fmt.Sprint(want) {
			t.Fatalf("announced %v, want %v", titles, want)
		}
	}
	announce := func(title, body string, pinned bool) Announcement {
		t.Helper()
		var made Announcement
		c.json("POST", "/api/admin/announcements", fmt.Sprintf(`{"title":%q,"body":%q,"pinned":%t}`, title, body, pinned), 201, &made)
		return made
	}

	// Anyone reads them; only the administrator writes them.
	read("")
	c.json("POST", "/api/admin/announcements", `{"title":"Closed"}`, 401, nil)
	c.signIn()

	// What is written is trimmed and its lines end one way; the newest is first.
	first := announce("  Maintenance  ", "  Tonight.\r\nBack at nine.\r  ", false)
	if first.Title != "Maintenance" || first.Body != "Tonight.\nBack at nine." || first.Pinned || first.PublishedAt.IsZero() {
		t.Fatalf("announced %+v", first)
	}
	second := announce("新歌", "", false)
	// Announced a moment apart, whatever the clock's grain.
	if _, err := pool.Exec(context.Background(), `UPDATE net.announcements SET published_at=published_at - interval '1 hour' WHERE id=$1`, first.ID); err != nil {
		t.Fatal(err)
	}
	read("", "新歌", "Maintenance")
	if page.Total != 2 || page.Page != 1 || page.PageSize != defaultAnnouncementPageSize {
		t.Fatalf("page %+v", page)
	}

	// A pinned one is before the others however old; editing does not move it in time.
	var saved Announcement
	c.json("PUT", "/api/admin/announcements/"+first.ID, `{"title":"Maintenance tonight","body":"","pinned":true}`, 200, &saved)
	if saved.Title != "Maintenance tonight" || saved.Body != "" || !saved.Pinned || saved.ID != first.ID || !saved.PublishedAt.Before(second.PublishedAt) {
		t.Fatalf("saved %+v", saved)
	}
	read("", "Maintenance tonight", "新歌")

	// A page at a time; one past the end is empty and still counts them.
	read("?pageSize=1", "Maintenance tonight")
	read("?pageSize=1&page=2", "新歌")
	read("?pageSize=1&page=3")
	if page.Total != 2 {
		t.Fatalf("total %d past the end", page.Total)
	}
	for _, query := range []string{"?pageSize=0", "?pageSize=51", "?pageSize=x", "?page=0"} {
		c.json("GET", "/api/announcements"+query, "", 400, nil)
	}

	// What cannot be kept changes nothing: no title, one too long or of several lines, a text
	// too long or with control characters.
	for name, body := range map[string]string{
		"empty": `{"title":"  ","body":"x"}`,
		"long":  fmt.Sprintf(`{"title":%q}`, strings.Repeat("长", maxAnnouncementTitle+1)),
		"lines": `{"title":"one\ntwo"}`,
		"text":  fmt.Sprintf(`{"title":"t","body":%q}`, strings.Repeat("长", maxAnnouncementBody+1)),
		"bell":  `{"title":"t","body":"a\u0007b"}`,
	} {
		for _, request := range [][2]string{{"POST", "/api/admin/announcements"}, {"PUT", "/api/admin/announcements/" + first.ID}} {
			w := c.do(request[0], request[1], body, nil)
			if w.Code != 422 || !strings.Contains(w.Body.String(), `"ANNOUNCEMENT_INVALID"`) {
				t.Fatalf("%s %s: %d %s", name, request[0], w.Code, w.Body.String())
			}
		}
	}
	read("", "Maintenance tonight", "新歌")
	// Exactly the limits are kept, counted in characters.
	limit := announce(strings.Repeat("长", maxAnnouncementTitle), strings.Repeat("文", maxAnnouncementBody), false)

	// One that is none cannot be saved or taken down; one taken down is gone.
	for _, id := range []string{strings.Repeat("0", 32), "none"} {
		c.json("PUT", "/api/admin/announcements/"+id, `{"title":"t"}`, 404, nil)
		c.json("DELETE", "/api/admin/announcements/"+id, "", 404, nil)
	}
	c.json("DELETE", "/api/admin/announcements/"+limit.ID, "", 204, nil)
	c.json("DELETE", "/api/admin/announcements/"+second.ID, "", 204, nil)
	read("", "Maintenance tonight")
}
