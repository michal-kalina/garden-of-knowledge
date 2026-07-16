// Package database provides a connection to PostgreSQL shared by the API and worker.
//
// We use the standard database/sql — the abstraction allows swapping the driver
// (e.g., with a stdlib-adapter pgx) without changes to the application code.
package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq" // postgres driver registers itself via side effect
)

// Open opens a connection pool and waits until the database is reachable.
// Retry is needed in docker-compose: the api container may start before
// PostgreSQL is ready to accept connections.
func Open(ctx context.Context, dsn string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	const maxAttempts = 15
	for attempt := 1; ; attempt++ {
		pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		err = db.PingContext(pingCtx)
		cancel()
		if err == nil {
			return db, nil
		}
		if attempt >= maxAttempts {
			db.Close()
			return nil, fmt.Errorf("database not reachable after %d attempts: %w", maxAttempts, err)
		}
		select {
		case <-ctx.Done():
			db.Close()
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}
