package httpserver

import (
	"context"
	"errors"
	"github.com/michal-kalina/garden-of-knowledge/backend/internal/chat"
	"net/http"

	"github.com/michal-kalina/garden-of-knowledge/backend/internal/conversations"
)

// ConversationsService is what the HTTP layer needs from chat history.
type ConversationsService interface {
	Create(ctx context.Context, userID string) (conversations.Conversation, error)
	List(ctx context.Context, userID string) ([]conversations.Conversation, error)
	Get(ctx context.Context, userID, id string) (conversations.Conversation, []conversations.Message, error)
	AppendExchange(ctx context.Context, userID, id, title, question, answer string, sources []chat.Source) error
	Delete(ctx context.Context, userID, id string) error
}

func (s *server) handleConversationList(w http.ResponseWriter, r *http.Request, userID string) {
	list, err := s.conversations.List(r.Context(), userID)
	if err != nil {
		s.logger.Error("list conversations failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list conversations")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversations": list})
}

type conversationDetail struct {
	conversations.Conversation
	Messages []conversations.Message `json:"messages"`
}

func (s *server) handleConversationGet(w http.ResponseWriter, r *http.Request, userID string) {
	conv, msgs, err := s.conversations.Get(r.Context(), userID, r.PathValue("id"))
	if errors.Is(err, conversations.ErrNotFound) {
		writeError(w, http.StatusNotFound, "conversation not found")
		return
	}
	if err != nil {
		s.logger.Error("get conversation failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to load conversation")
		return
	}
	writeJSON(w, http.StatusOK, conversationDetail{Conversation: conv, Messages: msgs})
}

func (s *server) handleConversationDelete(w http.ResponseWriter, r *http.Request, userID string) {
	err := s.conversations.Delete(r.Context(), userID, r.PathValue("id"))
	if errors.Is(err, conversations.ErrNotFound) {
		writeError(w, http.StatusNotFound, "conversation not found")
		return
	}
	if err != nil {
		s.logger.Error("delete conversation failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete conversation")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
