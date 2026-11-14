package httpapi

import (
	"log"
	"net/http"
	"regexp"
	"strings"

	"ourtaiko.dev/net/internal/account"
	"ourtaiko.dev/net/internal/groups"
	"ourtaiko.dev/net/internal/web"
)

// The manage panel's part about players' groups. Only the site's administrator (the account
// center's administrator of this site) may use it; being in a group does not let anyone give
// groups.

// groupCatalog is GET /api/v1/admin/groups: every group there is.
func (s *Server) groupCatalog(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.siteAdmin(w, r); !ok {
		return
	}
	web.Respond(w, 200, map[string]any{"groups": groups.All()})
}

// A login name as the account center accepts one in a question.
var loginName = regexp.MustCompile(`^\S{1,24}$`)

// How many players a search answers with.
const foundPlayers = 20

// groupMembers is GET /api/v1/admin/users: the players who are in any group; with
// ?ids=a,b (at most 100) those players and their groups, whether they have any or not; with
// ?q=text the players text finds, and their groups.
func (s *Server) groupMembers(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.siteAdmin(w, r); !ok {
		return
	}
	if q := strings.TrimSpace(r.URL.Query().Get("q")); q != "" {
		s.findPlayers(w, r, q)
		return
	}
	ids := []string{}
	for _, id := range strings.Split(r.URL.Query().Get("ids"), ",") {
		if id == "" {
			continue
		}
		if !account.ValidSubject(id) || len(ids) == 100 {
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
	web.Respond(w, 200, map[string]any{"users": members})
}

// findPlayers answers a search of the manage panel. text finds, in this order: the player
// whose login name it is, exactly (asked of the account center, so also someone who has never
// been here and can be given groups ahead of time); the player whose id it is; and the
// players whose nickname contains it.
func (s *Server) findPlayers(w http.ResponseWriter, r *http.Request, text string) {
	if len(text) > 100 {
		problem(w, 400, "QUERY_INVALID", "搜索内容过长")
		return
	}
	ctx := r.Context()
	found := []groups.Member{}
	if s.Config.SSO != nil && loginName.MatchString(text) {
		// Without the account center the nicknames the site keeps are still searched.
		if u, ok, err := s.Config.SSO.ByUsername(ctx, text); err != nil {
			log.Printf("manage: asking the account center for a login name: %v", err)
		} else if ok {
			known, err := groups.Members(ctx, s.DB, []string{u.ID})
			if err != nil {
				internal(w, err)
				return
			}
			known[0].Username, known[0].Nickname, known[0].AvatarURL = u.Username, u.Nickname, u.AvatarURL
			found = append(found, known[0])
		}
	}
	if account.ValidSubject(text) {
		known, err := groups.Members(ctx, s.DB, []string{text})
		if err != nil {
			internal(w, err)
			return
		}
		found = append(found, known...)
	}
	named, err := groups.Find(ctx, s.DB, text, foundPlayers)
	if err != nil {
		internal(w, err)
		return
	}
	users, seen := []groups.Member{}, map[string]bool{}
	for _, m := range append(found, named...) {
		if !seen[m.ID] && len(users) < foundPlayers {
			seen[m.ID] = true
			users = append(users, m)
		}
	}
	web.Respond(w, 200, map[string]any{"users": users})
}

// giveGroups is PUT /api/v1/admin/users/{id}/groups with {"groups": ["OFFICIAL_MANAGER", …]}:
// the player is from now on in exactly these groups. The change is recorded with who made it.
func (s *Server) giveGroups(w http.ResponseWriter, r *http.Request) {
	v, ok := s.siteAdmin(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !account.ValidSubject(id) {
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
	web.Respond(w, 200, map[string]any{"user": groups.Member{ID: id, Groups: set.Names(), Bits: set}, "before": before.Names()})
}
