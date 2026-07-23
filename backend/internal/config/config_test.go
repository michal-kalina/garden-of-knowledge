package config

import (
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	t.Run("requires DATABASE_URL", func(t *testing.T) {
		t.Setenv("AUTH_SECRET", "test-secret")
		t.Setenv("DATABASE_URL", "")
		if _, err := Load(); err == nil {
			t.Fatal("expected error when DATABASE_URL is empty")
		}
	})

	t.Run("applies defaults", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://x")
		t.Setenv("AUTH_SECRET", "test-secret")
		t.Setenv("HTTP_ADDR", "")
		cfg, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.HTTPAddr != ":8080" {
			t.Errorf("HTTPAddr = %q, want :8080", cfg.HTTPAddr)
		}
		if cfg.WorkerPollInterval != 2*time.Second {
			t.Errorf("WorkerPollInterval = %v, want 2s", cfg.WorkerPollInterval)
		}
		if cfg.S3Bucket != "documents" {
			t.Errorf("S3Bucket = %q, want documents", cfg.S3Bucket)
		}
		if cfg.MaxUploadBytes != 50<<20 {
			t.Errorf("MaxUploadBytes = %d, want %d", cfg.MaxUploadBytes, 50<<20)
		}
	})

	t.Run("parses poll interval", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://x")
		t.Setenv("AUTH_SECRET", "test-secret")
		t.Setenv("WORKER_POLL_INTERVAL", "500ms")
		cfg, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.WorkerPollInterval != 500*time.Millisecond {
			t.Errorf("WorkerPollInterval = %v, want 500ms", cfg.WorkerPollInterval)
		}
	})

	t.Run("rejects invalid poll interval", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://x")
		t.Setenv("AUTH_SECRET", "test-secret")
		t.Setenv("WORKER_POLL_INTERVAL", "banana")
		if _, err := Load(); err == nil {
			t.Fatal("expected error for invalid duration")
		}
	})

	t.Run("parses S3 and upload settings", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://x")
		t.Setenv("AUTH_SECRET", "test-secret")
		t.Setenv("S3_USE_SSL", "true")
		t.Setenv("MAX_UPLOAD_BYTES", "1024")
		cfg, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if !cfg.S3UseSSL {
			t.Error("S3UseSSL = false, want true")
		}
		if cfg.MaxUploadBytes != 1024 {
			t.Errorf("MaxUploadBytes = %d, want 1024", cfg.MaxUploadBytes)
		}
	})

	t.Run("embeddings provider defaults", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://x")
		t.Setenv("AUTH_SECRET", "test-secret")
		t.Setenv("EMBEDDINGS_PROVIDER", "")
		t.Setenv("VOYAGE_API_KEY", "")
		cfg, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.EmbeddingsProvider != "fake" {
			t.Errorf("provider = %q, want fake without key", cfg.EmbeddingsProvider)
		}

		t.Setenv("VOYAGE_API_KEY", "vk")
		cfg, err = Load()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.EmbeddingsProvider != "voyage" {
			t.Errorf("provider = %q, want voyage with key", cfg.EmbeddingsProvider)
		}
	})

	t.Run("voyage provider requires a key", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://x")
		t.Setenv("AUTH_SECRET", "test-secret")
		t.Setenv("EMBEDDINGS_PROVIDER", "voyage")
		t.Setenv("VOYAGE_API_KEY", "")
		if _, err := Load(); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("rejects unknown embeddings provider", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://x")
		t.Setenv("AUTH_SECRET", "test-secret")
		t.Setenv("EMBEDDINGS_PROVIDER", "quantum")
		if _, err := Load(); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("llm provider is inferred from keys", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://x")
		t.Setenv("AUTH_SECRET", "test-secret")
		t.Setenv("LLM_PROVIDER", "")
		t.Setenv("LLM_MODEL", "")
		t.Setenv("ANTHROPIC_API_KEY", "")
		t.Setenv("OPENROUTER_API_KEY", "")

		cfg, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.LLMProvider != "" {
			t.Errorf("provider = %q, want disabled without keys", cfg.LLMProvider)
		}

		t.Setenv("ANTHROPIC_API_KEY", "ak")
		cfg, err = Load()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.LLMProvider != "anthropic" || cfg.LLMAPIKey != "ak" || cfg.LLMModel == "" {
			t.Errorf("anthropic inference wrong: %+v", cfg)
		}

		t.Setenv("ANTHROPIC_API_KEY", "")
		t.Setenv("OPENROUTER_API_KEY", "ok")
		t.Setenv("LLM_MODEL", "anthropic/claude-sonnet-4.5")
		cfg, err = Load()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.LLMProvider != "openrouter" || cfg.LLMAPIKey != "ok" {
			t.Errorf("openrouter inference wrong: %+v", cfg)
		}
	})

	t.Run("openrouter requires an explicit model", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://x")
		t.Setenv("AUTH_SECRET", "test-secret")
		t.Setenv("OPENROUTER_API_KEY", "ok")
		t.Setenv("LLM_PROVIDER", "openrouter")
		t.Setenv("LLM_MODEL", "")
		if _, err := Load(); err == nil {
			t.Fatal("expected error without LLM_MODEL")
		}
	})

	t.Run("explicit provider without its key errors", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://x")
		t.Setenv("AUTH_SECRET", "test-secret")
		t.Setenv("LLM_PROVIDER", "anthropic")
		t.Setenv("ANTHROPIC_API_KEY", "")
		if _, err := Load(); err == nil {
			t.Fatal("expected error without ANTHROPIC_API_KEY")
		}
	})

	t.Run("voyage model defaults to voyage-4", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://x")
		t.Setenv("AUTH_SECRET", "test-secret")
		t.Setenv("VOYAGE_MODEL", "")
		cfg, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.VoyageModel != "voyage-4" {
			t.Errorf("VoyageModel = %q, want voyage-4", cfg.VoyageModel)
		}
	})

	t.Run("rerank is disabled by default", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://x")
		t.Setenv("AUTH_SECRET", "test-secret")
		t.Setenv("RERANK_PROVIDER", "")
		cfg, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.RerankProvider != "" {
			t.Errorf("RerankProvider = %q, want disabled by default", cfg.RerankProvider)
		}
		if cfg.RerankModel != "rerank-2.5" {
			t.Errorf("RerankModel = %q, want rerank-2.5 default", cfg.RerankModel)
		}
	})

	t.Run("rerank voyage requires VOYAGE_API_KEY", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://x")
		t.Setenv("AUTH_SECRET", "test-secret")
		t.Setenv("RERANK_PROVIDER", "voyage")
		t.Setenv("VOYAGE_API_KEY", "")
		if _, err := Load(); err == nil {
			t.Fatal("expected error without VOYAGE_API_KEY")
		}
		t.Setenv("VOYAGE_API_KEY", "vk")
		cfg, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.RerankProvider != "voyage" {
			t.Errorf("RerankProvider = %q", cfg.RerankProvider)
		}
	})

	t.Run("rejects unknown RERANK_PROVIDER", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://x")
		t.Setenv("AUTH_SECRET", "test-secret")
		t.Setenv("RERANK_PROVIDER", "quantum")
		if _, err := Load(); err == nil {
			t.Fatal("expected error for unknown provider")
		}
	})

	t.Run("rejects invalid MAX_UPLOAD_BYTES", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://x")
		t.Setenv("AUTH_SECRET", "test-secret")
		t.Setenv("MAX_UPLOAD_BYTES", "-1")
		if _, err := Load(); err == nil {
			t.Fatal("expected error for negative limit")
		}
	})
}

func TestAuthConfig(t *testing.T) {
	t.Run("requires AUTH_SECRET", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://x")
		t.Setenv("AUTH_SECRET", "")
		if _, err := Load(); err == nil {
			t.Fatal("expected error without AUTH_SECRET")
		}
	})

	t.Run("token ttl default and override", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://x")
		t.Setenv("AUTH_SECRET", "s")
		cfg, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.AuthTokenTTL != 7*24*time.Hour {
			t.Errorf("ttl = %v", cfg.AuthTokenTTL)
		}
		t.Setenv("AUTH_TOKEN_TTL", "1h")
		cfg, _ = Load()
		if cfg.AuthTokenTTL != time.Hour {
			t.Errorf("ttl override = %v", cfg.AuthTokenTTL)
		}
	})
}
