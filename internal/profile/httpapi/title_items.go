package httpapi

import (
	"encoding/json"
	"net/http"

	"ourtaiko.dev/net/internal/web"
)

// TitleItem is a part titles are made of (title_items): a band, an effect over it or an icon on
// its end. ID is its number; a title's layers name it by its type and name ("<type>/<name>"),
// which never change. Kind says whose plate it is cut
// for: a title may be made only of the player's; the AI plate's have another shape. Source is
// where it comes from: the arcade's set, or made for this game (custom). Hash is its file's
// SHA-256: the file is nameplate/items/<hash>.otanim under the distribution root.
type TitleItem struct {
	ID               int             `json:"id"`
	Type             string          `json:"type"`
	Name             string          `json:"name"`
	Kind             string          `json:"kind"`
	Source           string          `json:"source"`
	NameTranslations json.RawMessage `json:"nameTranslations"`
	Hash             string          `json:"hash"`
}

// TitlePreset is a look a title's maker starts from (title_presets), under its name. Kind as a
// part's: the player's presets, and the AI plate's own of the same names.
type TitlePreset struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Layers Layers `json:"layers"`
}

// titleItems is GET /api/title/items: every part and every preset, readable from any website,
// each in the order they were added.
func (s *Server) titleItems(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	ctx := r.Context()
	rows, err := s.DB.Query(ctx, `SELECT id,type,name,kind,source,name_translations,hash FROM title_items ORDER BY id`)
	if err != nil {
		internal(w, err)
		return
	}
	defer rows.Close()
	items := []TitleItem{}
	for rows.Next() {
		var item TitleItem
		var translations []byte
		if err = rows.Scan(&item.ID, &item.Type, &item.Name, &item.Kind, &item.Source, &translations, &item.Hash); err != nil {
			internal(w, err)
			return
		}
		item.NameTranslations = translations
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		internal(w, err)
		return
	}
	rows, err = s.DB.Query(ctx, `SELECT p.id,p.name,p.kind,`+layerColumns("p")+` FROM title_presets p`+layerJoins("p")+` ORDER BY p.id`)
	if err != nil {
		internal(w, err)
		return
	}
	defer rows.Close()
	presets := []TitlePreset{}
	for rows.Next() {
		var p TitlePreset
		if err = rows.Scan(append([]any{&p.ID, &p.Name, &p.Kind}, p.Layers.into()...)...); err != nil {
			internal(w, err)
			return
		}
		presets = append(presets, p)
	}
	if err = rows.Err(); err != nil {
		internal(w, err)
		return
	}
	web.Respond(w, 200, map[string]any{"items": items, "presets": presets})
}
