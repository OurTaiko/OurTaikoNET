// Package app puts OurTaikoNet together: one database, both song sources, the new address
// and the old ones.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/netip"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"ourtaiko.dev/net/db"
	"ourtaiko.dev/net/internal/account"
	"ourtaiko.dev/net/internal/awards"
	"ourtaiko.dev/net/internal/groups"
	"ourtaiko.dev/net/internal/inproc"
	"ourtaiko.dev/net/internal/kit/distribution"
	"ourtaiko.dev/net/internal/kit/objectstore"
	"ourtaiko.dev/net/internal/legacy"
	"ourtaiko.dev/net/internal/multiplay"
	"ourtaiko.dev/net/internal/netdb"
	profile "ourtaiko.dev/net/internal/profile/httpapi"
	"ourtaiko.dev/net/internal/site"
	songs "ourtaiko.dev/net/internal/songs/httpapi"
)

// SchemaVersion is the version of ourtaiko_net this program works with: the baseline and
// every change in db/migrations. New brings an older database up to it.
var SchemaVersion = db.Version()

// SessionCookie is the one cookie of the site. A session is the same row to all three parts,
// so with one name signing in once (at /api/v1/auth/…, the players' part) signs in everywhere.
const SessionCookie = "ourtaiko_net_session"

// Config is everything the two sources need. A nil Objects stores that source's files under
// StorageDir.
type Config struct {
	DatabaseURL string
	// Origin is the website's origin, checked on requests that change something.
	Origin string
	// StorageDir holds each source's files (or, with object storage, its staged uploads) in a
	// folder named after the source.
	StorageDir     string
	CookieSecure   bool
	TrustedProxies []netip.Prefix

	FanmadeObjects, OfficialObjects objectstore.Store
	// SSO is the site's client of the account center, used by every part. Nil when there is
	// none: sign-in is then unavailable.
	SSO *account.Client
	// DevUserID signs every sign-in in as this user, in every part. Local development only.
	DevUserID string

	// Where each source's chart pages are on the website; players' score lists link there.
	// Empty: <Origin>/fanmade and <Origin>/official.
	FanmadeSite, OfficialSite string

	// WebDir is where the built website is (web/'s dist), served for every address
	// that is not the API's. Empty: the API alone, as in development.
	WebDir string

	// The host names of the three old sites. Their APIs are answered as those sites answered,
	// and their pages are sent to the same pages under Origin (package legacy). Empty: that
	// old address is not served.
	LegacyFanmadeHost, LegacyOfficialHost, LegacyProfileHost string

	// Where the 3D Don's catalog is read (profile/httpapi don.go); empty: the public bucket's.
	DonCatalogURL       string
	DonMirrorCatalogURL string
	DonIPCountry        func(netip.Addr) string
	// Multiplay is Multi-Enso, the game's online rooms (multiplay.go). Off by default.
	Multiplay Multiplay
}

// App is the running service.
type App struct {
	Fanmade  *songs.Server
	Official *songs.Server
	// Players is what belongs to no source: titles, nameplates, awards, a player's scores
	// across sources. It is OurTaikoProfile's code.
	Players *profile.Server
	pools   []*pgxpool.Pool
	legacy  legacy.Hosts
	webDir  string
	origin  string
	// sso is the account center's client when there is one; Start refreshes its copy of
	// players' profiles.
	sso *account.Client
	// rooms is Multi-Enso when it is on, listening at roomsAddr once Start ran; roomsSite is
	// what the game is told about it.
	rooms     *multiplay.Server
	roomsAddr string
	roomsSite site.Multiplay
}

// sourceAwards records a source's awards with the players' part, under the source's name, in
// the transaction the source saves the attempt in.
type sourceAwards struct {
	players *profile.Server
	source  string
}

func (a sourceAwards) Grant(ctx context.Context, tx pgx.Tx, request awards.Request) (json.RawMessage, error) {
	return a.players.Grant(ctx, tx, a.source, request)
}

