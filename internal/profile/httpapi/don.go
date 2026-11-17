package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"ourtaiko.dev/net/internal/account"
	"ourtaiko.dev/net/internal/web"
)

// The 3D Don (OurTaikoPlay's Chara3D): each player's look is chosen on the website and kept in
// net.don_looks; the models live in the public bucket under 3dchara/, listed by its catalog.json
// (tools/3dchara). The server reads that catalog to check a choice and to tell the game which
// files a look needs; the files themselves are never served from here.

// DefaultDonCatalogURL is where the catalog is read unless Config.DonCatalogURL says otherwise.
const DefaultDonCatalogURL = "https://distrib.ourtaiko.org/3dchara/catalog.json"

// How long a read catalog is used before it is read again (a failed read keeps the last good one).
const donCatalogTTL = 5 * time.Minute

// DonLook is a player's Don. Paint is the makeup (0: none). Colors is body, face and rim as
// "#RRGGBB", or nil for the seat's.
type DonLook struct {
	Costume bool      `json:"costume"`
	Cos     int       `json:"cos"`
	Head    int       `json:"head"`
	Body    int       `json:"body"`
	Puchi   int       `json:"puchi"`
	Paint   int       `json:"paint"`
	Colors  *[]string `json:"colors"`
}

// DonFile is one file a look needs: its kind (cos, head, body, puchi, paint), number, address and
// content.
type DonFile struct {
	Kind   string `json:"kind"`
	ID     int    `json:"id"`
	URL    string `json:"url"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// DonAnswer is what the look endpoints answer.
type DonAnswer struct {
	Look  DonLook   `json:"look"`
	Files []DonFile `json:"files"`
}

type donFileRef struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type donItem struct {
	ID   int               `json:"id"`
	Name map[string]string `json:"name"`
	File donFileRef        `json:"file"`
	Icon *donFileRef       `json:"icon,omitempty"`
}

type donCatalog struct {
	Version int                   `json:"version"`
	Base    map[string]donFileRef `json:"base"`
	Items   map[string][]donItem  `json:"items"`
	// The bytes as read, answered as they are (with baseUrl added).
	raw json.RawMessage
	// Where the catalog's paths are relative to.
	baseURL string
}

func (c *donCatalog) item(kind string, id int) (donItem, bool) {
	for _, it := range c.Items[kind] {
		if it.ID == id {
			return it, true
		}
	}
	return donItem{}, false
}

func (c *donCatalog) url(ref donFileRef) string {
	v := ref.SHA256
	if len(v) > 12 {
		v = v[:12]
	}
	return c.baseURL + ref.Path + "?v=" + v
}

var errNoDonCatalog = errors.New("the 3dchara catalog cannot be read")

func (s *Server) donCatalogURL() string {
	if s.Config.DonCatalogURL != "" {
		return s.Config.DonCatalogURL
	}
	return DefaultDonCatalogURL
}

// donCatalog is the catalog, read again when it is older than donCatalogTTL.
func (s *Server) donCatalog(ctx context.Context) (*donCatalog, error) {
	s.donMu.Lock()
	defer s.donMu.Unlock()
	if s.don != nil && time.Since(s.donAt) < donCatalogTTL {
		return s.don, nil
	}
	c, err := s.readDonCatalog(ctx, s.donCatalogURL())
	if err != nil {
		if s.don != nil {
			s.donAt = time.Now() // keep the last good one; try again after a while
			return s.don, nil
		}
		return nil, fmt.Errorf("%w: %v", errNoDonCatalog, err)
	}
	s.don, s.donAt = c, time.Now()
	return c, nil
}

func (s *Server) readDonCatalog(ctx context.Context, address string) (*donCatalog, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", address, nil)
	if err != nil {
		return nil, err
	}
	client := s.Config.DonClient
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("%s answered %d", address, res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	var c donCatalog
	if err = json.Unmarshal(body, &c); err != nil {
		return nil, err
	}
	if c.Version != 1 {
		return nil, fmt.Errorf("catalog version %d", c.Version)
	}
	c.raw = body
	c.baseURL = address[:strings.LastIndex(address, "/")+1]
	return &c, nil
}

var donColor = regexp.MustCompile(`^#[0-9A-F]{6}$`)

// check says what is wrong with a look, given the catalog ("" when nothing is).
func (c *donCatalog) check(l *DonLook) string {
	if l.Colors != nil {
		if len(*l.Colors) != 3 {
			return "colors 要三种颜色"
		}
		for i, v := range *l.Colors {
			(*l.Colors)[i] = strings.ToUpper(v)
			if !donColor.MatchString((*l.Colors)[i]) {
				return "颜色要写成 #RRGGBB"
			}
		}
	}
	need := []struct {
		kind string
		id   int
	}{{"cos", l.Cos}, {"head", l.Head}, {"body", l.Body}}
	if l.Puchi != 0 {
		need = append(need, struct {
			kind string
			id   int
		}{"puchi", l.Puchi})
	}
	if l.Paint != 0 {
		need = append(need, struct {
			kind string
			id   int
		}{"paint", l.Paint})
	}
	for _, n := range need {
		if _, ok := c.item(n.kind, n.id); !ok {
			return fmt.Sprintf("没有 %s %d", n.kind, n.id)
		}
	}
	return ""
}

// files are what the game needs to draw the look: the costume, or the head and the body, and
// the puchichara and the makeup.
func (c *donCatalog) files(l DonLook, mirror *donMirror) []DonFile {
	var want [][2]any
	if l.Costume {
		want = append(want, [2]any{"cos", l.Cos})
	} else {
		want = append(want, [2]any{"body", l.Body}, [2]any{"head", l.Head})
	}
	if l.Puchi != 0 {
		want = append(want, [2]any{"puchi", l.Puchi})
	}
	if l.Paint != 0 {
		want = append(want, [2]any{"paint", l.Paint})
	}
	files := []DonFile{}
	for _, w := range want {
		kind, id := w[0].(string), w[1].(int)
		if it, ok := c.item(kind, id); ok {
			address := mirror.url(it.File)
			if address == "" {
				address = c.url(it.File)
			}
			files = append(files, DonFile{Kind: kind, ID: id, URL: address, Size: it.File.Size, SHA256: it.File.SHA256})
		}
	}
	return files
}

var plainDon = DonLook{Costume: true}

func (s *Server) donLook(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, user string) (DonLook, error) {
	l := DonLook{}
	var colors []string
	err := q.QueryRow(ctx, `SELECT costume, cos_id, head_id, body_id, puchi_id, paint_id, colors FROM don_looks WHERE user_id=$1`, user).
		Scan(&l.Costume, &l.Cos, &l.Head, &l.Body, &l.Puchi, &l.Paint, &colors)
	if errors.Is(err, pgx.ErrNoRows) {
		return plainDon, nil
	}
	if err != nil {
		return l, err
	}
	if colors != nil {
		l.Colors = &colors
	}
	return l, nil
}

// answerDon answers a look with the files it needs. Without a catalog the files are left out
// (the game then keeps the plain Don), but the look is still told.
func (s *Server) answerDon(w http.ResponseWriter, r *http.Request, l DonLook) {
	answer := DonAnswer{Look: l, Files: []DonFile{}}
	if s.donRegional() {
		// the files' addresses depend on where the request comes from
		w.Header().Set("Cache-Control", "private, no-store")
	}
	if c, err := s.donCatalog(r.Context()); err == nil {
		answer.Files = c.files(l, s.mirrorForDonRequest(r))
	}
	web.Respond(w, 200, answer)
}

// donCatalogAnswer is GET /api/v1/don/catalog: the bucket's catalog as it is, with baseUrl, the
// address its paths are relative to. Readable from any website.
func (s *Server) donCatalogAnswer(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	c, err := s.donCatalog(r.Context())
	if err != nil {
		problem(w, 503, "DON_CATALOG_UNAVAILABLE", "服装目录暂时读不到")
		return
	}
	var all map[string]json.RawMessage
	if err = json.Unmarshal(c.raw, &all); err != nil {
		internal(w, err)
		return
	}
	base, _ := json.Marshal(c.baseURL)
	all["baseUrl"] = base
	if s.donRegional() {
		w.Header().Set("Cache-Control", "private, max-age=60")
		delete(all, "mirrorUrls")
		if mirror := s.mirrorForDonRequest(r); mirror != nil {
			if urls := mirror.urls(c); len(urls) > 0 {
				all["mirrorUrls"], _ = json.Marshal(urls)
			}
		}
	} else {
		w.Header().Set("Cache-Control", "public, max-age=60")
	}
	web.Respond(w, 200, all)
}

// userDon is GET /api/v1/users/{id}/don: a player's Don and its files, for the game. Readable
// from any website, like the nameplate.
func (s *Server) userDon(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	id := r.PathValue("id")
	if !account.ValidSubject(id) {
		problem(w, 404, "USER_NOT_FOUND", "用户不存在")
		return
	}
	l, err := s.donLook(r.Context(), s.DB, id)
	if err != nil {
		internal(w, err)
		return
	}
	s.answerDon(w, r, l)
}

// myDon is GET /api/v1/me/don.
func (s *Server) myDon(w http.ResponseWriter, r *http.Request) {
	v, ok := s.required(w, r, false)
	if !ok {
		return
	}
	l, err := s.donLook(r.Context(), s.DB, v.User.ID)
	if err != nil {
		internal(w, err)
		return
	}
	s.answerDon(w, r, l)
}

// wearDon is PUT /api/v1/me/don: {costume, cos, head, body, puchi, paint, colors} (paint left out
// is 0, no makeup). Every number must be
// in the catalog (the costume's even when wearing a head and body, and the other way round, so a
// switch back keeps what was chosen); colors null is the seat's defaults.
func (s *Server) wearDon(w http.ResponseWriter, r *http.Request) {
	v, ok := s.required(w, r, true)
	if !ok {
		return
	}
	var l DonLook
	if !decode(w, r, &l) {
		return
	}
	c, err := s.donCatalog(r.Context())
	if err != nil {
		problem(w, 503, "DON_CATALOG_UNAVAILABLE", "服装目录暂时读不到，请稍后再试")
		return
	}
	if why := c.check(&l); why != "" {
		problem(w, 422, "DON_LOOK_INVALID", why)
		return
	}
	var colors []string
	if l.Colors != nil {
		colors = *l.Colors
	}
	_, err = s.DB.Exec(r.Context(), `INSERT INTO don_looks(user_id,costume,cos_id,head_id,body_id,puchi_id,paint_id,colors,updated_at)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,now())
 ON CONFLICT(user_id) DO UPDATE SET costume=EXCLUDED.costume,cos_id=EXCLUDED.cos_id,head_id=EXCLUDED.head_id,
 body_id=EXCLUDED.body_id,puchi_id=EXCLUDED.puchi_id,paint_id=EXCLUDED.paint_id,colors=EXCLUDED.colors,updated_at=now()`,
		v.User.ID, l.Costume, l.Cos, l.Head, l.Body, l.Puchi, l.Paint, colors)
	if err != nil {
		internal(w, err)
		return
	}
	s.answerDon(w, r, l)
}
