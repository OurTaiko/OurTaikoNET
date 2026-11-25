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

// Title is one title. Layers is how a nameplate wearing it looks; Style is the band of the six
// that game versions released before the layers draw (legacyStyle). OwnerID is the player who
// made it (null for the titles the title managers make themselves).
//
// Visibility is private or public. A private title is its maker's alone: they hold it, change it
// as they like, and nobody else can be given it. A public one stands in the catalog with how it
// is obtained (Obtain, and its rules), and only title managers decide who gets it; it became
// public by a review its maker asked for (title_reviews.go), or a manager made it.
type Title struct {
	ID               string          `json:"id"`
	Name             string          `json:"name"`
	NameTranslations json.RawMessage `json:"nameTranslations"`
	Description      string          `json:"description"`
	Obtain           string          `json:"obtain"`
	Style            string          `json:"style"`
	Layers           Layers          `json:"layers"`
	Visibility       string          `json:"visibility"`
	OwnerID          *string         `json:"ownerId"`
}

const private, public = "private", "public"

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

// A player keeps at most five private titles, looking any way they like. One that a review made
// public no longer takes a place.
const playerTitleLimit = 5

const titleColumns = `t.id,t.name,t.name_translations,t.description,t.obtain,t.style,t.band_item,t.effect_item,t.icon_item,t.visibility,t.owner_id`

type rowScanner interface{ Scan(...any) error }

