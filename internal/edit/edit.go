// Package edit serves OurTaikoEdit's web version (the chart editor, a separate repository) as a
// bare page at /edit/app/, which the website shows inside its own frame: at /edit (the editor with
// the person's projects in the browser) and on a chart's edit page, where the page hands the chart
// to the editor and stores what it saves (web/src/kit/editor-frame.tsx).
//
// OurTaikoEdit publishes each build of its web version to the public bucket as a folder that
// never changes, edit/<build id>/, and names the current one in edit-nightly.json (its
// build/publish-web.py). /edit/app/ answers with that build's index.html and a <base> pointing at
// the build's folder on the visitor's root: the mirror for a mainland visitor, the canonical CDN for
// everyone else (package distribution). The page's scripts, styles and WebAssembly then come from
// there, with CORS, and only the page itself from this site; the editor's projects are kept in
// the browser's storage for this site's origin.
package edit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"ourtaiko.dev/net/internal/kit/distribution"
)

const (
	// Prefix is where the bare editor is on the site.
	Prefix = "/edit/app/"
	// manifestTTL is how long a manifest read is used before it is read again.
	manifestTTL = time.Minute
	maxManifest = 1 << 20
	maxPage     = 64 << 10
)

var buildPath = regexp.MustCompile(`^/edit/[0-9a-f]{16}/index\.html$`)

// Config says where the builds are.
type Config struct {
	// Root is the canonical root the manifest and the builds are read from ("https://…/").
	Root string
	// Manifest is the manifest's path under Root ("edit-nightly.json").
	Manifest string
	// Router chooses a mainland visitor's root; nil sends everyone to Root.
	Router *distribution.Router
	// Client reads Root; nil is a client with a ten-second timeout.
	Client *http.Client
}

type server struct {
	cfg  Config
	next http.Handler

	mu      sync.Mutex
	path    string // the current build's index.html, from the manifest
	readAt  time.Time
	pages   map[string][]byte // a build's index.html by its path: it never changes
	reading *sync.Mutex
}

// Handler serves /edit/app/ (and sends /edit/app there); every other address, /edit itself
// included, goes to next.
func Handler(cfg Config, next http.Handler) http.Handler {
	if cfg.Client == nil {
		cfg.Client = &http.Client{Timeout: 10 * time.Second}
	}
	cfg.Root = strings.TrimRight(cfg.Root, "/") + "/"
	return &server{cfg: cfg, next: next, pages: map[string][]byte{}, reading: &sync.Mutex{}}
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == strings.TrimSuffix(Prefix, "/"):
		target := Prefix
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, target, http.StatusMovedPermanently)
	case r.URL.Path == Prefix:
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.page(w, r)
	case strings.HasPrefix(r.URL.Path, Prefix):
		// The build's own files are on the CDN, never here.
		http.NotFound(w, r)
	default:
		s.next.ServeHTTP(w, r)
	}
}

func (s *server) page(w http.ResponseWriter, r *http.Request) {
	path, page, err := s.current(r.Context())
	if err != nil {
		http.Error(w, "The editor is not available right now", http.StatusServiceUnavailable)
		return
	}
	root := s.cfg.Router.MirrorBase(r)
	if root == "" {
		root = s.cfg.Root
	}
	base := root + strings.TrimPrefix(strings.TrimSuffix(path, "index.html"), "/")
	body := withBase(page, base)
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	// The page names the current build and the visitor's root: never kept by a shared cache.
	h.Set("Cache-Control", "private, no-cache")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Length", fmt.Sprint(len(body)))
	if r.Method == http.MethodHead {
		return
	}
	w.Write(body)
}

// current is the current build's index.html path and page. A manifest is read again after
// manifestTTL; when that read fails, the build last read is kept.
func (s *server) current(ctx context.Context) (string, []byte, error) {
	s.reading.Lock()
	defer s.reading.Unlock()
	s.mu.Lock()
	path, fresh := s.path, time.Since(s.readAt) < manifestTTL
	s.mu.Unlock()
	if !fresh {
		read, err := s.readManifest(ctx)
		s.mu.Lock()
		switch {
		case err == nil:
			path = read
			s.path, s.readAt = read, time.Now()
		case path == "":
			s.mu.Unlock()
			return "", nil, err
		default:
			// Try again shortly, not on every request.
			s.readAt = time.Now().Add(-manifestTTL + 10*time.Second)
		}
		s.mu.Unlock()
	}
	s.mu.Lock()
	page, ok := s.pages[path]
	s.mu.Unlock()
	if ok {
		return path, page, nil
	}
	page, err := s.get(ctx, strings.TrimPrefix(path, "/"), maxPage)
	if err != nil {
		return "", nil, err
	}
	s.mu.Lock()
	s.pages[path] = page
	s.mu.Unlock()
	return path, page, nil
}

func (s *server) readManifest(ctx context.Context) (string, error) {
	body, err := s.get(ctx, s.cfg.Manifest, maxManifest)
	if err != nil {
		return "", err
	}
	var manifest struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(body, &manifest); err != nil {
		return "", fmt.Errorf("edit manifest: %w", err)
	}
	if !buildPath.MatchString(manifest.Path) {
		return "", fmt.Errorf("edit manifest: path %q is not a build's index.html", manifest.Path)
	}
	return manifest.Path, nil
}

// get reads path under Root.
func (s *server) get(ctx context.Context, path string, limit int64) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, s.cfg.Root+path, nil)
	if err != nil {
		return nil, err
	}
	response, err := s.cfg.Client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", request.URL, response.Status)
	}
	// OurTaikoEdit stores the page and the manifest as they are (its other files are Brotli).
	if encoding := response.Header.Get("Content-Encoding"); encoding != "" && encoding != "identity" {
		return nil, fmt.Errorf("%s: stored with Content-Encoding %s", request.URL, encoding)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, errors.New(request.URL.String() + ": too large")
	}
	return body, nil
}

// withBase is page with <base href=base> as the first thing in its head.
func withBase(page []byte, base string) []byte {
	tag := `<base href="` + strings.ReplaceAll(base, `"`, "%22") + `">`
	text := string(page)
	at := strings.Index(strings.ToLower(text), "<head>")
	if at < 0 {
		return []byte(tag + text)
	}
	at += len("<head>")
	return []byte(text[:at] + tag + text[at:])
}
