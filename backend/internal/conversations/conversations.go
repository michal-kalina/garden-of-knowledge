// Package conversations persists chat history: conversations and their
// messages, scoped per user like everything else in Phase 3.
package conversations

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/michal-kalina/garden-of-knowledge/backend/internal/chat"
)

var ErrNotFound = errors.New("conversation not found")

type Conversation struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Message struct {
	ID        int64         `json:"id"`
	Role      string        `json:"role"` // "user" | "assistant"
	Content   string        `json:"content"`
	Sources   []chat.Source `json:"sources,omitempty"`
	CreatedAt time.Time     `json:"created_at"`
}

type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

// Create starts a new, empty conversation. Title is set later, once the
// first exchange gives us something to summarize it with.
func (r *Repository) Create(ctx context.Context, userID string) (Conversation, error) {
	var c Conversation
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO conversations (user_id) VALUES ($1)
		RETURNING id, title, created_at, updated_at
	`, userID).Scan(&c.ID, &c.Title, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

func (r *Repository) List(ctx context.Context, userID string) ([]Conversation, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, title, created_at, updated_at
		FROM conversations
		WHERE user_id = $1
		ORDER BY updated_at DESC
		LIMIT 100
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Conversation{}
	for rows.Next() {
		var c Conversation
		if err := rows.Scan(&c.ID, &c.Title, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Get verifies ownership (userID) and returns the conversation with its
// full message history in order.
func (r *Repository) Get(ctx context.Context, userID, id string) (Conversation, []Message, error) {
	if _, err := uuid.Parse(id); err != nil {
		return Conversation{}, nil, ErrNotFound
	}

	var c Conversation
	err := r.db.QueryRowContext(ctx, `
		SELECT id, title, created_at, updated_at
		FROM conversations WHERE id = $1 AND user_id = $2
	`, id, userID).Scan(&c.ID, &c.Title, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Conversation{}, nil, ErrNotFound
	}
	if err != nil {
		return Conversation{}, nil, err
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, role, content, sources, created_at
		FROM messages WHERE conversation_id = $1 ORDER BY id
	`, id)
	if err != nil {
		return Conversation{}, nil, err
	}
	defer rows.Close()

	msgs := []Message{}
	for rows.Next() {
		var m Message
		var rawSources []byte
		if err := rows.Scan(&m.ID, &m.Role, &m.Content, &rawSources, &m.CreatedAt); err != nil {
			return Conversation{}, nil, err
		}
		if err := json.Unmarshal(rawSources, &m.Sources); err != nil {
			return Conversation{}, nil, fmt.Errorf("decode stored sources: %w", err)
		}
		msgs = append(msgs, m)
	}
	return c, msgs, rows.Err()
}

// AppendExchange records one user question and its assistant answer, and
// bumps updated_at so the conversation resurfaces at the top of the list —
// the ordering a user expects from "recent chats". If title is non-empty
// (first exchange) it is set at the same time. userID is verified against
// the conversation's owner inside the transaction, so a forged conversation
// id from another user's account is rejected atomically rather than by a
// separate check the caller could forget.
func (r *Repository) AppendExchange(ctx context.Context, userID, convID, title, question, answer string, sources []chat.Source) error {
	sourcesJSON, err := json.Marshal(sources)
	if err != nil {
		return fmt.Errorf("encode sources: %w", err)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // rollback after commit is a no-op

	var owns bool
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM conversations WHERE id = $1 AND user_id = $2)
	`, convID, userID).Scan(&owns); err != nil {
		return err
	}
	if !owns {
		return ErrNotFound
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO messages (conversation_id, role, content, sources)
		VALUES ($1, 'user', $2, '[]')
	`, convID, question); err != nil {
		return fmt.Errorf("insert user message: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO messages (conversation_id, role, content, sources)
		VALUES ($1, 'assistant', $2, $3)
	`, convID, answer, sourcesJSON); err != nil {
		return fmt.Errorf("insert assistant message: %w", err)
	}

	if title != "" {
		if _, err := tx.ExecContext(ctx, `
			UPDATE conversations SET title = $2, updated_at = now() WHERE id = $1
		`, convID, title); err != nil {
			return fmt.Errorf("set title: %w", err)
		}
	} else {
		if _, err := tx.ExecContext(ctx, `
			UPDATE conversations SET updated_at = now() WHERE id = $1
		`, convID); err != nil {
			return fmt.Errorf("bump updated_at: %w", err)
		}
	}
	return tx.Commit()
}

// Delete removes a conversation (and its messages via ON DELETE CASCADE),
// scoped to its owner.
func (r *Repository) Delete(ctx context.Context, userID, id string) error {
	res, err := r.db.ExecContext(ctx, `
		DELETE FROM conversations WHERE id = $1 AND user_id = $2
	`, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

const maxTitleLen = 60

// TitleFrom derives a short conversation title from the first question —
// good enough for a sidebar label without another LLM round-trip.
func TitleFrom(question string) string {
	r := []rune(question)
	if len(r) <= maxTitleLen {
		return question
	}
	return string(r[:maxTitleLen]) + "…"
}

// Service is a thin façade over Repository for the HTTP layer — mirrors the
// documents.Service / retrieval.Searcher pattern used elsewhere so handlers
// depend on small interfaces, not concrete repositories.
type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(ctx context.Context, userID string) (Conversation, error) {
	return s.repo.Create(ctx, userID)
}

func (s *Service) List(ctx context.Context, userID string) ([]Conversation, error) {
	return s.repo.List(ctx, userID)
}

func (s *Service) Get(ctx context.Context, userID, id string) (Conversation, []Message, error) {
	return s.repo.Get(ctx, userID, id)
}

func (s *Service) AppendExchange(ctx context.Context, userID, id, title, question, answer string, sources []chat.Source) error {
	return s.repo.AppendExchange(ctx, userID, id, title, question, answer, sources)
}

func (s *Service) Delete(ctx context.Context, userID, id string) error {
	return s.repo.Delete(ctx, userID, id)
}
