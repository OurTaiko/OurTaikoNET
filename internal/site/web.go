package site

import (
	"bytes"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
)

// Web serves the website in front of the API.
//
// The website is web/ in this repository, built to a directory of files: index.html, and under
// assets/ the scripts and styles it names, each with a hash of its content in its file name.
// It is one page that draws every address itself, so an address that is no file gets
// index.html. dir is read on every request: a new release is put beside the old one and the
// directory (a link to the current release) switched, with no restart.
//
// A song's, a playlist's, a 段位 course's or a player's page gets index.html with its own title,
// description and link-card tags (head.go), looked up through api; origin is the site's address
// for the absolute addresses those tags hold (empty: the request's own).
//
// An empty dir gives api alone, as in development, where the website's own server is in front.
func Web(dir, origin string, api http.Handler) http.Handler {
	if dir == "" {
		return api
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isAPI(r.URL.Path) || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
			api.ServeHTTP(w, r)
			return
		}
		files := os.DirFS(dir)
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name == "" {
			name = "index.html"
		}
		if fs.ValidPath(name) && serveFile(w, r, files, name) {
			return
		}
		// A missing script, style or picture is missing: answering with the page instead
		// would hand a browser HTML where it asked for something else.
		if name != "index.html" && (strings.HasPrefix(name, "assets/") || path.Ext(name) != "") {
			http.NotFound(w, r)
			return
		}
		if head, ok := lookUpHead(r, api, siteOrigin(r, origin)); ok && servePage(w, r, files, head) {
			return
		}
		if !serveFile(w, r, files, "index.html") {
			http.Error(w, "The website is not installed", http.StatusServiceUnavailable)
		}
	})
}

func isAPI(p string) bool {
	return strings.HasPrefix(p, "/api/") || p == "/healthz" || p == "/readyz"
}

// siteOrigin is origin, or when it is empty the address r came to.
func siteOrigin(r *http.Request, origin string) string {
	if origin != "" {
		return strings.TrimSuffix(origin, "/")
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// servePage answers with index.html, its head written for the page (withHead), and reports
// whether there is an index.html.
func servePage(w http.ResponseWriter, r *http.Request, files fs.FS, head pageHead) bool {
	page, err := fs.ReadFile(files, "index.html")
	if err != nil {
		return false
	}
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// No modification time: the page changes with what it is about, not only with the file.
	http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(withHead(page, head)))
	return true
}

// serveFile answers with the file name of files and reports whether there is such a file.
func serveFile(w http.ResponseWriter, r *http.Request, files fs.FS, name string) bool {
	f, err := files.Open(name)
	if err != nil {
		return false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	content, ok := f.(interface {
		Read([]byte) (int, error)
		Seek(int64, int) (int64, error)
	})
	if !ok {
		return false
	}
	switch {
	case strings.HasPrefix(name, "assets/"):
		// Named by its content: it never changes, so it need never be asked for again.
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	case name == "index.html":
		// Names the current assets: always asked for again, so a release is seen at once.
		w.Header().Set("Cache-Control", "no-cache")
	default:
		w.Header().Set("Cache-Control", "public, max-age=3600")
	}
	// The image has no table of file types, and Go's own does not know this one.
	if path.Ext(name) == ".webmanifest" {
		w.Header().Set("Content-Type", "application/manifest+json")
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, name, info.ModTime(), content)
	return true
}
