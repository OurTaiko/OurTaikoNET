package httpapi

import (
	"context"
	"strconv"
	"testing"
)

// The chart list is the same code on both sources, so this one takes every filter and order
// the official source's list does; a page here has this source's own number of charts.
func TestChartListFiltersAndOrders(t *testing.T) {
	c, s := interactionFixture(t)
	ctx := context.Background()
	if _, err := s.DB.Exec(ctx, `UPDATE charts SET bpm=200 WHERE id='song'`); err != nil {
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
		if code := c.do("", "GET", "/api/v1/charts"+query, "", &p); code != 200 {
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
	if p := list(""); p.PageSize != pageSize || pageSize != 12 || p.Total != 2 {
		t.Fatalf("page %+v", p)
	}
	for query, want := range map[string]string{
		"": "other", "?order=title": "other", "?order=bpm": "song", "?order=updated": "song",
		"?order=level": "other", "?order=nijiiro": "other", "?order=top": "other",
	} {
		if got := first(query); got != want {
			t.Fatal(query, "starts with", got)
		}
	}
	for query, want := range map[string]int{
		"?files=yes": 2, "?files=no": 0, "?nijiiro=no": 2, "?nijiiro=yes": 0, "?q=%20Song%20": 1, "?q=nothing": 0,
	} {
		if p := list(query); p.Total != want || len(p.Items) != want {
			t.Fatal(query, p.Total, len(p.Items))
		}
	}
	// A number as keyword also finds the chart with that number.
	var song Chart
	c.do("", "GET", "/api/v1/charts/song?by=id", "", &song)
	if p := list("?q=" + strconv.FormatInt(song.NumberID, 10)); p.Total != 1 || p.Items[0].ID != "song" {
		t.Fatal("keyword number", p.Total)
	}
	for _, query := range []string{"?order=newest", "?files=maybe", "?nijiiro=1", "?level=11", "?course=Ura", "?category=missing", "?page=10001"} {
		if code := c.do("", "GET", "/api/v1/charts"+query, "", nil); code != 400 {
			t.Fatal(query, code)
		}
	}
}
