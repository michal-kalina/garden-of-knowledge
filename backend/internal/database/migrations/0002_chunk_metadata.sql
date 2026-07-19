-- Phase 1 step 2: chunk metadata needed for citations (Phase 2).
-- Adding these later would force re-ingesting every document, so the columns
-- land together with the chunking implementation.

ALTER TABLE chunks
    ADD COLUMN heading    TEXT NOT NULL DEFAULT '',
    ADD COLUMN page_start INT,
    ADD COLUMN page_end   INT;
