package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"ourtaiko.dev/net/internal/files"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"ourtaiko.dev/net/internal/account"
	"ourtaiko.dev/net/internal/web"
)

// Title is one title of the catalog. Layers is how a nameplate wearing it looks; Style is the band
// of the six that game versions released before the layers draw (legacyStyle). OwnerID is the
// player who made it (null for the titles administrators keep).
type Title struct {
	ID               string          `json:"id"`
	Name             string          `json:"name"`
	NameTranslations json.RawMessage `json:"nameTranslations"`
	Description      string          `json:"description"`
	Style            string          `json:"style"`
	Layers           Layers          `json:"layers"`
	Official         bool            `json:"official"`
	OwnerID          *string         `json:"ownerId"`
}

// Layers are the band, the effect over it and the icon on its end that a nameplate wears for a
// title (OurTaikoPlay's NameplateLayers): items of the public nameplate catalog
// (nameplate/lumen.json), "<slot>/<name>". A title has a band; Effect and Icon may be none.
type Layers struct {
	Band   string  `json:"band"`
	Effect *string `json:"effect"`
	Icon   *string `json:"icon"`
}

// DanBadge is a player's best result of one 段位 (五級 6 .. 24 達人; 0–5 are not offered). The
// emblem has two layers, like a song's crown: Gold (verdict 4–6) colours the text, Combo picks
// the ring: 0 銀粋, 1 金粋 (full combo), 2 虹粋 (all 良).
type DanBadge struct {
	Index      int       `json:"index"`
	Verdict    int       `json:"verdict"`
	Gold       bool      `json:"gold"`
	Combo      int       `json:"combo"`
	AchievedAt time.Time `json:"achievedAt"`
}

// The first 段位 the game offers (五級); lower indexes exist in the numbering only.
const danFirst = 6

func (d *DanBadge) tiers() {
	d.Gold = d.Verdict >= 4
	if d.Verdict >= 1 && d.Verdict <= 6 {
		d.Combo = (d.Verdict - 1) % 3
	}
}

// Nameplate is what a player wears, as the game and the websites draw it.
type Nameplate struct {
	UserID     string    `json:"userId"`
	Nickname   string    `json:"nickname"`
	AvatarPath string    `json:"avatarPath"`
	Title      *Title    `json:"title"`
	Dan        *DanBadge `json:"dan"`
	ShowDan    bool      `json:"showDan"`
}

var titleID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// An item of the nameplate catalog, as the database allows it in each slot.
var layerItem = map[string]*regexp.Regexp{}

func init() {
	for _, slot := range []string{"band", "effect", "icon"} {
		layerItem[slot] = regexp.MustCompile(`^` + slot + `/[a-z0-9][a-z0-9_.-]{0,63}$`)
	}
}

// legacyStyle is the band of the six that game versions released before the layers know (wood,
// teal, wood2, purple, gold, rainbow) nearest a title's own: what such a game draws for it.
func legacyStyle(band string) string {
	for _, style := range []string{"gold", "purple", "rainbow"} {
		if band == "band/lumen-title-"+style {
			return style
		}
	}
	return "wood"
}

// Players make community titles for their own 段位 courses: at most five each, looking any way
// they like.
const playerTitleLimit = 5

const titleColumns = `t.id,t.name,t.name_translations,t.description,t.style,t.band_item,t.effect_item,t.icon_item,t.official,t.owner_id`

type rowScanner interface{ Scan(...any) error }

func scanTitle(row rowScanner, extra ...any) (Title, error) {
	var t Title
	var translations []byte
	err := row.Scan(append([]any{&t.ID, &t.Name, &translations, &t.Description, &t.Style, &t.Layers.Band, &t.Layers.Effect, &t.Layers.Icon, &t.Official, &t.OwnerID}, extra...)...)
	t.NameTranslations = translations
	return t, err
}

type queryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

