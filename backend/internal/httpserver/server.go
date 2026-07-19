// Package httpserver contains the API router and basic middleware.
package httpserver

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

type server struct {
	db             *sql.DB
	docs           DocumentService
	search         SearchService
	logger         *slog.Logger
	maxUploadBytes int64
}

// Deps carries the server's dependencies. A struct (rather than a growing
// positional parameter list) keeps call sites readable as endpoints
// accumulate.
type Deps struct {
	DB             *sql.DB
	Documents      DocumentService
	Search         SearchService
	MaxUploadBytes int64
	Logger         *slog.Logger
}

// New builds the API router.
func New(d Deps) http.Handler {
	s := &server{
		db:             d.DB,
		docs:           d.Documents,
		search:         d.Search,
		logger:         d.Logger,
		maxUploadBytes: d.MaxUploadBytes,
	}

	mux := http.NewServeMux()

	// Liveness: the process is up. Checks no dependencies.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Readiness: the service can take traffic (database reachable).
	// The liveness/readiness split matters under Kubernetes (Phase 5).
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := s.db.PingContext(r.Context()); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"status": "unavailable", "reason": "database unreachable",
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})

	mux.HandleFunc("POST /documents", s.handleDocumentUpload)
	mux.HandleFunc("GET /documents", s.handleDocumentList)
	mux.HandleFunc("GET /documents/{id}", s.handleDocumentGet)
	mux.HandleFunc("POST /search", s.handleSearch)

	return logging(d.Logger)(mux)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// logging is a minimal structured access-log middleware.
func logging(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r)
			logger.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", sw.status,
				"duration_ms", time.Since(start).Milliseconds(),
			)
		})
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}
