package apiv1

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"ourtaiko.dev/net/internal/files"
	"ourtaiko.dev/net/internal/web"
)

// The version-free API answers files as paths under the distribution root (package files),
// with names that say what they are: songCoverPath, songListCoverPath, avatarPath,
// authorAvatarPath, 3dCharaPath, a resource's path. Version 1 answered full addresses under
// its own names, a mainland visitor's on the mirror. converter turns the one into the other;
// the names it looks for are this API's own, never a user's text.
type converter struct {
	cfg Config
	// The request answered, as the client sent it (/api/v1/…); nil for an answer that goes to
	// no visitor in particular (canonical addresses).
	r *http.Request
	// varies is set when an address depends on where the request came from; changed when the
	// answer is not the version-free one (otherwise it is sent as it came, byte for byte).
	varies, changed bool
}

// convert is body, a version-free JSON answer, as version 1 gives it; body itself when it is
// not JSON this code can read.
func (c *converter) convert(body []byte) []byte {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var v any
	if decoder.Decode(&v) != nil {
		return body
	}
	if m, ok := v.(map[string]any); ok && c.r != nil {
		switch {
		case strings.HasSuffix(c.r.URL.Path, "/don/catalog"):
			c.catalog(m, "3dchara/")
		case strings.HasSuffix(c.r.URL.Path, "/nameplate-art/catalog"):
			c.catalog(m, "nameplate/")
		}
	}
	c.walk(v)
	if !c.changed {
		return body
	}
	var out bytes.Buffer
	if json.NewEncoder(&out).Encode(v) != nil {
		return body
	}
	return out.Bytes()
}

func (c *converter) walk(v any) {
	switch v := v.(type) {
	case map[string]any:
		c.object(v)
		for _, child := range v {
			c.walk(child)
		}
	case []any:
		for _, child := range v {
			c.walk(child)
		}
	}
}

// root is the root the visitor reads files from (regional.go).
func (c *converter) root() string {
	if c.cfg.Router.Regional() {
		c.varies = true
	}
	return root(c.cfg.Router, c.r)
}

// song is a song file's address for the visitor, and the canonical one when they differ.
func (c *converter) song(p string) (address, fallback string) {
	canonical := files.URL("", p)
	if c.cfg.Router.Regional() {
		c.varies = true
	}
	if selected := songAddress(c.cfg.Router, c.r, canonical); selected != canonical {
		return selected, canonical
	}
	return canonical, ""
}

func str(m map[string]any, key string) (string, bool) {
	s, ok := m[key].(string)
	return s, ok
}

func (c *converter) object(m map[string]any) {
	before := len(m)
	defer func() {
		if len(m) != before {
			c.changed = true
		}
	}()
	if p, ok := str(m, "songCoverPath"); ok {
		c.changed = true
		delete(m, "songCoverPath")
		if !c.cfg.FilesOnDisk {
			m["coverUrl"] = files.URL("", p)
		}
	}
	if p, ok := str(m, "songListCoverPath"); ok {
		c.changed = true
		delete(m, "songListCoverPath")
		if digest, game := m["songListCoverSha256"]; game {
			// The game's folder: a downloadable link, as the resources' (below).
			size := m["songListCoverSize"]
			delete(m, "songListCoverSha256")
			delete(m, "songListCoverSize")
			if !c.cfg.FilesOnDisk && p != "" {
				address, fallback := c.song(p)
				cover := map[string]any{"url": address, "headUrl": address, "sha256": digest, "size": size, "contentType": "image/webp"}
				if fallback != "" {
					cover["fallbackUrl"] = fallback
				}
				m["cover"] = cover
			}
		} else if !c.cfg.FilesOnDisk && p != "" {
			m["coverUrl"] = files.URL("", p)
		}
	}
	if p, ok := str(m, "avatarPath"); ok {
		c.changed = true
		delete(m, "avatarPath")
		m["avatarUrl"] = files.URL("", p)
	}
	if p, ok := str(m, "authorAvatarPath"); ok {
		c.changed = true
		delete(m, "authorAvatarPath")
		if _, chart := m["ownerIds"]; chart {
			// A chart named its author the uploader.
			m["uploader"] = m["author"]
			delete(m, "author")
			m["uploaderAvatarUrl"] = files.URL("", p)
		} else {
			m["authorAvatarUrl"] = files.URL("", p)
		}
	}
	if preview, ok := m["audioPreview"].(map[string]any); ok {
		if id, ok := str(m, "id"); ok {
			c.changed = true
			delete(preview, "path")
			preview["url"] = "/api/v1/charts/" + url.PathEscape(id) + "/audio"
		}
	}
	if p, ok := str(m, "3dCharaPath"); ok {
		c.changed = true
		delete(m, "3dCharaPath")
		address := files.URL(c.root(), p)
		if _, file := m["kind"]; file {
			m["url"] = address // a Don's file (/users/{id}/don)
		} else {
			m["picture"] = address // a Don part given as a reward
		}
	}
	if resources, ok := m["resources"].(map[string]any); ok {
		if chart, ok := str(m, "chartId"); ok {
			for kind, entry := range resources {
				if link, ok := entry.(map[string]any); ok {
					c.resource(chart, kind, link)
				}
			}
			m["urlsExpire"] = false
			c.changed = true
		}
	}
}

