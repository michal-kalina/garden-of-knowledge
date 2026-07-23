# Retrieval eval — 18 cases (no reranking)

| Metric | Value |
|---|---|
| MRR | 0.833 |
| Recall@1 | 0.556 |
| Recall@3 | 0.944 |
| Recall@5 | 0.944 |
| Recall@10 | 1.000 |

## Per-case detail

### vectorstore-choice

> Why does this project use pgvector instead of a separate vector database?

RR: 1.000 · Recall@1: 0.50 · Recall@3: 1.00 · Recall@5: 1.00 · Recall@10: 1.00

Expected: `README.md#Design decisions` `0001-postgres-pgvector.md#Decision`

Retrieved (top 5):

1. [✓] `README.md#Design decisions`
2. [✓] `0001-postgres-pgvector.md#Decision`
3. [ ] `0001-postgres-pgvector.md#Consequences`
4. [ ] `0001-postgres-pgvector.md#ADR-0001: PostgreSQL + pgvector as the vector store`
5. [ ] `README.md#Architecture`

### vectorstore-swap

> Could this project switch from pgvector to Qdrant later?

RR: 1.000 · Recall@1: 0.50 · Recall@3: 1.00 · Recall@5: 1.00 · Recall@10: 1.00

Expected: `0001-postgres-pgvector.md#Consequences` `README.md#Design decisions`

Retrieved (top 5):

1. [✓] `README.md#Design decisions`
2. [✓] `0001-postgres-pgvector.md#Consequences`
3. [ ] `0001-postgres-pgvector.md#Decision`
4. [ ] `0001-postgres-pgvector.md#ADR-0001: PostgreSQL + pgvector as the vector store`
5. [ ] `README.md#Architecture`

### no-message-broker

> Why doesn't this project use a message broker like RabbitMQ or Kafka for ingestion?

RR: 0.500 · Recall@1: 0.00 · Recall@3: 0.50 · Recall@5: 0.50 · Recall@10: 1.00

Expected: `0002-postgres-job-queue.md#Decision` `0002-postgres-job-queue.md#Context`

Retrieved (top 5):

1. [ ] `0002-postgres-job-queue.md#Consequences`
2. [✓] `0002-postgres-job-queue.md#Context`
3. [ ] `README.md#Design decisions`
4. [ ] `README.md#Architecture`
5. [ ] `0002-postgres-job-queue.md#ADR-0002: Job queue in Postgres instead of a message broker`

### job-queue-tradeoffs

> What are the downsides of using Postgres as a job queue?

RR: 0.333 · Recall@1: 0.00 · Recall@3: 1.00 · Recall@5: 1.00 · Recall@10: 1.00

Expected: `0002-postgres-job-queue.md#Consequences`

Retrieved (top 5):

1. [ ] `0002-postgres-job-queue.md#Decision`
2. [ ] `0002-postgres-job-queue.md#ADR-0002: Job queue in Postgres instead of a message broker`
3. [✓] `0002-postgres-job-queue.md#Consequences`
4. [ ] `README.md#Design decisions`
5. [ ] `0001-postgres-pgvector.md#Consequences`

### skip-locked

> How does the worker avoid two processes picking up the same job?

RR: 1.000 · Recall@1: 0.50 · Recall@3: 1.00 · Recall@5: 1.00 · Recall@10: 1.00

Expected: `0002-postgres-job-queue.md#Decision` `0002-postgres-job-queue.md#Consequences`

Retrieved (top 5):

1. [✓] `0002-postgres-job-queue.md#Decision`
2. [✓] `0002-postgres-job-queue.md#Consequences`
3. [ ] `0002-postgres-job-queue.md#Context`
4. [ ] `0005-embedded-migrations.md#Decision`
5. [ ] `0002-postgres-job-queue.md#ADR-0002: Job queue in Postgres instead of a message broker`

### parser-language

> Why is the document parser written in Python instead of Go?

