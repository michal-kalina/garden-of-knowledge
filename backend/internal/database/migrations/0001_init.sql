-- Phase 0: base schema.
-- Applied by the embedded migration runner (internal/database/migrate.go)
-- at service startup; tracked in schema_migrations. See ADR-0005.

CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE documents (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    filename     TEXT        NOT NULL,
    content_type TEXT        NOT NULL,
    size_bytes   BIGINT      NOT NULL,
    storage_key  TEXT        NOT NULL,             -- object key in MinIO/S3
    status       TEXT        NOT NULL DEFAULT 'pending'
                 CHECK (status IN ('pending','processing','ready','failed')),
    error        TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Job queue in Postgres (ADR-0002). The worker claims jobs via
-- SELECT ... FOR UPDATE SKIP LOCKED, which gives safe concurrent consumption
-- by multiple workers without an extra broker.
CREATE TABLE ingestion_jobs (
    id           BIGSERIAL PRIMARY KEY,
    document_id  UUID        NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    status       TEXT        NOT NULL DEFAULT 'queued'
                 CHECK (status IN ('queued','running','done','failed')),
    attempts     INT         NOT NULL DEFAULT 0,
    max_attempts INT         NOT NULL DEFAULT 3,
    last_error   TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_ingestion_jobs_queued ON ingestion_jobs (created_at) WHERE status = 'queued';

-- Chunks with embeddings (populated in Phase 1).
-- Dimension 1024 fits e.g. voyage-3 / bge-m3; adjust to the chosen embedding model.
CREATE TABLE chunks (
    id          BIGSERIAL PRIMARY KEY,
    document_id UUID    NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    ordinal     INT     NOT NULL,                  -- chunk position within the document
    content     TEXT    NOT NULL,
    embedding   vector(1024),
    tsv         tsvector GENERATED ALWAYS AS (to_tsvector('simple', content)) STORED,
    UNIQUE (document_id, ordinal)
);

CREATE INDEX idx_chunks_tsv ON chunks USING gin (tsv);
-- The vector index (HNSW) arrives in Phase 1 once there is data:
-- CREATE INDEX idx_chunks_embedding ON chunks USING hnsw (embedding vector_cosine_ops);
