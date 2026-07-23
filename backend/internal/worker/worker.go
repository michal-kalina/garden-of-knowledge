// Package worker implements the ingestion pipeline: it claims jobs from the
// Postgres-backed queue and drives a document from raw bytes in object
// storage to embedded chunks in pgvector.
//
// Pipeline: fetch from storage -> parse (Python service) -> chunk -> embed
// -> store chunks + mark ready, with attempt-limited retry on failure.
package worker

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/michal-kalina/garden-of-knowledge/backend/internal/chunking"
	"github.com/michal-kalina/garden-of-knowledge/backend/internal/cost"
	"github.com/michal-kalina/garden-of-knowledge/backend/internal/database"
	"github.com/michal-kalina/garden-of-knowledge/backend/internal/embeddings"
	"github.com/michal-kalina/garden-of-knowledge/backend/internal/storage"
)

// Parser is what the worker needs from the parsing service client.
type Parser interface {
	Parse(ctx context.Context, filename, contentType string, r io.Reader) ([]chunking.Block, error)
}

// staleAfter is how long a job may sit in 'running' before another worker is
// allowed to take it over — the recovery path for a worker that crashed
// mid-job (its claim transaction committed, so SKIP LOCKED no longer guards
// the row).
const staleAfter = "10 minutes"

// Processor wires the pipeline's dependencies. All of them are interfaces or
// injectable values, which is what makes the integration test able to run
// the real pipeline against a real database with in-memory fakes for the
// parser, storage and embeddings.
type Processor struct {
	DB       *sql.DB
	Store    storage.ObjectStore
	Parser   Parser
	Embedder embeddings.Embedder
	Chunker  chunking.Chunker
	Logger   *slog.Logger
}

// task is a claimed job joined with its document.
type task struct {
	jobID       int64
	attempts    int
	maxAttempts int
	docID       string
	filename    string
	contentType string
	storageKey  string
}

// Run polls the queue until the context is cancelled. After each tick it
// drains the queue completely before sleeping again, so a batch upload does
// not pay the poll interval per document.
func (p *Processor) Run(ctx context.Context, poll time.Duration) {
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			p.Logger.Info("worker shutting down")
			return
		case <-ticker.C:
			for ctx.Err() == nil {
				worked, err := p.ProcessOne(ctx)
				if err != nil {
					p.Logger.Error("process job", "error", err)
					break
				}
				if !worked {
					break
				}
			}
		}
	}
}

// ProcessOne claims and processes a single job. It reports whether a job was
// found; pipeline failures are handled internally via the retry bookkeeping
// and are not returned as errors.
func (p *Processor) ProcessOne(ctx context.Context) (bool, error) {
	t, ok, err := p.claim(ctx)
	if err != nil || !ok {
		return false, err
	}

	log := p.Logger.With("job_id", t.jobID, "document_id", t.docID, "attempt", t.attempts)
	log.Info("processing document", "filename", t.filename)

	if err := p.process(ctx, t); err != nil {
		log.Error("pipeline failed", "error", err)
		if ferr := p.fail(ctx, t, err); ferr != nil {
			return true, fmt.Errorf("record failure: %w", ferr)
		}
		return true, nil
	}

	log.Info("document ready")
	return true, nil
}

