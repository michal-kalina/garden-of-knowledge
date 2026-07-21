-- Phase 3 step 2: persisted conversations.
--
-- Messages store a denormalized snapshot of sources (JSONB) rather than a
-- join table to chunks: chunks can be re-chunked or deleted on re-ingestion,
-- but a past answer's citations must keep pointing at what the model
-- actually saw when it answered. History is a record of what was said, not
-- a live view of the current corpus.

CREATE TABLE conversations (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title      TEXT        NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_conversations_user ON conversations (user_id, updated_at DESC);

CREATE TABLE messages (
    id              BIGSERIAL PRIMARY KEY,
    conversation_id UUID        NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    role            TEXT        NOT NULL CHECK (role IN ('user','assistant')),
    content         TEXT        NOT NULL,
    sources         JSONB       NOT NULL DEFAULT '[]',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_messages_conversation ON messages (conversation_id, id);
