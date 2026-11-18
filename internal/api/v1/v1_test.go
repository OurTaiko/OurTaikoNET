package apiv1

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"

	"ourtaiko.dev/net/internal/files"
	"ourtaiko.dev/net/internal/kit/distribution"
)

const mirror = "https://mirror.example/ourtaiko/"

func testRouter() *distribution.Router {
	return distribution.FromDonCatalog(mirror+"3dchara/catalog.json", func(ip netip.Addr) string {
		return map[string]string{"203.0.113.1": "CN", "203.0.113.2": "US"}[ip.String()]
	}, nil)
}

// serve answers path with the version-free body through version 1, from ip.
func serve(t *testing.T, cfg Config, path, ip, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/api/v1/") {
			t.Errorf("the API was asked %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Write([]byte(body))
	})
	r := httptest.NewRequest("GET", path, nil)
	r.RemoteAddr = ip + ":1"
	w := httptest.NewRecorder()
	Handler(api, cfg).ServeHTTP(w, r)
	var answer map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &answer); err != nil {
		t.Fatalf("%s: %v %s", path, err, w.Body)
	}
	return w, answer
}

func asJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestNamesAndAddresses(t *testing.T) {
	cfg := Config{Router: testRouter()}
	sha := strings.Repeat("c", 64)
	_, chart := serve(t, cfg, "/api/v1/sources/fanmade/charts/x", "203.0.113.1", `{"id":"x y","ownerIds":["o"],"author":"Don","authorAvatarPath":"sso/avatars/a/b.webp",
		"owners":[{"id":"o","nickname":"Don","avatarPath":""}],"songCoverPath":"fanmade/`+sha+`.webp","previewPath":"fanmade/p.ogg",
		"audioPreview":{"path":"fanmade/a.ogg","startSeconds":1},"description":"songCoverPath avatarPath"}`)
	want := `{"audioPreview":{"startSeconds":1,"url":"/api/v1/charts/x%20y/audio"},"coverUrl":"` + files.Root + `fanmade/` + sha + `.webp","description":"songCoverPath avatarPath","id":"x y",` +
		`"ownerIds":["o"],"owners":[{"avatarUrl":"","id":"o","nickname":"Don"}],"previewPath":"fanmade/p.ogg","uploader":"Don","uploaderAvatarUrl":"` + files.Root + `sso/avatars/a/b.webp"}`
	if got := asJSON(chart); got != want {
		t.Fatalf("chart\n%s\nwant\n%s", got, want)
	}
	// A comment's author keeps its name; a playlist's cover is its coverUrl.
	_, comment := serve(t, cfg, "/api/v1/sources/fanmade/comments/1", "203.0.113.2", `{"author":"Ka","authorAvatarPath":"sso/avatars/c/d.webp","playlist":{"songListCoverPath":"fanmade/l.webp"}}`)
	if asJSON(comment) != `{"author":"Ka","authorAvatarUrl":"`+files.Root+`sso/avatars/c/d.webp","playlist":{"coverUrl":"`+files.Root+`fanmade/l.webp"}}` {
		t.Fatal(asJSON(comment))
	}
}

