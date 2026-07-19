//go:build integration

// Integration test: runs the real pipeline — migrations, transactional
// upload, job claim, chunking, vector insert — against a real Postgres with
// pgvector. Only the pure-I/O edges (object storage, parser HTTP call,
// embedding provider) are in-memory fakes.
//
// Requires DATABASE_URL pointing at a disposable database; the CI job
// provides one via a pgvector service container. Run locally with:
//
//	DATABASE_URL=postgres://... go test -tags integration ./internal/worker/
package worker

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/michal-kalina/garden-of-knowledge/backend/internal/chunking"
	"github.com/michal-kalina/garden-of-knowledge/backend/internal/database"
	"github.com/michal-kalina/garden-of-knowledge/backend/internal/documents"
	"github.com/michal-kalina/garden-of-knowledge/backend/internal/embeddings"
)

// memStore is an in-memory ObjectStore.
type memStore struct {
	objects map[string][]byte
}

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

// mdParser is a minimal stand-in for the Python service: headings and
// paragraphs from Markdown-ish text, enough to exercise the chunker.
type mdParser struct{}

func (mdParser) Parse(_ context.Context, _, _ string, r io.Reader) ([]chunking.Block, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var blocks []chunking.Block
	for _, part := range strings.Split(string(data), "\n\n") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.HasPrefix(part, "# ") {
			blocks = append(blocks, chunking.Block{Type: "heading", Text: strings.TrimPrefix(part, "# ")})
			continue
		}
		blocks = append(blocks, chunking.Block{Type: "paragraph", Text: part})
	}
	return blocks, nil
}

// failingParser fails a fixed number of times before succeeding — drives the
// retry path.
type failingParser struct {
	failures *int
	inner    Parser
}

func (f failingParser) Parse(ctx context.Context, name, ct string, r io.Reader) ([]chunking.Block, error) {
	if *f.failures > 0 {
		*f.failures--
		return nil, io.ErrUnexpectedEOF
	}
	return f.inner.Parse(ctx, name, ct, r)
}

func TestPipelineIntegration(t *testing.T) {
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

	// Start from a clean slate so the test is rerunnable.
	for _, stmt := range []string{
		"DROP TABLE IF EXISTS chunks, ingestion_jobs, documents, schema_migrations CASCADE",
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}

	if err := database.Migrate(ctx, db, logger); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Idempotency: a second run must be a no-op.
	if err := database.Migrate(ctx, db, logger); err != nil {
		t.Fatalf("second migrate: %v", err)
	}

	store := &memStore{objects: map[string][]byte{}}
	repo := documents.NewRepository(db)
	svc := documents.NewService(store, repo)

	content := "# Terms\n\nAlpha beta gamma.\n\n# Fees\n\nDelta epsilon zeta."
	doc, err := svc.Upload(ctx, documents.UploadInput{
		Filename:    "contract.md",
		ContentType: "text/markdown",
		Size:        int64(len(content)),
		Content:     strings.NewReader(content),
	})
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if doc.Status != "pending" {
		t.Fatalf("status after upload = %q, want pending", doc.Status)
	}

	proc := &Processor{
		DB:       db,
		Store:    store,
		Parser:   mdParser{},
		Embedder: embeddings.Fake{},
		Chunker:  chunking.StructureChunker{TargetTokens: 500},
		Logger:   logger,
	}

	worked, err := proc.ProcessOne(ctx)
	if err != nil {
		t.Fatalf("ProcessOne: %v", err)
	}
	if !worked {
		t.Fatal("ProcessOne found no job")
	}

	got, err := repo.Get(ctx, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "ready" {
		t.Fatalf("document status = %q (error: %v), want ready", got.Status, got.Error)
	}

	var n int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM chunks
		WHERE document_id = $1 AND embedding IS NOT NULL AND heading <> ''
	`, doc.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("embedded chunks with headings = %d, want 2", n)
	}

	// Vector search must be usable end-to-end: nearest chunk to the "fees"
	// fake query vector should be findable via the HNSW index / <=> operator.
	var nearest string
	qvec, _ := embeddings.Fake{}.Embed(ctx, []string{"anything"}, embeddings.InputQuery)
	if err := db.QueryRowContext(ctx, `
		SELECT content FROM chunks WHERE document_id = $1
		ORDER BY embedding <=> $2::vector LIMIT 1
	`, doc.ID, database.VectorLiteral(qvec[0])).Scan(&nearest); err != nil {
		t.Fatalf("vector search: %v", err)
	}
	if nearest == "" {
		t.Fatal("vector search returned empty content")
	}

	t.Run("retry path requeues then succeeds", func(t *testing.T) {
		failures := 1
		procRetry := &Processor{
			DB:    db,
			Store: store,
			Parser: failingParser{
				failures: &failures,
				inner:    mdParser{},
			},
			Embedder: embeddings.Fake{},
			Chunker:  chunking.StructureChunker{},
			Logger:   logger,
		}

		doc2, err := svc.Upload(ctx, documents.UploadInput{
			Filename:    "second.md",
			ContentType: "text/markdown",
			Size:        5,
			Content:     strings.NewReader("hello"),
		})
		if err != nil {
			t.Fatal(err)
		}

		// Attempt 1: parser fails -> job back to queued, doc back to pending.
		if _, err := procRetry.ProcessOne(ctx); err != nil {
			t.Fatal(err)
		}
		mid, _ := repo.Get(ctx, doc2.ID)
		if mid.Status != "pending" {
			t.Fatalf("status after failed attempt = %q, want pending", mid.Status)
		}
		var lastErr string
		if err := db.QueryRowContext(ctx,
			`SELECT last_error FROM ingestion_jobs WHERE document_id = $1`, doc2.ID,
		).Scan(&lastErr); err != nil {
			t.Fatal(err)
		}
		if lastErr == "" {
			t.Fatal("last_error not recorded on retry")
		}

		// Attempt 2: succeeds.
		if _, err := procRetry.ProcessOne(ctx); err != nil {
			t.Fatal(err)
		}
		final, _ := repo.Get(ctx, doc2.ID)
		if final.Status != "ready" {
			t.Fatalf("status after retry = %q (error: %v), want ready", final.Status, final.Error)
		}
	})
}