func scanTitle(row rowScanner, extra ...any) (Title, error) {
	var t Title
	var translations []byte
	err := row.Scan(append([]any{&t.ID, &t.Name, &translations, &t.Description, &t.Obtain, &t.Style, &t.Layers.Band, &t.Layers.Effect, &t.Layers.Icon, &t.Visibility, &t.OwnerID}, extra...)...)
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

// ListedTitle is a title in a list of titles: with how many players hold it, its maker's
// nickname as the site keeps it (empty without a maker, or one never seen), and whether a review
// of it is pending.
type ListedTitle struct {
	Title
	Holders   int    `json:"holders"`
	OwnerName string `json:"ownerName"`
	Pending   bool   `json:"pending"`
}

const listedTitles = `SELECT ` + titleColumns + `,(SELECT count(*) FROM user_titles u WHERE u.title_id=t.id),COALESCE(o.nickname,''),
 EXISTS(SELECT 1 FROM title_reviews r WHERE r.title_id=t.id AND r.status='pending')
 FROM titles t LEFT JOIN users o ON o.id=t.owner_id`

// listTitles reads the titles query gives (listedTitles and what follows it).
func listTitles(ctx context.Context, db queryer, query string, args ...any) ([]ListedTitle, error) {
	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	titles := []ListedTitle{}
	for rows.Next() {
		var t ListedTitle
		if t.Title, err = scanTitle(rows, &t.Holders, &t.OwnerName, &t.Pending); err != nil {
			return nil, err
		}
		titles = append(titles, t)
	}
	return titles, rows.Err()
}

// titleCatalog is GET /api/titles: every public title, the managers' own first, readable from any
// website, and the rules that give titles by themselves (title_rules.go). A private title is not
// in it: it is seen on its maker's nameplate and in their own list (myTitles).
func (s *Server) titleCatalog(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	titles, err := listTitles(r.Context(), s.DB, listedTitles+` WHERE t.visibility='public' ORDER BY t.owner_id IS NOT NULL,t.created_at,t.id`)
	if err != nil {
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

// adminTitles is GET /api/admin/titles: every title there is, private ones too (a private title
// is worn in public, so the managers see them all and may remove one), newest first.
func (s *Server) adminTitles(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.admin(w, r); !ok {
		return
	}
	titles, err := listTitles(r.Context(), s.DB, listedTitles+` ORDER BY t.created_at DESC,t.id`)
	if err != nil {
		internal(w, err)
		return
	}
	web.Respond(w, 200, map[string]any{"titles": titles})
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

// The longest of the texts that go with a title's review, in characters: how the title is
// obtained, what its maker asks for, what the manager replies.
const titleNoteLimit = 500

// note is such a text without the space around it, and whether it is short enough.
func note(text string) (string, bool) {
	text = strings.TrimSpace(text)
	return text, utf8.RuneCountInString(text) <= titleNoteLimit
}

// saveTitle is PUT /api/admin/titles/{id} {name, nameTranslations, description, obtain, layers}:
// a title manager makes a title of the site's own (public, without a maker), or changes a public
// title, whoever made it. A private title is its maker's to change: the managers may only remove
// one (deleteTitle).
func (s *Server) saveTitle(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.admin(w, r); !ok {
		return
	}
	id := r.PathValue("id")
	var body struct {
		titleBody
		Obtain string `json:"obtain"`
	}
	if !decode(w, r, &body) {
		return
	}
	translations, valid := body.text()
	layers, style, looks := body.look()
	obtain, fits := note(body.Obtain)
	if !valid || !looks || !fits || !titleID.MatchString(id) {
		problem(w, 422, "TITLE_INVALID", "称号信息无效")
		return
	}
	t, err := scanTitle(s.DB.QueryRow(r.Context(), `INSERT INTO titles AS t(id,name,name_translations,description,obtain,style,band_item,effect_item,icon_item,visibility) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'public')
 ON CONFLICT(id) DO UPDATE SET name=EXCLUDED.name,name_translations=EXCLUDED.name_translations,description=EXCLUDED.description,obtain=EXCLUDED.obtain,
 style=EXCLUDED.style,band_item=EXCLUDED.band_item,effect_item=EXCLUDED.effect_item,icon_item=EXCLUDED.icon_item
 WHERE t.visibility='public' RETURNING `+titleColumns, id, body.Name, translations, body.Description, obtain, style, layers.Band, layers.Effect, layers.Icon))
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, 409, "TITLE_PRIVATE", "私有称号只有它的创建者可以修改")
		return
	}
	if err != nil {
		internal(w, err)
		return
	}
	web.Respond(w, 200, t)
}

// deleteTitle is DELETE /api/admin/titles/{id}: a title manager removes a title, private or
// public. The players who hold it lose it, a nameplate wearing it wears none, and its maker, if
// it is another player's, is told in 称号中心.
func (s *Server) deleteTitle(w http.ResponseWriter, r *http.Request) {
	v, ok := s.admin(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	t, err := scanTitle(tx.QueryRow(ctx, `DELETE FROM titles t WHERE t.id=$1 RETURNING `+titleColumns, r.PathValue("id")))
	if errors.Is(err, pgx.ErrNoRows) {
		problem(w, 404, "TITLE_NOT_FOUND", "称号不存在")
		return
	}
	if err == nil && t.OwnerID != nil && *t.OwnerID != v.User.ID {
		err = tellOfTitle(ctx, tx, []string{*t.OwnerID}, "title_removed", map[string]any{"title": t})
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		internal(w, err)
		return
	}
	web.Respond(w, 200, map[string]bool{"ok": true})
}

// createOwnTitle is POST /api/me/titles {name, nameTranslations, description, layers}: the
// player makes a title. It is private and they hold it at once. Its id is made here.
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
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM titles WHERE owner_id=$1 AND visibility='private'`, v.User.ID).Scan(&count); err != nil {
		internal(w, err)
		return
	}
	if count >= playerTitleLimit {
		problem(w, 409, "TITLE_LIMIT_REACHED", "每人最多 5 个私有称号")
		return
	}
	t, err := scanTitle(tx.QueryRow(ctx, `INSERT INTO titles AS t(id,name,name_translations,description,style,band_item,effect_item,icon_item,visibility,owner_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'private',$9)
 RETURNING `+titleColumns, web.NewID(), body.Name, translations, body.Description, style, layers.Band, layers.Effect, layers.Icon, v.User.ID))
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO user_titles(user_id,title_id) VALUES($1,$2)`, v.User.ID, t.ID)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		internal(w, err)
		return
	}
	web.Respond(w, 201, t)
}