func TestRegionalFiles(t *testing.T) {
	cfg := Config{Router: testRouter()}
	sha := strings.Repeat("a", 64)
	resources := `{"chartId":"x","resources":{"audio":{"path":"fanmade/` + sha + `.ogg","sha256":"` + sha + `","size":3,"contentType":"audio/ogg"},"tja":{"path":"fanmade/notes.tja","sha256":"` + sha + `","size":1,"contentType":"application/octet-stream"}}}`
	canonical := files.Root + "fanmade/" + sha + ".ogg"
	for _, tc := range []struct{ ip, query, url, fallback string }{
		{"203.0.113.2", "", canonical, ""},
		{"203.0.113.1", "", mirror + "fanmade/" + sha + ".ogg", canonical},
		{"203.0.113.1", "?origin=1", canonical, ""},
	} {
		w, answer := serve(t, cfg, "/api/v1/sources/fanmade/charts/x/resources"+tc.query, tc.ip, resources)
		audio := answer["resources"].(map[string]any)["audio"].(map[string]any)
		tja := answer["resources"].(map[string]any)["tja"].(map[string]any)
		if audio["url"] != tc.url || audio["headUrl"] != tc.url || audio["fallbackUrl"] != nilIfEmpty(tc.fallback) || audio["path"] != nil || answer["urlsExpire"] != false ||
			tja["url"] != files.Root+"fanmade/notes.tja" || w.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatalf("%+v: %s %v", tc, asJSON(answer), w.Header())
		}
	}
	// A Don's files and a part given as a reward come from the visitor's root.
	_, don := serve(t, cfg, "/api/v1/users/u/don", "203.0.113.1", `{"look":{},"files":[{"kind":"cos","id":0,"3dCharaPath":"3dchara/cos/0.glb","size":1,"sha256":"`+sha+`"}]}`)
	if file := don["files"].([]any)[0].(map[string]any); file["url"] != mirror+"3dchara/cos/0.glb" || file["3dCharaPath"] != nil {
		t.Fatal(asJSON(don))
	}
	_, reward := serve(t, cfg, "/api/v1/me/rewards", "203.0.113.2", `{"rewards":[{"kind":"don_item","data":{"part":"puchi","itemId":1,"3dCharaPath":"3dchara/puchi/1.png"}}]}`)
	if data := reward["rewards"].([]any)[0].(map[string]any)["data"].(map[string]any); data["picture"] != files.Root+"3dchara/puchi/1.png" {
		t.Fatal(asJSON(reward))
	}
	// The game's folder cover is a link, with the canonical one behind the mirror's.
	_, game := serve(t, cfg, "/api/v1/sources/fanmade/game/playlists", "203.0.113.1", `{"playlists":[{"id":"p","songListCoverPath":"fanmade/test/`+sha+`.webp","songListCoverSha256":"`+sha+`","songListCoverSize":9}]}`)
	cover := game["playlists"].([]any)[0].(map[string]any)
	if asJSON(cover) != `{"cover":{"contentType":"image/webp","fallbackUrl":"`+files.Root+`fanmade/test/`+sha+`.webp","headUrl":"`+mirror+`fanmade/test/`+sha+`.webp","sha256":"`+sha+`","size":9,"url":"`+mirror+`fanmade/test/`+sha+`.webp"},"id":"p"}` {
		t.Fatal(asJSON(cover))
	}
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func TestCatalogs(t *testing.T) {
	cfg := Config{Router: testRouter()}
	don := `{"version":1,"base":{"face":{"path":"3dchara/base/face.png","size":1,"sha256":"a"}},"items":{"cos":[{"id":0,"file":{"path":"3dchara/cos/0.glb","size":1,"sha256":"b"},"icon":{"path":"3dchara/icons/cos/0.png","size":1,"sha256":"c"}}]}}`
	w, overseas := serve(t, cfg, "/api/v1/don/catalog", "203.0.113.2", don)
	if overseas["baseUrl"] != files.Root+"3dchara/" || overseas["mirrorBaseUrl"] != nil || !strings.Contains(asJSON(overseas), `"path":"cos/0.glb"`) || w.Header().Get("Cache-Control") != "private, max-age=60" {
		t.Fatal(asJSON(overseas), w.Header())
	}
	_, mainland := serve(t, cfg, "/api/v1/don/catalog", "203.0.113.1", don)
	if mainland["mirrorBaseUrl"] != mirror+"3dchara/" || asJSON(mainland["mirrorPaths"]) != `["base/face.png","cos/0.glb","icons/cos/0.png"]` {
		t.Fatal(asJSON(mainland))
	}
	plate := `{"version":1,"files":{"shadow":{"path":"nameplate/shadow.png","size":1,"sha256":"a"}}}`
	_, art := serve(t, cfg, "/api/v1/nameplate-art/catalog", "203.0.113.1", plate)
	if asJSON(art) != `{"baseUrl":"`+files.Root+`nameplate/","files":{"shadow":{"path":"shadow.png","sha256":"a","size":1}},"mirrorBaseUrl":"`+mirror+`nameplate/","mirrorPaths":["shadow.png"],"version":1}` {
		t.Fatal(asJSON(art))
	}
	if _, art = serve(t, cfg, "/api/v1/nameplate-art/catalog?origin=1", "203.0.113.1", plate); art["mirrorBaseUrl"] != nil {
		t.Fatal("origin bypass ignored")
	}
}

