package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/michal-kalina/garden-of-knowledge/backend/internal/chat"
)

// ChatService is what the HTTP layer needs from the chat orchestration.
type ChatService interface {
	Ask(ctx context.Context, userID, question string,
		onSources func([]chat.Source) error, onDelta func(string) error) error
}

type chatRequest struct {
	Query string `json:"query"`
}

// handleChat streams the answer as Server-Sent Events:
//
//	event: sources  data: [ ...citation panel payload... ]   (once, first)
//	event: delta    data: {"text":"..."}                     (many)
//	event: done     data: {}                                 (on success)
//	event: error    data: {"error":"..."}                    (terminal)
//
// Sources go out before generation starts so the client can render the
// citation panel while text is still streaming.
func (s *server) handleChat(w http.ResponseWriter, r *http.Request) {
	if s.chat == nil {
		writeError(w, http.StatusServiceUnavailable,
			"chat is not configured: set ANTHROPIC_API_KEY")
		return
	}

	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Query = strings.TrimSpace(req.Query)
	if req.Query == "" {
		writeError(w, http.StatusBadRequest, `"query" is required`)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	// Disables buffering in nginx-style reverse proxies; harmless elsewhere.
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	send := func(event string, payload any) error {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		if _, err := w.Write([]byte("event: " + event + "\ndata: " + string(data) + "\n\n")); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	err := s.chat.Ask(r.Context(), userIDFrom(r.Context()), req.Query,
		func(sources []chat.Source) error { return send("sources", sources) },
		func(text string) error { return send("delta", map[string]string{"text": text}) },
	)
	if err != nil {
		s.logger.Error("chat failed", "error", err)
		// Headers are already out — the failure must travel in-band as an
		// SSE event; a late WriteHeader would be ignored anyway.
		_ = send("error", map[string]string{"error": "failed to generate an answer"})
		return
	}
	_ = send("done", map[string]string{})
}
