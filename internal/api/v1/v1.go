// Package apiv1 is version 1 of the API, at /api/v1/: the API as released clients of the game
// know it. It is the version-free API (songs, profile, site) mounted under its prefix (package
// mount). When no client uses it any more, this package and the line mounting it go.
package apiv1

import (
	"net/http"

	"ourtaiko.dev/net/internal/api/mount"
)

// Version is this API's name in its paths: /api/v1/.
const Version = "v1"

// Handler serves api, the version-free API, at /api/v1/.
func Handler(api http.Handler) http.Handler {
	return mount.Mount(Version, api)
}
