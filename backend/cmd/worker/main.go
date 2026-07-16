// Command worker processes tasks from the ingestion queue in PostgreSQL.
//
// Phase 0: skeleton — worker picks up a task via SELECT ... FOR UPDATE SKIP LOCKED
// and marks it as done. The actual pipeline (file download from MinIO,
// parsing by Python service, chunking, embeddings) will be added in Phase 1.
package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/michal-kalina/garden-of-knowledge/backend/internal/config"
	"github.com/michal-kalina/garden-of-knowledge/backend/internal/database"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	db, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	logger.Info("worker started", "poll_interval", cfg.WorkerPollInterval.String())

	ticker := time.NewTicker(cfg.WorkerPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("worker shutting down")
			return nil
		case <-ticker.C:
			if err := processOne(ctx, db, logger); err != nil {
				logger.Error("process job", "error", err)
			}
		}
	}
}

// processOne takes a single task from the queue and processes it.
// SKIP LOCKED ensures that multiple worker instances will not pick up the same task.
func processOne(ctx context.Context, db *sql.DB, logger *slog.Logger) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // rollback after commit is no-open

	var jobID int64
	var documentID string
	err = tx.QueryRowContext(ctx, `
		SELECT id, document_id
		FROM ingestion_jobs
		WHERE status = 'queued'
		ORDER BY created_at
		FOR UPDATE SKIP LOCKED
		LIMIT 1
	`).Scan(&jobID, &documentID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // empty queue — this is a normal state
	}
	if err != nil {
		return err
	}

	logger.Info("picked job", "job_id", jobID, "document_id", documentID)

	// TODO(Phase 1): fetch file from MinIO, call parser, chunking, embeddings,
	// save chunks and set documents.status = 'ready'.
	// For now we mark the job as done to practice the queue mechanism.
	_, err = tx.ExecContext(ctx, `
		UPDATE ingestion_jobs
		SET status = 'done', attempts = attempts + 1, updated_at = now()
		WHERE id = $1
	`, jobID)
	if err != nil {
		return err
	}

	return tx.Commit()
}
