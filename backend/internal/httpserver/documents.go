package httpserver

import (
	"context"
	"errors"
	"net/http"

	"github.com/michal-kalina/garden-of-knowledge/backend/internal/documents"
)

// DocumentService is what the HTTP layer needs from the documents package.
// Depending on an interface (not the concrete Service) keeps handlers
// unit-testable with an in-memory fake — no database or MinIO required.
type DocumentService interface {
	Upload(ctx context.Context, userID string, in documents.UploadInput) (documents.Document, error)
	List(ctx context.Context, userID string) ([]documents.Document, error)
	Get(ctx context.Context, userID, id string) (documents.Document, error)
	Retry(ctx context.Context, userID, id string) (documents.Document, error)
	Delete(ctx context.Context, userID, id string) error
}

// allowedContentTypes is the ingestion allowlist. It grows together with the
// parser service's actual capabilities — accepting a format we cannot parse
// would just produce failed jobs.
var allowedContentTypes = map[string]bool{
	"application/pdf": true,
	"text/markdown":   true,
	"text/x-markdown": true,
	"text/plain":      true,
}

func (s *server) handleDocumentUpload(w http.ResponseWriter, r *http.Request, userID string) {
	// MaxBytesReader protects the server before multipart parsing starts;
	// exceeding the limit surfaces as *http.MaxBytesError below.
	r.Body = http.MaxBytesReader(w, r.Body, s.maxUploadBytes)

	file, header, err := r.FormFile("file")
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "file exceeds upload limit")
			return
		}
		writeError(w, http.StatusBadRequest, `multipart form field "file" is required`)
		return
	}
	defer file.Close()

	contentType := header.Header.Get("Content-Type")
	if !allowedContentTypes[contentType] {
		writeError(w, http.StatusUnsupportedMediaType,
			"unsupported content type; expected PDF, Markdown or plain text")
		return
	}

	doc, err := s.docs.Upload(r.Context(), userID, documents.UploadInput{
		Filename:    header.Filename,
		ContentType: contentType,
		Size:        header.Size,
		Content:     file,
	})
	if err != nil {
		s.logger.Error("upload failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to store document")
		return
	}

	writeJSON(w, http.StatusCreated, doc)
}

func (s *server) handleDocumentList(w http.ResponseWriter, r *http.Request, userID string) {
	docs, err := s.docs.List(r.Context(), userID)
	if err != nil {
		s.logger.Error("list documents failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list documents")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"documents": docs})
}

func (s *server) handleDocumentGet(w http.ResponseWriter, r *http.Request, userID string) {
	doc, err := s.docs.Get(r.Context(), userID, r.PathValue("id"))
	if errors.Is(err, documents.ErrNotFound) {
		writeError(w, http.StatusNotFound, "document not found")
		return
	}
	if err != nil {
		s.logger.Error("get document failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to fetch document")
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// handleDocumentRetry re-enqueues a failed document for ingestion. Only
// valid from the 'failed' state (see documents.ErrNotFailed) — surfaced as
// 409 Conflict, since the request is well-formed but the resource's current
// state doesn't allow it.
func (s *server) handleDocumentRetry(w http.ResponseWriter, r *http.Request, userID string) {
	doc, err := s.docs.Retry(r.Context(), userID, r.PathValue("id"))
	if errors.Is(err, documents.ErrNotFound) {
		writeError(w, http.StatusNotFound, "document not found")
		return
	}
	if errors.Is(err, documents.ErrNotFailed) {
		writeError(w, http.StatusConflict, "only a failed document can be retried")
		return
	}
	if err != nil {
		s.logger.Error("retry document failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to retry document")
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

func (s *server) handleDocumentDelete(w http.ResponseWriter, r *http.Request, userID string) {
	err := s.docs.Delete(r.Context(), userID, r.PathValue("id"))
	if errors.Is(err, documents.ErrNotFound) {
		writeError(w, http.StatusNotFound, "document not found")
		return
	}
	if err != nil {
		s.logger.Error("delete document failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete document")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
