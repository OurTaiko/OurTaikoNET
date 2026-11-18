package apiv1

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"

	"ourtaiko.dev/net/internal/files"
	"ourtaiko.dev/net/internal/kit/distribution"
)

// Version 1 gives full addresses, and a mainland visitor's are the mirror's. Version 2 gives
// paths and leaves the root to the client; this file is version 1's choice of root.

// songAddress is canonical, a song file's canonical address, as the visitor of request reads it:
// the mirror's copy for a mainland visitor. Only public, content-addressed song objects move;
// signed URLs, other hosts, other folders and unknown kinds of file keep their address.
func songAddress(router *distribution.Router, request *http.Request, canonical string) string {
	if request == nil || request.URL.Query().Get("origin") == "1" {
		return canonical
	}
	base := router.MirrorBase(request)
	if base == "" {
		return canonical
	}
	u, err := url.Parse(canonical)
	if err != nil || u.Scheme != "https" || u.Host != "distrib.ourtaiko.org" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" {
		return canonical
	}
	if !(strings.HasPrefix(u.Path, "/fanmade/") || strings.HasPrefix(u.Path, "/database/")) || path.Clean(u.Path) != u.Path {
		return canonical
	}
	name := path.Base(u.Path)
	ext := path.Ext(name)
	digest := strings.TrimSuffix(name, ext)
	if _, err := hex.DecodeString(digest); err != nil || len(digest) != 64 {
		return canonical
	}
	switch ext {
	case ".tja", ".ogg", ".mp3", ".webp", ".png", ".jpg", ".jpeg", ".mp4":
	default:
		return canonical
	}
	return strings.TrimSuffix(base, "/") + u.Path
}

var avatarFile = regexp.MustCompile(`^/sso/avatars/[a-f0-9]{32}/[a-f0-9]{32}\.webp$`)

// distributionRoutes answers version 1's /api/v1/distribution/ routes: the visitor's root
// (source, which version 2 has too) and one avatar's address (avatar, which the game asks for
// each picture). Neither is cached anywhere: the answer is the visitor's own.
func distributionRoutes(router *distribution.Router, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/api/v1/distribution/avatar":
		original := r.URL.Query().Get("url")
		u, err := url.Parse(original)
		if err != nil || u.Scheme != "https" || u.Host != "distrib.ourtaiko.org" || u.User != nil ||
			u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" || !avatarFile.MatchString(u.Path) {
			http.Error(w, `{"error":"RESOURCE_URL_INVALID"}`, http.StatusBadRequest)
			return
		}
		selected := original
		if base := router.MirrorBase(r); base != "" {
			selected = strings.TrimSuffix(base, "/") + u.Path
		}
		if r.URL.Query().Get("redirect") == "1" {
			w.Header().Del("Content-Type")
			http.Redirect(w, r, selected, http.StatusTemporaryRedirect)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"url": selected, "fallbackUrl": original})
	case "/api/v1/distribution/source":
		json.NewEncoder(w).Encode(router.Source(r))
	default:
		http.NotFound(w, r)
	}
}

// root is where a visitor reads files from: the mirror's root for a mainland visitor, else the
// canonical one.
func root(router *distribution.Router, request *http.Request) string {
	if request != nil && request.URL.Query().Get("origin") != "1" {
		if base := router.MirrorBase(request); base != "" {
			return base
		}
	}
	return files.Root
}
