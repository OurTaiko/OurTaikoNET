package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const testDonCatalog = `{"version":1,
 "base":{"animations":{"path":"base/animations.glb","size":10,"sha256":"aaaaaaaaaaaaaaaaaaaa"}},
 "defaultColors":{"p1":["#68BFC0","#F94728","#F9F0E1"]},
 "items":{
  "cos":[{"id":0,"name":{},"file":{"path":"cos/0.glb","size":1,"sha256":"c0c0c0c0c0c0c0c0"}},
         {"id":30,"name":{"ja":"鏡もち"},"file":{"path":"cos/30.glb","size":3,"sha256":"3030303030303030"}}],
  "head":[{"id":0,"name":{},"file":{"path":"head/0.glb","size":2,"sha256":"h0h0h0h0h0h0h0h0"}},
          {"id":3,"name":{},"file":{"path":"head/3.glb","size":4,"sha256":"h3h3h3h3h3h3h3h3"}}],
  "body":[{"id":0,"name":{},"file":{"path":"body/0.glb","size":5,"sha256":"b0b0b0b0b0b0b0b0"}},
          {"id":4,"name":{},"file":{"path":"body/4.glb","size":6,"sha256":"b4b4b4b4b4b4b4b4"}}],
  "puchi":[{"id":1,"name":{},"file":{"path":"puchi/1.png","size":7,"sha256":"p1p1p1p1p1p1p1p1"}}],
  "paint":[{"id":2,"name":{},"file":{"path":"paint/2.png","size":8,"sha256":"m2m2m2m2m2m2m2m2"}}]}}`

