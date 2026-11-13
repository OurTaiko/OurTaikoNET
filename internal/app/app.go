// Package app puts OurTaikoNet together: one database, both song sources, the new address
// and the old ones.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"ourtaiko.dev/net/db"
	"ourtaiko.dev/net/internal/account"
	"ourtaiko.dev/net/internal/awards"
	fanmade "ourtaiko.dev/net/internal/fanmade/httpapi"
	"ourtaiko.dev/net/internal/groups"
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
	a.Players = profile.New(shared, profile.Config{
		Origin: cfg.Origin, CookieSecure: cfg.CookieSecure, SSO: cfg.SSO, DevUserID: cfg.DevUserID, TrustedProxies: cfg.TrustedProxies,
		SessionCookie: SessionCookie, Groups: true,
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
	fanmadeProfile, err := fanmade.NewProfile(fanmade.ProfileConfig{URL: "http://127.0.0.1", ServiceID: "fanmade", ServiceKey: unused})
	if err != nil {
		a.Close()
		return nil, err
	}
	officialProfile, err := official.NewProfile(official.ProfileConfig{URL: "http://127.0.0.1", ServiceID: "official", ServiceKey: unused})
	if err != nil {
		a.Close()
		return nil, err
	}
	fanmadeProfile.UseTransport(catalog)
	officialProfile.UseTransport(catalog)

	a.Fanmade = fanmade.New(fanmadePool, fanmade.Config{
		Origin: cfg.Origin, Storage: filepath.Join(cfg.StorageDir, "fanmade"), Objects: cfg.FanmadeObjects,
		CookieSecure: cfg.CookieSecure, SSO: cfg.SSO, DevUserID: cfg.DevUserID, TrustedProxies: cfg.TrustedProxies,
		Profile: fanmadeProfile, Awards: sourceAwards{a.Players, "fanmade"}, SessionCookie: SessionCookie, Manager: in(groups.FanmadeManager),
	})
	a.Official = official.New(officialPool, official.Config{
		Origin: cfg.Origin, Storage: filepath.Join(cfg.StorageDir, "official"), Objects: cfg.OfficialObjects,
		CookieSecure: cfg.CookieSecure, SSO: cfg.SSO, DevUserID: cfg.DevUserID, TrustedProxies: cfg.TrustedProxies,
		Profile: officialProfile, Awards: sourceAwards{a.Players, "official"}, SessionCookie: SessionCookie, Manager: in(groups.OfficialManager),
	})
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
	go a.Official.RunFileCleanup(ctx)
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
