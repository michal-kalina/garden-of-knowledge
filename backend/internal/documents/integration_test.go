//go:build integration

package documents

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/michal-kalina/garden-of-knowledge/backend/internal/database"
	"github.com/michal-kalina/garden-of-knowledge/backend/internal/users"
)

// memStore is a minimal in-memory ObjectStore, mirroring the one used in
// the worker's integration test — real MinIO isn't needed to verify the
// DB-level cascade and service-level orchestration this test targets.
type memStore struct{ objects map[string][]byte }

func (m *memStore) Put(_ context.Context, key string, r io.Reader, _ int64, _ string) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	m.objects[key] = data
	return nil
}
func (m *memStore) Get(_ context.Context, key string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(m.objects[key])), nil
}
func (m *memStore) Delete(_ context.Context, key string) error {
	delete(m.objects, key)
	return nil
}

func TestDocumentsIntegration(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.ExecContext(ctx,
		"DROP TABLE IF EXISTS messages, conversations, chunks, ingestion_jobs, documents, users, schema_migrations CASCADE",
	); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, db, logger); err != nil {
		t.Fatal(err)
	}

	owner, err := users.NewRepository(db).Create(ctx, "docs-test@example.com", "x")
	if err != nil {
		t.Fatal(err)
	}
	other, err := users.NewRepository(db).Create(ctx, "docs-other@example.com", "x")
	if err != nil {
		t.Fatal(err)
	}

	store := &memStore{objects: map[string][]byte{}}
	repo := NewRepository(db)
	svc := NewService(store, repo, logger)

	t.Run("retry resets a failed document and its job", func(t *testing.T) {
		doc, err := svc.Upload(ctx, owner.ID, UploadInput{
			Filename: "a.md", ContentType: "text/markdown", Size: 5, Content: strings.NewReader("hello"),
		})
		if err != nil {
			t.Fatal(err)
		}

		// Simulate the worker having exhausted retries.
		errMsg := "boom"
		if _, err := db.ExecContext(ctx, `
			UPDATE documents SET status = 'failed', error = $2 WHERE id = $1
		`, doc.ID, errMsg); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, `
			UPDATE ingestion_jobs SET status = 'failed', attempts = 3, last_error = $2
			WHERE document_id = $1
		`, doc.ID, errMsg); err != nil {
			t.Fatal(err)
		}

		got, err := svc.Retry(ctx, owner.ID, doc.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != "pending" || got.Error != nil {
			t.Fatalf("document after retry = %+v", got)
		}

		var jobStatus string
		var attempts int
		if err := db.QueryRowContext(ctx,
			`SELECT status, attempts FROM ingestion_jobs WHERE document_id = $1`, doc.ID,
		).Scan(&jobStatus, &attempts); err != nil {
			t.Fatal(err)
		}
		if jobStatus != "queued" || attempts != 0 {
			t.Errorf("job after retry: status=%q attempts=%d, want queued/0", jobStatus, attempts)
		}
	})

	t.Run("retry rejects a document that isn't failed", func(t *testing.T) {
		doc, err := svc.Upload(ctx, owner.ID, UploadInput{
			Filename: "b.md", ContentType: "text/markdown", Size: 5, Content: strings.NewReader("hello"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Retry(ctx, owner.ID, doc.ID); err == nil {
			t.Fatal("expected ErrNotFailed for a pending document")
		}
	})

	t.Run("retry is scoped to the owner", func(t *testing.T) {
		doc, err := svc.Upload(ctx, owner.ID, UploadInput{
			Filename: "c.md", ContentType: "text/markdown", Size: 5, Content: strings.NewReader("hello"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, `UPDATE documents SET status='failed' WHERE id=$1`, doc.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Retry(ctx, other.ID, doc.ID); err != ErrNotFound {
			t.Fatalf("other user retrying: err = %v, want ErrNotFound", err)
		}
	})

	t.Run("delete removes the document, cascades chunks and jobs, and clears storage", func(t *testing.T) {
		doc, err := svc.Upload(ctx, owner.ID, UploadInput{
			Filename: "d.md", ContentType: "text/markdown", Size: 5, Content: strings.NewReader("hello"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, `
			INSERT INTO chunks (document_id, ordinal, content, heading) VALUES ($1, 0, 'hi', 'H')
		`, doc.ID); err != nil {
			t.Fatal(err)
		}

		var storageKey string
		if err := db.QueryRowContext(ctx,
			`SELECT storage_key FROM documents WHERE id = $1`, doc.ID,
		).Scan(&storageKey); err != nil {
			t.Fatal(err)
		}
		if _, ok := store.objects[storageKey]; !ok {
			t.Fatal("test setup: expected the uploaded object in the fake store")
		}

		if err := svc.Delete(ctx, owner.ID, doc.ID); err != nil {
			t.Fatal(err)
		}

		if _, err := repo.Get(ctx, owner.ID, doc.ID); err != ErrNotFound {
			t.Errorf("document still fetchable after delete: err = %v", err)
		}
		var chunkCount, jobCount int
		db.QueryRowContext(ctx, `SELECT count(*) FROM chunks WHERE document_id = $1`, doc.ID).Scan(&chunkCount)
		db.QueryRowContext(ctx, `SELECT count(*) FROM ingestion_jobs WHERE document_id = $1`, doc.ID).Scan(&jobCount)
		if chunkCount != 0 || jobCount != 0 {
			t.Errorf("cascade left chunks=%d jobs=%d, want 0/0", chunkCount, jobCount)
		}
		// Only THIS document's object should be gone — other subtests'
		// uploads (a.md, b.md, c.md) share the same fake store and must
		// survive untouched.
		if _, ok := store.objects[storageKey]; ok {
			t.Errorf("storage object %q not deleted", storageKey)
		}
	})

	t.Run("delete is scoped to the owner", func(t *testing.T) {
		doc, err := svc.Upload(ctx, owner.ID, UploadInput{
			Filename: "e.md", ContentType: "text/markdown", Size: 5, Content: strings.NewReader("hello"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := svc.Delete(ctx, other.ID, doc.ID); err != ErrNotFound {
			t.Fatalf("other user deleting: err = %v, want ErrNotFound", err)
		}
		if _, err := repo.Get(ctx, owner.ID, doc.ID); err != nil {
			t.Fatalf("document should survive another user's failed delete attempt: %v", err)
		}
	})
}
