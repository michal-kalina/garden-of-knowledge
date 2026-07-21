package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/michal-kalina/garden-of-knowledge/backend/internal/users"
)

// UserService is what the HTTP layer needs from accounts.
type UserService interface {
	Register(ctx context.Context, email, password string) (users.User, string, error)
	Login(ctx context.Context, email, password string) (users.User, string, error)
}

// TokenVerifier checks a session token and returns the user id.
type TokenVerifier func(token string) (string, error)

type credentialsRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type sessionResponse struct {
	Token string     `json:"token"`
	User  users.User `json:"user"`
}

func (s *server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req credentialsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	u, token, err := s.users.Register(r.Context(), req.Email, req.Password)
	if errors.Is(err, users.ErrEmailTaken) {
		writeError(w, http.StatusConflict, "this email is already registered")
		return
	}
	if err != nil {
		// Validation messages (email format, password length) are safe and
		// actionable for the client.
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, sessionResponse{Token: token, User: u})
}

func (s *server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req credentialsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	u, token, err := s.users.Login(r.Context(), req.Email, req.Password)
	if errors.Is(err, users.ErrInvalidCredentials) {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}
	if err != nil {
		s.logger.Error("login failed", "error", err)
		writeError(w, http.StatusInternalServerError, "login failed")
		return
	}
	writeJSON(w, http.StatusOK, sessionResponse{Token: token, User: u})
}

// authedHandler is an HTTP handler that additionally receives the verified
// user id as a plain parameter — not a context value. The signature is the
// point: every authed route's dependency on tenancy is visible in its type,
// and every call site is forced by the compiler to thread it onward into
// documents/retrieval/chat, which take userID as an explicit argument for
// exactly the same reason. A forgotten context key would fail silently at
// runtime (empty string, matching nothing in SQL); a forgotten parameter
// fails loudly at compile time. See the discussion in commit history for
// the full trade-off.
type authedHandler func(w http.ResponseWriter, r *http.Request, userID string)

// requireAuth wraps an authedHandler with Bearer-token authentication.
func (s *server) requireAuth(next authedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		token, ok := strings.CutPrefix(header, "Bearer ")
		if !ok || token == "" {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		userID, err := s.verify(token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid or expired session")
			return
		}
		next(w, r, userID)
	}
}
