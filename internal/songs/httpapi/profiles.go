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

// newChartOwner is who owns a chart uploader adds: the official source's charts all belong to
// the built-in account.
func (s *Server) newChartOwner(uploader string) string {
	if s.source().Official {
		return account.SystemUserID
	}
	return uploader
}

// ownerProfile is what a chart shows of an owner. The built-in account has no name to show.
func ownerProfile(profiles map[string]account.PublicProfile, id string) account.PublicProfile {
	if id == account.SystemUserID {
		return account.PublicProfile{}
	}
	p, ok := profiles[id]
	if !ok {
		p.Nickname = "未知用户"
	}
	return p
}

func (c *Chart) setOwners(profiles map[string]account.PublicProfile) {
	c.Owners = []NotificationActor{}
	for _, id := range c.OwnerIDs {
		p := ownerProfile(profiles, id)
		c.Owners = append(c.Owners, NotificationActor{ID: id, Nickname: p.Nickname, AvatarPath: files.Path(p.AvatarURL)})
	}
	c.setUploader(ownerProfile(profiles, c.OwnerID))
}

func (c *Chart) setUploader(p account.PublicProfile) {
	c.Author = p.Nickname
	c.AuthorAvatarPath = files.Path(p.AvatarURL)
}
