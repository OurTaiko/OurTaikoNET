package httpapi

import (
	"context"
	"strconv"
	"testing"
)

// The chart list is the same code on both sources, so this one takes every filter and order
// the official source's list does.
func TestChartListFiltersAndOrders(t *testing.T) {
	world := interactionWorldOf(t, Fanmade)
	c, s := world.c, world.s
	ctx := context.Background()
	// Other is the one with a block and a Nijiiro ID, so those orders do not fall to the IDs.
	world.slot(t, world.other, "Oni")
	if _, err := s.DB.Exec(ctx, `UPDATE charts SET nijiiro_id=7 WHERE id=$1`, world.other); err != nil {
		t.Fatal(err)
	}
	// Song is the faster one, and by this the one changed last.
	if _, err := s.DB.Exec(ctx, `UPDATE charts SET bpm=200 WHERE id='`+world.song+`'`); err != nil {
		t.Fatal(err)
	}
	type page struct {
		Items    []Chart `json:"items"`
		Total    int     `json:"total"`
		PageSize int     `json:"pageSize"`
	}
	list := func(query string) page {
		t.Helper()
		var p page
		if code := c.do("", "GET", "/api/charts"+query, "", &p); code != 200 {
			t.Fatal(query, code)
		}
		return p
	}
	first := func(query string) string {
		t.Helper()
		p := list(query)
		if len(p.Items) != 2 {
			t.Fatal(query, len(p.Items))
		}
		return p.Items[0].ID
	}
	if p := list(""); p.PageSize != 24 || p.Total != 2 {
		t.Fatalf("page %+v", p)
	}
	for query, want := range map[string]string{
		"": world.other, "?order=title": world.other, "?order=bpm": world.song, "?order=updated": world.song,
		"?order=level": world.other, "?order=nijiiro": world.other, "?order=top": world.other,
	} {
		if got := first(query); got != want {
			t.Fatal(query, "starts with", got)
		}
	}
	for query, want := range map[string]int{
		"?files=yes": 2, "?files=no": 0, "?nijiiro=no": 1, "?nijiiro=yes": 1, "?q=%20Song%20": 1, "?q=nothing": 0,
	} {
		if p := list(query); p.Total != want || len(p.Items) != want {
			t.Fatal(query, p.Total, len(p.Items))
		}
	}
	// A number as keyword also finds the chart with that number.
	var song Chart
	c.do("", "GET", "/api/charts/"+world.song+"?by=id", "", &song)
	if p := list("?q=" + strconv.FormatInt(song.NumberID, 10)); p.Total != 1 || p.Items[0].ID != world.song {
		t.Fatal("keyword number", p.Total)
	}
	// The client can say how many charts a page has; the pages then follow that number.
	one, two := list("?pageSize=1"), list("?pageSize=1&page=2")
	if one.PageSize != 1 || one.Total != 2 || len(one.Items) != 1 || len(two.Items) != 1 || one.Items[0].ID == two.Items[0].ID {
		t.Fatalf("pages of one: %+v, %+v", one, two)
	}
	if p := list("?pageSize=100"); p.PageSize != 100 || len(p.Items) != 2 {
		t.Fatalf("page of the most %+v", p)
	}
	for _, query := range []string{"?pageSize=0", "?pageSize=-1", "?pageSize=101", "?pageSize=many", "?order=newest", "?files=maybe", "?nijiiro=1", "?level=11", "?course=Ura", "?category=missing", "?page=10001"} {
		if code := c.do("", "GET", "/api/charts"+query, "", nil); code != 400 {
			t.Fatal(query, code)
		}
	}
}
