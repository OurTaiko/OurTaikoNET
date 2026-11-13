// Package netdb opens connections to ourtaiko_net.
//
// The database has one schema per song source (fanmade, official) and one, net, for what
// the site has once. Code that works on a source uses a pool opened for that source: its
// search_path is "<source>, net", so SQL written without schema names finds the source's
// tables and the shared ones.
package netdb

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Sources are the song sources, in the order they are shown.
var Sources = []string{"fanmade", "official"}

// IsSource reports whether name is a song source.
func IsSource(name string) bool {
	for _, s := range Sources {
		if s == name {
			return true
		}
	}
	return false
}

// SearchPath is the search_path of a connection that works on one source.
func SearchPath(source string) string {
	return pgx.Identifier{source}.Sanitize() + ", net"
}

// OpenShared connects to ourtaiko_net for work that belongs to no source: players, titles,
// nameplates. Its search_path is net alone.
func OpenShared(ctx context.Context, url string) (*pgxpool.Pool, error) {
	return open(ctx, url, "net")
}

// Open connects to ourtaiko_net for one source.
func Open(ctx context.Context, url, source string) (*pgxpool.Pool, error) {
	if !IsSource(source) {
		return nil, fmt.Errorf("unknown song source %q", source)
	}
	return open(ctx, url, SearchPath(source))
}

func open(ctx context.Context, url, searchPath string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("invalid database URL")
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = searchPath
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err = pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}
