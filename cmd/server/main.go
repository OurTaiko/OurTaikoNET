// Command server is OurTaikoNet: both song sources from one database, at one address.
package main

import (
	"context"
	"encoding/hex"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"ourtaiko.dev/net/internal/kit/distribution"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/oschwald/maxminddb-golang/v2"
	"ourtaiko.dev/net/internal/account"
	"ourtaiko.dev/net/internal/app"
	"ourtaiko.dev/net/internal/kit/charthash"
	"ourtaiko.dev/net/internal/kit/objectstore"
	"ourtaiko.dev/net/internal/kit/tja"
	"ourtaiko.dev/net/internal/web"
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func loopback(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return host == "localhost" || (ip != nil && ip.IsLoopback())
}

func validUserID(id string) bool {
	b, err := hex.DecodeString(id)
	return err == nil && len(b) == 16 && strings.ToLower(id) == id
}

// objects is one source's file store: a folder of STORAGE_DIR, its own prefix of the shared
// bucket, or (public) that prefix read over its public address and never written: what a
// preview running beside the site that owns the files is given.
func objects(source, storage string) (objectstore.Store, error) {
	prefix := strings.ToUpper(source)
	switch os.Getenv("STORAGE_BACKEND") {
	case "", "local":
		return objectstore.Local{Root: filepath.Join(storage, source)}, nil
	case "s3":
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		// A source may have its own access key (FANMADE_AWS_ACCESS_KEY_ID and
		// FANMADE_AWS_SECRET_ACCESS_KEY, and the same two for OFFICIAL): the keys the two old
		// sites used are each allowed their own prefix only. Without them, the environment's.
		store, err := objectstore.NewS3As(ctx, os.Getenv("S3_BUCKET"), os.Getenv("AWS_REGION"), os.Getenv(prefix+"_S3_PREFIX"), os.Getenv(prefix+"_AWS_ACCESS_KEY_ID"), os.Getenv(prefix+"_AWS_SECRET_ACCESS_KEY"))
		if err != nil {
			return nil, err
		}
		return store, store.SetPublicBaseURL(os.Getenv(prefix + "_S3_PUBLIC_BASE_URL"))
	case "public":
		return objectstore.NewPublic(os.Getenv(prefix+"_S3_PREFIX"), os.Getenv(prefix+"_S3_PUBLIC_BASE_URL"))
	default:
		return nil, errors.New("STORAGE_BACKEND must be local, s3 or public")
	}
}

func main() {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Fatal("Cannot parse .env; check its syntax")
	}
	initOnly := flag.Bool("init", false, "create the schema in an empty database and exit")
	flag.Parse()
	url := env("DATABASE_URL", "postgres:///ourtaiko_net_dev?host=/tmp&sslmode=disable")
	start, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	if *initOnly {
		if err := app.Init(start, url); err != nil {
			log.Fatal(err)
		}
		log.Print("schema created")
		return
	}
	listen := env("LISTEN_ADDR", "127.0.0.1:8090")
	storage := env("STORAGE_DIR", ".data")
	cfg := app.Config{
		DatabaseURL: url, Origin: env("APP_ORIGIN", "http://127.0.0.1:5173"), StorageDir: storage,
		CookieSecure:      env("COOKIE_SECURE", "false") == "true",
		WebDir:            os.Getenv("WEB_DIR"),
		LegacyFanmadeHost: os.Getenv("LEGACY_FANMADE_HOST"), LegacyOfficialHost: os.Getenv("LEGACY_OFFICIAL_HOST"), LegacyProfileHost: os.Getenv("LEGACY_PROFILE_HOST"),
		FanmadeSite: strings.TrimRight(os.Getenv("FANMADE_SITE"), "/"), OfficialSite: strings.TrimRight(os.Getenv("OFFICIAL_SITE"), "/"),
	}
	var err error
	if cfg.TrustedProxies, err = web.ParseTrustedProxies(os.Getenv("TRUSTED_PROXY_CIDRS")); err != nil {
		log.Fatal(err)
	}
	cfg.DonCatalogURL = os.Getenv("DON_CATALOG_URL")
	cfg.DonMirrorCatalogURL = os.Getenv("DON_MIRROR_CATALOG_URL")
	// The regional Don mirror needs a good catalog address and the IP country database.
	if cfg.DonMirrorCatalogURL != "" {
		if _, err := distribution.ParseMirrorCatalogURL(cfg.DonMirrorCatalogURL); err != nil {
			log.Fatal(err)
		}
		db, err := maxminddb.Open(os.Getenv("DON_GEOIP_DB"))
		if err != nil {
			log.Fatalf("DON_GEOIP_DB: %v", err)
		}
		defer db.Close()
		cfg.DonIPCountry = func(ip netip.Addr) string {
			var country string
			if err := db.Lookup(ip).DecodePath(&country, "country", "iso_code"); err != nil {
				return ""
			}
			return country
		}
	}
	// Multi-Enso, the game's online rooms: a gRPC service on its own port in this program
	// (MULTIPLAY_LISTEN_ADDR), or the program cmd/rooms, which asks this one at
	// MULTIPLAY_INTERNAL_ADDR. Off without either.
	cfg.Multiplay = app.Multiplay{ListenAddr: os.Getenv("MULTIPLAY_LISTEN_ADDR"), InternalAddr: os.Getenv("MULTIPLAY_INTERNAL_ADDR"), PublicURL: strings.TrimRight(os.Getenv("MULTIPLAY_PUBLIC_URL"), "/")}
	if raw := os.Getenv("MULTIPLAY_MIN_PLAYERS"); raw != "" {
		if cfg.Multiplay.MinPlayers, err = strconv.Atoi(raw); err != nil || cfg.Multiplay.MinPlayers < 1 {
			log.Fatal("MULTIPLAY_MIN_PLAYERS must be a number from 1 up")
		}
	}
	devUser := os.Getenv("DEV_USER_ID")
	if os.Getenv("SSO_ISSUER") != "" {
		if devUser != "" {
			log.Fatal("DEV_USER_ID cannot be combined with SSO_ISSUER")
		}
		// One Application of the account center for the whole site, and one client of it.
		if cfg.SSO, err = account.New(account.Config{Issuer: os.Getenv("SSO_ISSUER"), AvatarBaseURL: os.Getenv("SSO_AVATAR_BASE_URL"), ConnectAddress: os.Getenv("SSO_CONNECT_ADDRESS"), ServiceID: os.Getenv("SSO_SERVICE_ID"), ServiceKey: os.Getenv("SSO_SERVICE_KEY"), ClientID: os.Getenv("SSO_CLIENT_ID"), ClientSecret: os.Getenv("SSO_CLIENT_SECRET"), RedirectURL: os.Getenv("SSO_REDIRECT_URL"), EncryptionKey: os.Getenv("SESSION_ENCRYPTION_KEY")}); err != nil {
			log.Fatal(err)
		}
	} else if devUser != "" {
		if !validUserID(devUser) || !loopback(listen) {
			log.Fatal("DEV_USER_ID needs a 32-character lowercase hexadecimal ID and a loopback LISTEN_ADDR")
		}
		// The development game session opens Multi-Enso as well.
		if cfg.Multiplay.ListenAddr != "" && !loopback(cfg.Multiplay.ListenAddr) {
			log.Fatal("DEV_USER_ID needs a loopback MULTIPLAY_LISTEN_ADDR")
		}
		if cfg.Multiplay.InternalAddr != "" && !loopback(cfg.Multiplay.InternalAddr) {
			log.Fatal("DEV_USER_ID needs a loopback MULTIPLAY_INTERNAL_ADDR")
		}
		cfg.DevUserID = devUser
		log.Printf("development sign-in enabled for user %s; never use this outside local development", devUser)
	} else {
		log.Print("no SSO_ISSUER: catalogues are public, sign-in and uploads are unavailable")
	}
	if cfg.FanmadeObjects, err = objects("fanmade", storage); err != nil {
		log.Fatal(err)
	}
	if cfg.OfficialObjects, err = objects("official", storage); err != nil {
		log.Fatal(err)
	}
	// TJAParser reads chart bodies for every upload; a missing or broken tool is a deployment
	// error, found now rather than at the first upload.
	tja.Inspector = charthash.CLI{Bin: env("TJAPARSER_BIN", "tjaparser")}
	tja.InspectDir = filepath.Join(storage, "inspect")
	if err = os.MkdirAll(tja.InspectDir, 0o700); err != nil {
		log.Fatal(err)
	}
	toolCtx, cancelTool := context.WithTimeout(context.Background(), 30*time.Second)
	version, err := tja.Inspector.(charthash.CLI).Version(toolCtx)
	cancelTool()
	if err != nil {
		log.Fatalf("tjaparser is not usable: %v", err)
	}
	log.Printf("tjaparser hash algorithm %s", version)

	a, err := app.New(start, cfg)
	cancel()
	if err != nil {
		log.Fatal(err)
	}
	defer a.Close()
	background, stopBackground := context.WithCancel(context.Background())
	defer stopBackground()
	if err = a.Start(background); err != nil {
		log.Fatal(err)
	}
	srv := &http.Server{Addr: listen, Handler: a.Handler(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 180 * time.Second, WriteTimeout: 240 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 * 1024}
	go func() {
		log.Printf("OurTaikoNet listening on http://%s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	// The rooms first: their players are told the room is closed, which takes a moment, while
	// requests in flight may take the whole of what Shutdown allows.
	a.StopRooms()
	shutdown, cancelShutdown := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelShutdown()
	srv.Shutdown(shutdown)
}
