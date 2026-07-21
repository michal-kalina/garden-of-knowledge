package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/michal-kalina/garden-of-knowledge/backend/internal/retrieval"
)

// SearchService is what the HTTP layer needs from retrieval.
type SearchService interface {
	Search(ctx context.Context, userID, query string, limit int) ([]retrieval.Result, error)
}

type searchRequest struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

// handleSearch exposes hybrid retrieval directly. Besides powering future
// tooling, it is the debugging window into the RAG: the response carries
// per-retriever ranks, so "why did the chat cite this?" has an answer.
func (s *server) handleSearch(w http.ResponseWriter, r *http.Request, userID string) {
	var req searchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Query = strings.TrimSpace(req.Query)
	if req.Query == "" {
		writeError(w, http.StatusBadRequest, `"query" is required`)
		return
	}

	results, err := s.search.Search(r.Context(), userID, req.Query, req.Limit)
	if err != nil {
		s.logger.Error("search failed", "error", err)
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}