// New connects to the database and builds both sources. The database must already hold the
// schema this program was built for; see Init.
func New(ctx context.Context, cfg Config) (*App, error) {
	a := &App{legacy: legacy.Hosts{}, webDir: cfg.WebDir, origin: cfg.Origin}
	// A release that changes the schema carries the change and applies it as it starts.
	applied, err := netdb.Migrate(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	for _, version := range applied {
		log.Printf("database schema brought to version %d", version)
	}
	open := func(source string) (*pgxpool.Pool, error) {
		pool, err := netdb.Open(ctx, cfg.DatabaseURL, source)
		if err == nil {
			a.pools = append(a.pools, pool)
		}
		return pool, err
	}
	fanmadePool, err := open("fanmade")
	if err != nil {
		return nil, err
	}
	var version int
	if err = fanmadePool.QueryRow(ctx, `SELECT COALESCE(max(version),0) FROM net.schema_migrations`).Scan(&version); err != nil {
		a.Close()
		return nil, fmt.Errorf("this database is not ourtaiko_net (run with -init on an empty database): %w", err)
	}
	if version != SchemaVersion {
		a.Close()
		return nil, fmt.Errorf("database schema is version %d, this program needs %d", version, SchemaVersion)
	}
	officialPool, err := open("official")
	if err != nil {
		a.Close()
		return nil, err
	}
	shared, err := netdb.OpenShared(ctx, cfg.DatabaseURL)
	if err != nil {
		a.Close()
		return nil, err
	}
	a.pools = append(a.pools, shared)
	// Names and avatars are shown from the site's own copy (internal/account, profiles.go).
	if cfg.SSO != nil {
		cfg.SSO.KeepProfiles(shared)
		a.sso = cfg.SSO
	}

	// What each part's code calls an administrator is, here, a member of that part's user
	// group. The account center's administrator manages the site and is in no group for it.
	in := func(group groups.Set) func(context.Context, string) (bool, error) {
		return func(ctx context.Context, user string) (bool, error) {
			set, err := groups.Of(ctx, shared, user)
			return set.Has(group), err
		}
	}
	fanmadeSite, officialSite := cfg.FanmadeSite, cfg.OfficialSite
	if fanmadeSite == "" {
		fanmadeSite = cfg.Origin + "/fanmade"
	}
	if officialSite == "" {
		officialSite = cfg.Origin + "/official"
	}
	resources := distribution.FromDonCatalog(cfg.DonMirrorCatalogURL, cfg.DonIPCountry, cfg.TrustedProxies)
	a.Players = profile.New(shared, profile.Config{
		Origin: cfg.Origin, CookieSecure: cfg.CookieSecure, SSO: cfg.SSO, DevUserID: cfg.DevUserID, TrustedProxies: cfg.TrustedProxies,
		SessionCookie: SessionCookie, Groups: true, DonCatalogURL: cfg.DonCatalogURL,
		DonMirrorCatalogURL: cfg.DonMirrorCatalogURL, DonIPCountry: cfg.DonIPCountry,
		Distribution: resources,
		Sources: []profile.Source{
			{ID: "fanmade", Name: "OurTaiko Fanmade", Site: fanmadeSite},
			{ID: "official", Name: "OurTaiko Database", Site: officialSite},
		},
	})

	// What a dan attempt earns is recorded by the players' part in the attempt's own
	// transaction (Awards). The sources still read the title catalog as the old sites read
	// OurTaikoProfile's, over its HTTP API, but inside this program (package inproc); the
	// key that API once wanted for awards is never sent, so any will do.
	catalog := inproc.Transport{Handler: a.Players.Handler()}
	unused := strings.Repeat("0", 64)
	fanmadeProfile, err := songs.NewProfile(songs.ProfileConfig{URL: "http://127.0.0.1", ServiceID: "fanmade", ServiceKey: unused})
	if err != nil {
		a.Close()
		return nil, err
	}
	officialProfile, err := songs.NewProfile(songs.ProfileConfig{URL: "http://127.0.0.1", ServiceID: "official", ServiceKey: unused})
	if err != nil {
		a.Close()
		return nil, err
	}
	fanmadeProfile.UseTransport(catalog)
	officialProfile.UseTransport(catalog)

	a.Fanmade = songs.New(fanmadePool, songs.Config{
		Distribution: resources,
		Source:       songs.Fanmade, Origin: cfg.Origin, APIBase: cfg.Origin + "/api/v1/sources/fanmade", Storage: filepath.Join(cfg.StorageDir, "fanmade"), Objects: cfg.FanmadeObjects,
		CookieSecure: cfg.CookieSecure, SSO: cfg.SSO, DevUserID: cfg.DevUserID, TrustedProxies: cfg.TrustedProxies,
		Profile: fanmadeProfile, Awards: sourceAwards{a.Players, "fanmade"}, SessionCookie: SessionCookie, Manager: in(groups.FanmadeManager),
	})
	a.Official = songs.New(officialPool, songs.Config{
		Distribution: resources,
		Source:       songs.Official, Origin: cfg.Origin, APIBase: cfg.Origin + "/api/v1/sources/official", Storage: filepath.Join(cfg.StorageDir, "official"), Objects: cfg.OfficialObjects,
		CookieSecure: cfg.CookieSecure, SSO: cfg.SSO, DevUserID: cfg.DevUserID, TrustedProxies: cfg.TrustedProxies,
		Profile: officialProfile, Awards: sourceAwards{a.Players, "official"}, SessionCookie: SessionCookie, Manager: in(groups.OfficialManager),
	})
	if err = errors.Join(a.Fanmade.EnsureStorage(), a.Official.EnsureStorage()); err != nil {
		a.Close()
		return nil, err
	}
	// Each old host still answers its API; its pages are those of the new site now.
	if cfg.LegacyFanmadeHost != "" {
		a.legacy[cfg.LegacyFanmadeHost] = legacy.Site{API: a.Fanmade.Handler(), Page: legacy.Under(cfg.Origin, "/fanmade")}
	}
	if cfg.LegacyOfficialHost != "" {
		a.legacy[cfg.LegacyOfficialHost] = legacy.Site{API: a.Official.Handler(), Page: legacy.Under(cfg.Origin, "/official")}
	}
	if cfg.LegacyProfileHost != "" {
		a.legacy[cfg.LegacyProfileHost] = legacy.Site{API: legacy.PublicOnly(a.Players.Handler()), Page: legacy.Players(cfg.Origin)}
	}
	if cfg.Multiplay.ListenAddr != "" {
		timings := multiplay.DefaultTimings
		if cfg.Multiplay.MinPlayers != 0 {
			timings.MinPlayers = cfg.Multiplay.MinPlayers
		}
		if timings.MinPlayers < 1 || timings.MinPlayers > timings.MaxMembers {
			a.Close()
			return nil, fmt.Errorf("a Multi-Enso round needs between 1 and %d players, not %d", timings.MaxMembers, timings.MinPlayers)
		}
		a.rooms = multiplay.NewServer(multiplay.ServerConfig{Catalog: newRoomSongs(a.Fanmade, a.Official), Authenticate: gamePlayer(a.Fanmade), Timings: timings})
		a.roomsAddr = cfg.Multiplay.ListenAddr
		a.roomsSite = site.Multiplay{URL: cfg.Multiplay.PublicURL, Protocol: multiplay.MinProtocol, MaxProtocol: multiplay.Protocol, MaxMembers: timings.MaxMembers}
		if a.roomsSite.URL == "" {
			a.roomsSite.URL = cfg.Origin
		}
	}
	return a, nil
}

// Handler serves the new address, and the old ones for the hosts configured.
func (a *App) Handler() http.Handler {
	var options []site.Option
	if a.rooms != nil {
		options = append(options, site.WithMultiplay(a.roomsSite))
	}
	current := site.Handler([]site.Source{
		{ID: "fanmade", Name: "OurTaiko Fanmade", API: a.Fanmade.Handler()},
		{ID: "official", Name: "OurTaiko Database", API: a.Official.Handler()},
	}, a.Players.Handler(), options...)
	// Both sources and the players' part read one database, so one readiness check speaks
	// for the site.
	ready := a.Fanmade.Handler()
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/readyz" {
			ready.ServeHTTP(w, r)
			return
		}
		current.ServeHTTP(w, r)
	})
	return legacy.Handler(a.legacy, site.Web(a.webDir, a.origin, api))
}

