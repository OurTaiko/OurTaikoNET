// Package app puts OurTaikoNet together: one database, both song sources, the new address
// and the old ones.
package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"path/filepath"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"ourtaiko.dev/net/db"
	fanmade "ourtaiko.dev/net/internal/fanmade/httpapi"
	"ourtaiko.dev/net/internal/inproc"
	"ourtaiko.dev/net/internal/kit/objectstore"
	"ourtaiko.dev/net/internal/legacy"
	"ourtaiko.dev/net/internal/netdb"
	official "ourtaiko.dev/net/internal/official/httpapi"
	profile "ourtaiko.dev/net/internal/profile/httpapi"
	"ourtaiko.dev/net/internal/site"
)

// SchemaVersion is the version of ourtaiko_net this program works with.
const SchemaVersion = 1

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
	// Nil when there is no account center: sign-in is then unavailable.
	FanmadeSSO  *fanmade.SSOClient
	OfficialSSO *official.SSOClient
	ProfileSSO  *profile.SSOClient
	// DevUserID signs every sign-in to the official source and the players' pages in as this
	// user. Local development only.
	DevUserID string

	// Where each source's chart pages are on the website; players' score lists link there.
	// Empty: <Origin>/fanmade and <Origin>/official.
	FanmadeSite, OfficialSite string

	// The host names of the three old sites, answered as those sites answered (package
	// legacy). Empty: that old address is not served.
	LegacyFanmadeHost, LegacyOfficialHost, LegacyProfileHost string
}

// App is the running service.
type App struct {
	Fanmade  *fanmade.Server
	Official *official.Server
	// Players is what belongs to no source: titles, nameplates, awards, a player's scores
	// across sources. It is OurTaikoProfile's code.
	Players *profile.Server
	pools   []*pgxpool.Pool
	legacy  legacy.Hosts
}

// later is a handler that is set after the things that call it are built: the three parts
// call one another.
type later struct{ http.Handler }

func (l *later) ServeHTTP(w http.ResponseWriter, r *http.Request) { l.Handler.ServeHTTP(w, r) }

// New connects to the database and builds both sources. The database must already hold the
// schema this program was built for; see Init.
func New(ctx context.Context, cfg Config) (*App, error) {
	a := &App{legacy: legacy.Hosts{}}
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

	// The sources still ask the players' part for what a dan attempt earns as the old sites
	// asked OurTaikoProfile, over its HTTP API, but inside this program (package inproc).
	players := &later{}
	awards := func(id, name string) (config struct{ URL, ServiceID, ServiceKey string }, err error) {
		// A fresh key each start: nothing outside this program needs to know it.
		key, err := profile.RegisterService(ctx, shared, id, name)
		config.URL, config.ServiceID, config.ServiceKey = "http://127.0.0.1", id, key
		return config, err
	}
	fanmadeAwards, err := awards("fanmade", "OurTaiko Fanmade")
	if err != nil {
		a.Close()
		return nil, err
	}
	officialAwards, err := awards("official", "OurTaiko Database")
	if err != nil {
		a.Close()
		return nil, err
	}
	fanmadeProfile, err := fanmade.NewProfile(fanmade.ProfileConfig(fanmadeAwards))
	if err != nil {
		a.Close()
		return nil, err
	}
	officialProfile, err := official.NewProfile(official.ProfileConfig(officialAwards))
	if err != nil {
		a.Close()
		return nil, err
	}
	fanmadeProfile.UseTransport(inproc.Transport{Handler: players})
	officialProfile.UseTransport(inproc.Transport{Handler: players})

	a.Fanmade = fanmade.New(fanmadePool, fanmade.Config{
		Origin: cfg.Origin, Storage: filepath.Join(cfg.StorageDir, "fanmade"), Objects: cfg.FanmadeObjects,
		CookieSecure: cfg.CookieSecure, SSO: cfg.FanmadeSSO, TrustedProxies: cfg.TrustedProxies, Profile: fanmadeProfile,
	})
	a.Official = official.New(officialPool, official.Config{
		Origin: cfg.Origin, Storage: filepath.Join(cfg.StorageDir, "official"), Objects: cfg.OfficialObjects,
		CookieSecure: cfg.CookieSecure, SSO: cfg.OfficialSSO, DevUserID: cfg.DevUserID, TrustedProxies: cfg.TrustedProxies, Profile: officialProfile,
	})
	fanmadeSite, officialSite := cfg.FanmadeSite, cfg.OfficialSite
	if fanmadeSite == "" {
		fanmadeSite = cfg.Origin + "/fanmade"
	}
	if officialSite == "" {
		officialSite = cfg.Origin + "/official"
	}
	a.Players = profile.New(shared, profile.Config{
		Origin: cfg.Origin, CookieSecure: cfg.CookieSecure, SSO: cfg.ProfileSSO, DevUserID: cfg.DevUserID, TrustedProxies: cfg.TrustedProxies,
		Sources: []profile.Source{
			{ID: "fanmade", Name: "OurTaiko Fanmade", Site: fanmadeSite},
			{ID: "official", Name: "OurTaiko Database", Site: officialSite},
		},
	})
	players.Handler = a.Players.Handler()
	if err = errors.Join(a.Fanmade.EnsureStorage(), a.Official.EnsureStorage()); err != nil {
		a.Close()
		return nil, err
	}
	if cfg.LegacyFanmadeHost != "" {
		a.legacy[cfg.LegacyFanmadeHost] = a.Fanmade.Handler()
	}
	if cfg.LegacyOfficialHost != "" {
		a.legacy[cfg.LegacyOfficialHost] = a.Official.Handler()
	}
	if cfg.LegacyProfileHost != "" {
		a.legacy[cfg.LegacyProfileHost] = legacy.PublicOnly(a.Players.Handler())
	}
	return a, nil
}

// Handler serves the new address, and the old ones for the hosts configured.
func (a *App) Handler() http.Handler {
	current := site.Handler([]site.Source{
		{ID: "fanmade", Name: "OurTaiko Fanmade", API: a.Fanmade.Handler()},
		{ID: "official", Name: "OurTaiko Database", API: a.Official.Handler()},
	}, a.Players.Handler())
	return legacy.Handler(a.legacy, current)
}

// Start fails the uploads a restart interrupted and runs each source's background work until
// ctx ends.
func (a *App) Start(ctx context.Context) error {
	if err := errors.Join(a.Fanmade.FailInterruptedUploads(ctx), a.Official.FailInterruptedUploads(ctx)); err != nil {
		return err
	}
	go a.Fanmade.RunFileCleanup(ctx)
	go a.Fanmade.RunPreviewBackfill(ctx)
	go a.Fanmade.RunFileRekey(ctx)
	go a.Fanmade.RunDifficultyConstants(ctx)
	go a.Fanmade.RunNotificationPrune(ctx)
	go a.Official.RunNotificationPrune(ctx)
	go a.Official.RunDifficultyConstants(ctx)
	go a.Official.RunPreviewBackfill(ctx)
	return nil
}

// Close releases the database connections.
func (a *App) Close() {
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
	_, err = conn.PgConn().Exec(ctx, db.Baseline).ReadAll()
	return err
}
