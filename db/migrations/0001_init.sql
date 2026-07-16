-- Phase 0: base schema.
-- In dev, migrations are executed by docker-entrypoint-initdb.d (only on the first start of the volume).
-- TODO(Phase 1): switch to a migration tool (goose / golang-migrate).

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

-- Ingestion queue in PostgreSQL (ADR-0002). Worker picks up jobs via
-- SELECT ... FOR UPDATE SKIP LOCKED, which provides safe concurrent access
-- for multiple workers without an additional broker.
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

-- chunks with embeddings (filled in Phase 1).
-- Dimension 1024 fits e.g. voyage-3 / bge-m3; adjust to the chosen embedding model.
CREATE TABLE chunks (
    id          BIGSERIAL PRIMARY KEY,
    document_id UUID    NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    ordinal     INT     NOT NULL,                  -- position of the chunk in the document
    content     TEXT    NOT NULL,
    embedding   vector(1024),
    tsv         tsvector GENERATED ALWAYS AS (to_tsvector('simple', content)) STORED,
    UNIQUE (document_id, ordinal)
);

CREATE INDEX idx_chunks_tsv ON chunks USING gin (tsv);
-- Vector index (HNSW) we will add in Phase 1, when data becomes available:
-- CREATE INDEX idx_chunks_embedding ON chunks USING hnsw (embedding vector_cosine_ops);