// claim atomically takes one job: queued, or running-but-stale (crashed
// worker takeover). Claiming is a short transaction separate from the actual
// work — holding row locks across a multi-second pipeline (network calls to
// the parser and embedding provider) would serialize all workers.
func (p *Processor) claim(ctx context.Context) (task, bool, error) {
	tx, err := p.DB.BeginTx(ctx, nil)
	if err != nil {
		return task{}, false, err
	}
	defer tx.Rollback() //nolint:errcheck // rollback after commit is a no-op

	var t task
	err = tx.QueryRowContext(ctx, `
		SELECT j.id, j.attempts, j.max_attempts,
		       d.id, d.filename, d.content_type, d.storage_key
		FROM ingestion_jobs j
		JOIN documents d ON d.id = j.document_id
		WHERE j.status = 'queued'
		   OR (j.status = 'running' AND j.updated_at < now() - $1::interval)
		ORDER BY j.created_at
		FOR UPDATE OF j SKIP LOCKED
		LIMIT 1
	`, staleAfter).Scan(&t.jobID, &t.attempts, &t.maxAttempts,
		&t.docID, &t.filename, &t.contentType, &t.storageKey)
	if errors.Is(err, sql.ErrNoRows) {
		return task{}, false, nil // empty queue is the normal idle state
	}
	if err != nil {
		return task{}, false, err
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE ingestion_jobs
		SET status = 'running', attempts = attempts + 1, updated_at = now()
		WHERE id = $1
	`, t.jobID); err != nil {
		return task{}, false, err
	}
	t.attempts++

	if _, err := tx.ExecContext(ctx, `
		UPDATE documents SET status = 'processing', updated_at = now() WHERE id = $1
	`, t.docID); err != nil {
		return task{}, false, err
	}

	if err := tx.Commit(); err != nil {
		return task{}, false, err
	}
	return t, true, nil
}

var tracer = otel.Tracer("gok/worker")

// process runs the pipeline for a claimed task and finalizes it.
func (p *Processor) process(ctx context.Context, t task) error {
	ctx, span := tracer.Start(ctx, "worker.process_document")
	defer span.End()
	span.SetAttributes(
		attribute.String("gok.document_id", t.docID),
		attribute.String("gok.filename", t.filename),
		attribute.Int("gok.attempt", t.attempts),
	)

	obj, err := func() (io.ReadCloser, error) {
		_, span := tracer.Start(ctx, "worker.fetch")
		defer span.End()
		o, err := p.Store.Get(ctx, t.storageKey)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "fetch failed")
		}
		return o, err
	}()
	if err != nil {
		return fmt.Errorf("fetch from storage: %w", err)
	}
	defer obj.Close()

	var blocks []chunking.Block
	err = func() error {
		parseCtx, span := tracer.Start(ctx, "worker.parse")
		defer span.End()
		b, err := p.Parser.Parse(parseCtx, t.filename, t.contentType, obj)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "parse failed")
			return err
		}
		span.SetAttributes(attribute.Int("gok.block_count", len(b)))
		blocks = b
		return nil
	}()
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	_, chunkSpan := tracer.Start(ctx, "worker.chunk")
	chunks := p.Chunker.Chunk(blocks)
	chunkSpan.SetAttributes(attribute.Int("gok.chunk_count", len(chunks)))
	chunkSpan.End()

	var vectors [][]float32
	if len(chunks) > 0 {
		embedCtx, span := tracer.Start(ctx, "worker.embed")
		texts := make([]string, len(chunks))
		approxTokens := 0
		for i, c := range chunks {
			texts[i] = c.Content
			approxTokens += chunking.ApproxTokens(c.Content)
		}
		vectors, err = p.Embedder.Embed(embedCtx, texts, embeddings.InputDocument)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "embed failed")
			span.End()
			return fmt.Errorf("embed: %w", err)
		}
		if len(vectors) != len(chunks) {
			span.End()
			return fmt.Errorf("embedder returned %d vectors for %d chunks", len(vectors), len(chunks))
		}
		model := p.Embedder.ModelName()
		span.SetAttributes(
			attribute.String("gok.model", model),
			attribute.Int("gok.batch_size", len(chunks)),
			attribute.Float64("gok.cost_usd_estimate", cost.EmbeddingUSD(model, approxTokens)),
		)
		span.End()
	}

	// Finalize in one transaction. The DELETE makes reprocessing idempotent:
	// a retry (or stale-job takeover) can never leave duplicate chunks.
	ctx, storeSpan := tracer.Start(ctx, "worker.store")
	defer storeSpan.End()

	tx, err := p.DB.BeginTx(ctx, nil)
	if err != nil {
		storeSpan.RecordError(err)
		storeSpan.SetStatus(codes.Error, "begin tx failed")
		return err
	}
	defer tx.Rollback() //nolint:errcheck // rollback after commit is a no-op

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM chunks WHERE document_id = $1`, t.docID); err != nil {
		storeSpan.RecordError(err)
		storeSpan.SetStatus(codes.Error, "clear chunks failed")
		return fmt.Errorf("clear previous chunks: %w", err)
	}

	for i, c := range chunks {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO chunks (document_id, ordinal, content, heading, page_start, page_end, embedding)
			VALUES ($1, $2, $3, $4, $5, $6, $7::vector)
		`, t.docID, i, c.Content, c.Heading, c.StartPage, c.EndPage,
			database.VectorLiteral(vectors[i])); err != nil {
			storeSpan.RecordError(err)
			storeSpan.SetStatus(codes.Error, "insert chunk failed")
			return fmt.Errorf("insert chunk %d: %w", i, err)
		}
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE ingestion_jobs SET status = 'done', updated_at = now() WHERE id = $1
	`, t.jobID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE documents SET status = 'ready', error = NULL, updated_at = now() WHERE id = $1
	`, t.docID); err != nil {
		return err
	}
	return tx.Commit()
}

// fail records a pipeline failure: requeue while attempts remain, otherwise
// mark both the job and the document as failed with the error preserved for
// the API to surface.
func (p *Processor) fail(ctx context.Context, t task, procErr error) error {
	msg := procErr.Error()
	if len(msg) > 1000 {
		msg = msg[:1000]
	}

	tx, err := p.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // rollback after commit is a no-op

	if t.attempts >= t.maxAttempts {
		if _, err := tx.ExecContext(ctx, `
			UPDATE ingestion_jobs SET status = 'failed', last_error = $2, updated_at = now() WHERE id = $1
		`, t.jobID, msg); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE documents SET status = 'failed', error = $2, updated_at = now() WHERE id = $1
		`, t.docID, msg); err != nil {
			return err
		}
	} else {
		if _, err := tx.ExecContext(ctx, `
			UPDATE ingestion_jobs SET status = 'queued', last_error = $2, updated_at = now() WHERE id = $1
		`, t.jobID, msg); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE documents SET status = 'pending', updated_at = now() WHERE id = $1
		`, t.docID); err != nil {
			return err
		}
	}
	return tx.Commit()
}
