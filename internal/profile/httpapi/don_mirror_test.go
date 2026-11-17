package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"
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
	oldBody := body
	oldBody.SHA256 = strings.Repeat("f", 64)
	wrongSize := puchi
	wrongSize.Size++
	mirrored := donCatalog{
		Version: 1,
		Base:    map[string]donFileRef{"face": face},
		Items: map[string][]donItem{
			"head":  {{ID: 3, File: head}},
			"body":  {{ID: 4, File: oldBody}},
			"puchi": {{ID: 1, File: wrongSize}},
			"paint": {{ID: 2, File: paint}},
		},
	}
	var mirrorDown atomic.Bool
	var mirrorReads atomic.Int32
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/canonical/catalog.json":
			json.NewEncoder(w).Encode(canonical)
		case "/staging/3dchara/catalog.json":
			mirrorReads.Add(1)
			if mirrorDown.Load() {
				http.Error(w, "unavailable", 503)
				return
			}
			json.NewEncoder(w).Encode(mirrored)
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
				if (i == 1 || i == 3) && tc.headMirrored {
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
			if !tc.headMirrored && mirrorReads.Load() != 0 {
				t.Fatal("non-mainland request fetched the mirror catalog")
			}
		})
	}
	catalogAnswer := func(t *testing.T, server *Server, ip string) (map[string]string, string) {
		t.Helper()
		r := httptest.NewRequest("GET", "/api/v1/don/catalog", nil)
		r.RemoteAddr = "172.20.0.1:1234"
		r.Header.Set("X-Real-IP", ip)
		w := httptest.NewRecorder()
		server.Handler().ServeHTTP(w, r)
		var catalog struct {
			BaseURL    string            `json:"baseUrl"`
			MirrorURLs map[string]string `json:"mirrorUrls"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &catalog); err != nil || catalog.BaseURL != origin.URL+"/canonical/" {
			t.Fatalf("catalog %s: %v", w.Body, err)
		}
		return catalog.MirrorURLs, w.Header().Get("Cache-Control")
	}
	if urls, cache := catalogAnswer(t, s, "203.0.113.1"); len(urls) != 3 || cache != "private, no-store" ||
		urls[head.Path] != origin.URL+"/staging/3dchara/"+head.Path+"?v="+head.SHA256[:12] ||
		urls[paint.Path] != origin.URL+"/staging/3dchara/"+paint.Path+"?v="+paint.SHA256[:12] ||
		urls[face.Path] != origin.URL+"/staging/3dchara/"+face.Path+"?v="+face.SHA256[:12] {
		t.Fatalf("mainland catalog URLs = %v, cache = %q", urls, cache)
	}
	if urls, _ := catalogAnswer(t, s, "203.0.113.2"); len(urls) != 0 {
		t.Fatalf("overseas catalog URLs = %v", urls)
	}
	if got := mirrorReads.Load(); got != 1 {
		t.Fatalf("mirror catalog fetched %d times", got)
	}
	refreshed := func(server *Server) {
		server.donMirrorMu.Lock()
		done := server.donMirrorDone
		server.donMirrorMu.Unlock()
		if done != nil {
			<-done
		}
	}
	// A due refresh runs on its own; the last good mirror is kept while it runs and after it fails.
	mirrorDown.Store(true)
	s.donMirrorMu.Lock()
	s.donMirrorUntil = time.Time{}
	s.donMirrorMu.Unlock()
	for range 2 {
		if urls, _ := catalogAnswer(t, s, "203.0.113.1"); len(urls) != 3 {
			t.Fatalf("failed refresh dropped mirror URLs: %v", urls)
		}
		refreshed(s)
	}
	if got := mirrorReads.Load(); got != 2 {
		t.Fatalf("mirror catalog fetched %d times after a failed refresh", got)
	}
	// Without a good mirror ever read, everything stays canonical.
	fresh := New(nil, cfg)
	for range 2 {
		if urls, _ := catalogAnswer(t, fresh, "203.0.113.1"); len(urls) != 0 {
			t.Fatalf("first failed read gave mirror URLs: %v", urls)
		}
	}
	if got := mirrorReads.Load(); got != 3 {
		t.Fatalf("mirror catalog fetched %d times after a first failed read", got)
	}
	cfg.DonMirrorCatalogURL = ""
	if urls, cache := catalogAnswer(t, New(nil, cfg), "203.0.113.1"); len(urls) != 0 || cache != "public, max-age=60" {
		t.Fatalf("disabled mirror catalog URLs = %v, cache = %q", urls, cache)
	}
}
