package site

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"ourtaiko.dev/net/internal/files"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf16"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// What a page says about itself to the readers that do not run the website's script: the
// link-card makers of QQ, WeChat, Discord, X, Telegram. The website writes the same tags in
// the browser (web/src/kit/head.tsx, usePageMeta); here the server writes them into index.html
// before sending it, for a song, a playlist, a 段位 course and a player. The values are the
// ones the website writes for a visitor whose language is the site's own, Chinese (an address
// says no language), so the script finds them already right.
//
// The page is looked up through the site's own API, as a visitor who is not signed in: what
// that visitor may not see (a private playlist) is not told. A lookup that fails or takes
// longer than headLookupTime gives index.html as it is.

// headLookupTime is the longest a page waits for what it is about.
const headLookupTime = 500 * time.Millisecond

// headLookupLimit is the largest answer read; a playlist lists its songs.
const headLookupLimit = 8 << 20

// descriptionLimit is head.tsx's DESCRIPTION_LIMIT, counted as JavaScript counts: in UTF-16.
const descriptionLimit = 160

const defaultImage = "/icons/icon-512.png"

// The website's words this needs, in Chinese. They are copies of the website's locale files
// (web/src/kit/locales/zh-Hans.json, and each area's own under web/src/sources/*/locales and
// web/src/players/locales); TestHeadWordsAreTheWebsites keeps them the same.
type headArea struct{ name, description string }

var headAreas = map[string]headArea{
	"fanmade":  {"OurTaikoNET · 自制", "OurTaikoNET 自制曲源：试听、下载玩家上传的太鼓自制谱面，查看排行榜、歌单和段位道场。"},
	"official": {"OurTaikoNET · 官方", "OurTaikoNET：太鼓自制谱面社区。试听、下载玩家创作的自制谱面，分享歌单，挑战段位道场，记录你的成绩与称号。"},
	"players":  {"OurTaikoNET", "OurTaikoNET：太鼓自制谱面社区。试听、下载玩家创作的自制谱面，分享歌单，挑战段位道场，记录你的成绩与称号。"},
}

var headWords = map[string]string{
	"meta.chart":                             "「{{title}}」的太鼓谱面（{{courses}}）。在 OurTaikoNET 试听、下载，查看排行榜与评论。",
	"meta.chartBy":                           "{{maker}} 制作的「{{title}}」太鼓谱面（{{courses}}）。在 OurTaikoNET 试听、下载，查看排行榜与评论。",
	"meta.playlist":                          "歌单「{{name}}」：{{songs}}。在 OurTaikoNET 试听与游玩。",
	"meta.playlistBy":                        "{{owner}} 整理的歌单「{{name}}」：{{songs}}。在 OurTaikoNET 试听与游玩。",
	"meta.dan":                               "段位道场「{{title}}」，由 {{owner}} 制作：{{songs}}。",
	"meta.player":                            "{{name}} 在 OurTaikoNET 的玩家主页：成绩、作品、歌单、段位与称号。",
	"messages.easy":                          "简单",
	"messages.normal":                        "普通",
	"messages.hard":                          "困难",
	"messages.oni":                           "魔王",
	"messages.uraOni":                        "里谱",
	"playlists.songCount_other":              "{{count}} 首歌曲",
	"dans.unknownOwner":                      "未知用户",
	"playlists.system.popular.name":          "大家都在玩",
	"playlists.system.popular.description":   "最近 7 天游玩次数最多的 20 首歌曲。",
	"playlists.system.daily.name":            "今日课题",
	"playlists.system.daily.description":     "每天随机抽取的 10 首歌曲。",
	"playlists.system.liked.name":            "我赞过的",
	"playlists.system.liked.description":     "你点过赞的歌曲，最近的排在前面。",
	"playlists.system.acclaimed.name":        "近期好评",
	"playlists.system.acclaimed.description": "最近 30 天上传的歌曲中评价最高的 20 首。",
}

// courseWords: courses.ts's names, by the COURSE the API gives.
var courseWords = map[string]string{"Easy": "messages.easy", "Normal": "messages.normal", "Hard": "messages.hard", "Oni": "messages.oni", "Edit": "messages.uraOni"}