// An answer with nothing of version 1's to change is sent as the API wrote it, byte for byte.
func TestUnchangedAnswers(t *testing.T) {
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"z":1,"a":"<b>"}` + "\n"))
	})
	w := httptest.NewRecorder()
	Handler(api, Config{}).ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/titles", nil))
	if w.Body.String() != `{"z":1,"a":"<b>"}`+"\n" {
		t.Fatal(w.Body.String())
	}
	if got := ConvertJSON([]byte(`{"songCoverPath":"fanmade/a.webp"}`)); string(got) != `{"coverUrl":"`+files.Root+`fanmade/a.webp"}` {
		t.Fatal(string(got))
	}
}

func TestFilesOnDisk(t *testing.T) {
	cfg := Config{FilesOnDisk: true, Origin: "http://localhost", SourceBase: "/api/v1/sources/fanmade"}
	_, answer := serve(t, cfg, "/api/v1/charts/x/resources", "203.0.113.2", `{"chartId":"x","resources":{"tja":{"path":"fanmade/a.tja","sha256":"a","size":1}}}`)
	if tja := answer["resources"].(map[string]any)["tja"].(map[string]any); tja["url"] != "http://localhost/api/v1/sources/fanmade/charts/x/tja" {
		t.Fatal(asJSON(answer))
	}
	_, chart := serve(t, Config{FilesOnDisk: true}, "/api/v1/sources/fanmade/charts/x", "203.0.113.2", `{"id":"x","songCoverPath":"fanmade/a.webp"}`)
	if _, has := chart["coverUrl"]; has {
		t.Fatal("a cover on disk has no address")
	}
}

// A song file's redirect goes to the visitor's root; only public content-addressed song files
// move.
func TestRedirectsAndSongAddresses(t *testing.T) {
	router := testRouter()
	digest := strings.Repeat("a", 64)
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, files.Root+"fanmade/"+digest+".ogg", http.StatusFound)
	})
	for ip, want := range map[string]string{"203.0.113.1": mirror + "fanmade/" + digest + ".ogg", "203.0.113.2": files.Root + "fanmade/" + digest + ".ogg"} {
		r := httptest.NewRequest("GET", "/api/v1/sources/fanmade/charts/x/audio", nil)
		r.RemoteAddr = ip + ":1"
		w := httptest.NewRecorder()
		Handler(api, Config{Router: router}).ServeHTTP(w, r)
		if w.Code != 302 || w.Header().Get("Location") != want {
			t.Fatalf("%s: %d %s", ip, w.Code, w.Header().Get("Location"))
		}
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "203.0.113.1:1"
	for _, collection := range []string{"fanmade", "database"} {
		for _, ext := range []string{"tja", "ogg", "mp3", "webp", "png", "jpg", "jpeg", "mp4"} {
			if got := songAddress(router, r, files.Root+collection+"/"+digest+"."+ext); got != mirror+collection+"/"+digest+"."+ext {
				t.Fatal(got)
			}
		}
	}
	for _, address := range []string{
		files.Root + "fanmade/" + digest + ".ogg?X-Amz-Signature=secret",
		files.Root + "fanmade/../database/" + digest + ".ogg",
		files.Root + "fanmade/%2e%2e/database/" + digest + ".ogg",
		files.Root + "fanmade/catalog.json", files.Root + "other/" + digest + ".ogg",
		"https://cdn.example/fanmade/" + digest + ".ogg", "http://distrib.ourtaiko.org/fanmade/" + digest + ".ogg",
		files.Root + "fanmade/" + digest + ".exe", files.Root + "fanmade/short.ogg",
	} {
		if got := songAddress(router, r, address); got != address {
			t.Fatalf("unsafe mapping: %s", got)
		}
	}
}

func TestDistributionRoutes(t *testing.T) {
	router := testRouter()
	original := files.Root + "sso/avatars/" + strings.Repeat("a", 32) + "/" + strings.Repeat("b", 32) + ".webp"
	get := func(path, ip string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		r.RemoteAddr = ip + ":1"
		w := httptest.NewRecorder()
		Handler(http.NotFoundHandler(), Config{Router: router}).ServeHTTP(w, r)
		return w
	}
	for ip, want := range map[string]string{"203.0.113.1": mirror + strings.TrimPrefix(original, files.Root), "203.0.113.2": original} {
		w := get("/api/v1/distribution/avatar?"+url.Values{"url": {original}, "redirect": {"1"}}.Encode(), ip)
		if w.Code != 307 || w.Header().Get("Location") != want || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("avatar for %s: %d %s", ip, w.Code, w.Header().Get("Location"))
		}
	}
	for _, invalid := range []string{original + "?signature=x", strings.Replace(original, "distrib.ourtaiko.org", "private.example", 1), files.Root + "sso/avatars/../secret.webp"} {
		if w := get("/api/v1/distribution/avatar?"+url.Values{"url": {invalid}}.Encode(), "203.0.113.1"); w.Code != 400 {
			t.Fatal("accepted unsafe avatar URL", invalid)
		}
	}
	var answer distribution.SourceAnswer
	if w := get("/api/v1/distribution/source", "203.0.113.1"); w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &answer) != nil || !answer.Mirror || answer.BaseURL != mirror {
		t.Fatal(w.Body.String())
	}
}
