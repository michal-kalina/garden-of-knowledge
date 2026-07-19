//go:build integration

// Integration test for hybrid retrieval against real Postgres + pgvector.
//
// Determinism trick: the Fake embedder maps identical text to identical
// vectors, so a query that repeats a chunk's exact content has cosine
// distance 0 to it — the vector retriever must rank it first. The full-text
// side is deterministic by nature. This lets us assert on ranking behaviour
// without a real embedding model.
package retrieval

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/michal-kalina/garden-of-knowledge/backend/internal/database"
	"github.com/michal-kalina/garden-of-knowledge/backend/internal/embeddings"
)

func TestHybridSearchIntegration(t *testing.T) {
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
		"DROP TABLE IF EXISTS chunks, ingestion_jobs, documents, schema_migrations CASCADE",
	); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, db, logger); err != nil {
		t.Fatal(err)
	}

	// Seed: one ready document with three chunks, one pending document that
	// must never surface in results.
	var readyID, pendingID string
	if err := db.QueryRowContext(ctx, `
		INSERT INTO documents (filename, content_type, size_bytes, storage_key, status)
		VALUES ('handbook.md', 'text/markdown', 1, 'k1', 'ready')
		RETURNING id`).Scan(&readyID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `
		INSERT INTO documents (filename, content_type, size_bytes, storage_key, status)
		VALUES ('draft.md', 'text/markdown', 1, 'k2', 'pending')
		RETURNING id`).Scan(&pendingID); err != nil {
		t.Fatal(err)
	}

	fake := embeddings.Fake{}
	insert := func(docID string, ordinal int, content, heading string) {
		t.Helper()
		vecs, err := fake.Embed(ctx, []string{content}, embeddings.InputDocument)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, `
			INSERT INTO chunks (document_id, ordinal, content, heading, embedding)
			VALUES ($1, $2, $3, $4, $5::vector)
		`, docID, ordinal, content, heading, database.VectorLiteral(vecs[0])); err != nil {
			t.Fatal(err)
		}
	}

	insert(readyID, 0, "Vacation policy grants twenty six days of paid leave annually.", "Leave")
	insert(readyID, 1, "The zorbafex device requires quarterly maintenance by certified staff.", "Equipment")
	insert(readyID, 2, "Remote work is allowed up to three days per week after probation.", "Remote work")
	insert(pendingID, 0, "Vacation policy in the unfinished draft.", "Leave")

	s := New(db, fake)

	t.Run("exact-content query wins via the vector retriever", func(t *testing.T) {
		query := "The zorbafex device requires quarterly maintenance by certified staff."
		got, err := s.Search(ctx, query, 3)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) == 0 {
			t.Fatal("no results")
		}
		if got[0].Ordinal != 1 {
			t.Fatalf("top result ordinal = %d, want 1 (distance-0 chunk); results: %+v", got[0].Ordinal, got)
		}
		if got[0].VectorRank != 1 {
			t.Errorf("top result vector rank = %d, want 1", got[0].VectorRank)
		}
	})

	t.Run("rare term is found by full-text search", func(t *testing.T) {
		got, err := s.Search(ctx, "zorbafex", 3)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) == 0 {
			t.Fatal("no results for rare term")
		}
		if got[0].Ordinal != 1 {
			t.Fatalf("top result ordinal = %d, want 1", got[0].Ordinal)
		}
		if got[0].TextRank == 0 {
			t.Error("expected the rare-term hit to come from the text retriever")
		}
		if got[0].Heading != "Equipment" || got[0].Filename != "handbook.md" {
			t.Errorf("metadata not carried: %+v", got[0])
		}
	})

	t.Run("chunk hit by both retrievers outranks single-retriever hits", func(t *testing.T) {
		// Exact content (vector distance 0) + a word present in the text
		// ("vacation") — the RRF sum from two lists must beat any chunk
		// present in only one list.
		query := "Vacation policy grants twenty six days of paid leave annually."
		got, err := s.Search(ctx, query, 3)
		if err != nil {
			t.Fatal(err)
		}
		top := got[0]
		if top.Ordinal != 0 {
			t.Fatalf("top ordinal = %d, want 0", top.Ordinal)
		}
		if top.VectorRank == 0 || top.TextRank == 0 {
			t.Errorf("expected hits from both retrievers, got vector=%d text=%d",
				top.VectorRank, top.TextRank)
		}
	})

	t.Run("non-ready documents are excluded", func(t *testing.T) {
		got, err := s.Search(ctx, "vacation policy", 10)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range got {
			if r.DocumentID == pendingID {
				t.Fatalf("pending document leaked into results: %+v", r)
			}
		}
	})

	t.Run("limit is applied and capped", func(t *testing.T) {
		got, err := s.Search(ctx, "the", 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) > 1 {
			t.Fatalf("limit ignored: %d results", len(got))
		}
	})
}
