// Package config loads application configuration from environment variables.
//
// Deliberately no config library (viper etc.) — a handful of env vars does
// not justify the dependency.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	// DatabaseURL is the Postgres DSN, e.g. postgres://user:pass@host:5432/db?sslmode=disable
	DatabaseURL string
	// HTTPAddr is the API listen address, e.g. ":8080".
	HTTPAddr string
	// ParserURL is the base URL of the document parsing service (Python).
	ParserURL string
	// WorkerPollInterval controls how often the worker polls the job queue.
	WorkerPollInterval time.Duration

	// S3Endpoint is the object storage endpoint as host:port, without scheme
	// (the scheme is chosen by S3UseSSL, as the MinIO SDK expects).
	S3Endpoint  string
	S3AccessKey string
	S3SecretKey string
	S3Bucket    string
	S3UseSSL    bool

	// MaxUploadBytes caps the size of a single document upload.
	MaxUploadBytes int64

	// EmbeddingsProvider selects the Embedder implementation: "voyage" or
	// "fake". Defaults to "voyage" when VOYAGE_API_KEY is set, otherwise
	// "fake" (offline development).
	EmbeddingsProvider string
	VoyageAPIKey       string
}

func Load() (Config, error) {
	cfg := Config{
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		HTTPAddr:           getenvDefault("HTTP_ADDR", ":8080"),
		ParserURL:          getenvDefault("PARSER_URL", "http://localhost:8000"),
		WorkerPollInterval: 2 * time.Second,
		S3Endpoint:         getenvDefault("S3_ENDPOINT", "localhost:9000"),
		// Defaults match docker-compose dev credentials; production overrides
		// them via env (secret management arrives with Phase 5 deployment).
		S3AccessKey:    getenvDefault("S3_ACCESS_KEY", "gok"),
		S3SecretKey:    getenvDefault("S3_SECRET_KEY", "gok_dev_password"),
		S3Bucket:       getenvDefault("S3_BUCKET", "documents"),
		MaxUploadBytes: 50 << 20, // 50 MiB
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if v := os.Getenv("WORKER_POLL_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("invalid WORKER_POLL_INTERVAL %q: %w", v, err)
		}
		cfg.WorkerPollInterval = d
	}
	if v := os.Getenv("S3_USE_SSL"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return Config{}, fmt.Errorf("invalid S3_USE_SSL %q: %w", v, err)
		}
		cfg.S3UseSSL = b
	}
	cfg.VoyageAPIKey = os.Getenv("VOYAGE_API_KEY")
	cfg.EmbeddingsProvider = os.Getenv("EMBEDDINGS_PROVIDER")
	if cfg.EmbeddingsProvider == "" {
		if cfg.VoyageAPIKey != "" {
			cfg.EmbeddingsProvider = "voyage"
		} else {
			cfg.EmbeddingsProvider = "fake"
		}
	}
	switch cfg.EmbeddingsProvider {
	case "voyage":
		if cfg.VoyageAPIKey == "" {
			return Config{}, fmt.Errorf("EMBEDDINGS_PROVIDER=voyage requires VOYAGE_API_KEY")
		}
	case "fake":
		// Explicitly allowed: offline dev without an API key.
	default:
		return Config{}, fmt.Errorf("unknown EMBEDDINGS_PROVIDER %q", cfg.EmbeddingsProvider)
	}
	if v := os.Getenv("MAX_UPLOAD_BYTES"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			return Config{}, fmt.Errorf("invalid MAX_UPLOAD_BYTES %q", v)
		}
		cfg.MaxUploadBytes = n
	}
	return cfg, nil
}

func getenvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