// profile looks a player up at the account center: nickname and avatar, and whether the
// account center answered. Without one, only the development user has a name.
func (s *Server) profile(ctx context.Context, id string) (account.PublicProfile, bool, bool) {
	if s.Config.SSO == nil {
		if id == s.Config.DevUserID && id != "" {
			return account.PublicProfile{Nickname: devUser(id).Nickname}, true, true
		}
		return account.PublicProfile{}, false, true
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	found, err := s.Config.SSO.Profiles(ctx, []string{id})
	if err != nil {
		return account.PublicProfile{}, false, false
	}
	p, ok := found[id]
	return p, ok, true
}

// nameplate reads what a player wears: the chosen title while they still hold it, and the
// highest 段位 unless they hide it.
func (s *Server) nameplate(ctx context.Context, db queryer, id string) (Nameplate, error) {
	plate := Nameplate{UserID: id, ShowDan: true}
	var titleID *string
	err := db.QueryRow(ctx, `SELECT title_id,show_dan FROM nameplates WHERE user_id=$1`, id).Scan(&titleID, &plate.ShowDan)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return plate, err
	}
	if titleID != nil {
		t, err := scanTitle(db.QueryRow(ctx, `SELECT `+titleColumns+` FROM titles t JOIN user_titles u ON u.title_id=t.id AND u.user_id=$2 WHERE t.id=$1`, *titleID, id))
		if err == nil {
			plate.Title = &t
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return plate, err
		}
	}
	if plate.ShowDan {
		var d DanBadge
		err = db.QueryRow(ctx, `SELECT dan,verdict,achieved_at FROM user_dans WHERE user_id=$1 ORDER BY dan DESC LIMIT 1`, id).Scan(&d.Index, &d.Verdict, &d.AchievedAt)
		if err == nil {
			d.tiers()
			plate.Dan = &d
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return plate, err
		}
	}
	return plate, nil
}

// userNameplate is GET /api/users/{id}/nameplate, public: the game reads it for each
// signed-in seat. Nickname and avatar are empty while the account center does not answer.
func (s *Server) userNameplate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	id := r.PathValue("id")
	if !account.ValidSubject(id) {
		problem(w, 404, "USER_NOT_FOUND", "用户不存在")
		return
	}
	plate, err := s.nameplate(r.Context(), s.DB, id)
	if err != nil {
		internal(w, err)
		return
	}
	p, _, _ := s.profile(r.Context(), id)
	plate.Nickname, plate.AvatarPath = p.Nickname, files.Path(p.AvatarURL)
	web.Respond(w, 200, plate)
}

// nameplates is GET /api/nameplates?ids=a,b,…: up to 100 players' titles and 段位 at once,
// for the other OurTaiko websites to draw next to the names they show (their own nicknames; this
// asks the account center nothing). Readable from any website.
func (s *Server) nameplates(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	ids := strings.Split(r.URL.Query().Get("ids"), ",")
	seen := map[string]bool{}
	plates := map[string]Nameplate{}
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		if !account.ValidSubject(id) || len(seen) > 100 {
			problem(w, 400, "QUERY_INVALID", "需要至多 100 个用户 ID")
			return
		}
		plate, err := s.nameplate(r.Context(), s.DB, id)
		if err != nil {
			internal(w, err)
			return
		}
		plates[id] = plate
	}
	web.Respond(w, 200, map[string]any{"nameplates": plates})
}

// userProfile is GET /api/users/{id}: who the player is, what they did on all sources together,
// their nameplate and the services shown. 404 when the account center knows no such active user.
// The page outlives its numbers: when they cannot be added up, totalsAvailable says so.
func (s *Server) userProfile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !account.ValidSubject(id) || id == account.SystemUserID {
		problem(w, 404, "USER_NOT_FOUND", "用户不存在")
		return
	}
	p, found, answered := s.profile(r.Context(), id)
	if answered && !found && s.Config.SSO != nil {
		problem(w, 404, "USER_NOT_FOUND", "用户不存在")
		return
	}
	plate, err := s.nameplate(r.Context(), s.DB, id)
	if err != nil {
		internal(w, err)
		return
	}
	plate.Nickname, plate.AvatarPath = p.Nickname, files.Path(p.AvatarURL)
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	totals, err := s.playerTotals(ctx, id)
	if err != nil {
		log.Printf("request=%s player totals: %v", w.Header().Get("X-Request-ID"), err)
		totals = PlayerTotals{}
	}
	web.Respond(w, 200, map[string]any{
		"user": struct {
			ID         string `json:"id"`
			Nickname   string `json:"nickname"`
			AvatarPath string `json:"avatarPath"`
			PlayerTotals
		}{id, p.Nickname, files.Path(p.AvatarURL), totals},
		"nameplate":         plate,
		"sources":           s.sourcesAt(r.Context()),
		"profilesAvailable": answered,
		"totalsAvailable":   err == nil,
	})
}

