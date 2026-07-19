package database

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"path"
	"sort"
)

// Migrations are embedded into the binary and applied at startup by both the
// api and the worker. A ~80-line runner instead of goose/golang-migrate is a
// deliberate trade-off (ADR-0005): zero extra dependencies, and the whole
// mechanism — version table, locking, per-file transactions — stays readable
// on one screen.
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

// migrateLockKey is an arbitrary application-wide advisory lock id. It makes
// concurrent startups (api + worker, or several replicas under Kubernetes)
// serialize on migrations instead of racing.
const migrateLockKey = 743651209

// Migrate applies all pending migrations in filename order. Each file runs
// in its own transaction and is recorded in schema_migrations, so a failed
// migration leaves the database at the last known-good version.
func Migrate(ctx context.Context, db *sql.DB, logger *slog.Logger) error {
	// The advisory lock is session-scoped, so everything must happen on one
	// pinned connection — db.Exec could use a different pool connection for
	// the unlock.
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection: %w", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", migrateLockKey); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() {
		// Unlock with a fresh context: the lock must be released even when
		// the caller's context is already cancelled during shutdown.
		_, _ = conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", migrateLockKey)
	}()

	if _, err := conn.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}

	names, err := fs.Glob(migrationsFS, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)

	for _, name := range names {
		version := path.Base(name)

		var applied bool
		if err := conn.QueryRowContext(ctx,
			`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, version,
		).Scan(&applied); err != nil {
			return fmt.Errorf("check migration %s: %w", version, err)
		}
		if applied {
			continue
		}

		contents, err := migrationsFS.ReadFile(name)
		if err != nil {
			return err
		}

		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		// Exec without parameters uses the simple query protocol, so a file
		// may contain multiple statements.
		if _, err := tx.ExecContext(ctx, string(contents)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", version, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO schema_migrations (version) VALUES ($1)`, version,
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %s: %w", version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", version, err)
		}
		logger.Info("applied migration", "version", version)
	}
	return nil
}
