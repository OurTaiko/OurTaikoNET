// Package app puts OurTaikoNet together: one database, both song sources, the new address
// and the old ones.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/netip"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"ourtaiko.dev/net/db"
	"ourtaiko.dev/net/internal/account"
	"ourtaiko.dev/net/internal/api/mount"
	apiv1 "ourtaiko.dev/net/internal/api/v1"
	apiv2 "ourtaiko.dev/net/internal/api/v2"
	"ourtaiko.dev/net/internal/awards"
	"ourtaiko.dev/net/internal/edit"
	"ourtaiko.dev/net/internal/files"
	"ourtaiko.dev/net/internal/groups"
	"ourtaiko.dev/net/internal/inproc"
	"ourtaiko.dev/net/internal/kit/distribution"
	"ourtaiko.dev/net/internal/kit/objectstore"
	"ourtaiko.dev/net/internal/legacy"
	"ourtaiko.dev/net/internal/multiplay"
	"ourtaiko.dev/net/internal/multiplay/roomsapi"
	"ourtaiko.dev/net/internal/netdb"
	profile "ourtaiko.dev/net/internal/profile/httpapi"
	"ourtaiko.dev/net/internal/rewards"
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
	// rooms is Multi-Enso when it runs in this program, listening at roomsAddr once Start ran.
	// roomsAPI is what cmd/rooms asks when it runs there, listening at roomsAPIAddr. roomsSite
	// is what the game is told about either, nil when the service is off.
	rooms        *multiplay.Server
	roomsAddr    string
	roomsAPI     *http.Server
	roomsAPIAddr net.Addr
	roomsSite    *site.Multiplay
	// rewards tells players what they were given (Handler); shared is the pool it listens on (Start).
	rewards rewards.Store
	shared  *pgxpool.Pool
	// v1 and v2 are what each version of the API needs (packages apiv1, apiv2).
	v1 apiv1.Config
	v2 apiv2.Config
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
	a.rewards = rewards.NewStore(shared)
	a.shared = shared
	// Version 1 gives full addresses, a mainland visitor's on the mirror (package apiv1).
	_, bucket := cfg.FanmadeObjects.(*objectstore.S3)
	a.v1 = apiv1.Config{Router: distribution.FromDonCatalog(cfg.DonMirrorCatalogURL, cfg.DonIPCountry, cfg.TrustedProxies), FilesOnDisk: !bucket, Origin: cfg.Origin}
	a.Players = profile.New(shared, profile.Config{
		Origin: cfg.Origin, CookieSecure: cfg.CookieSecure, SSO: cfg.SSO, DevUserID: cfg.DevUserID, TrustedProxies: cfg.TrustedProxies,
		SessionCookie: SessionCookie, Groups: true, DonCatalogURL: cfg.DonCatalogURL,
		Sources: []profile.Source{
			{ID: "fanmade", Name: "OurTaiko Fanmade", Site: fanmadeSite, Categories: songs.Fanmade.CategoryBits()},
			{ID: "official", Name: "OurTaiko Database", Site: officialSite, Categories: songs.Official.CategoryBits()},
		},
	})
	// A reward of a Don part is described from the catalog the players' part reads.
	a.rewards.Describe = a.Players.DescribeReward

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
		Source: songs.Fanmade, Origin: cfg.Origin, Storage: filepath.Join(cfg.StorageDir, "fanmade"), Objects: cfg.FanmadeObjects,
		CookieSecure: cfg.CookieSecure, SSO: cfg.SSO, DevUserID: cfg.DevUserID, TrustedProxies: cfg.TrustedProxies,
		Profile: fanmadeProfile, Awards: sourceAwards{a.Players, "fanmade"}, SessionCookie: SessionCookie, Manager: in(groups.FanmadeManager),
	})
	a.Official = songs.New(officialPool, songs.Config{
		Source: songs.Official, Origin: cfg.Origin, Storage: filepath.Join(cfg.StorageDir, "official"), Objects: cfg.OfficialObjects,
		CookieSecure: cfg.CookieSecure, SSO: cfg.SSO, DevUserID: cfg.DevUserID, TrustedProxies: cfg.TrustedProxies,
		Profile: officialProfile, Awards: sourceAwards{a.Players, "official"}, SessionCookie: SessionCookie, Manager: in(groups.OfficialManager),
	})
	if err = errors.Join(a.Fanmade.EnsureStorage(), a.Official.EnsureStorage()); err != nil {
		a.Close()
		return nil, err
	}
	// Each old host still answers its API; its pages are those of the new site now.
	if cfg.LegacyFanmadeHost != "" {
		a.legacy[cfg.LegacyFanmadeHost] = legacy.Site{API: a.versions(a.withRewards(a.Fanmade.Handler()), "/api/v1/sources/fanmade"), Page: legacy.Under(cfg.Origin, "/fanmade")}
	}
	if cfg.LegacyOfficialHost != "" {
		a.legacy[cfg.LegacyOfficialHost] = legacy.Site{API: a.versions(a.withRewards(a.Official.Handler()), "/api/v1/sources/official"), Page: legacy.Under(cfg.Origin, "/official")}
	}
	if cfg.LegacyProfileHost != "" {
		a.legacy[cfg.LegacyProfileHost] = legacy.Site{API: legacy.PublicOnly(a.versions(a.Players.Handler(), "")), Page: legacy.Players(cfg.Origin)}
	}
	if cfg.Multiplay.ListenAddr != "" && cfg.Multiplay.InternalAddr != "" {
		a.Close()
		return nil, errors.New("Multi-Enso runs in this program (MULTIPLAY_LISTEN_ADDR) or in cmd/rooms (MULTIPLAY_INTERNAL_ADDR), not both")
	}
	if cfg.Multiplay.ListenAddr != "" || cfg.Multiplay.InternalAddr != "" {
		timings, err := multiplay.TimingsWith(cfg.Multiplay.MinPlayers)
		if err != nil {
			a.Close()
			return nil, err
		}
		catalog, identify := newRoomSongs(a.Fanmade, a.Official), gamePlayer(a.Fanmade)
		if cfg.Multiplay.ListenAddr != "" {
			a.rooms = multiplay.NewServer(multiplay.ServerConfig{Catalog: catalog, Authenticate: identify, Timings: timings})
			a.roomsAddr = cfg.Multiplay.ListenAddr
		} else {
			a.roomsAPI = &http.Server{Addr: cfg.Multiplay.InternalAddr, Handler: roomsapi.Handler(catalog, identify), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
		}
		a.roomsSite = &site.Multiplay{URL: cfg.Multiplay.PublicURL, Protocol: multiplay.MinProtocol, MaxProtocol: multiplay.Protocol, MaxMembers: timings.MaxMembers}
		if a.roomsSite.URL == "" {
			a.roomsSite.URL = cfg.Origin
		}
	}
	// Version 2 gives paths; in development the files on disk are served under its own root.
	a.v2 = apiv2.Config{Router: a.v1.Router, Origin: cfg.Origin}
	if !bucket {
		a.v2.OnDisk = map[string]objectstore.Store{songs.Fanmade.Folder: a.Fanmade.Config.Objects, songs.Official.Folder: a.Official.Config.Objects}
	}
	return a, nil
}