// word is i18next's t for headWords: {{name}} replaced by its value.
func word(key string, values ...string) string {
	s := headWords[key]
	for i := 0; i+1 < len(values); i += 2 {
		s = strings.ReplaceAll(s, "{{"+values[i]+"}}", values[i+1])
	}
	return s
}

// pageMeta is head.tsx's PageMetaProps.
type pageMeta struct {
	title, description, image, kind string
}

// pageHead is what head.tsx's headFor makes of a page: the tags' values.
type pageHead struct {
	title, description, image, canonical, kind, robots, locale, card, siteName string
}

func headFor(meta pageMeta, area headArea, origin, escapedPath string) pageHead {
	title := area.name
	if t := jsTrim(meta.title); t != "" {
		title = t + " | " + area.name
	}
	description := jsTrim(meta.description)
	if description == "" {
		description = area.description
	}
	image := meta.image
	if image == "" {
		image = defaultImage
	}
	kind := meta.kind
	if kind == "" {
		kind = "website"
	}
	card := "summary"
	// A wide card for a cover; a player's square avatar sits better small.
	if meta.image != "" && kind != "profile" {
		card = "summary_large_image"
	}
	return pageHead{
		title:       title,
		description: summarize(description, descriptionLimit),
		image:       absolute(origin, image),
		// The page itself, without what a visitor chose on it (?tab, ?page, a search).
		canonical: absolute(origin, escapedPath),
		kind:      kind,
		robots:    "index, follow",
		locale:    "zh_CN",
		card:      card,
		siteName:  area.name,
	}
}

func absolute(origin, address string) string {
	base, err := url.Parse(origin + "/")
	if err != nil {
		return address
	}
	ref, err := url.Parse(address)
	if err != nil {
		return address
	}
	return base.ResolveReference(ref).String()
}

// jsSpace is JavaScript's \s, which trim() and the /\s+/ of summarize go by.
func jsSpace(r rune) bool {
	return r == '\uFEFF' || (r != '\u0085' && unicode.IsSpace(r))
}

func jsTrim(s string) string { return strings.TrimFunc(s, jsSpace) }

// summarize is head.tsx's: text on one line, at most limit characters (UTF-16 units, as
// JavaScript counts), cut at a word where it can be.
func summarize(text string, limit int) string {
	flat := strings.Join(strings.FieldsFunc(text, jsSpace), " ")
	units := utf16.Encode([]rune(flat))
	if len(units) <= limit {
		return flat
	}
	cut := units[:limit-1]
	if space := lastIndex(cut, ' '); float64(space) > float64(limit)*0.6 {
		cut = cut[:space]
	}
	return string(utf16.Decode(cut)) + "…"
}

func lastIndex(units []uint16, u uint16) int {
	for i := len(units) - 1; i >= 0; i-- {
		if units[i] == u {
			return i
		}
	}
	return -1
}

// pageLookup says which API answer a page is about and what the page says of it.
type pageLookup struct {
	area string
	api  string
	meta func(body []byte) (pageMeta, bool)
}

// lookupFor matches the website's routes (web/src/songs.tsx, web/src/players/app.tsx) for the
// pages that speak for something of their own. p is the address's path, decoded.
func lookupFor(p string) (pageLookup, bool) {
	parts := strings.Split(strings.Trim(p, "/"), "/")
	for _, part := range parts {
		if part == "" {
			return pageLookup{}, false
		}
	}
	// A comment's own address is its page's (charts/:id/comments/:commentId).
	commented := len(parts) == 3 || (len(parts) == 5 && parts[3] == "comments")
	switch {
	case len(parts) == 2 && parts[0] == "players":
		return pageLookup{"players", "/api/v2/users/" + url.PathEscape(parts[1]), playerMeta}, true
	case parts[0] != "fanmade" && parts[0] != "official" || len(parts) < 3:
		return pageLookup{}, false
	}
	source, kind, id := parts[0], parts[1], parts[2]
	api := "/api/v2/sources/" + source + "/" + kind + "/" + url.PathEscape(id)
	switch {
	case kind == "charts" && commented:
		return pageLookup{source, api, func(b []byte) (pageMeta, bool) { return chartMeta(source, b) }}, true
	case kind == "dans" && commented && id != "new":
		return pageLookup{source, api, danMeta}, true
	case kind == "playlists" && len(parts) == 3:
		return pageLookup{source, api, func(b []byte) (pageMeta, bool) { return playlistMeta(source, b) }}, true
	}
	return pageLookup{}, false
}

