package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
)

type Category struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Genre string `json:"genre"`
	// Published charts in the category; a chart may be in several.
	ChartCount int `json:"chartCount"`
}

// CategoryFlags are persisted in charts.category_flags. Never renumber or reuse a bit; a new
// category also needs the database's CHECK on the column widened. Each source has its own
// categories (Source.Categories).
type CategoryFlags int32

func (src *Source) categoryFlag(id string) (CategoryFlags, bool) {
	for _, category := range src.Categories {
		if category.ID == id {
			return category.Flag, true
		}
	}
	return 0, false
}

func (src *Source) encodeCategories(ids []string) (CategoryFlags, error) {
	var flags CategoryFlags
	for _, id := range ids {
		flag, ok := src.categoryFlag(id)
		if !ok {
			return 0, errors.New("invalid category ID")
		}
		flags |= flag
	}
	return flags, nil
}

// categoryIDs lists the set categories: in the catalog's order, or by ID where the source's
// answers always had them so.
func (src *Source) categoryIDs(flags CategoryFlags) []string {
	ids := []string{}
	for _, category := range src.Categories {
		if flags&category.Flag != 0 {
			ids = append(ids, category.ID)
		}
	}
	if src.CategoryIDsSorted {
		sort.Strings(ids)
	}
	return ids
}

// categories lists the catalog with each category's published charts (a chart may be in
// several) and their total.
func (s *Server) categories(w http.ResponseWriter, r *http.Request) {
	src := s.source()
	counts := make([]int, len(src.Categories))
	rows, err := s.DB.Query(r.Context(), `SELECT c.category_flags,count(*) FROM charts c WHERE `+publishedChart+` GROUP BY 1`)
	if err != nil {
		internal(w, err)
		return
	}
	defer rows.Close()
	total := 0
	for rows.Next() {
		var flags CategoryFlags
		var n int
		if err = rows.Scan(&flags, &n); err != nil {
			internal(w, err)
			return
		}
		total += n
		for i, category := range src.Categories {
			if flags&category.Flag != 0 {
				counts[i] += n
			}
		}
	}
	if err = rows.Err(); err != nil {
		internal(w, err)
		return
	}
	items := make([]Category, 0, len(src.Categories))
	for i, category := range src.Categories {
		category.ChartCount = counts[i]
		items = append(items, category.Category)
	}
	respond(w, 200, map[string]any{"items": items, "chartCount": total})
}

// categorySelection reads a JSON array of category IDs. Omitted, null or empty means
// Variety. The result is in one order whatever order the IDs came in, so a repeated upload
// is recognised as the same.
func (src *Source) categorySelection(raw json.RawMessage) ([]string, error) {
	var ids []string
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &ids); err != nil {
			return nil, err
		}
	}
	if len(ids) == 0 {
		return []string{"variety"}, nil
	}
	sort.Strings(ids)
	unique := ids[:0]
	for _, id := range ids {
		if len(unique) == 0 || unique[len(unique)-1] != id {
			unique = append(unique, id)
		}
	}
	if _, err := src.encodeCategories(unique); err != nil {
		return nil, err
	}
	return unique, nil
}

func (src *Source) categoryByFlag(flag CategoryFlags) Category {
	for _, category := range src.Categories {
		if category.Flag == flag {
			return category.Category
		}
	}
	return Category{}
}

// playlistCategory is the category a system playlist is filed under.
func (src *Source) playlistCategory(id string) Category {
	flag, _ := src.categoryFlag(src.PlaylistCategories[id])
	return src.categoryByFlag(flag)
}
