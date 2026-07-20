// Command api runs the Garden of Knowledge HTTP server.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/michal-kalina/garden-of-knowledge/backend/internal/chat"
	"github.com/michal-kalina/garden-of-knowledge/backend/internal/config"
	"github.com/michal-kalina/garden-of-knowledge/backend/internal/database"
	"github.com/michal-kalina/garden-of-knowledge/backend/internal/documents"
	"github.com/michal-kalina/garden-of-knowledge/backend/internal/embeddings"
	"github.com/michal-kalina/garden-of-knowledge/backend/internal/httpserver"
	"github.com/michal-kalina/garden-of-knowledge/backend/internal/llm"
	"github.com/michal-kalina/garden-of-knowledge/backend/internal/retrieval"
	"github.com/michal-kalina/garden-of-knowledge/backend/internal/storage"
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
	if err := database.Migrate(ctx, db, logger); err != nil {
		return err
	}
	logger.Info("database ready")

	store, err := storage.NewMinIO(cfg.S3Endpoint, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3Bucket, cfg.S3UseSSL)
	if err != nil {
		return err
	}
	if err := store.EnsureBucket(ctx); err != nil {
		return err
	}
	logger.Info("object storage ready", "bucket", cfg.S3Bucket)

	docs := documents.NewService(store, documents.NewRepository(db))
	embedder := embeddings.FromProvider(cfg.EmbeddingsProvider, cfg.VoyageAPIKey, cfg.VoyageModel, logger)
	searcher := retrieval.New(db, embedder)

	var chatSvc httpserver.ChatService
	if cfg.LLMProvider != "" {
		streamer, err := llm.FromProvider(cfg.LLMProvider, cfg.LLMModel, cfg.LLMAPIKey)
		if err != nil {
			return err
		}
		chatSvc = chat.New(searcher, streamer)
		logger.Info("chat enabled", "provider", cfg.LLMProvider, "model", cfg.LLMModel)
	} else {
		logger.Warn("chat disabled: set ANTHROPIC_API_KEY, or OPENROUTER_API_KEY with LLM_MODEL")
	}

	srv := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: httpserver.New(httpserver.Deps{
			DB:             db,
			Documents:      docs,
			Search:         searcher,
			Chat:           chatSvc,
			MaxUploadBytes: cfg.MaxUploadBytes,
			Logger:         logger,
		}),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("api listening", "addr", cfg.HTTPAddr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		logger.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return nil
	}
}
