// Package documents owns the document lifecycle: upload, persistence and
// status tracking. The Service orchestrates object storage and the database;
// the Repository is the only place that talks SQL for documents.
package documents

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"github.com/michal-kalina/garden-of-knowledge/backend/internal/storage"
)

// ErrNotFound is returned when a document does not exist.
var ErrNotFound = errors.New("document not found")

// ErrNotFailed is returned when Retry is called on a document that isn't
// currently in the 'failed' state — retrying a document mid-flight or
// already ready would race the worker or silently duplicate chunks.
var ErrNotFailed = errors.New("document is not in a failed state")

// Document mirrors a row in the documents table.
// StorageKey is an internal detail and is never serialized to clients.
type Document struct {
	ID          string    `json:"id"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	SizeBytes   int64     `json:"size_bytes"`
	StorageKey  string    `json:"-"`
	Status      string    `json:"status"`
	Error       *string   `json:"error,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Repository provides database access for documents and their ingestion jobs.
type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// CreateWithJob inserts the document (owned by userID) and enqueues its
// ingestion job in a single transaction. This is the payoff of the Postgres-backed queue
// (ADR-0002): there is no window where a document exists without a job,
// so no outbox pattern or reconciliation is needed.
func (r *Repository) CreateWithJob(ctx context.Context, userID string, d Document) (Document, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Document{}, err
	}
	defer tx.Rollback() //nolint:errcheck // rollback after commit is a no-op

	err = tx.QueryRowContext(ctx, `
		INSERT INTO documents (filename, content_type, size_bytes, storage_key, user_id)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, status, created_at, updated_at
	`, d.Filename, d.ContentType, d.SizeBytes, d.StorageKey, userID).
		Scan(&d.ID, &d.Status, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return Document{}, fmt.Errorf("insert document: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO ingestion_jobs (document_id) VALUES ($1)
	`, d.ID)
	if err != nil {
		return Document{}, fmt.Errorf("enqueue ingestion job: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return Document{}, err
	}
	return d, nil
}

const docColumns = `id, filename, content_type, size_bytes, storage_key, status, error, created_at, updated_at`

// List returns the most recent documents. Pagination arrives with the
// frontend in Phase 2; the cap keeps responses bounded until then.
func (r *Repository) List(ctx context.Context, userID string) ([]Document, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT `+docColumns+`
		FROM documents
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT 100
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	docs := []Document{}
	for rows.Next() {
		d, err := scanDocument(rows)
		if err != nil {
			return nil, err
		}
		docs = append(docs, d)
	}
	return docs, rows.Err()
}

// Get scopes by owner: another user's document id yields ErrNotFound, not a
// permission error — no confirmation that the id exists at all.
func (r *Repository) Get(ctx context.Context, userID, id string) (Document, error) {
	if _, err := uuid.Parse(id); err != nil {
		return Document{}, ErrNotFound
	}
	row := r.db.QueryRowContext(ctx, `
		SELECT `+docColumns+`
		FROM documents
		WHERE id = $1 AND user_id = $2
	`, id, userID)
	d, err := scanDocument(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Document{}, ErrNotFound
	}
	return d, err
}

// Retry moves a failed document back into the ingestion queue: it resets
// the document to 'pending' and its job to 'queued' with a fresh attempt
// budget, in one transaction. Restricted to documents currently 'failed' —
// see ErrNotFailed — so a retry can never race a job the worker still has
// claimed, and can never silently re-enqueue a document that's already
// 'ready' (which would duplicate its chunks on the next worker pass).
func (r *Repository) Retry(ctx context.Context, userID, id string) (Document, error) {
	if _, err := uuid.Parse(id); err != nil {
		return Document{}, ErrNotFound
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Document{}, err
	}
	defer tx.Rollback() //nolint:errcheck // rollback after commit is a no-op

	row := tx.QueryRowContext(ctx, `
		UPDATE documents
		SET status = 'pending', error = NULL, updated_at = now()
		WHERE id = $1 AND user_id = $2 AND status = 'failed'
		RETURNING `+docColumns, id, userID)
	d, err := scanDocument(row)
	if errors.Is(err, sql.ErrNoRows) {
		// The UPDATE matched nothing — find out whether that's because the
		// document doesn't exist/isn't ours, or because it exists but isn't
		// failed, so the caller can tell those apart (404 vs 409).
		var status string
		lookupErr := tx.QueryRowContext(ctx,
			`SELECT status FROM documents WHERE id = $1 AND user_id = $2`, id, userID,
		).Scan(&status)
		if errors.Is(lookupErr, sql.ErrNoRows) {
			return Document{}, ErrNotFound
		}
		if lookupErr != nil {
			return Document{}, lookupErr
		}
		return Document{}, fmt.Errorf("%w: current status is %q", ErrNotFailed, status)
	}
	if err != nil {
		return Document{}, err
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE ingestion_jobs
		SET status = 'queued', attempts = 0, last_error = NULL, updated_at = now()
		WHERE document_id = $1
	`, id); err != nil {
		return Document{}, fmt.Errorf("reset ingestion job: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return Document{}, err
	}
	return d, nil
}

// Delete removes a document and returns its storage key so the caller can
// also remove the underlying object. Ownership and existence are checked in
// the same statement that deletes — there is no separate lookup to race.
// Chunks and the ingestion job cascade via their ON DELETE CASCADE foreign
// keys (migration 0001): deleting the document row is what "erases the
// trace of having parsed this file" that the caller asked for.
func (r *Repository) Delete(ctx context.Context, userID, id string) (storageKey string, err error) {
	if _, err := uuid.Parse(id); err != nil {
		return "", ErrNotFound
	}
	err = r.db.QueryRowContext(ctx, `
		DELETE FROM documents WHERE id = $1 AND user_id = $2
		RETURNING storage_key
	`, id, userID).Scan(&storageKey)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return storageKey, err
}

type scanner interface {
	Scan(dest ...any) error
}

func scanDocument(s scanner) (Document, error) {
	var d Document
	err := s.Scan(&d.ID, &d.Filename, &d.ContentType, &d.SizeBytes,
		&d.StorageKey, &d.Status, &d.Error, &d.CreatedAt, &d.UpdatedAt)
	return d, err
}

// Service orchestrates the upload flow: store the file, then record it.
type Service struct {
	store  storage.ObjectStore
	repo   *Repository
	logger *slog.Logger
}

func NewService(store storage.ObjectStore, repo *Repository, logger *slog.Logger) *Service {
	return &Service{store: store, repo: repo, logger: logger}
}

// UploadInput carries a validated upload from the HTTP layer.
type UploadInput struct {
	Filename    string
	ContentType string
	Size        int64
	Content     io.Reader
}

// Upload writes the file to object storage first, then inserts the document
// and its job transactionally. If the insert fails we may leave an orphaned
// object behind — accepted for now; a periodic cleanup job is cheaper and
// simpler than distributed-transaction gymnastics.
// TODO(phase-4): sweep orphaned objects.
func (s *Service) Upload(ctx context.Context, userID string, in UploadInput) (Document, error) {
	// The storage key is content-independent and unguessable; the original
	// filename lives only in the database (S3 keys have their own character
	// rules, so user input never becomes a key).
	key := uuid.NewString() + filepath.Ext(in.Filename)

	if err := s.store.Put(ctx, key, in.Content, in.Size, in.ContentType); err != nil {
		return Document{}, fmt.Errorf("store upload: %w", err)
	}

	return s.repo.CreateWithJob(ctx, userID, Document{
		Filename:    in.Filename,
		ContentType: in.ContentType,
		SizeBytes:   in.Size,
		StorageKey:  key,
	})
}

func (s *Service) List(ctx context.Context, userID string) ([]Document, error) {
	return s.repo.List(ctx, userID)
}

func (s *Service) Get(ctx context.Context, userID, id string) (Document, error) {
	return s.repo.Get(ctx, userID, id)
}

func (s *Service) Retry(ctx context.Context, userID, id string) (Document, error) {
	return s.repo.Retry(ctx, userID, id)
}

// Delete removes the document (and, via cascade, its chunks and ingestion
// job) then best-effort deletes the underlying object. The DB row goes
// first: if the storage delete then fails, we're left with an orphaned
// object in the bucket — harmless and sweepable later. The reverse order
// would be worse: a storage delete succeeding while the DB row survives
// would leave a 'ready' document whose file no longer exists, breaking any
// future read of it. This mirrors the ordering trade-off already made in
// Upload (store-then-insert), just reversed because deletion runs it
// back to front.
func (s *Service) Delete(ctx context.Context, userID, id string) error {
	storageKey, err := s.repo.Delete(ctx, userID, id)
	if err != nil {
		return err
	}
	if err := s.store.Delete(ctx, storageKey); err != nil {
		// The document is already gone from the user's (and the API's)
		// point of view — the DB commit succeeded, chunks are cascaded
		// away, it will no longer show up anywhere. A leftover object in
		// the bucket is a sweepable orphan, not a failed delete; reporting
		// this to the client as an error would be misleading.
		s.logger.Error("storage cleanup failed after document delete",
			"document_id", id, "storage_key", storageKey, "error", err)
	}
	return nil
}
