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

	// VoyageModel selects the Voyage embedding model. It must produce
	// vectors matching the vector(1024) column (the client pins the
	// dimension explicitly).
	VoyageModel string

	// LLMProvider selects the chat backend: "anthropic" or "openrouter".
	// Empty (no key configured) leaves /chat unconfigured (503) while the
	// rest of the API works. Inferred from which API key is set when not
	// given explicitly; anthropic wins if both keys are present.
	LLMProvider string
	// LLMModel is provider-specific: an Anthropic model id, or an
	// OpenRouter id like "anthropic/claude-sonnet-4.5". Required for
	// openrouter (hundreds of models — an implicit default would be a
	// guess); defaults for anthropic.
	LLMModel string
	// LLMAPIKey is resolved from the provider-specific env var.
	LLMAPIKey string
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
	cfg.VoyageModel = getenvDefault("VOYAGE_MODEL", "voyage-4")

	anthropicKey := os.Getenv("ANTHROPIC_API_KEY")
	openrouterKey := os.Getenv("OPENROUTER_API_KEY")
	cfg.LLMProvider = os.Getenv("LLM_PROVIDER")
	if cfg.LLMProvider == "" {
		switch {
		case anthropicKey != "":
			cfg.LLMProvider = "anthropic"
		case openrouterKey != "":
			cfg.LLMProvider = "openrouter"
		}
	}
	cfg.LLMModel = os.Getenv("LLM_MODEL")
	switch cfg.LLMProvider {
	case "":
		// Chat stays disabled.
	case "anthropic":
		if anthropicKey == "" {
			return Config{}, fmt.Errorf("LLM_PROVIDER=anthropic requires ANTHROPIC_API_KEY")
		}
		cfg.LLMAPIKey = anthropicKey
		if cfg.LLMModel == "" {
			cfg.LLMModel = "claude-sonnet-4-6"
		}
	case "openrouter":
		if openrouterKey == "" {
			return Config{}, fmt.Errorf("LLM_PROVIDER=openrouter requires OPENROUTER_API_KEY")
		}
		cfg.LLMAPIKey = openrouterKey
		if cfg.LLMModel == "" {
			return Config{}, fmt.Errorf(
				"LLM_PROVIDER=openrouter requires LLM_MODEL (e.g. anthropic/claude-sonnet-4.5)")
		}
	default:
		return Config{}, fmt.Errorf("unknown LLM_PROVIDER %q", cfg.LLMProvider)
	}
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
