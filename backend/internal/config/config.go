// Package config reads application configuration from environment variables.
//
// Intentionally not using a library (viper etc.) — at this stage a few
// environment variables don't justify an additional dependency.
package config

import (
	"fmt"
	"os"
	"time"
)

type Config struct {
	// DatabaseURL is the Data Source Name for the PostgreSQL database, e.g., postgres://user:pass@host:5432/db?sslmode=disable
	DatabaseURL string
	// HTTPAddr is the address to listen on for the API, e.g., ":8080".
	HTTPAddr string
	// ParserURL is the base URL for the document parsing service (Python).
	ParserURL string
	// WorkerPollInterval is the interval at which the worker checks the task queue.
	WorkerPollInterval time.Duration
}

func Load() (Config, error) {
	cfg := Config{
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		HTTPAddr:           getenvDefault("HTTP_ADDR", ":8080"),
		ParserURL:          getenvDefault("PARSER_URL", "http://localhost:8000"),
		WorkerPollInterval: 2 * time.Second,
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
	return cfg, nil
}

func getenvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