// chartMeta is pages.tsx's chartMeta.
func chartMeta(source string, body []byte) (pageMeta, bool) {
	var c struct {
		ID                string            `json:"id"`
		Title             string            `json:"title"`
		TitleTranslations map[string]string `json:"titleTranslations"`
		Description       string            `json:"description"`
		Maker             string            `json:"maker"`
		SongCoverPath     string            `json:"songCoverPath"`
		Difficulties      []struct {
			Course string `json:"course"`
			Side   string `json:"side"`
			Level  int    `json:"level"`
		} `json:"difficulties"`
	}
	if json.Unmarshal(body, &c) != nil || c.ID == "" {
		return pageMeta{}, false
	}
	// chart-language.ts's chartText for Chinese: the Chinese name, else the original.
	title := ""
	for _, v := range []string{c.TitleTranslations["zh"], c.TitleTranslations["origin"], c.Title} {
		if jsTrim(v) != "" {
			title = v
			break
		}
	}
	courses := make([]string, len(c.Difficulties))
	for i, d := range c.Difficulties {
		name := d.Course
		if key, ok := courseWords[d.Course]; ok {
			name = word(key)
		}
		switch d.Side {
		case "p1":
			name += " 1P"
		case "p2":
			name += " 2P"
		}
		courses[i] = name + " ★" + strconv.Itoa(d.Level)
	}
	all := strings.Join(courses, " / ")
	description := jsTrim(c.Description)
	if description == "" {
		if maker := jsTrim(c.Maker); maker != "" {
			description = word("meta.chartBy", "title", title, "maker", maker, "courses", all)
		} else {
			description = word("meta.chart", "title", title, "courses", all)
		}
	}
	meta := pageMeta{title: title, description: description, kind: "music.song"}
	// The cover's address in the public bucket; there is no API route for it.
	if c.SongCoverPath != "" {
		meta.image = files.URL("", c.SongCoverPath)
	}
	return meta, true
}

// playlistMeta is playlists.tsx's playlistMeta. A private playlist is its owner's alone: it
// is not told at all.
func playlistMeta(source string, body []byte) (pageMeta, bool) {
	var p struct {
		ID                string `json:"id"`
		Name              string `json:"name"`
		Description       string `json:"description"`
		Public            bool   `json:"public"`
		System            bool   `json:"system"`
		SongListCoverPath string `json:"songListCoverPath"`
		ChartCount        int    `json:"chartCount"`
		Owner             *struct {
			Nickname string `json:"nickname"`
		} `json:"owner"`
	}
	if json.Unmarshal(body, &p) != nil || p.ID == "" || (!p.Public && !p.System) {
		return pageMeta{}, false
	}
	// playlist-card.tsx's usePlaylistText: the system playlists in the page's language.
	name, text := p.Name, p.Description
	if _, ok := headWords["playlists.system."+p.ID+".name"]; ok && p.System {
		name, text = word("playlists.system."+p.ID+".name"), word("playlists.system."+p.ID+".description")
	}
	songs := word("playlists.songCount_other", "count", strconv.Itoa(p.ChartCount))
	description := jsTrim(text)
	if description == "" {
		if p.Owner != nil && p.Owner.Nickname != "" {
			description = word("meta.playlistBy", "name", name, "owner", p.Owner.Nickname, "songs", songs)
		} else {
			description = word("meta.playlist", "name", name, "songs", songs)
		}
	}
	meta := pageMeta{title: name, description: description, kind: "music.playlist"}
	// The cover's address in the public bucket; there is no API route for it.
	if p.SongListCoverPath != "" {
		meta.image = files.URL("", p.SongListCoverPath)
	}
	return meta, true
}

