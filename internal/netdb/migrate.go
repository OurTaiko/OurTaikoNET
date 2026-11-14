package netdb

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"ourtaiko.dev/net/db"
)

// migrateLock keeps two programs starting at once from applying the same change twice.
const migrateLock = 0x6f746e6d6967 // "otnmig"

// Migrate brings a database that holds ourtaiko_net up to the schema this program was built
// with, applying each change it lacks in a transaction of its own, and reports the versions
// applied. A database newer than the program is refused: an older program must not run on it.
func Migrate(ctx context.Context, url string) ([]int, error) {
	list, err := db.Migrations()
	if err != nil {
		return nil, err
	}
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		return nil, err
	}
	defer conn.Close(context.WithoutCancel(ctx))
	if _, err = conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, int64(migrateLock)); err != nil {
		return nil, err
	}
	defer conn.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock($1)`, int64(migrateLock))
	var version int
	if err = conn.QueryRow(ctx, `SELECT COALESCE(max(version),0) FROM net.schema_migrations`).Scan(&version); err != nil {
		return nil, fmt.Errorf("this database is not ourtaiko_net (run with -init on an empty database): %w", err)
	}
	if version > db.Version() {
		return nil, fmt.Errorf("database schema is version %d, this program needs %d", version, db.Version())
	}
	var applied []int
	for _, m := range list {
		if m.Version <= version {
			continue
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			return applied, err
		}
		// The simple protocol: a migration is several statements.
		if _, err = tx.Conn().PgConn().Exec(ctx, m.SQL).ReadAll(); err == nil {
			_, err = tx.Exec(ctx, `INSERT INTO net.schema_migrations(version) VALUES ($1)`, m.Version)
		}
		if err != nil {
			tx.Rollback(context.WithoutCancel(ctx))
			return applied, fmt.Errorf("migration %04d_%s: %w", m.Version, m.Name, err)
		}
		if err = tx.Commit(ctx); err != nil {
			return applied, err
		}
		applied = append(applied, m.Version)
	}
	return applied, nil
}
