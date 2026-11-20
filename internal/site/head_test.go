package site

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	xhtml "golang.org/x/net/html"
)

// The head of web/index.html as Vite builds it.
const builtIndex = `<!doctype html>
<html lang="zh-CN">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <meta name="theme-color" content="white" />
    <script>
      ;(() => { if (1 < 2 && '<meta name="description">') {} })()
    </script>
    <!-- The site's own words -->
    <meta
      name="description"
      content="OurTaikoNET：太鼓谱面社区。"
    />
    <meta property="og:site_name" content="OurTaikoNET" />
    <meta property="og:type" content="website" />
    <meta property="og:title" content="OurTaikoNET" />
    <meta property="og:image" content="https://net.ourtaiko.org/icons/icon-512.png" />
    <meta name="twitter:card" content="summary" />
    <link rel="icon" href="/favicon.ico" sizes="any" />
    <link rel="manifest" href="/site.webmanifest" />
    <title>OurTaikoNET</title>
    <script type="module" crossorigin src="/assets/index-2lfOheJm.js"></script>
  </head>
  <body>
    <div id="root"></div>
  </body>
</html>
`

type headTag struct{ key, value string }

// headTags reads a page's head as a reader of link cards does: each title, meta and canonical
// link with what it says.
func headTags(t *testing.T, page string) []headTag {
	t.Helper()
	doc, err := xhtml.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	var tags []headTag
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode {
			attr := map[string]string{}
			for _, a := range n.Attr {
				attr[a.Key] = a.Val
			}
			switch {
			case n.Data == "title" && n.FirstChild != nil:
				tags = append(tags, headTag{"title", n.FirstChild.Data})
			case n.Data == "title":
				tags = append(tags, headTag{"title", ""})
			case n.Data == "meta" && attr["name"] != "":
				tags = append(tags, headTag{attr["name"], attr["content"]})
			case n.Data == "meta" && attr["property"] != "":
				tags = append(tags, headTag{attr["property"], attr["content"]})
			case n.Data == "link" && attr["rel"] == "canonical":
				tags = append(tags, headTag{"canonical", attr["href"]})
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return tags
}

// one gives the value of the page's only tag key, failing when there are none or several.
func one(t *testing.T, tags []headTag, key string) string {
	t.Helper()
	var found []string
	for _, tag := range tags {
		if tag.key == key {
			found = append(found, tag.value)
		}
	}
	if len(found) != 1 {
		t.Fatalf("%s: %d tags %q", key, len(found), found)
	}
	return found[0]
}

func TestThePageHeadReplacesTheSitesOwnTags(t *testing.T) {
	head := headFor(pageMeta{title: `A "quoted" <b>song</b> & co`, description: "Line one\n\nline two", image: "/api/v2/sources/fanmade/charts/abc/cover", kind: "music.song"},
		headAreas["fanmade"], "https://net.ourtaiko.org", "/fanmade/charts/abc")
	page := string(withHead([]byte(builtIndex), head))
	tags := headTags(t, page)
	want := map[string]string{
		"title":               `A "quoted" <b>song</b> & co | OurTaikoNET · 自制`,
		"description":         "Line one line two",
		"robots":              "index, follow",
		"canonical":           "https://net.ourtaiko.org/fanmade/charts/abc",
		"og:site_name":        "OurTaikoNET · 自制",
		"og:title":            `A "quoted" <b>song</b> & co | OurTaikoNET · 自制`,
		"og:description":      "Line one line two",
		"og:type":             "music.song",
		"og:url":              "https://net.ourtaiko.org/fanmade/charts/abc",
		"og:image":            "https://net.ourtaiko.org/api/v2/sources/fanmade/charts/abc/cover",
		"og:locale":           "zh_CN",
		"twitter:card":        "summary_large_image",
		"twitter:title":       `A "quoted" <b>song</b> & co | OurTaikoNET · 自制`,
		"twitter:description": "Line one line two",
		"twitter:image":       "https://net.ourtaiko.org/api/v2/sources/fanmade/charts/abc/cover",
		// What is not the page's stays.
		"viewport":    "width=device-width, initial-scale=1.0",
		"theme-color": "white",
	}
	for key, value := range want {
		if got := one(t, tags, key); got != value {
			t.Errorf("%s = %q, want %q", key, got, value)
		}
	}
	if len(tags) != len(want) {
		t.Errorf("%d tags, want %d: %q", len(tags), len(want), tags)
	}
	// Escaped, not taken for markup; the charset first; the script and the rest untouched.
	if strings.Contains(page, "<b>") || !strings.Contains(page, "&lt;b&gt;") {
		t.Error("the title is not escaped:", page)
	}
	if !strings.HasPrefix(strings.SplitN(page, "<head>", 2)[1], "\n    <meta charset") {
		t.Error("the charset is no longer first:", page)
	}
	for _, kept := range []string{`'<meta name="description">'`, `<!-- The site's own words -->`, `<link rel="icon" href="/favicon.ico" sizes="any" />`, `<script type="module" crossorigin src="/assets/index-2lfOheJm.js"></script>`, `<div id="root"></div>`} {
		if !strings.Contains(page, kept) {
			t.Errorf("%s is gone:\n%s", kept, page)
		}
	}
	// The new tags stand where the description stood, one to a line, nothing left blank.
	if !strings.Contains(page, "<!-- The site's own words -->\n    <title>") || strings.Contains(page, "\n\n") || strings.Contains(page, "\n    \n") {
		t.Error("the tags are not in place:\n", page)
	}
}

// The website's own index.html, as it is in the repository: every tag it has is replaced, none
// added twice.
func TestThePageHeadFitsTheWebsitesIndex(t *testing.T) {
	source, err := os.ReadFile("../../web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	head := headFor(pageMeta{title: "Song"}, headAreas["official"], "https://net.ourtaiko.org", "/official/charts/1")
	tags := headTags(t, string(withHead(source, head)))
	seen := map[string]int{}
	for _, tag := range tags {
		seen[tag.key]++
	}
	for key, n := range seen {
		if n != 1 {
			t.Errorf("%d %s tags", n, key)
		}
	}
	if got := one(t, tags, "og:title"); got != "Song | OurTaikoNET · 官方" {
		t.Error(got)
	}
}

func TestAPageWithoutAHeadIsLeftAsItIs(t *testing.T) {
	head := headFor(pageMeta{title: "x"}, headAreas["players"], "https://net.test", "/players/x")
	for _, page := range []string{"<!doctype html>page", ""} {
		if got := string(withHead([]byte(page), head)); got != page {
			t.Errorf("%q became %q", page, got)
		}
	}
	// A head with none of the tags gets them before it ends.
	got := string(withHead([]byte("<html>\n  <head>\n    <meta charset=\"UTF-8\" />\n  </head>\n</html>"), head))
	if !strings.Contains(got, "    <title>x | OurTaikoNET</title>\n") || !strings.HasSuffix(got, "  </head>\n</html>") {
		t.Error(got)
	}
}

func TestSummarizeIsTheWebsites(t *testing.T) {
	if got := summarize("a\n\n b\tc　d ", descriptionLimit); got != "a b c d" {
		t.Error(got)
	}
	cut := summarize(strings.Repeat("word ", 60), descriptionLimit)
	if len(cut) > descriptionLimit+2 || !strings.HasSuffix(cut, "word…") {
		t.Error(cut)
	}
	cut = summarize(strings.Repeat("谱", 300), descriptionLimit)
	if n := len([]rune(cut)); n != descriptionLimit || !strings.HasSuffix(cut, "…") {
		t.Error(n, cut)
	}
}

// fakeAPI answers the site's API with the answers given by path; 404 for any other.
func fakeAPI(answers map[string]any) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(r.Cookies()) > 0 {
			w.WriteHeader(http.StatusTeapot) // the lookup is no one's
			return
		}
		answer, ok := answers[r.URL.EscapedPath()]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"code":"NOT_FOUND"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(answer)
	})
}

