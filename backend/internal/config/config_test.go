package config

import (
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	t.Run("requires DATABASE_URL", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "")
		if _, err := Load(); err == nil {
			t.Fatal("expected error when DATABASE_URL is empty")
		}
	})

	t.Run("applies defaults", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://x")
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
		t.Setenv("WORKER_POLL_INTERVAL", "banana")
		if _, err := Load(); err == nil {
			t.Fatal("expected error for invalid duration")
		}
	})

	t.Run("parses S3 and upload settings", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://x")
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
		t.Setenv("EMBEDDINGS_PROVIDER", "voyage")
		t.Setenv("VOYAGE_API_KEY", "")
		if _, err := Load(); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("rejects unknown embeddings provider", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://x")
		t.Setenv("EMBEDDINGS_PROVIDER", "quantum")
		if _, err := Load(); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("anthropic model has a default and is overridable", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://x")
		t.Setenv("ANTHROPIC_MODEL", "")
		cfg, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.AnthropicModel == "" {
			t.Error("AnthropicModel default missing")
		}
		t.Setenv("ANTHROPIC_MODEL", "claude-x")
		cfg, _ = Load()
		if cfg.AnthropicModel != "claude-x" {
			t.Errorf("AnthropicModel = %q", cfg.AnthropicModel)
		}
	})

	t.Run("rejects invalid MAX_UPLOAD_BYTES", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://x")
		t.Setenv("MAX_UPLOAD_BYTES", "-1")
		if _, err := Load(); err == nil {
			t.Fatal("expected error for negative limit")
		}
	})
}