// lockOwnTitle reads the title id that maker made in order to change it, holding its row so that
// a change and a request for its review take turns. pending is whether a review of it is pending.
// pgx.ErrNoRows: the player made no such title.
func lockOwnTitle(ctx context.Context, tx pgx.Tx, id, maker string) (t Title, pending bool, err error) {
	if t, err = scanTitle(tx.QueryRow(ctx, `SELECT `+titleColumns+` FROM titles t WHERE t.id=$1 AND t.owner_id=$2 FOR UPDATE`, id, maker)); err != nil {
		return t, false, err
	}
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM title_reviews WHERE title_id=$1 AND status='pending')`, id).Scan(&pending)
	return t, pending, err
}

// saveOwnTitle is PUT /api/me/titles/{id}, with the body of createOwnTitle: the maker changes a
// private title, as often as they like, but not while a review of it is pending. A public one
// changes by a review of the change (requestReview).
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
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	t, pending, err := lockOwnTitle(ctx, tx, r.PathValue("id"), v.User.ID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		problem(w, 404, "TITLE_NOT_FOUND", "称号不存在")
		return
	case err != nil:
		internal(w, err)
		return
	case t.Visibility == public:
		problem(w, 409, "TITLE_PUBLIC", "公开称号要提交更改申请，由称号管理员审核")
		return
	case pending:
		problem(w, 409, "TITLE_REVIEW_PENDING", "称号正在审核，撤回申请后才能修改")
		return
	}
	translations, valid := body.text()
	layers, style, looks := body.look()
	if !valid || !looks {
		problem(w, 422, "TITLE_INVALID", "称号信息无效")
		return
	}
	t, err = scanTitle(tx.QueryRow(ctx, `UPDATE titles t SET name=$2,name_translations=$3,description=$4,style=$5,band_item=$6,effect_item=$7,icon_item=$8 WHERE t.id=$1 RETURNING `+titleColumns,
		t.ID, body.Name, translations, body.Description, style, layers.Band, layers.Effect, layers.Icon))
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		internal(w, err)
		return
	}
	web.Respond(w, 200, t)
}

// deleteOwnTitle is DELETE /api/me/titles/{id}: the maker removes a private title, and with it
// a pending request for its review. A public title, which other players may hold, only a title
// manager removes.
func (s *Server) deleteOwnTitle(w http.ResponseWriter, r *http.Request) {
	v, ok := s.required(w, r, true)
	if !ok {
		return
	}
	ctx := r.Context()
	tag, err := s.DB.Exec(ctx, `DELETE FROM titles WHERE id=$1 AND owner_id=$2 AND visibility='private'`, r.PathValue("id"), v.User.ID)
	if err != nil {
		internal(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		var made bool
		if err = s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM titles WHERE id=$1 AND owner_id=$2)`, r.PathValue("id"), v.User.ID).Scan(&made); err != nil {
			internal(w, err)
		} else if made {
			problem(w, 403, "TITLE_PUBLIC", "公开称号只有称号管理员可以删除")
		} else {
			problem(w, 404, "TITLE_NOT_FOUND", "称号不存在")
		}
		return
	}
	web.Respond(w, 200, map[string]bool{"ok": true})
}

// OwnTitle is a title the player made, with the latest request for its review, if there was one.
type OwnTitle struct {
	ListedTitle
	Review *Review `json:"review"`
}

// myTitles is GET /api/me/titles: the titles the player made, newest first, each with its latest
// review, and how many of their places for private titles are taken (used of limit).
func (s *Server) myTitles(w http.ResponseWriter, r *http.Request) {
	v, ok := s.required(w, r, false)
	if !ok {
		return
	}
	ctx := r.Context()
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	listed, err := listTitles(ctx, tx, listedTitles+` WHERE t.owner_id=$1 ORDER BY t.created_at DESC,t.id`, v.User.ID)
	if err != nil {
		internal(w, err)
		return
	}
	rows, err := tx.Query(ctx, `SELECT DISTINCT ON (r.title_id) `+reviewColumns+` FROM title_reviews r WHERE r.user_id=$1 ORDER BY r.title_id,r.created_at DESC,r.id`, v.User.ID)
	if err != nil {
		internal(w, err)
		return
	}
	defer rows.Close()
	latest := map[string]*Review{}
	for rows.Next() {
		review, err := scanReview(rows)
		if err != nil {
			internal(w, err)
			return
		}
		latest[review.TitleID] = &review
	}
	if err = rows.Err(); err != nil {
		internal(w, err)
		return
	}
	titles, used := []OwnTitle{}, 0
	for _, t := range listed {
		if t.Visibility == private {
			used++
		}
		titles = append(titles, OwnTitle{ListedTitle: t, Review: latest[t.ID]})
	}
	web.Respond(w, 200, map[string]any{"titles": titles, "limit": playerTitleLimit, "used": used})
}

type grantBody struct {
	UserID  string `json:"userId"`
	TitleID string `json:"titleId"`
}

// grantTitle is POST /api/admin/grants {userId, titleId}: a title manager gives a public title
// by hand (events, tests). It is recorded as an award of the service "admin".
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
	var visibility string
	err = tx.QueryRow(r.Context(), `SELECT visibility FROM titles WHERE id=$1 FOR SHARE`, body.TitleID).Scan(&visibility)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		problem(w, 404, "TITLE_NOT_FOUND", "称号不存在")
		return
	case err != nil:
		internal(w, err)
		return
	case visibility != public:
		problem(w, 422, "TITLE_PRIVATE", "私有称号不能发放给其他玩家")
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
