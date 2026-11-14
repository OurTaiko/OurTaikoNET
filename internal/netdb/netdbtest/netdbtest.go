// Package netdbtest gives each test its own ourtaiko_net.
package netdbtest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"ourtaiko.dev/net/db"
	"ourtaiko.dev/net/internal/netdb"
)

var serial atomic.Int64

// templateLock serializes making the template across test processes.
const templateLock = 73420119

// New returns a pool for one source of a fresh, empty ourtaiko_net on the server named by
// DATABASE_TEST_URL (a role that may create databases). The database is dropped when the
// test ends. Without DATABASE_TEST_URL the test is skipped.
//
// Each database is a copy of a template made once per baseline, so a test costs a file copy,
// not a replay of the schema.
func New(t *testing.T, source string) *pgxpool.Pool {
	t.Helper()
	pool, _ := NewURL(t, source)
	return pool
}

// NewShared is New for work that belongs to no source (netdb.OpenShared).
func NewShared(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, _ := NewURL(t, "")
	return pool
}

// NewURL is New, also returning the database's URL for code that opens its own connections.
// An empty source gives a shared pool.
func NewURL(t *testing.T, source string) (*pgxpool.Pool, string) {
	t.Helper()
	admin := os.Getenv("DATABASE_TEST_URL")
	if admin == "" {
		t.Skip("set DATABASE_TEST_URL to test against PostgreSQL")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	template, err := ensureTemplate(ctx, conn, admin)
	if err != nil {
		conn.Close(ctx)
		t.Fatal(err)
	}
	name := fmt.Sprintf("otn_test_%d_%d_%d", os.Getpid(), time.Now().UnixNano(), serial.Add(1))
	if _, err = conn.Exec(ctx, "CREATE DATABASE "+name+" TEMPLATE "+template); err != nil {
		conn.Close(ctx)
		t.Fatal(err)
	}
	target := withDatabase(admin, name)
	var pool *pgxpool.Pool
	if source == "" {
		pool, err = netdb.OpenShared(ctx, target)
	} else {
		pool, err = netdb.Open(ctx, target, source)
	}
	t.Cleanup(func() {
		if pool != nil {
			pool.Close()
		}
		conn.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
		conn.Close(ctx)
	})
	if err != nil {
		t.Fatal(err)
	}
	return pool, target
}

func withDatabase(admin, name string) string {
	u, err := url.Parse(admin)
	if err != nil {
		panic(err)
	}
	u.Path = "/" + name
	return u.String()
}

// ensureTemplate makes the template database for the current baseline if it is not there.
func ensureTemplate(ctx context.Context, conn *pgx.Conn, admin string) (string, error) {
	// The template is the baseline with every later change applied; another set of changes is
	// another template.
	changes, err := db.Migrations()
	if err != nil {
		return "", err
	}
	text := db.Baseline
	for _, m := range changes {
		text += "\x00" + m.SQL
	}
	sum := sha256.Sum256([]byte(text))
	name := "otn_template_" + hex.EncodeToString(sum[:6])
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", templateLock); err != nil {
		return "", err
	}
	defer conn.Exec(ctx, "SELECT pg_advisory_unlock($1)", templateLock)
	var ready bool
	if err := conn.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname=$1 AND datistemplate)", name).Scan(&ready); err != nil {
		return "", err
	}
	if ready {
		return name, nil
	}
	// A leftover from an interrupted run is not marked as a template yet: start over.
	if _, err := conn.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)"); err != nil {
		return "", err
	}
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		return "", err
	}
	build, err := pgx.Connect(ctx, withDatabase(admin, name))
	if err != nil {
		return "", err
	}
	_, err = build.PgConn().Exec(ctx, db.Baseline).ReadAll()
	build.Close(ctx)
	if err != nil {
		return "", fmt.Errorf("baseline: %w", err)
	}
	if _, err = netdb.Migrate(ctx, withDatabase(admin, name)); err != nil {
		return "", err
	}
	if _, err = conn.Exec(ctx, "ALTER DATABASE "+name+" IS_TEMPLATE true"); err != nil {
		return "", err
	}
	return name, nil
}
