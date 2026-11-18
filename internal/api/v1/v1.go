// Package apiv1 is version 1 of the API, at /api/v1/: the API as released clients of the game
// know it. It is the version-free API (songs, profile, site) mounted under its prefix (package
// mount), with its answers given version 1's way: files as full addresses (convert.go), a
// mainland visitor's on the mirror (regional.go). When no client uses it any more, this
// package and the line mounting it go.
package apiv1

import (
	"bytes"
	"net/http"
	"strings"

	"ourtaiko.dev/net/internal/api/mount"
	"ourtaiko.dev/net/internal/files"
	"ourtaiko.dev/net/internal/kit/distribution"
	"ourtaiko.dev/net/internal/web"
)

// Version is this API's name in its paths: /api/v1/.
const Version = "v1"

// Config is what version 1 needs besides the API itself.
type Config struct {
	// Router picks a visitor's root (nil: always the canonical one).
	Router *distribution.Router
	// FilesOnDisk is set when the song files are kept on this server's disk (development):
	// version 1 then linked to this API's own file routes, under Origin.
	FilesOnDisk bool
	Origin      string
	// SourceBase is the source's own API routes, for an old host that serves one source at
	// /api/v1/ (/api/v1/sources/fanmade); empty takes them from the request's path.
	SourceBase string
}

// Handler serves api, the version-free API, at /api/v1/.
func Handler(api http.Handler, cfg Config) http.Handler {
	mounted := mount.Mount(Version, api)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v1/distribution/") && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
			distributionRoutes(cfg.Router, w, r)
			return
		}
		// The answer is rewritten before it is sent, and compressed then.
		inner := r.Clone(r.Context())
		inner.Header.Del("Accept-Encoding")
		c := &capture{w: w, r: r, conv: &converter{cfg: cfg, r: r}}
		mounted.ServeHTTP(c, inner)
		c.finish()
	})
}

// capture passes an answer through, except one in JSON, which it holds to convert. A redirect to
// a song file goes to the visitor's root.
type capture struct {
	w         http.ResponseWriter
	r         *http.Request
	conv      *converter
	status    int
	decided   bool
	buffering bool
	body      bytes.Buffer
}

func (c *capture) Header() http.Header { return c.w.Header() }

func (c *capture) WriteHeader(status int) {
	if c.decided {
		return
	}
	c.decided, c.status = true, status
	h := c.w.Header()
	if location := h.Get("Location"); status >= 300 && status < 400 && strings.HasPrefix(location, files.Root) {
		h.Set("Location", songAddress(c.conv.cfg.Router, c.r, location))
	}
	if strings.HasPrefix(h.Get("Content-Type"), "application/json") && status != http.StatusNoContent && status != http.StatusNotModified && c.r.Method != http.MethodHead {
		c.buffering = true
		return
	}
	c.w.WriteHeader(status)
}

func (c *capture) Write(b []byte) (int, error) {
	if !c.decided {
		c.WriteHeader(http.StatusOK)
	}
	if c.buffering {
		return c.body.Write(b)
	}
	return c.w.Write(b)
}

func (c *capture) Flush() {
	if f, ok := c.w.(http.Flusher); ok && !c.buffering {
		f.Flush()
	}
}

func (c *capture) Unwrap() http.ResponseWriter { return c.w }

func (c *capture) finish() {
	if !c.decided {
		c.WriteHeader(http.StatusOK)
	}
	if !c.buffering {
		return
	}
	body := c.conv.convert(c.body.Bytes())
	h := c.w.Header()
	h.Del("Content-Length")
	if c.conv.varies && !strings.Contains(h.Get("Cache-Control"), "no-store") {
		// Addresses that depend on where the request came from are no one else's.
		if strings.HasSuffix(c.r.URL.Path, "/catalog") {
			h.Set("Cache-Control", "private, max-age=60")
		} else {
			h.Set("Cache-Control", "private, no-store")
		}
	}
	w, done := web.GzipJSON(c.w, c.r)
	defer done()
	w.WriteHeader(c.status)
	w.Write(body)
}