// Start fails the uploads a restart interrupted and runs each source's background work until
// ctx ends. With Multi-Enso on, its gRPC service starts listening; StopRooms ends it.
func (a *App) Start(ctx context.Context) error {
	if err := errors.Join(a.Fanmade.FailInterruptedUploads(ctx), a.Official.FailInterruptedUploads(ctx)); err != nil {
		return err
	}
	if err := a.startRooms(); err != nil {
		return err
	}
	// Deleting replaced files, renaming them and writing missing preview clips is the work of
	// the server that owns the files. One that only reads them goes by a copy of the database
	// that falls behind, and must do none of it.
	if a.Fanmade.ReadOnlyFiles() {
		log.Print("fanmade files are read-only: no file cleanup, rekey or preview backfill")
	} else {
		go a.Fanmade.RunFileCleanup(ctx)
		go a.Fanmade.RunPreviewBackfill(ctx)
		go a.Fanmade.RunFileRekey(ctx)
	}
	if a.Official.ReadOnlyFiles() {
		log.Print("official files are read-only: no file cleanup or preview backfill")
	} else {
		go a.Official.RunFileCleanup(ctx)
		go a.Official.RunPreviewBackfill(ctx)
	}
	if a.sso != nil {
		go a.sso.RunProfileRefresh(ctx)
	}
	go a.Players.RunVisitUpkeep(ctx)
	go a.Fanmade.RunDifficultyConstants(ctx)
	go a.Fanmade.RunNotificationPrune(ctx)
	go a.Official.RunNotificationPrune(ctx)
	go a.Official.RunDifficultyConstants(ctx)
	return nil
}

