-- Phase 1 step 2: chunk metadata needed for citations (Phase 2).
-- Adding these later would force re-ingesting every document, so the columns
-- land together with the chunking implementation.
--
-- Dev note: docker-entrypoint-initdb.d applies migration files in order, but
-- only on a FRESH volume. To apply this one locally:
--   docker compose down -v && make up
-- (dev data is disposable; a proper migration tool arrives in step 3).

ALTER TABLE chunks
    ADD COLUMN heading    TEXT NOT NULL DEFAULT '',
    ADD COLUMN page_start INT,
    ADD COLUMN page_end   INT;
