package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
)

func TestDonMirrorRouting(t *testing.T) {
	head := donFileRef{"head/3.glb", 4, strings.Repeat("a", 64)}
	body := donFileRef{"body/4.glb", 6, strings.Repeat("b", 64)}
	puchi := donFileRef{"puchi/1.png", 7, strings.Repeat("c", 64)}
	face := donFileRef{"base/face.png", 9, strings.Repeat("d", 64)}
	icon := donFileRef{"icons/head/3.png", 10, strings.Repeat("e", 64)}
	paint := donFileRef{"paint/2.png", 8, strings.Repeat("1", 64)}
	canonical := donCatalog{
		Version: 1,
		Base:    map[string]donFileRef{"face": face},
		Items: map[string][]donItem{
			"head":  {{ID: 3, File: head, Icon: &icon}},
			"body":  {{ID: 4, File: body}},
			"puchi": {{ID: 1, File: puchi}},
			"paint": {{ID: 2, File: paint}},
		},
	}
	var mirrorReads atomic.Int32
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/canonical/catalog.json":
			json.NewEncoder(w).Encode(canonical)
		case "/staging/3dchara/catalog.json":
			mirrorReads.Add(1)
			http.Error(w, "the mirror catalog is not read", 500)
		default:
			http.NotFound(w, r)
		}
	}))
	defer origin.Close()
	cfg := Config{
		DonCatalogURL:       origin.URL + "/canonical/catalog.json",
		DonMirrorCatalogURL: origin.URL + "/staging/3dchara/catalog.json",
		DonClient:           origin.Client(),
		TrustedProxies:      []netip.Prefix{netip.MustParsePrefix("172.20.0.1/32")},
		DonIPCountry: func(ip netip.Addr) string {
			return map[string]string{"203.0.113.1": "CN", "203.0.113.2": "US", "203.0.113.3": "HK", "203.0.113.4": "MO", "203.0.113.5": "TW"}[ip.String()]
		},
	}
	s := New(nil, cfg)
	look := DonLook{Head: 3, Body: 4, Puchi: 1, Paint: 2}
	for _, tc := range []struct {
		name, peer, client string
		headMirrored       bool
	}{
		{"overseas", "172.20.0.1:1234", "203.0.113.2", false},
		{"Hong Kong", "172.20.0.1:1234", "203.0.113.3", false},
		{"Macao", "172.20.0.1:1234", "203.0.113.4", false},
		{"Taiwan", "172.20.0.1:1234", "203.0.113.5", false},
		{"unknown", "172.20.0.1:1234", "203.0.113.6", false},
		{"untrusted forwarded IP", "203.0.113.2:1234", "203.0.113.1", false},
		{"mainland", "172.20.0.1:1234", "203.0.113.1", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/api/v1/users/test/don", nil)
			r.RemoteAddr = tc.peer
			r.Header.Set("X-Real-IP", tc.client)
			w := httptest.NewRecorder()
			s.answerDon(w, r, look)
			var answer DonAnswer
			if err := json.Unmarshal(w.Body.Bytes(), &answer); err != nil || len(answer.Files) != 4 {
				t.Fatalf("answer %s: %v", w.Body, err)
			}
			for i, ref := range []donFileRef{body, head, puchi, paint} {
				prefix := "/canonical/"
				if tc.headMirrored {
					prefix = "/staging/3dchara/"
				}
				want := origin.URL + prefix + ref.Path + "?v=" + ref.SHA256[:12]
				file := answer.Files[i]
				if file.URL != want || file.Size != ref.Size || file.SHA256 != ref.SHA256 {
					t.Fatalf("got %+v, want %s with canonical bytes", file, want)
				}
			}
			if got := w.Header().Get("Cache-Control"); got != "private, no-store" {
				t.Fatalf("Cache-Control = %q", got)
			}
			if mirrorReads.Load() != 0 {
				t.Fatal("the mirror catalog was fetched")
			}
		})
	}
	mirrorBase := origin.URL + "/staging/3dchara/"
	catalogAnswer := func(t *testing.T, server *Server, ip string) ([]string, string) {
		t.Helper()
		r := httptest.NewRequest("GET", "/api/v1/don/catalog", nil)
		r.RemoteAddr = "172.20.0.1:1234"
		r.Header.Set("X-Real-IP", ip)
		w := httptest.NewRecorder()
		server.Handler().ServeHTTP(w, r)
		var catalog struct {
			BaseURL       string   `json:"baseUrl"`
			MirrorBaseURL *string  `json:"mirrorBaseUrl"`
			MirrorPaths   []string `json:"mirrorPaths"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &catalog); err != nil || catalog.BaseURL != origin.URL+"/canonical/" ||
			(catalog.MirrorBaseURL == nil) != (catalog.MirrorPaths == nil) ||
			catalog.MirrorBaseURL != nil && *catalog.MirrorBaseURL != mirrorBase {
			t.Fatalf("catalog %s: %v", w.Body, err)
		}
		return catalog.MirrorPaths, w.Header().Get("Cache-Control")
	}
	all := []string{face.Path, body.Path, head.Path, icon.Path, paint.Path, puchi.Path}
	slices.Sort(all)
	if paths, cache := catalogAnswer(t, s, "203.0.113.1"); cache != "private, max-age=60" || !slices.Equal(paths, all) {
		t.Fatalf("mainland catalog paths = %v, cache = %q", paths, cache)
	}
	first := s.donMirrorPaths
	if paths, _ := catalogAnswer(t, s, "203.0.113.1"); len(paths) != len(all) || s.donMirrorPaths != first {
		t.Fatalf("mirror paths worked out again: %v", paths)
	}
	if paths, _ := catalogAnswer(t, s, "203.0.113.2"); len(paths) != 0 {
		t.Fatalf("overseas catalog paths = %v", paths)
	}
	if got := mirrorReads.Load(); got != 0 {
		t.Fatalf("mirror catalog fetched %d times", got)
	}
	noCountry := cfg
	noCountry.DonIPCountry = nil
	if paths, cache := catalogAnswer(t, New(nil, noCountry), "203.0.113.1"); len(paths) != 0 || cache != "public, max-age=60" {
		t.Fatalf("catalog without country lookup paths = %v, cache = %q", paths, cache)
	}
	cfg.DonMirrorCatalogURL = ""
	if paths, cache := catalogAnswer(t, New(nil, cfg), "203.0.113.1"); len(paths) != 0 || cache != "public, max-age=60" {
		t.Fatalf("disabled mirror catalog paths = %v, cache = %q", paths, cache)
	}
}
