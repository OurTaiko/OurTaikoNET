// backup is a host-side maintenance command, never an HTTP endpoint or API startup job: it
// dumps one database of the shared PostgreSQL container, proves the dump by restoring it,
// keeps it for BACKUP_RETENTION_DAYS and uploads it to the private bucket. It came from the
// Fanmade repository, where the same program backs up the old sites' databases.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"ourtaiko.dev/net/internal/kit/backup"
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Database backup failed:", err)
		os.Exit(1)
	}
}
func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 110*time.Minute)
	defer cancel()
	days, err := strconv.Atoi(env("BACKUP_RETENTION_DAYS", "30"))
	if err != nil {
		return fmt.Errorf("invalid retention days")
	}
	database := env("BACKUP_DATABASE", "ourtaiko_net")
	runner := backup.Runner{Config: backup.Config{Database: database, Directory: env("BACKUP_DIRECTORY", "/var/backups/ourtaiko-net/daily"), RetentionDays: days}, Dumper: backup.DockerDumper{Container: env("BACKUP_POSTGRES_CONTAINER", "ourtaiko-postgres"), User: env("BACKUP_POSTGRES_USER", "postgres")}}
	enabled, err := strconv.ParseBool(env("BACKUP_S3_ENABLED", "true"))
	if err != nil {
		return fmt.Errorf("invalid BACKUP_S3_ENABLED")
	}
	if enabled {
		runner.Remote, err = backup.NewS3(ctx, os.Getenv("S3_BUCKET"), os.Getenv("AWS_REGION"), env("BACKUP_S3_PREFIX", "net/backups/postgresql"), database)
		if err != nil {
			return err
		}
	}
	m, err := runner.Run(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("Database backup completed: id=%s database=%s bytes=%d s3=%t\n", m.ID, m.Database, m.Bytes, enabled)
	return nil
}
