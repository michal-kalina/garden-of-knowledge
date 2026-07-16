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
}