// Handler serves the new address, and the old ones for the hosts configured.
func (a *App) Handler() http.Handler {
	var options []site.Option
	if a.roomsSite != nil {
		options = append(options, site.WithMultiplay(*a.roomsSite))
	}
	current := site.Handler([]site.Source{
		{ID: "fanmade", Name: "OurTaiko Fanmade", API: a.Fanmade.Handler()},
		{ID: "official", Name: "OurTaiko Database", API: a.Official.Handler()},
	}, a.Players.Handler(), options...)
	// Both sources and the players' part read one database, so one readiness check speaks
	// for the site.
	ready := a.Fanmade.Handler()
	routes := http.NewServeMux()
	routes.Handle("/readyz", ready)
	// The website and new clients use version 2; released games, version 1. The old hosts
	// (a.legacy) only ever had version 1.
	api := a.withRewards(current)
	v1 := apiv1.Handler(api, a.v1)
	routes.Handle("/", mount.Serve(map[string]http.Handler{apiv1.Version: v1, apiv2.Version: apiv2.Handler(api, a.v2)}, api))
	// Every answer, the old domains' too, says how many rewards its player has not seen.
	// The chart editor (OurTaikoEdit's web version) as a bare page at /edit/app/, its build from the
	// public bucket; the website shows it in its frame.
	page := edit.Handler(edit.Config{Root: files.Root, Manifest: "edit-nightly.json", Router: a.v1.Router}, site.Web(a.webDir, a.origin, routes))
	return a.rewards.Announce(legacy.Handler(a.legacy, page))
}

