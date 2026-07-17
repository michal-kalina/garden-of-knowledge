// Command worker processes ingestion jobs from the Postgres-backed queue.
//
// Phase 1 step 1: the worker picks a job via SELECT ... FOR UPDATE SKIP LOCKED
// and completes it, moving the document to 'ready' so the status flow is
// observable end-to-end. The real pipeline (fetch file from MinIO, parse via
// the Python service, chunk, embed) lands in the next steps of Phase 1.
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

// processOne claims a single queued job and processes it. SKIP LOCKED ensures
// multiple worker instances never claim the same job.
func processOne(ctx context.Context, db *sql.DB, logger *slog.Logger) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // rollback after commit is a no-op

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
		return nil // empty queue is the normal idle state
	}
	if err != nil {
		return err
	}

	logger.Info("picked job", "job_id", jobID, "document_id", documentID)

	// TODO(phase-1): fetch the file from MinIO, call the parser, chunk,
	// embed, store chunks — then split this into claim -> work -> finalize
	// with retry accounting. For now the job completes immediately so the
	// pending -> ready status transition is visible in the API.
	_, err = tx.ExecContext(ctx, `
		UPDATE ingestion_jobs
		SET status = 'done', attempts = attempts + 1, updated_at = now()
		WHERE id = $1
	`, jobID)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE documents
		SET status = 'ready', updated_at = now()
		WHERE id = $1
	`, documentID)
	if err != nil {
		return err
	}

	return tx.Commit()
}