// startRooms makes Multi-Enso listen, when it is on. A port that cannot be had is a
// deployment error, found as the program starts.
func (a *App) startRooms() error {
	if a.rooms == nil {
		return nil
	}
	if err := a.rooms.Start(a.roomsAddr); err != nil {
		return fmt.Errorf("Multi-Enso cannot listen on %s: %w", a.roomsAddr, err)
	}
	log.Printf("Multi-Enso (gRPC, h2c) listening on %s; the game is told to dial %s", a.rooms.Addr(), a.roomsSite.URL)
	return nil
}

// RoomsAddr is the address Multi-Enso listens on once Start ran; empty when it is off.
func (a *App) RoomsAddr() string {
	if a.rooms == nil || a.rooms.Addr() == nil {
		return ""
	}
	return a.rooms.Addr().String()
}

// StopRooms ends Multi-Enso: every room is closed, its players are told (ROOM_CLOSED), and
// the gRPC service stops. Rooms are in memory, so a restart ends them either way; this says
// so to the players. It does nothing when the service is off or already stopped.
func (a *App) StopRooms() {
	if a.rooms != nil {
		a.rooms.Stop()
	}
}

// Close ends Multi-Enso and releases the database connections.
func (a *App) Close() {
	a.StopRooms()
	for _, p := range a.pools {
		p.Close()
	}
}

// Init creates the schema in an empty database. It refuses a database that already has
// tables of its own.
func Init(ctx context.Context, url string) error {
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	var tables int
	if err = conn.QueryRow(ctx, `SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
 WHERE c.relkind IN ('r','p') AND n.nspname NOT LIKE 'pg\_%' AND n.nspname<>'information_schema'`).Scan(&tables); err != nil {
		return err
	}
	if tables != 0 {
		return errors.New("the database is not empty")
	}
	if _, err = conn.PgConn().Exec(ctx, db.Baseline).ReadAll(); err != nil {
		return err
	}
	_, err = netdb.Migrate(ctx, url)
	return err
}
