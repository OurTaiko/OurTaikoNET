package httpapi

import (
	"context"
	"net/http"
	"strconv"

	"ourtaiko.dev/net/internal/account"
	"ourtaiko.dev/net/internal/donitems"
	"ourtaiko.dev/net/internal/web"
)

// Which parts of the 3D Don must be owned to be worn, who owns them, and how they are given
// (package donitems). The catalog says what the parts are; this file only asks it whether a part
// exists.

// worn is what a look puts on: the costume, or the head and the body, and the puchichara and the
// makeup.
func (l DonLook) worn() []donitems.Item {
	items := []donitems.Item{{Part: "puchi", ID: l.Puchi}, {Part: "paint", ID: l.Paint}}
	if l.Costume {
		return append(items, donitems.Item{Part: "cos", ID: l.Cos})
	}
	return append(items, donitems.Item{Part: "head", ID: l.Head}, donitems.Item{Part: "body", ID: l.Body})
}

// donRules are the rules that give parts of the Don.
func (s *Server) donRules(ctx context.Context) ([]TitleRule, error) {
	all, err := s.titleRules(ctx)
	if err != nil {
		return nil, err
	}
	rules := []TitleRule{}
	for _, r := range all {
		if r.DonPart != nil {
			rules = append(rules, r)
		}
	}
	return rules, nil
}

// donItems is GET /api/v1/don/items, public: {locked: [{part, id}], rules: [rule…]}, the parts that
// must be owned to be worn and the rules that give parts.
func (s *Server) donItems(w http.ResponseWriter, r *http.Request) {
	locked, err := donitems.Locked(r.Context(), s.DB)
	if err != nil {
		internal(w, err)
		return
	}
	rules, err := s.donRules(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	web.Respond(w, 200, map[string]any{"locked": locked, "rules": rules})
}

// myDonItems is GET /api/v1/me/don/items: {owned: [{part, id, acquiredAt}]}.
func (s *Server) myDonItems(w http.ResponseWriter, r *http.Request) {
	v, ok := s.required(w, r, false)
	if !ok {
		return
	}
	owned, err := donitems.Of(r.Context(), s.DB, v.User.ID)
	if err != nil {
		internal(w, err)
		return
	}
	web.Respond(w, 200, map[string]any{"owned": owned})
}

// donItemOf is the part a request names by {part} and {id}, when the catalog has it.
func (s *Server) donItemOf(w http.ResponseWriter, r *http.Request) (donitems.Item, bool) {
	id, err := strconv.Atoi(r.PathValue("id"))
	it := donitems.Item{Part: r.PathValue("part"), ID: id}
	if err != nil || !it.Valid() {
		problem(w, 404, "DON_ITEM_NOT_FOUND", "没有这件咚酱装扮")
		return it, false
	}
	c, err := s.donCatalog(r.Context())
	if err != nil {
		problem(w, 503, "DON_CATALOG_UNAVAILABLE", "服装目录暂时读不到，请稍后再试")
		return it, false
	}
	if _, ok := c.item(it.Part, it.ID); !ok {
		problem(w, 404, "DON_ITEM_NOT_FOUND", "没有这件咚酱装扮")
		return it, false
	}
	return it, true
}

// lockDonItem is PUT /api/v1/admin/don/locks/{part}/{id} {locked}: a part must be owned to be
// worn, or is anyone's again. Locking gives it to everyone wearing it: {given} says how many.
func (s *Server) lockDonItem(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.admin(w, r); !ok {
		return
	}
	it, ok := s.donItemOf(w, r)
	if !ok {
		return
	}
	var body struct {
		Locked *bool `json:"locked"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Locked == nil {
		problem(w, 400, "REQUEST_INVALID", "需要 locked")
		return
	}
	if !*body.Locked {
		if err := donitems.Unlock(r.Context(), s.DB, it); err != nil {
			internal(w, err)
			return
		}
		web.Respond(w, 200, map[string]int{"given": 0})
		return
	}
	given, err := donitems.Lock(r.Context(), s.DB, it)
	if err != nil {
		internal(w, err)
		return
	}
	web.Respond(w, 200, map[string]int{"given": given})
}

type donGrantBody struct {
	UserID string `json:"userId"`
	Part   string `json:"part"`
	ID     int    `json:"id"`
}

// grantDonItem is POST /api/v1/admin/don/grants {userId, part, id}: an administrator gives a
// part, recorded as an award of the service "admin". {given} is false when they had it.
func (s *Server) grantDonItem(w http.ResponseWriter, r *http.Request) {
	v, ok := s.admin(w, r)
	if !ok {
		return
	}
	var body donGrantBody
	if !decode(w, r, &body) {
		return
	}
	if !account.ValidSubject(body.UserID) {
		problem(w, 422, "USER_INVALID", "用户 ID 无效")
		return
	}
	r.SetPathValue("part", body.Part)
	r.SetPathValue("id", strconv.Itoa(body.ID))
	it, ok := s.donItemOf(w, r)
	if !ok {
		return
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	given, err := donitems.Give(r.Context(), tx, body.UserID, it, "admin", web.NewID(), map[string]any{"by": v.User.ID})
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		internal(w, err)
		return
	}
	web.Respond(w, 200, map[string]bool{"given": given})
}

// revokeDonItem is DELETE /api/v1/admin/don/grants {userId, part, id}: the part is taken back,
// and off the player if they wear it and it is locked.
func (s *Server) revokeDonItem(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.admin(w, r); !ok {
		return
	}
	var body donGrantBody
	if !decode(w, r, &body) {
		return
	}
	it := donitems.Item{Part: body.Part, ID: body.ID}
	if !account.ValidSubject(body.UserID) || !it.Valid() {
		problem(w, 422, "REQUEST_INVALID", "用户或装扮无效")
		return
	}
	had, err := donitems.Take(r.Context(), s.DB, body.UserID, it)
	if err != nil {
		internal(w, err)
		return
	}
	if !had {
		problem(w, 404, "GRANT_NOT_FOUND", "该用户没有这件装扮")
		return
	}
	web.Respond(w, 200, map[string]bool{"ok": true})
}
