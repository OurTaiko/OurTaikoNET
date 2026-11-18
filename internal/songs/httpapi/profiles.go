package httpapi

import (
	"context"
	"ourtaiko.dev/net/internal/files"
	"slices"
	"time"

	"ourtaiko.dev/net/internal/account"
)

// publicProfiles names users for public pages. Catalogs stay available during an
// identity-provider outage (or without SSO in development): unknown users then
// show a placeholder. Account checks never use this fallback.
func (s *Server) publicProfiles(ctx context.Context, ids []string) map[string]account.PublicProfile {
	result := map[string]account.PublicProfile{}
	unique := []string{}
	for _, id := range ids {
		if _, ok := result[id]; !ok {
			unique = append(unique, id)
			result[id] = account.PublicProfile{Nickname: "未知用户"}
		}
	}
	if s.Config.SSO == nil || len(unique) == 0 {
		return result
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if names, e := s.Config.SSO.Profiles(ctx, unique); e == nil {
		for id, name := range names {
			result[id] = name
		}
	}
	return result
}

// avatarOf is the signed-in user's avatar, or none without SSO.
func (s *Server) avatarOf(u User) string {
	if s.Config.SSO == nil {
		return ""
	}
	return s.Config.SSO.AvatarURL(u)
}

// Public catalogs stay available during an identity-provider outage.
func (s *Server) chartNames(ctx context.Context, charts []Chart) {
	ids, seen := []string{}, map[string]bool{}
	for _, c := range charts {
		for _, id := range c.OwnerIDs {
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	profiles := s.profiles(ctx, ids)
	for i := range charts {
		charts[i].setOwners(profiles)
	}
}

func (c *Chart) hasOwner(id string) bool { return slices.Contains(c.OwnerIDs, id) }

func (c *Chart) setOwners(profiles map[string]account.PublicProfile) {
	c.Owners = []NotificationActor{}
	for _, id := range c.OwnerIDs {
		p, ok := profiles[id]
		if !ok {
			p.Nickname = "未知用户"
		}
		c.Owners = append(c.Owners, NotificationActor{ID: id, Nickname: p.Nickname, AvatarPath: files.Path(p.AvatarURL)})
	}
	p, ok := profiles[c.OwnerID]
	if !ok {
		p.Nickname = "未知用户"
	}
	c.setUploader(p)
}

func (c *Chart) setUploader(p account.PublicProfile) {
	c.Author = p.Nickname
	c.AuthorAvatarPath = files.Path(p.AvatarURL)
}