RR: 0.500 · Recall@1: 0.00 · Recall@3: 0.50 · Recall@5: 0.50 · Recall@10: 1.00

Expected: `0003-python-parser-service.md#Decision` `README.md#Architecture`

Retrieved (top 5):

1. [ ] `0003-python-parser-service.md#Context`
2. [✓] `0003-python-parser-service.md#Decision`
3. [ ] `README.md#Design decisions`
4. [ ] `0004-lightweight-parsing.md#Context`
5. [ ] `0003-python-parser-service.md#Consequences`

### parser-service-boundary

> Is the parsing service a separate process from the main backend?

RR: 1.000 · Recall@1: 0.50 · Recall@3: 1.00 · Recall@5: 1.00 · Recall@10: 1.00

Expected: `0003-python-parser-service.md#Decision` `0003-python-parser-service.md#Consequences`

Retrieved (top 5):

1. [✓] `0003-python-parser-service.md#Consequences`
2. [✓] `0003-python-parser-service.md#Decision`
3. [ ] `0003-python-parser-service.md#ADR-0003: Separate Python service for document parsing`
4. [ ] `README.md#Architecture`
5. [ ] `README.md#Design decisions`

### no-docling

> Why doesn't this project use docling for PDF parsing?

RR: 1.000 · Recall@1: 0.50 · Recall@3: 1.00 · Recall@5: 1.00 · Recall@10: 1.00

Expected: `0004-lightweight-parsing.md#Decision` `0004-lightweight-parsing.md#Context`

Retrieved (top 5):

1. [✓] `0004-lightweight-parsing.md#Context`
2. [ ] `0004-lightweight-parsing.md#Consequences`
3. [✓] `0004-lightweight-parsing.md#Decision`
4. [ ] `0003-python-parser-service.md#Context`
5. [ ] `0004-lightweight-parsing.md#ADR-0004: Lightweight parsing first, docling as the upgrade path`

### scanned-pdfs

> Does this project support scanned PDFs that have no text layer?

RR: 1.000 · Recall@1: 1.00 · Recall@3: 1.00 · Recall@5: 1.00 · Recall@10: 1.00

Expected: `0004-lightweight-parsing.md#Consequences`

Retrieved (top 5):

1. [✓] `0004-lightweight-parsing.md#Consequences`
2. [ ] `0004-lightweight-parsing.md#Context`
3. [ ] `0003-python-parser-service.md#Context`
4. [ ] `0004-lightweight-parsing.md#Decision`
5. [ ] `0003-python-parser-service.md#ADR-0003: Separate Python service for document parsing`

### migrations-tooling

> Why doesn't this project use goose or golang-migrate for database migrations?

RR: 1.000 · Recall@1: 0.50 · Recall@3: 1.00 · Recall@5: 1.00 · Recall@10: 1.00

Expected: `0005-embedded-migrations.md#Decision` `0005-embedded-migrations.md#Context`

Retrieved (top 5):

1. [✓] `0005-embedded-migrations.md#Context`
2. [ ] `0005-embedded-migrations.md#Consequences`
3. [✓] `0005-embedded-migrations.md#Decision`
4. [ ] `0005-embedded-migrations.md#ADR-0005: Embedded migration runner instead of goose`
5. [ ] `README.md#Design decisions`

### migrations-mechanism

> How are SQL migrations applied when the application starts?

RR: 1.000 · Recall@1: 1.00 · Recall@3: 1.00 · Recall@5: 1.00 · Recall@10: 1.00

Expected: `0005-embedded-migrations.md#Decision`

Retrieved (top 5):

1. [✓] `0005-embedded-migrations.md#Decision`
2. [ ] `0005-embedded-migrations.md#Consequences`
3. [ ] `0005-embedded-migrations.md#Context`
4. [ ] `0005-embedded-migrations.md#ADR-0005: Embedded migration runner instead of goose`
5. [ ] `README.md#Repository layout`

