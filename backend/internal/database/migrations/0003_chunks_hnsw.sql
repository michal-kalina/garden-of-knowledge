-- Phase 1 step 3: approximate-nearest-neighbour index for vector search.
-- HNSW over cosine distance — the operator class must match the distance
-- used at query time (<=> in Phase 2 retrieval).
CREATE INDEX IF NOT EXISTS idx_chunks_embedding
    ON chunks USING hnsw (embedding vector_cosine_ops);
