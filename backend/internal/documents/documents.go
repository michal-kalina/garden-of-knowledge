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
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"github.com/michal-kalina/garden-of-knowledge/backend/internal/storage"
)

// ErrNotFound is returned when a document does not exist.
var ErrNotFound = errors.New("document not found")

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

// CreateWithJob inserts the document and enqueues its ingestion job in a
// single transaction. This is the payoff of the Postgres-backed queue
// (ADR-0002): there is no window where a document exists without a job,
// so no outbox pattern or reconciliation is needed.
func (r *Repository) CreateWithJob(ctx context.Context, d Document) (Document, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Document{}, err
	}
	defer tx.Rollback() //nolint:errcheck // rollback after commit is a no-op

	err = tx.QueryRowContext(ctx, `
		INSERT INTO documents (filename, content_type, size_bytes, storage_key)
		VALUES ($1, $2, $3, $4)
		RETURNING id, status, created_at, updated_at
	`, d.Filename, d.ContentType, d.SizeBytes, d.StorageKey).
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
func (r *Repository) List(ctx context.Context) ([]Document, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT `+docColumns+`
		FROM documents
		ORDER BY created_at DESC
		LIMIT 100
	`)
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

func (r *Repository) Get(ctx context.Context, id string) (Document, error) {
	if _, err := uuid.Parse(id); err != nil {
		return Document{}, ErrNotFound
	}
	row := r.db.QueryRowContext(ctx, `
		SELECT `+docColumns+`
		FROM documents
		WHERE id = $1
	`, id)
	d, err := scanDocument(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Document{}, ErrNotFound
	}
	return d, err
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
	store storage.ObjectStore
	repo  *Repository
}

func NewService(store storage.ObjectStore, repo *Repository) *Service {
	return &Service{store: store, repo: repo}
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
func (s *Service) Upload(ctx context.Context, in UploadInput) (Document, error) {
	// The storage key is content-independent and unguessable; the original
	// filename lives only in the database (S3 keys have their own character
	// rules, so user input never becomes a key).
	key := uuid.NewString() + filepath.Ext(in.Filename)

	if err := s.store.Put(ctx, key, in.Content, in.Size, in.ContentType); err != nil {
		return Document{}, fmt.Errorf("store upload: %w", err)
	}

	return s.repo.CreateWithJob(ctx, Document{
		Filename:    in.Filename,
		ContentType: in.ContentType,
		SizeBytes:   in.Size,
		StorageKey:  key,
	})
}

func (s *Service) List(ctx context.Context) ([]Document, error) {
	return s.repo.List(ctx)
}

func (s *Service) Get(ctx context.Context, id string) (Document, error) {
	return s.repo.Get(ctx, id)
}