func pageTags(t *testing.T, h http.Handler, address string) (*httptest.ResponseRecorder, []headTag) {
	t.Helper()
	r := httptest.NewRequest("GET", address, nil)
	r.AddCookie(&http.Cookie{Name: "session", Value: "someone"})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("%s: %d, cache %q", address, w.Code, w.Header().Get("Cache-Control"))
	}
	return w, headTags(t, w.Body.String())
}

func TestEachPageSpeaksForItselfToLinkCards(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "index.html", builtIndex)
	api := fakeAPI(map[string]any{
		"/api/v2/sources/fanmade/charts/123": map[string]any{
			"id": "0123456789abcdef0123456789abcdef", "title": "Senbonzakura", "titleTranslations": map[string]string{"zh": "千本樱", "origin": "千本桜"},
			"description": "", "maker": " Kirisame ", "coverHash": "c0ffee",
			"songCoverPath": "fanmade/c0ffee.webp",
			"difficulties":  []map[string]any{{"course": "Oni", "side": "single", "level": 9}, {"course": "Edit", "side": "p1", "level": 10}},
		},
		"/api/v2/sources/official/charts/0123456789abcdef0123456789abcdef": map[string]any{
			"id": "0123456789abcdef0123456789abcdef", "title": "夏祭り", "description": "  An official song.\n", "maker": "", "difficulties": []any{},
		},
		"/api/v2/sources/fanmade/charts/456": map[string]any{
			"id": "456", "title": "Tentai Kansoku", "titleTranslations": map[string]string{"ja": "天体観測", "en": "Stargazing"}, "difficulties": []any{},
		},
		"/api/v2/sources/fanmade/playlists/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa": map[string]any{
			"id": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "name": "Night & Day", "description": "", "public": true, "coverHash": "beef",
			"songListCoverPath": "fanmade/beef.webp", "chartCount": 12,
			"owner": map[string]string{"nickname": "Don"},
		},
		"/api/v2/sources/official/playlists/daily": map[string]any{
			"id": "daily", "name": "Daily", "description": "Ten songs a day", "system": true, "chartCount": 10, "owner": nil,
		},
		"/api/v2/sources/fanmade/playlists/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb": map[string]any{
			"id": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "name": "Secret", "public": false, "chartCount": 1,
		},
		"/api/v2/sources/official/dans/dddddddddddddddddddddddddddddddd": map[string]any{
			"id": "dddddddddddddddddddddddddddddddd", "title": map[string]string{"ja": "十段", "zh": "十段（中）"}, "owner": "",
			"songs": []map[string]any{{"title": "A", "titleTranslations": map[string]string{"zh": "甲"}}, {"title": "B"}},
		},
		"/api/v2/sources/fanmade/dans/eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee": map[string]any{
			"id": "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", "title": map[string]string{"en": "Tenth", "ko": "십단"}, "owner": "Ka",
			"songs": []map[string]any{{"title": "A"}, {"title": "", "hidden": true}, {"title": "C"}},
		},
		"/api/v2/users/de000000000000000000000000000001/nameplate": map[string]any{
			"userId": "de000000000000000000000000000001", "nickname": "Wada", "avatarPath": "sso/avatars/1.webp",
		},
		"/api/v2/users/de000000000000000000000000000002/nameplate": map[string]any{
			"userId": "de000000000000000000000000000002", "nickname": "", "avatarPath": "",
		},
	})
	h := Web(dir, "https://net.ourtaiko.org", api)
	type want map[string]string
	for address, w := range map[string]want{
		"/fanmade/charts/123?tab=chart": {
			"title":        "千本樱 | OurTaikoNET · 自制",
			"description":  "Kirisame 制作的「千本樱」太鼓谱面（魔王 ★9 / 里谱 1P ★10）。在 OurTaikoNET 试听、下载，查看排行榜与评论。",
			"canonical":    "https://net.ourtaiko.org/fanmade/charts/123",
			"og:type":      "music.song",
			"og:image":     "https://distrib.ourtaiko.org/fanmade/c0ffee.webp",
			"twitter:card": "summary_large_image",
		},
		"/fanmade/charts/123/comments/77": {
			"title":     "千本樱 | OurTaikoNET · 自制",
			"canonical": "https://net.ourtaiko.org/fanmade/charts/123/comments/77",
		},
		"/official/charts/0123456789abcdef0123456789abcdef": {
			"title":        "夏祭り | OurTaikoNET · 官方",
			"description":  "An official song.",
			"og:image":     "https://net.ourtaiko.org/icons/icon-512.png",
			"twitter:card": "summary",
			"og:site_name": "OurTaikoNET · 官方",
		},
		"/fanmade/playlists/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa": {
			"title":        "Night & Day | OurTaikoNET · 自制",
			"description":  "Don 整理的歌单「Night & Day」：12 首歌曲。在 OurTaikoNET 试听与游玩。",
			"og:type":      "music.playlist",
			"og:image":     "https://distrib.ourtaiko.org/fanmade/beef.webp",
			"twitter:card": "summary_large_image",
		},
		"/official/playlists/daily": {
			"title":       "今日课题 | OurTaikoNET · 官方",
			"description": "每天随机抽取的 10 首歌曲。",
		},
		"/official/dans/dddddddddddddddddddddddddddddddd": {
			"title":       "十段（中） | OurTaikoNET · 官方",
			"description": "段位道场「十段（中）」，由 未知用户 制作：甲 / B。",
			"og:type":     "website",
		},
		"/fanmade/dans/eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee": {
			"title":       "Tenth | OurTaikoNET · 自制",
			"description": "段位道场「Tenth」，由 Ka 制作：3 首歌曲。",
		},
		"/players/de000000000000000000000000000001": {
			"title":        "Wada | OurTaikoNET",
			"description":  "Wada 在 OurTaikoNET 的玩家主页：成绩、作品、歌单、段位与称号。",
			"og:type":      "profile",
			"og:image":     "https://distrib.ourtaiko.org/sso/avatars/1.webp",
			"twitter:card": "summary",
		},
		"/players/de000000000000000000000000000002": {
			"title":       "OurTaikoNET",
			"description": headAreas["players"].description,
			"og:image":    "https://net.ourtaiko.org/icons/icon-512.png",
		},
	} {
		_, tags := pageTags(t, h, address)
		for key, value := range w {
			if got := one(t, tags, key); got != value {
				t.Errorf("%s %s = %q, want %q", address, key, got, value)
			}
		}
		if one(t, tags, "og:title") != one(t, tags, "title") || one(t, tags, "og:url") != one(t, tags, "canonical") {
			t.Errorf("%s: the tags disagree: %q", address, tags)
		}
	}
	// A song without a Chinese name is named in the first language the browser asks for that
	// it has, as the page names it (chartText); the page says so to caches.
	for header, title := range map[string]string{"": "Tentai Kansoku", "fr, ja;q=0.5": "天体観測", "ko,en;q=0.9,ja;q=0.8": "Stargazing"} {
		r := httptest.NewRequest("GET", "/fanmade/charts/456", nil)
		r.Header.Set("Accept-Language", header)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if got := one(t, headTags(t, w.Body.String()), "title"); got != title+" | OurTaikoNET · 自制" || w.Header().Get("Vary") != "Accept-Language" {
			t.Errorf("Accept-Language %q: %q, Vary %q", header, got, w.Header().Get("Vary"))
		}
	}
	// What a visitor who is not signed in may not see, what is not found, and every other page:
	// index.html as it is.
	for _, address := range []string{
		"/fanmade/playlists/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "/fanmade/charts/404", "/official/dans/new",
		"/fanmade/charts/123/edit", "/fanmade/me/playlists", "/players", "/fanmade/", "/titles", "/elsewhere/charts/123",
	} {
		w, _ := pageTags(t, h, address)
		if w.Body.String() != builtIndex {
			t.Errorf("%s is not the plain page:\n%s", address, w.Body.String())
		}
	}
}

