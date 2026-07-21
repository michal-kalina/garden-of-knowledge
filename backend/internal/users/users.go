// Package users owns accounts: registration, login and the repository.
package users

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/michal-kalina/garden-of-knowledge/backend/internal/auth"
)

var (
	ErrEmailTaken         = errors.New("email already registered")
	ErrInvalidCredentials = errors.New("invalid email or password")
)

type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

func (r *Repository) Create(ctx context.Context, email, passwordHash string) (User, error) {
	var u User
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO users (email, password_hash) VALUES ($1, $2)
		RETURNING id, email, created_at
	`, email, passwordHash).Scan(&u.ID, &u.Email, &u.CreatedAt)
	if err != nil {
		// 23505 = unique_violation; matching on the SQLSTATE in the error
		// string keeps us off driver-specific error types.
		if strings.Contains(err.Error(), "23505") ||
			strings.Contains(err.Error(), "duplicate key") {
			return User{}, ErrEmailTaken
		}
		return User{}, fmt.Errorf("insert user: %w", err)
	}
	return u, nil
}

func (r *Repository) GetByEmail(ctx context.Context, email string) (User, string, error) {
	var u User
	var hash string
	err := r.db.QueryRowContext(ctx, `
		SELECT id, email, created_at, password_hash FROM users WHERE email = $1
	`, email).Scan(&u.ID, &u.Email, &u.CreatedAt, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, "", ErrInvalidCredentials
	}
	return u, hash, err
}

// Service implements registration and login on top of the repository.
type Service struct {
	repo   *Repository
	tokens *auth.Tokens
}

func NewService(repo *Repository, tokens *auth.Tokens) *Service {
	return &Service{repo: repo, tokens: tokens}
}

const minPasswordLen = 8

// Register creates the account and returns a session token, so the client
// is signed in immediately after signing up.
func (s *Service) Register(ctx context.Context, email, password string) (User, string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || !strings.Contains(email, "@") {
		return User{}, "", fmt.Errorf("a valid email is required")
	}
	if len(password) < minPasswordLen {
		return User{}, "", fmt.Errorf("password must be at least %d characters", minPasswordLen)
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return User{}, "", err
	}
	u, err := s.repo.Create(ctx, email, hash)
	if err != nil {
		return User{}, "", err
	}
	token, err := s.tokens.Issue(u.ID)
	return u, token, err
}

// Login verifies credentials and returns a session token. Wrong email and
// wrong password produce the same error on purpose — no account enumeration.
func (s *Service) Login(ctx context.Context, email, password string) (User, string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	u, hash, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		return User{}, "", err
	}
	if !auth.VerifyPassword(password, hash) {
		return User{}, "", ErrInvalidCredentials
	}
	token, err := s.tokens.Issue(u.ID)
	return u, token, err
}