// danMeta is what dans.tsx's DanPage says.
func danMeta(body []byte) (pageMeta, bool) {
	var d struct {
		ID    string            `json:"id"`
		Title map[string]string `json:"title"`
		Owner string            `json:"owner"`
		Songs []struct {
			Title  string `json:"title"`
			Hidden bool   `json:"hidden"`
		} `json:"songs"`
	}
	if json.Unmarshal(body, &d) != nil || d.ID == "" {
		return pageMeta{}, false
	}
	// dan-title.ts's danTitle for Chinese: the Chinese name, else the Japanese, else any (in
	// the order of the answer, whose keys Go writes sorted).
	title := d.Title["zh"]
	if title == "" {
		title = d.Title["ja"]
	}
	if title == "" {
		keys := make([]string, 0, len(d.Title))
		for k := range d.Title {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if d.Title[k] != "" {
				title = d.Title[k]
				break
			}
		}
	}
	if title == "" {
		title = "—"
	}
	owner := d.Owner
	if owner == "" {
		owner = word("dans.unknownOwner")
	}
	// The songs by name, unless some are hidden: then only how many.
	titles := make([]string, len(d.Songs))
	hidden := false
	for i, s := range d.Songs {
		titles[i] = s.Title
		hidden = hidden || s.Hidden
	}
	songs := strings.Join(titles, " / ")
	if hidden {
		songs = word("playlists.songCount_other", "count", strconv.Itoa(len(d.Songs)))
	}
	return pageMeta{title: title, description: word("meta.dan", "title", title, "owner", owner, "songs", songs)}, true
}

// playerMeta is what players/pages/user.tsx's UserPage says.
func playerMeta(body []byte) (pageMeta, bool) {
	var p struct {
		User struct {
			ID         string `json:"id"`
			Nickname   string `json:"nickname"`
			AvatarPath string `json:"avatarPath"`
		} `json:"user"`
	}
	if json.Unmarshal(body, &p) != nil || p.User.ID == "" {
		return pageMeta{}, false
	}
	meta := pageMeta{image: files.URL("", p.User.AvatarPath), kind: "profile"}
	if name := p.User.Nickname; name != "" {
		meta.title, meta.description = name, word("meta.player", "name", name)
	}
	return meta, true
}

// lookUpHead asks api about the page at r's address, as a visitor who is not signed in, and
// gives what its tags say; false when the address is no such page or the answer did not come
// in time.
func lookUpHead(r *http.Request, api http.Handler, origin string) (pageHead, bool) {
	page, ok := lookupFor(r.URL.Path)
	if !ok {
		return pageHead{}, false
	}
	ctx, cancel := context.WithTimeout(r.Context(), headLookupTime)
	defer cancel()
	inner, err := http.NewRequestWithContext(ctx, http.MethodGet, page.api, nil)
	if err != nil {
		return pageHead{}, false
	}
	inner.RemoteAddr = "127.0.0.1:0"
	inner.Header.Set("Accept", "application/json")
	answer := &captured{header: http.Header{}}
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			if recover() != nil {
				answer.code = http.StatusInternalServerError
			}
		}()
		api.ServeHTTP(answer, inner)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		// The handler goes on with its own copy; nothing here reads it again.
		return pageHead{}, false
	}
	if answer.code != 0 && answer.code != http.StatusOK || answer.over {
		return pageHead{}, false
	}
	meta, ok := page.meta(answer.body.Bytes())
	if !ok {
		return pageHead{}, false
	}
	return headFor(meta, headAreas[page.area], origin, r.URL.EscapedPath()), true
}

// captured is a ResponseWriter that keeps the answer.
type captured struct {
	header http.Header
	code   int
	body   bytes.Buffer
	over   bool
}

func (c *captured) Header() http.Header { return c.header }
func (c *captured) WriteHeader(code int) {
	if c.code == 0 {
		c.code = code
	}
}
func (c *captured) Write(b []byte) (int, error) {
	if c.code == 0 {
		c.code = http.StatusOK
	}
	if c.body.Len()+len(b) > headLookupLimit {
		c.over = true
		return 0, io.ErrShortWrite
	}
	return c.body.Write(b)
}