// versions serves api, the version-free API (paths /api/…), at each version's prefix: /api/v1/
// (package apiv1). Health checks and /internal/ reach api as they are.
// An old host serves one source at /api/v1/ itself; sourceBase is that source's routes on the
// new site, which version 1 linked to.
func (a *App) versions(api http.Handler, sourceBase string) http.Handler {
	v1 := a.v1
	v1.SourceBase = sourceBase
	return mount.Serve(map[string]http.Handler{apiv1.Version: apiv1.Handler(api, v1)}, api)
}

// withRewards serves the rewards' routes (package rewards) in front of an API: the website asks
// with its session, the game with its token. Either source knows a game's token; the players'
// part knows the website. The old domains have them too, for a game that still uses one.
func (a *App) withRewards(next http.Handler) http.Handler {
	mux := http.NewServeMux()
	a.rewards.Routes(mux, "/api/me/rewards", a.Players.Require)
	a.rewards.Routes(mux, "/api/game/rewards", a.Fanmade.Require)
	mux.Handle("/", next)
	return mux
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
	// The rewards' counts are kept while it listens on a connection of the shared pool.
	go a.rewards.Run(ctx, func(ctx context.Context) (rewards.Listener, func(), error) {
		conn, err := a.shared.Acquire(ctx)
		if err != nil {
			return nil, nil, err
		}
		return conn.Conn(), conn.Release, nil
	})
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

// startRooms makes Multi-Enso listen, or what cmd/rooms asks, when it is on. A port that cannot
// be had is a deployment error, found as the program starts.
func (a *App) startRooms() error {
	if a.roomsAPI != nil {
		l, err := net.Listen("tcp", a.roomsAPI.Addr)
		if err != nil {
			return fmt.Errorf("the rooms' internal API cannot listen on %s: %w", a.roomsAPI.Addr, err)
		}
		a.roomsAPIAddr = l.Addr()
		go func() {
			if err := a.roomsAPI.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Printf("the rooms' internal API stopped: %v", err)
			}
		}()
		log.Printf("the rooms' internal API is listening on %s; the game is told to dial %s", l.Addr(), a.roomsSite.URL)
		return nil
	}
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

// RoomsAPIAddr is the address cmd/rooms asks once Start ran; empty when it runs nowhere.
func (a *App) RoomsAPIAddr() string {
	if a.roomsAPIAddr == nil {
		return ""
	}
	return a.roomsAPIAddr.String()
}

// StopRooms ends Multi-Enso when it runs in this program: every room is closed, its players
// are told (ROOM_CLOSED), and the gRPC service stops. Rooms are in memory, so a restart ends
// them either way; this says so to the players. Rooms in cmd/rooms go on: only what they ask
// of this program stops answering. It does nothing when the service is off or already stopped.
func (a *App) StopRooms() {
	if a.rooms != nil {
		a.rooms.Stop()
	}
	if a.roomsAPI != nil {
		a.roomsAPI.Close()
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