// titleCatalog is GET /api/titles: every title, readable from any website (the 段位 editors of
// the other OurTaiko websites pick reward titles from it), and the rules that give titles by
// themselves (title_rules.go).
func (s *Server) titleCatalog(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	rows, err := s.DB.Query(r.Context(), `SELECT `+titleColumns+` FROM titles t ORDER BY t.official DESC,t.created_at,t.id`)
	if err != nil {
		internal(w, err)
		return
	}
	defer rows.Close()
	titles := []Title{}
	for rows.Next() {
		t, err := scanTitle(rows)
		if err != nil {
			internal(w, err)
			return
		}
		titles = append(titles, t)
	}
	if err = rows.Err(); err != nil {
		internal(w, err)
		return
	}
	rules, err := s.titleRules(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	web.Respond(w, 200, map[string]any{"titles": titles, "rules": rules})
}

// HeldTitle is a title a player holds and what gave it.
type HeldTitle struct {
	Title       Title     `json:"title"`
	GrantedAt   time.Time `json:"grantedAt"`
	Service     string    `json:"service"`
	CourseTitle string    `json:"courseTitle"`
}

func (s *Server) userTitles(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !account.ValidSubject(id) {
		problem(w, 404, "USER_NOT_FOUND", "用户不存在")
		return
	}
	rows, err := s.DB.Query(r.Context(), `SELECT `+titleColumns+`,u.granted_at,COALESCE(a.service,''),COALESCE(a.course_title,'')
 FROM user_titles u JOIN titles t ON t.id=u.title_id LEFT JOIN awards a ON a.id=u.award_id
 WHERE u.user_id=$1 ORDER BY u.granted_at DESC,t.id`, id)
	if err != nil {
		internal(w, err)
		return
	}
	defer rows.Close()
	held := []HeldTitle{}
	for rows.Next() {
		var h HeldTitle
		if h.Title, err = scanTitle(rows, &h.GrantedAt, &h.Service, &h.CourseTitle); err != nil {
			internal(w, err)
			return
		}
		held = append(held, h)
	}
	if err = rows.Err(); err != nil {
		internal(w, err)
		return
	}
	web.Respond(w, 200, map[string]any{"titles": held})
}

// AwardRecord is one 段位 attempt that asked for awards.
type AwardRecord struct {
	Service     string          `json:"service"`
	CourseTitle string          `json:"courseTitle"`
	Official    bool            `json:"official"`
	Verdict     int             `json:"verdict"`
	Dan         *int            `json:"dan"`
	Granted     json.RawMessage `json:"granted"`
	CreatedAt   time.Time       `json:"createdAt"`
}

func (s *Server) userDans(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !account.ValidSubject(id) {
		problem(w, 404, "USER_NOT_FOUND", "用户不存在")
		return
	}
	badges := []DanBadge{}
	rows, err := s.DB.Query(r.Context(), `SELECT dan,verdict,achieved_at FROM user_dans WHERE user_id=$1 ORDER BY dan DESC`, id)
	if err != nil {
		internal(w, err)
		return
	}
	for rows.Next() {
		var d DanBadge
		if err = rows.Scan(&d.Index, &d.Verdict, &d.AchievedAt); err != nil {
			rows.Close()
			internal(w, err)
			return
		}
		d.tiers()
		badges = append(badges, d)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		internal(w, err)
		return
	}
	history := []AwardRecord{}
	rows, err = s.DB.Query(r.Context(), `SELECT service,course_title,official,verdict,dan,granted,created_at FROM awards WHERE user_id=$1 AND service NOT IN ('admin','rule') ORDER BY created_at DESC,id DESC LIMIT 50`, id)
	if err != nil {
		internal(w, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var a AwardRecord
		var granted []byte
		if err = rows.Scan(&a.Service, &a.CourseTitle, &a.Official, &a.Verdict, &a.Dan, &granted, &a.CreatedAt); err != nil {
			internal(w, err)
			return
		}
		a.Granted = granted
		history = append(history, a)
	}
	if err = rows.Err(); err != nil {
		internal(w, err)
		return
	}
	web.Respond(w, 200, map[string]any{"badges": badges, "history": history})
}

// wearNameplate is PUT /api/me/nameplate {titleId: string|null, showDan}: one of the
// player's own titles, or none.
func (s *Server) wearNameplate(w http.ResponseWriter, r *http.Request) {
	v, ok := s.required(w, r, true)
	if !ok {
		return
	}
	var body struct {
		TitleID *string `json:"titleId"`
		ShowDan *bool   `json:"showDan"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.ShowDan == nil {
		problem(w, 400, "REQUEST_INVALID", "需要 showDan")
		return
	}
	if body.TitleID != nil {
		var held bool
		if err := s.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM user_titles WHERE user_id=$1 AND title_id=$2)`, v.User.ID, *body.TitleID).Scan(&held); err != nil {
			internal(w, err)
			return
		}
		if !held {
			problem(w, 422, "TITLE_NOT_HELD", "你还没有获得这个称号")
			return
		}
	}
	_, err := s.DB.Exec(r.Context(), `INSERT INTO nameplates(user_id,title_id,show_dan,updated_at) VALUES($1,$2,$3,now())
 ON CONFLICT(user_id) DO UPDATE SET title_id=EXCLUDED.title_id,show_dan=EXCLUDED.show_dan,updated_at=now()`, v.User.ID, body.TitleID, *body.ShowDan)
	if err != nil {
		internal(w, err)
		return
	}
	plate, err := s.nameplate(r.Context(), s.DB, v.User.ID)
	if err != nil {
		internal(w, err)
		return
	}
	plate.Nickname, plate.AvatarPath = v.User.Nickname, files.Path(v.User.AvatarURL)
	web.Respond(w, 200, plate)
}

// titleBody is a title as the website sends it.
type titleBody struct {
	Name             string            `json:"name"`
	NameTranslations map[string]string `json:"nameTranslations"`
	Description      string            `json:"description"`
	Layers           *Layers           `json:"layers"`
	Official         bool              `json:"official"`
}

// look checks the layers and gives them with the style older games draw.
func (b *titleBody) look() (Layers, string, bool) {
	if b.Layers == nil {
		return Layers{}, "", false
	}
	l := *b.Layers
	for slot, item := range map[string]*string{"effect": l.Effect, "icon": l.Icon} {
		if item != nil && *item == "" {
			item = nil
		}
		if slot == "effect" {
			l.Effect = item
		} else {
			l.Icon = item
		}
		if item != nil && !layerItem[slot].MatchString(*item) {
			return Layers{}, "", false
		}
	}
	if !layerItem["band"].MatchString(l.Band) {
		return Layers{}, "", false
	}
	return l, legacyStyle(l.Band), true
}

// text checks the name, its translations and the description, and gives the translations as
// stored.
func (b *titleBody) text() ([]byte, bool) {
	b.Name = strings.TrimSpace(b.Name)
	valid := b.Name != "" && utf8.RuneCountInString(b.Name) <= 40 && utf8.RuneCountInString(b.Description) <= 500
	for language, name := range b.NameTranslations {
		if !map[string]bool{"zh-Hans": true, "en": true, "ja": true, "ko": true}[language] || utf8.RuneCountInString(name) > 40 {
			valid = false
		}
	}
	if b.NameTranslations == nil {
		b.NameTranslations = map[string]string{}
	}
	translations, _ := json.Marshal(b.NameTranslations)
	return translations, valid
}

// saveTitle is PUT /api/admin/titles/{id}: create or change a title of the catalog. A
// player's title keeps its maker; making it official frees their place for another.
func (s *Server) saveTitle(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.admin(w, r); !ok {
		return
	}
	id := r.PathValue("id")
	var body titleBody
	if !decode(w, r, &body) {
		return
	}
	translations, valid := body.text()
	layers, style, looks := body.look()
	if !valid || !looks || !titleID.MatchString(id) {
		problem(w, 422, "TITLE_INVALID", "称号信息无效")
		return
	}
	t, err := scanTitle(s.DB.QueryRow(r.Context(), `INSERT INTO titles AS t(id,name,name_translations,description,style,band_item,effect_item,icon_item,official) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)
 ON CONFLICT(id) DO UPDATE SET name=EXCLUDED.name,name_translations=EXCLUDED.name_translations,description=EXCLUDED.description,
 style=EXCLUDED.style,band_item=EXCLUDED.band_item,effect_item=EXCLUDED.effect_item,icon_item=EXCLUDED.icon_item,official=EXCLUDED.official
 RETURNING `+titleColumns, id, body.Name, translations, body.Description, style, layers.Band, layers.Effect, layers.Icon, body.Official))
	if err != nil {
		internal(w, err)
		return
	}
	web.Respond(w, 200, t)
}

func (s *Server) deleteTitle(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.admin(w, r); !ok {
		return
	}
	tag, err := s.DB.Exec(r.Context(), `DELETE FROM titles WHERE id=$1`, r.PathValue("id"))
	if err != nil {
		internal(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		problem(w, 404, "TITLE_NOT_FOUND", "称号不存在")
		return
	}
	web.Respond(w, 200, map[string]bool{"ok": true})
}

// createOwnTitle is POST /api/me/titles {name, nameTranslations, description, layers}: a
// community title the player can give in their own 段位 courses. Its id is made here.
func (s *Server) createOwnTitle(w http.ResponseWriter, r *http.Request) {
	v, ok := s.required(w, r, true)
	if !ok {
		return
	}
	var body titleBody
	if !decode(w, r, &body) {
		return
	}
	translations, valid := body.text()
	layers, style, looks := body.look()
	if !valid || !looks {
		problem(w, 422, "TITLE_INVALID", "称号信息无效")
		return
	}
	ctx := r.Context()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	// A player's titles one at a time, so two requests cannot both take the last place.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('titles:'||$1,0))`, v.User.ID); err != nil {
		internal(w, err)
		return
	}
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM titles WHERE owner_id=$1 AND NOT official`, v.User.ID).Scan(&count); err != nil {
		internal(w, err)
		return
	}
	if count >= playerTitleLimit {
		problem(w, 409, "TITLE_LIMIT_REACHED", "每人最多 5 个社区称号")
		return
	}
	t, err := scanTitle(tx.QueryRow(ctx, `INSERT INTO titles AS t(id,name,name_translations,description,style,band_item,effect_item,icon_item,official,owner_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,false,$9)
 RETURNING `+titleColumns, web.NewID(), body.Name, translations, body.Description, style, layers.Band, layers.Effect, layers.Icon, v.User.ID))
	if err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		internal(w, err)
		return
	}
	web.Respond(w, 201, t)
}

// saveOwnTitle is PUT /api/me/titles/{id}, the same body: the maker changes a community
// title. Of an official one they change the look only; the rest is ignored.
func (s *Server) saveOwnTitle(w http.ResponseWriter, r *http.Request) {
	v, ok := s.required(w, r, true)
	if !ok {
		return
	}
	var body titleBody
	if !decode(w, r, &body) {
		return
	}
	ctx := r.Context()
	var official bool
	err := s.DB.QueryRow(ctx, `SELECT official FROM titles WHERE id=$1 AND owner_id=$2`, r.PathValue("id"), v.User.ID).Scan(&official)
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, 404, "TITLE_NOT_FOUND", "称号不存在")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	var t Title
	layers, style, looks := body.look()
	if official {
		if !looks {
			problem(w, 422, "TITLE_INVALID", "称号信息无效")
			return
		}
		t, err = scanTitle(s.DB.QueryRow(ctx, `UPDATE titles t SET style=$3,band_item=$4,effect_item=$5,icon_item=$6 WHERE id=$1 AND owner_id=$2 RETURNING `+titleColumns,
			r.PathValue("id"), v.User.ID, style, layers.Band, layers.Effect, layers.Icon))
	} else {
		translations, valid := body.text()
		if !valid || !looks {
			problem(w, 422, "TITLE_INVALID", "称号信息无效")
			return
		}
		// official stays false: an administrator may have made it official in the meantime.
		t, err = scanTitle(s.DB.QueryRow(ctx, `UPDATE titles t SET name=$3,name_translations=$4,description=$5,style=$6,band_item=$7,effect_item=$8,icon_item=$9 WHERE id=$1 AND owner_id=$2 AND NOT official RETURNING `+titleColumns,
			r.PathValue("id"), v.User.ID, body.Name, translations, body.Description, style, layers.Band, layers.Effect, layers.Icon))
	}
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, 409, "TITLE_CHANGED", "称号刚被管理员修改，请刷新后重试")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	web.Respond(w, 200, t)
}

// deleteOwnTitle is DELETE /api/me/titles/{id}: the maker removes a community title. The
// players who earned it lose it, and a nameplate wearing it wears none.
func (s *Server) deleteOwnTitle(w http.ResponseWriter, r *http.Request) {
	v, ok := s.required(w, r, true)
	if !ok {
		return
	}
	ctx := r.Context()
	tag, err := s.DB.Exec(ctx, `DELETE FROM titles WHERE id=$1 AND owner_id=$2 AND NOT official`, r.PathValue("id"), v.User.ID)
	if err != nil {
		internal(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		var official bool
		err = s.DB.QueryRow(ctx, `SELECT official FROM titles WHERE id=$1 AND owner_id=$2`, r.PathValue("id"), v.User.ID).Scan(&official)
		switch {
		case err == nil:
			problem(w, 403, "TITLE_OFFICIAL", "官方称号只有管理员可以删除")
		case errors.Is(err, pgx.ErrNoRows):
			problem(w, 404, "TITLE_NOT_FOUND", "称号不存在")
		default:
			internal(w, err)
		}
		return
	}
	web.Respond(w, 200, map[string]bool{"ok": true})
}

type grantBody struct {
	UserID  string `json:"userId"`
	TitleID string `json:"titleId"`
}

// grantTitle is POST /api/admin/grants {userId, titleId}: an administrator gives a title
// (events, tests). It is recorded as an award of the service "admin".
func (s *Server) grantTitle(w http.ResponseWriter, r *http.Request) {
	v, ok := s.admin(w, r)
	if !ok {
		return
	}
	var body grantBody
	if !decode(w, r, &body) {
		return
	}
	if !account.ValidSubject(body.UserID) {
		problem(w, 422, "USER_INVALID", "用户 ID 无效")
		return
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var exists bool
	if err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM titles WHERE id=$1)`, body.TitleID).Scan(&exists); err != nil {
		internal(w, err)
		return
	}
	if !exists {
		problem(w, 404, "TITLE_NOT_FOUND", "称号不存在")
		return
	}
	granted, _ := json.Marshal(map[string]any{"titles": []string{body.TitleID}, "by": v.User.ID})
	var award int64
	if err = tx.QueryRow(r.Context(), `INSERT INTO awards(service,request_key,user_id,granted) VALUES('admin',$1,$2,$3) RETURNING id`, web.NewID(), body.UserID, granted).Scan(&award); err != nil {
		internal(w, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO user_titles(user_id,title_id,award_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, body.UserID, body.TitleID, award); err != nil {
		internal(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		internal(w, err)
		return
	}
	web.Respond(w, 200, map[string]bool{"ok": true})
}

func (s *Server) revokeTitle(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.admin(w, r); !ok {
		return
	}
	var body grantBody
	if !decode(w, r, &body) {
		return
	}
	tag, err := s.DB.Exec(r.Context(), `DELETE FROM user_titles WHERE user_id=$1 AND title_id=$2`, body.UserID, body.TitleID)
	if err != nil {
		internal(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		problem(w, 404, "GRANT_NOT_FOUND", "该用户没有这个称号")
		return
	}
	web.Respond(w, 200, map[string]bool{"ok": true})
}