func TestASlowOrBrokenLookupGivesThePlainPage(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "index.html", builtIndex)
	var finished atomic.Bool
	slow := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(5 * time.Second):
		case <-r.Context().Done():
		}
		finished.Store(true)
		io.WriteString(w, `{"id":"x","title":"late"}`)
	})
	start := time.Now()
	w, _ := pageTags(t, Web(dir, "", slow), "/fanmade/charts/1")
	if took := time.Since(start); took > headLookupTime+time.Second || w.Body.String() != builtIndex {
		t.Fatalf("took %v: %s", took, w.Body.String())
	}
	broken := map[string]http.Handler{
		"panics":   http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { panic("no") }),
		"not JSON": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "<html>") }),
		"fails":    http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "down", 500) }),
		"too big": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, `{"id":"x","title":"`+strings.Repeat("a", headLookupLimit)+`"}`)
		}),
	}
	for name, api := range broken {
		w, _ := pageTags(t, Web(dir, "", api), "/fanmade/charts/1")
		if !bytes.Equal(w.Body.Bytes(), []byte(builtIndex)) {
			t.Errorf("%s: %s", name, w.Body.String())
		}
	}
	// Without a configured origin, the address the page was asked at.
	w, tags := pageTags(t, Web(dir, "", fakeAPI(map[string]any{"/api/v2/users/u/nameplate": map[string]string{"userId": "u", "nickname": "N"}})), "/players/u")
	if got := one(t, tags, "canonical"); got != "http://example.com/players/u" {
		t.Error(got, w.Body.String())
	}
}

