// Package apiv2 is version 2 of the API, at /api/v2/: the version-free API (songs, profile,
// site) mounted under its prefix (package mount), its answers as they are. Every public file is
// a path under the distribution root (package files); a client asks once which root it reads
// from (GET /api/v2/distribution/source) and joins the two.
package apiv2

import (
	"net/http"
	"strings"

	"ourtaiko.dev/net/internal/api/mount"
	"ourtaiko.dev/net/internal/files"
	"ourtaiko.dev/net/internal/kit/distribution"
	"ourtaiko.dev/net/internal/kit/objectstore"
	"ourtaiko.dev/net/internal/web"
)

// Version is this API's name in its paths: /api/v2/.
const Version = "v2"

// Config is what version 2 needs besides the API itself.
type Config struct {
	// Router picks a visitor's root (nil: always the canonical one).
	Router *distribution.Router
	// OnDisk, in development, holds the folders whose files are kept on this server's disk
	// ("fanmade/", "database/") and the store of each. The root is then this server's
	// /api/v2/files/, which serves those folders and sends every other path to the canonical
	// root. Empty in production: the files are in the bucket.
	OnDisk map[string]objectstore.Store
	// Origin is this site's address, the root's on disk (or the other address of the site the
	// request came to, web.Origins).
	Origin string
}

// Handler serves api, the version-free API, at /api/v2/.
func Handler(api http.Handler, cfg Config) http.Handler {
	mounted := mount.Mount(Version, api)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		read := r.Method == http.MethodGet || r.Method == http.MethodHead
		switch {
		case read && r.URL.Path == "/api/v2/distribution/source":
			w.Header().Set("Cache-Control", "no-store")
			web.Respond(w, 200, cfg.source(r))
		case read && len(cfg.OnDisk) > 0 && strings.HasPrefix(r.URL.Path, "/api/v2/files/"):
			cfg.file(w, r, strings.TrimPrefix(r.URL.Path, "/api/v2/files/"))
		default:
			mounted.ServeHTTP(w, r)
		}
	})
}

// source is the visitor's root: the mirror's for a mainland visitor, the canonical one for
// everyone else, and this server's in development with files on disk.
func (cfg Config) source(r *http.Request) distribution.SourceAnswer {
	if len(cfg.OnDisk) > 0 {
		return distribution.SourceAnswer{BaseURL: web.Origin(r.Context(), cfg.Origin) + "/api/v2/files/", CanonicalBaseURL: files.Root}
	}
	return cfg.Router.Source(r)
}

// file serves a file of a folder kept on disk; any other path is the canonical root's.
func (cfg Config) file(w http.ResponseWriter, r *http.Request, p string) {
	if !files.Valid(p) {
		http.NotFound(w, r)
		return
	}
	for folder, store := range cfg.OnDisk {
		key, ok := strings.CutPrefix(p, folder)
		if !ok {
			continue
		}
		f, err := store.Open(r.Context(), key)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		// A file is never replaced in place: its path names its bytes.
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		http.ServeContent(w, r, p, f.ModTime(), f)
		return
	}
	http.Redirect(w, r, files.URL("", p), http.StatusFound)
}
