package httpapi

import (
	"net/http"
	"strings"

	"ourtaiko.dev/net/internal/groups"
)

// The manage panel's part about players' groups. Only the site's administrator (the account
// center's administrator of this site) may use it; being in a group does not let anyone give
// groups.

// groupCatalog is GET /api/v1/admin/groups: every group there is.
func (s *Server) groupCatalog(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.siteAdmin(w, r); !ok {
		return
	}
	respond(w, 200, map[string]any{"groups": groups.All()})
}

// groupMembers is GET /api/v1/admin/users: the players who are in any group, or with
// ?ids=a,b (at most 100) those players and their groups, whether they have any or not.
func (s *Server) groupMembers(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.siteAdmin(w, r); !ok {
		return
	}
	ids := []string{}
	for _, id := range strings.Split(r.URL.Query().Get("ids"), ",") {
		if id == "" {
			continue
		}
		if !validSubject(id) || len(ids) == 100 {
			problem(w, 400, "QUERY_INVALID", "需要至多 100 个用户 ID")
			return
		}
		ids = append(ids, id)
	}
	members, err := groups.Members(r.Context(), s.DB, ids)
	if err != nil {
		internal(w, err)
		return
	}
	respond(w, 200, map[string]any{"users": members})
}

// giveGroups is PUT /api/v1/admin/users/{id}/groups with {"groups": ["OFFICIAL_MANAGER", …]}:
// the player is from now on in exactly these groups. The change is recorded with who made it.
func (s *Server) giveGroups(w http.ResponseWriter, r *http.Request) {
	v, ok := s.siteAdmin(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !validSubject(id) {
		problem(w, 404, "USER_NOT_FOUND", "用户不存在")
		return
	}
	var body struct {
		Groups *[]string `json:"groups"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Groups == nil {
		problem(w, 422, "GROUPS_INVALID", "需要 groups 列表")
		return
	}
	set, err := groups.Parse(*body.Groups)
	if err != nil {
		problem(w, 422, "GROUPS_INVALID", "没有这个用户组")
		return
	}
	ctx := r.Context()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		internal(w, err)
		return
	}
	defer tx.Rollback(ctx)
	before, err := groups.Give(ctx, tx, id, set, v.User.ID)
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		internal(w, err)
		return
	}
	respond(w, 200, map[string]any{"user": groups.Member{ID: id, Groups: set.Names(), Bits: set}, "before": before.Names()})
}