// resource is one of a chart's files as version 1 linked it: the bucket's address (the
// mirror's for a mainland visitor, with the canonical one to fall back to), or this API's own
// route when the files are kept on the server's disk.
func (c *converter) resource(chart, kind string, link map[string]any) {
	p, _ := str(link, "path")
	delete(link, "path")
	address, fallback := "", ""
	if c.cfg.FilesOnDisk {
		// The source's own routes, taken from the request's path (/api/v1/sources/fanmade).
		base := "/api/v1"
		if c.r != nil {
			if i := strings.Index(c.r.URL.Path, "/charts/"); i >= 0 {
				base = c.r.URL.Path[:i]
			}
		}
		origin := c.cfg.Origin
		if c.r != nil {
			// A shared cache keeps answers of each host apart already: no need to vary.
			origin = web.Origin(c.r.Context(), origin)
		}
		address = origin + base + "/charts/" + chart + "/" + kind
	} else {
		address, fallback = c.song(p)
	}
	link["url"], link["headUrl"] = address, address
	if fallback != "" {
		link["fallbackUrl"] = fallback
	}
}

// catalog is a 3D Don or nameplate catalog as version 1 gave it: its files' paths relative to
// the catalog's folder, the folder's canonical address as baseUrl and, for a mainland visitor,
// the mirror's as mirrorBaseUrl with every path it holds as mirrorPaths.
func (c *converter) catalog(m map[string]any, folder string) {
	var paths []string
	unroot := func(ref any) {
		if file, ok := ref.(map[string]any); ok {
			if p, ok := str(file, "path"); ok {
				p = strings.TrimPrefix(p, folder)
				file["path"] = p
				paths = append(paths, p)
			}
		}
	}
	for _, key := range []string{"base", "files"} {
		if refs, ok := m[key].(map[string]any); ok {
			for _, ref := range refs {
				unroot(ref)
			}
		}
	}
	if parts, ok := m["items"].(map[string]any); ok {
		for _, items := range parts {
			list, _ := items.([]any)
			for _, it := range list {
				if item, ok := it.(map[string]any); ok {
					unroot(item["file"])
					unroot(item["icon"])
				}
			}
		}
	}
	c.changed = true
	m["baseUrl"] = files.Root + folder
	if root := c.root(); root != files.Root {
		slices.Sort(paths)
		m["mirrorBaseUrl"], m["mirrorPaths"] = root+folder, slices.Compact(paths)
	}
}

// ConvertJSON is body, a version-free JSON answer, as version 1 gives it to no visitor in
// particular: every address canonical. Online rooms hand charts to released games this way.
func ConvertJSON(body []byte) []byte {
	trailing := bytes.HasSuffix(body, []byte("\n"))
	out := (&converter{}).convert(body)
	if !trailing {
		out = bytes.TrimSuffix(out, []byte("\n"))
	}
	return out
}