// The words above are the website's, in Chinese: a change to its locale files must be made
// here too.
func TestHeadWordsAreTheWebsites(t *testing.T) {
	read := func(file string) map[string]any {
		t.Helper()
		data, err := os.ReadFile("../../web/src/" + file)
		if err != nil {
			t.Fatal(err)
		}
		var words map[string]any
		if err := json.Unmarshal(data, &words); err != nil {
			t.Fatal(file, err)
		}
		return words
	}
	kit := read("kit/locales/zh-Hans.json")
	areas := map[string]map[string]any{
		"fanmade":  read("sources/fanmade/locales/zh-Hans.json"),
		"official": read("sources/official/locales/zh-Hans.json"),
		"players":  read("players/locales/zh-Hans.json"),
	}
	lookup := func(words map[string]any, key string) (string, bool) {
		var at any = words
		for _, part := range strings.Split(key, ".") {
			m, ok := at.(map[string]any)
			if !ok {
				return "", false
			}
			at = m[part]
		}
		s, ok := at.(string)
		return s, ok
	}
	// An area's own word wins over the shared one, as web/src/kit/i18n.ts merges them.
	in := func(area, key string) string {
		if s, ok := lookup(areas[area], key); ok {
			return s
		}
		s, ok := lookup(kit, key)
		if !ok {
			t.Errorf("%s: no %s", area, key)
		}
		return s
	}
	for area, words := range headAreas {
		if got := in(area, "common.siteTitle"); got != words.name {
			t.Errorf("%s name %q, the website's %q", area, words.name, got)
		}
		if got := in(area, "meta.siteDescription"); got != words.description {
			t.Errorf("%s description %q, the website's %q", area, words.description, got)
		}
		for key, value := range headWords {
			if got := in(area, key); got != value {
				t.Errorf("%s %s = %q, the website's %q", area, key, value, got)
			}
		}
	}
}