// tags is the head as head.tsx's apply writes it, each line indented by indent.
func (h pageHead) tags(indent string) string {
	var b strings.Builder
	line := func(format string, values ...string) {
		escaped := make([]any, len(values))
		for i, v := range values {
			escaped[i] = html.EscapeString(v)
		}
		b.WriteString(indent)
		fmt.Fprintf(&b, format, escaped...)
		b.WriteString("\n")
	}
	line(`<title>%s</title>`, h.title)
	line(`<meta name="description" content="%s" />`, h.description)
	line(`<meta name="robots" content="%s" />`, h.robots)
	line(`<link rel="canonical" href="%s" />`, h.canonical)
	for _, p := range [][2]string{
		{"og:site_name", h.siteName}, {"og:title", h.title}, {"og:description", h.description},
		{"og:type", h.kind}, {"og:url", h.canonical}, {"og:image", h.image}, {"og:locale", h.locale},
	} {
		line(`<meta property="%s" content="%s" />`, p[0], p[1])
	}
	for _, n := range [][2]string{
		{"twitter:card", h.card}, {"twitter:title", h.title}, {"twitter:description", h.description}, {"twitter:image", h.image},
	} {
		line(`<meta name="%s" content="%s" />`, n[0], n[1])
	}
	return b.String()
}

// withHead is page with the tags head writes in place of its own: the title, the description,
// robots, canonical, and every og: and twitter: tag of its head go, and head's take the place
// of the first of them (or go last in the head). A page with no head is given back as it is.
func withHead(page []byte, head pageHead) []byte {
	type span struct{ from, to int }
	var gone []span
	z := xhtml.NewTokenizer(bytes.NewReader(page))
	offset, headEnd, titleFrom := 0, -1, -1
	inHead := false
scan:
	for {
		kind := z.Next()
		from := offset
		offset += len(z.Raw())
		switch kind {
		case xhtml.ErrorToken:
			break scan
		case xhtml.StartTagToken, xhtml.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			tag := atom.Lookup(name)
			switch {
			case tag == atom.Head:
				inHead = true
			case tag == atom.Body:
				break scan
			case !inHead:
			case tag == atom.Title && kind == xhtml.StartTagToken:
				titleFrom = from
			case tag == atom.Meta || tag == atom.Link:
				attrs := map[string]string{}
				for hasAttr {
					var k, v []byte
					k, v, hasAttr = z.TagAttr()
					attrs[string(k)] = string(v)
				}
				if ours(tag, attrs) {
					gone = append(gone, span{from, offset})
				}
			}
		case xhtml.EndTagToken:
			name, _ := z.TagName()
			switch atom.Lookup(name) {
			case atom.Title:
				if titleFrom >= 0 {
					gone = append(gone, span{titleFrom, offset})
					titleFrom = -1
				}
			case atom.Head:
				headEnd = from
				break scan
			}
		}
	}
	if headEnd < 0 {
		return page
	}
	// A tag alone on its line takes the line with it; the new tags are indented as the first
	// one that goes was, or one step in from </head>.
	indent := "    "
	whole := func(from, to int) (int, int, string, bool) {
		start := bytes.LastIndexByte(page[:from], '\n') + 1
		lead := string(page[start:from])
		end := bytes.IndexByte(page[to:], '\n')
		if strings.Trim(lead, " \t") != "" || end < 0 || strings.TrimSpace(string(page[to:to+end])) != "" {
			return from, to, "", false
		}
		return start, to + end + 1, lead, true
	}
	for i := range gone {
		from, to, lead, alone := whole(gone[i].from, gone[i].to)
		if i == 0 && alone {
			indent = lead
		}
		gone[i] = span{from, to}
	}
	at := headEnd
	if len(gone) > 0 {
		at = gone[0].from
	} else if start, _, lead, alone := whole(headEnd, headEnd+len("</head>")); alone {
		at, indent = start, lead+"  "
	}
	var out bytes.Buffer
	out.Grow(len(page) + 2048)
	out.Write(page[:at])
	out.WriteString(head.tags(indent))
	last := at
	for _, s := range gone {
		if s.from > last {
			out.Write(page[last:s.from])
		}
		last = max(last, s.to)
	}
	out.Write(page[last:])
	return out.Bytes()
}

// ours says whether a tag of the head is one withHead writes.
func ours(tag atom.Atom, attrs map[string]string) bool {
	if tag == atom.Link {
		return strings.EqualFold(attrs["rel"], "canonical")
	}
	name, property := strings.ToLower(attrs["name"]), strings.ToLower(attrs["property"])
	return name == "description" || name == "robots" || strings.HasPrefix(name, "twitter:") || strings.HasPrefix(property, "og:")
}
