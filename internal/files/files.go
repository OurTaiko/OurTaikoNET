// Package files names the public files clients download (song files, covers, avatars, the 3D
// Don, nameplate art, the web player) the way the API gives them: as paths under the
// distribution root, such as "fanmade/<sha256>.webp" or "3dchara/cos/0.glb". A client asks once
// which root it reads from (the global CDN or, in mainland China, its mirror) and joins the two.
//
// A published file is never replaced in place: a changed file gets a new path. Only JSON
// catalogs and manifests change.
package files

import (
	"net/url"
	"strings"
)

// Root is the canonical distribution root, the CDN in front of the public bucket.
const Root = "https://distrib.ourtaiko.org/"

// Path is address's path under Root ("sso/avatars/a/b.webp"), or "" when address is not a plain
// address there (another host, a query, credentials, a path that leaves the root).
func Path(address string) string {
	rest, ok := strings.CutPrefix(address, Root)
	if !ok || !Valid(rest) {
		return ""
	}
	return rest
}

// Valid reports whether p is a path a client may join to its root: relative, clean, with no
// query, fragment or escape.
func Valid(p string) bool {
	if p == "" || strings.HasPrefix(p, "/") || strings.ContainsAny(p, "?#\\%\x00") {
		return false
	}
	for _, part := range strings.Split(p, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	u, err := url.Parse(p)
	return err == nil && u.Scheme == "" && u.Host == "" && u.Path == p
}

// URL is p under root (Root when root is empty), or "" for no path.
func URL(root, p string) string {
	if p == "" {
		return ""
	}
	if root == "" {
		root = Root
	}
	return strings.TrimSuffix(root, "/") + "/" + p
}
