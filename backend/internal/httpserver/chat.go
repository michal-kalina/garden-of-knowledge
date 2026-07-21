package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/michal-kalina/garden-of-knowledge/backend/internal/chat"
	"github.com/michal-kalina/garden-of-knowledge/backend/internal/conversations"
)

// ChatService is what the HTTP layer needs from the chat orchestration.
type ChatService interface {
	Ask(ctx context.Context, userID, question string, history []chat.Turn,
		onSources func([]chat.Source) error, onDelta func(string) error) (string, error)
}

type chatRequest struct {
	Query string `json:"query"`
	// ConversationID continues an existing conversation; empty starts a new
	// one. The new id is always sent back as the first SSE event, so the
	// client can pick it up when it wasn't supplied.
	ConversationID string `json:"conversation_id"`
}

// handleChat streams the answer as Server-Sent Events:
//
//	event: conversation  data: {"id":"..."}                    (once, first)
//	event: sources       data: [ ...citation panel payload... ] (once)
//	event: delta         data: {"text":"..."}                   (many)
//	event: done          data: {}                               (on success)
//	event: error         data: {"error":"..."}                  (terminal)
//
// The exchange is persisted only after the model finishes successfully —
// a failed generation leaves no half-written assistant message in history.
func (s *server) handleChat(w http.ResponseWriter, r *http.Request, userID string) {
	if s.chat == nil {
		writeError(w, http.StatusServiceUnavailable,
			"chat is not configured: set ANTHROPIC_API_KEY or OPENROUTER_API_KEY")
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

	// Resolve (or create) the conversation and load its history BEFORE
	// switching into SSE mode: a bad conversation_id is a normal 4xx here,
	// but once headers are written the only way to signal failure is an
	// in-band "error" event.
	isNew := req.ConversationID == ""
	var convID string
	var history []chat.Turn

	if isNew {
		conv, err := s.conversations.Create(r.Context(), userID)
		if err != nil {
			s.logger.Error("create conversation failed", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to start a conversation")
			return
		}
		convID = conv.ID
	} else {
		_, msgs, err := s.conversations.Get(r.Context(), userID, req.ConversationID)
		if errors.Is(err, conversations.ErrNotFound) {
			writeError(w, http.StatusNotFound, "conversation not found")
			return
		}
		if err != nil {
			s.logger.Error("load conversation failed", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to load conversation")
			return
		}
		convID = req.ConversationID
		history = make([]chat.Turn, len(msgs))
		for i, m := range msgs {
			history[i] = chat.Turn{Role: m.Role, Content: m.Content}
		}
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
	_ = send("conversation", map[string]string{"id": convID})

	var sources []chat.Source
	answer, err := s.chat.Ask(r.Context(), userID, req.Query, history,
		func(srcs []chat.Source) error { sources = srcs; return send("sources", srcs) },
		func(text string) error { return send("delta", map[string]string{"text": text}) },
	)
	if err != nil {
		s.logger.Error("chat failed", "error", err)
		// Headers are already out — the failure must travel in-band as an
		// SSE event; a late WriteHeader would be ignored anyway.
		_ = send("error", map[string]string{"error": "failed to generate an answer"})
		return
	}

	title := ""
	if isNew {
		title = conversations.TitleFrom(req.Query)
	}
	if err := s.conversations.AppendExchange(r.Context(), userID, convID, title, req.Query, answer, sources); err != nil {
		// The answer already reached the client; a persistence failure
		// should not be reported as a chat failure, just logged. The
		// conversation may be missing this exchange on reload — acceptable
		// degradation versus re-showing an "error" for a successful answer.
		s.logger.Error("persist conversation exchange failed", "error", err, "conversation_id", convID)
	}
	_ = send("done", map[string]string{})
}