### quickstart

> How do I run this project locally for the first time?

RR: 1.000 · Recall@1: 1.00 · Recall@3: 1.00 · Recall@5: 1.00 · Recall@10: 1.00

Expected: `README.md#Quickstart`

Retrieved (top 5):

1. [✓] `README.md#Quickstart`
2. [ ] `README.md#Development`
3. [ ] `README.md#Garden of Knowledge`
4. [ ] `README.md#Repository layout`
5. [ ] `0006-retrieval-eval-harness.md#Decision`

### repo-layout

> What are the top-level folders in this repository and what do they contain?

RR: 1.000 · Recall@1: 1.00 · Recall@3: 1.00 · Recall@5: 1.00 · Recall@10: 1.00

Expected: `README.md#Repository layout`

Retrieved (top 5):

1. [✓] `README.md#Repository layout`
2. [ ] `0006-retrieval-eval-harness.md#Decision`
3. [ ] `README.md#Garden of Knowledge`
4. [ ] `0003-python-parser-service.md#Context`
5. [ ] `0001-postgres-pgvector.md#Context`

### kubernetes-support

> Does this project include Kubernetes deployment manifests?

RR: 1.000 · Recall@1: 1.00 · Recall@3: 1.00 · Recall@5: 1.00 · Recall@10: 1.00

Expected: `README.md#Repository layout`

Retrieved (top 5):

1. [✓] `README.md#Repository layout`
2. [ ] `0005-embedded-migrations.md#Consequences`
3. [ ] `README.md#Development`
4. [ ] `0005-embedded-migrations.md#ADR-0005: Embedded migration runner instead of goose`
5. [ ] `0005-embedded-migrations.md#Context`

### run-tests

> How do I run the test suite for this project?

RR: 1.000 · Recall@1: 1.00 · Recall@3: 1.00 · Recall@5: 1.00 · Recall@10: 1.00

Expected: `README.md#Development`

Retrieved (top 5):

1. [✓] `README.md#Development`
2. [ ] `0006-retrieval-eval-harness.md#Decision`
3. [ ] `README.md#Quickstart`
4. [ ] `0006-retrieval-eval-harness.md#Consequences`
5. [ ] `0006-retrieval-eval-harness.md#Context`

### ci-checks

> What does continuous integration check on every push?

RR: 1.000 · Recall@1: 1.00 · Recall@3: 1.00 · Recall@5: 1.00 · Recall@10: 1.00

Expected: `README.md#Development`

Retrieved (top 5):

1. [✓] `README.md#Development`
2. [ ] `0005-embedded-migrations.md#Consequences`
3. [ ] `0006-retrieval-eval-harness.md#Context`
4. [ ] `0006-retrieval-eval-harness.md#Consequences`
5. [ ] `0002-postgres-job-queue.md#Context`

### backend-languages

> What programming languages does the backend use and why?

RR: 0.333 · Recall@1: 0.00 · Recall@3: 1.00 · Recall@5: 1.00 · Recall@10: 1.00

Expected: `README.md#Architecture`

Retrieved (top 5):

1. [ ] `0003-python-parser-service.md#Consequences`
2. [ ] `0003-python-parser-service.md#Decision`
3. [✓] `README.md#Architecture`
4. [ ] `0003-python-parser-service.md#Context`
5. [ ] `README.md#Design decisions`

### sync-vs-async

> Why does this system separate document ingestion from answering questions?

RR: 0.333 · Recall@1: 0.00 · Recall@3: 1.00 · Recall@5: 1.00 · Recall@10: 1.00

Expected: `README.md#Architecture`

Retrieved (top 5):

1. [ ] `README.md#Design decisions`
2. [ ] `0002-postgres-job-queue.md#Context`
3. [✓] `README.md#Architecture`
4. [ ] `0004-lightweight-parsing.md#Consequences`
5. [ ] `0004-lightweight-parsing.md#Context`

