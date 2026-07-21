-- Phase 3 step 1: accounts and per-user document isolation.

CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email         TEXT        NOT NULL UNIQUE,
    password_hash TEXT        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Nullable on purpose: documents uploaded before this migration have no
-- owner and simply become invisible (every query now filters by user_id).
-- Dev data is disposable; re-upload after creating an account.
ALTER TABLE documents
    ADD COLUMN user_id UUID REFERENCES users(id) ON DELETE CASCADE;

CREATE INDEX idx_documents_user ON documents (user_id);