func TestPlayersDressTheirDon(t *testing.T) {
	var reads atomic.Int32
	var down atomic.Bool
	bucket := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reads.Add(1)
		if down.Load() || r.URL.Path != "/3dchara/catalog.json" {
			http.Error(w, "no", 500)
			return
		}
		w.Write([]byte(testDonCatalog))
	}))
	defer bucket.Close()
	app := New(testDB(t), Config{Origin: origin, DevUserID: player, DonCatalogURL: bucket.URL + "/3dchara/catalog.json", DonClient: bucket.Client()})
	c := &client{t: t, handler: app.Handler()}
	base := bucket.URL + "/3dchara/"

	// Nobody has chosen: the plain Don, whose one file is costume 0.
	var answer DonAnswer
	c.json("GET", "/api/v1/users/"+other+"/don", "", 200, &answer)
	if !answer.Look.Costume || answer.Look.Cos != 0 || answer.Look.Colors != nil || len(answer.Files) != 1 ||
		answer.Files[0].URL != base+"cos/0.glb?v=c0c0c0c0c0c0" || answer.Files[0].Size != 1 {
		t.Fatalf("plain Don %+v", answer)
	}
	c.json("GET", "/api/v1/users/not-an-id/don", "", 404, nil)

	// The catalog, as the bucket has it, with the address its paths are relative to.
	var catalog struct {
		BaseURL string `json:"baseUrl"`
		Items   map[string][]donItem
	}
	c.json("GET", "/api/v1/don/catalog", "", 200, &catalog)
	if catalog.BaseURL != base || len(catalog.Items["cos"]) != 2 || catalog.Items["cos"][1].Name["ja"] != "鏡もち" {
		t.Fatalf("catalog %+v", catalog)
	}

	// Choosing needs a session; every number must be in the catalog; colours are #RRGGBB.
	c.json("PUT", "/api/v1/me/don", `{"costume":true,"cos":30}`, 401, nil)
	c.signIn()
	c.json("PUT", "/api/v1/me/don", `{"costume":true,"cos":31,"head":0,"body":0,"puchi":0}`, 422, nil)
	c.json("PUT", "/api/v1/me/don", `{"costume":false,"cos":0,"head":3,"body":4,"puchi":2}`, 422, nil)
	c.json("PUT", "/api/v1/me/don", `{"costume":false,"cos":0,"head":3,"body":4,"puchi":1,"colors":["#fff","#000000","#123456"]}`, 422, nil)
	c.json("PUT", "/api/v1/me/don", `{"costume":false,"cos":30,"head":3,"body":4,"puchi":1,"colors":["#aabbcc","#000000","#123456"]}`, 200, &answer)
	if answer.Look.Costume || answer.Look.Cos != 30 || answer.Look.Colors == nil || (*answer.Look.Colors)[0] != "#AABBCC" {
		t.Fatalf("worn %+v", answer.Look)
	}
	kinds := []string{}
	for _, f := range answer.Files {
		kinds = append(kinds, f.Kind)
	}
	if strings.Join(kinds, ",") != "body,head,puchi" {
		t.Fatalf("files of a head and body %v", answer.Files)
	}

	// The game reads it by the player's id; the website by the session.
	c.json("GET", "/api/v1/users/"+player+"/don", "", 200, &answer)
	if answer.Look.Head != 3 || answer.Look.Body != 4 || answer.Look.Puchi != 1 || answer.Look.Cos != 30 {
		t.Fatalf("read back %+v", answer.Look)
	}
	c.json("GET", "/api/v1/me/don", "", 200, &answer)
	if len(answer.Files) != 3 {
		t.Fatalf("mine %+v", answer)
	}

	// Back to a costume, and to the seat's colours.
	c.json("PUT", "/api/v1/me/don", `{"costume":true,"cos":30,"head":3,"body":4,"puchi":0,"colors":null}`, 200, &answer)
	if answer.Look.Colors != nil || len(answer.Files) != 1 || answer.Files[0].Kind != "cos" || answer.Files[0].ID != 30 {
		t.Fatalf("costume again %+v", answer)
	}

	// Makeup: a number in the catalog, its sheet one more file; left out it is none.
	c.json("PUT", "/api/v1/me/don", `{"costume":true,"cos":30,"head":3,"body":4,"paint":3}`, 422, nil)
	c.json("PUT", "/api/v1/me/don", `{"costume":true,"cos":30,"head":3,"body":4,"paint":2}`, 200, &answer)
	if answer.Look.Paint != 2 || len(answer.Files) != 2 || answer.Files[1].Kind != "paint" ||
		answer.Files[1].URL != base+"paint/2.png?v=m2m2m2m2m2m2" {
		t.Fatalf("makeup %+v", answer)
	}
	c.json("GET", "/api/v1/users/"+player+"/don", "", 200, &answer)
	if answer.Look.Paint != 2 || len(answer.Files) != 2 {
		t.Fatalf("makeup read back %+v", answer)
	}
	c.json("PUT", "/api/v1/me/don", `{"costume":true,"cos":30,"head":3,"body":4}`, 200, &answer)
	if answer.Look.Paint != 0 || len(answer.Files) != 1 {
		t.Fatalf("makeup off %+v", answer)
	}

	// The catalog is read once and kept; when the bucket fails later the last good one stays.
	before := reads.Load()
	down.Store(true)
	app.donMu.Lock()
	app.donAt = app.donAt.Add(-donCatalogTTL * 2)
	app.donMu.Unlock()
	c.json("GET", "/api/v1/users/"+player+"/don", "", 200, &answer)
	if len(answer.Files) != 1 || reads.Load() != before+1 {
		t.Fatalf("stale catalog %+v (reads %d → %d)", answer, before, reads.Load())
	}

	// A server that has never read it: looks are still told, without files; choosing waits.
	fresh := New(testDB(t), Config{Origin: origin, DevUserID: player, DonCatalogURL: bucket.URL + "/3dchara/catalog.json", DonClient: bucket.Client()})
	f := &client{t: t, handler: fresh.Handler()}
	f.json("GET", "/api/v1/users/"+other+"/don", "", 200, &answer)
	if len(answer.Files) != 0 || !answer.Look.Costume {
		t.Fatalf("no catalog %+v", answer)
	}
	f.json("GET", "/api/v1/don/catalog", "", 503, nil)
	f.signIn()
	f.json("PUT", "/api/v1/me/don", `{"costume":true,"cos":0,"head":0,"body":0,"puchi":0}`, 503, nil)
}
