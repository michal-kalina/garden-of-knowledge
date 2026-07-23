// Command worker runs the ingestion pipeline against the Postgres-backed
// job queue. See internal/worker for the pipeline itself.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/michal-kalina/garden-of-knowledge/backend/internal/chunking"
	"github.com/michal-kalina/garden-of-knowledge/backend/internal/config"
	"github.com/michal-kalina/garden-of-knowledge/backend/internal/database"
	"github.com/michal-kalina/garden-of-knowledge/backend/internal/embeddings"
	"github.com/michal-kalina/garden-of-knowledge/backend/internal/parserclient"
	"github.com/michal-kalina/garden-of-knowledge/backend/internal/storage"
	"github.com/michal-kalina/garden-of-knowledge/backend/internal/telemetry"
	"github.com/michal-kalina/garden-of-knowledge/backend/internal/worker"
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

	shutdownTelemetry, err := telemetry.Setup(ctx, "gok-worker")
	if err != nil {
		return fmt.Errorf("setup telemetry: %w", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTelemetry(shutdownCtx); err != nil {
			logger.Error("telemetry shutdown failed", "error", err)
		}
	}()

	db, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := database.Migrate(ctx, db, logger); err != nil {
		return err
	}
	logger.Info("database ready")

	store, err := storage.NewMinIO(cfg.S3Endpoint, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3Bucket, cfg.S3UseSSL)
	if err != nil {
		return err
	}

	embedder := embeddings.FromProvider(cfg.EmbeddingsProvider, cfg.VoyageAPIKey, cfg.VoyageModel, logger)

	proc := &worker.Processor{
		DB:       db,
		Store:    store,
		Parser:   parserclient.New(cfg.ParserURL),
		Embedder: embedder,
		// Structure-aware is the default strategy (heading inheritance,
		// atomic tables); Phase 4 evals will compare it against the
		// recursive baseline and make this configurable.
		Chunker: chunking.StructureChunker{TargetTokens: 500},
		Logger:  logger,
	}

	logger.Info("worker started", "poll_interval", cfg.WorkerPollInterval.String())
	proc.Run(ctx, cfg.WorkerPollInterval)
	return nil
}
