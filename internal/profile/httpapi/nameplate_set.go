package httpapi

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"

	"ourtaiko.dev/net/internal/web"
)

// The plate itself, which no row holds: its layouts (states: what a plate in each state draws,
// in order, and where its text goes), the 段位 emblems, badges and blank looks its pictures have
// clips for, and those pictures (files: "composition/<name>" to SHA-256). It is the arcade
// nameplate's own layout, as OurTaikoPlay's Tools/nameplate-samples/lumen baked it; it changes
// when that is baked anew, which is a change to this file.
//
//go:embed nameplate_layout.json
var nameplateLayoutJSON []byte

type nameplateLayout struct {
	Layout json.RawMessage   `json:"layout"`
	Files  map[string]string `json:"files"`
}

var (
	plateFile = regexp.MustCompile(`^composition/[a-z0-9][a-z0-9_.-]{0,63}$`)
	fileHash  = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// plateLayout is the embedded layout, checked once: a build with a broken one does not start.
var plateLayout = func() nameplateLayout {
	var l nameplateLayout
	if err := json.Unmarshal(nameplateLayoutJSON, &l); err != nil {
		panic("nameplate_layout.json: " + err.Error())
	}
	var states struct {
		States map[string]json.RawMessage `json:"states"`
	}
	if json.Unmarshal(l.Layout, &states) != nil || len(states.States) == 0 || len(l.Files) == 0 {
		panic("nameplate_layout.json: no layouts or no files")
	}
	for id, hash := range l.Files {
		if !plateFile.MatchString(id) || !fileHash.MatchString(hash) {
			panic(fmt.Sprintf("nameplate_layout.json: file %q", id))
		}
	}
	return l
}()

// NameplateSet is everything a plate is drawn from. Files says where each picture is: the
// plate's own ("composition/<name>") and every part a title can be made of ("<type>/<name>"), to
// the SHA-256 that names its file, nameplate/items/<hash>.otanim under the distribution root.
type NameplateSet struct {
	Version int               `json:"version"`
	Layout  json.RawMessage   `json:"layout"`
	Files   map[string]string `json:"files"`
}

// nameplateSet is GET /api/nameplate/set, readable from any website: the plate's layouts and the
// pictures of the plate and of every part, as the site and the game draw nameplates from.
func (s *Server) nameplateSet(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	rows, err := s.DB.Query(r.Context(), `SELECT type||'/'||name,hash FROM title_items`)
	if err != nil {
		internal(w, err)
		return
	}
	defer rows.Close()
	files := map[string]string{}
	for id, hash := range plateLayout.Files {
		files[id] = hash
	}
	for rows.Next() {
		var id, hash string
		if err = rows.Scan(&id, &hash); err != nil {
			internal(w, err)
			return
		}
		files[id] = hash
	}
	if err = rows.Err(); err != nil {
		internal(w, err)
		return
	}
	web.Respond(w, 200, NameplateSet{Version: 1, Layout: plateLayout.Layout, Files: files})
}
