# Retrieval eval — 18 cases (reranked: rerank-2.5)

| Metric | Value |
|---|---|
| MRR | 0.866 |
| Recall@1 | 0.639 |
| Recall@3 | 0.806 |
| Recall@5 | 0.944 |
| Recall@10 | 1.000 |

## Per-case detail

### vectorstore-choice

> Why does this project use pgvector instead of a separate vector database?

RR: 1.000 · Recall@1: 0.50 · Recall@3: 0.50 · Recall@5: 1.00 · Recall@10: 1.00

Expected: `README.md#Design decisions` `0001-postgres-pgvector.md#Decision`

Retrieved (top 5):

1. [✓] `README.md#Design decisions` (rerank: 0.898)
2. [ ] `0001-postgres-pgvector.md#Consequences` (rerank: 0.738)
3. [ ] `README.md#Architecture` (rerank: 0.734)
4. [✓] `0001-postgres-pgvector.md#Decision` (rerank: 0.715)
5. [ ] `0001-postgres-pgvector.md#ADR-0001: PostgreSQL + pgvector as the vector store` (rerank: 0.582)

### vectorstore-swap

> Could this project switch from pgvector to Qdrant later?

RR: 1.000 · Recall@1: 0.50 · Recall@3: 1.00 · Recall@5: 1.00 · Recall@10: 1.00

Expected: `0001-postgres-pgvector.md#Consequences` `README.md#Design decisions`

Retrieved (top 5):

1. [✓] `README.md#Design decisions` (rerank: 0.723)
2. [✓] `0001-postgres-pgvector.md#Consequences` (rerank: 0.691)
3. [ ] `README.md#Architecture` (rerank: 0.480)
4. [ ] `0001-postgres-pgvector.md#Decision` (rerank: 0.459)
5. [ ] `0001-postgres-pgvector.md#ADR-0001: PostgreSQL + pgvector as the vector store` (rerank: 0.447)

### no-message-broker

> Why doesn't this project use a message broker like RabbitMQ or Kafka for ingestion?

RR: 0.250 · Recall@1: 0.00 · Recall@3: 0.00 · Recall@5: 0.50 · Recall@10: 1.00

Expected: `0002-postgres-job-queue.md#Decision` `0002-postgres-job-queue.md#Context`

Retrieved (top 5):

1. [ ] `README.md#Design decisions` (rerank: 0.762)
2. [ ] `0002-postgres-job-queue.md#Consequences` (rerank: 0.750)
3. [ ] `0002-postgres-job-queue.md#ADR-0002: Job queue in Postgres instead of a message broker` (rerank: 0.648)
4. [✓] `0002-postgres-job-queue.md#Context` (rerank: 0.574)
5. [ ] `README.md#Architecture` (rerank: 0.527)

### job-queue-tradeoffs

> What are the downsides of using Postgres as a job queue?

RR: 0.500 · Recall@1: 0.00 · Recall@3: 1.00 · Recall@5: 1.00 · Recall@10: 1.00

Expected: `0002-postgres-job-queue.md#Consequences`

Retrieved (top 5):

1. [ ] `README.md#Design decisions` (rerank: 0.531)
2. [✓] `0002-postgres-job-queue.md#Consequences` (rerank: 0.516)
3. [ ] `0001-postgres-pgvector.md#Consequences` (rerank: 0.488)
4. [ ] `README.md#Architecture` (rerank: 0.475)
5. [ ] `0002-postgres-job-queue.md#Decision` (rerank: 0.453)

### skip-locked

> How does the worker avoid two processes picking up the same job?

RR: 1.000 · Recall@1: 0.50 · Recall@3: 1.00 · Recall@5: 1.00 · Recall@10: 1.00

Expected: `0002-postgres-job-queue.md#Decision` `0002-postgres-job-queue.md#Consequences`

Retrieved (top 5):

1. [✓] `0002-postgres-job-queue.md#Decision` (rerank: 0.598)
2. [✓] `0002-postgres-job-queue.md#Consequences` (rerank: 0.539)
3. [ ] `0005-embedded-migrations.md#Decision` (rerank: 0.463)
4. [ ] `README.md#Design decisions` (rerank: 0.412)
5. [ ] `README.md#Architecture` (rerank: 0.393)

### parser-language

> Why is the document parser written in Python instead of Go?

RR: 0.500 · Recall@1: 0.00 · Recall@3: 0.50 · Recall@5: 1.00 · Recall@10: 1.00

Expected: `0003-python-parser-service.md#Decision` `README.md#Architecture`

Retrieved (top 5):

1. [ ] `README.md#Design decisions` (rerank: 0.828)
2. [✓] `README.md#Architecture` (rerank: 0.828)
3. [ ] `0003-python-parser-service.md#Context` (rerank: 0.820)
4. [✓] `0003-python-parser-service.md#Decision` (rerank: 0.711)
5. [ ] `0003-python-parser-service.md#Consequences` (rerank: 0.680)

### parser-service-boundary

> Is the parsing service a separate process from the main backend?

RR: 0.333 · Recall@1: 0.00 · Recall@3: 0.50 · Recall@5: 0.50 · Recall@10: 1.00

Expected: `0003-python-parser-service.md#Decision` `0003-python-parser-service.md#Consequences`

Retrieved (top 5):

1. [ ] `README.md#Design decisions` (rerank: 0.773)
2. [ ] `README.md#Architecture` (rerank: 0.773)
3. [✓] `0003-python-parser-service.md#Decision` (rerank: 0.734)
4. [ ] `README.md#Repository layout` (rerank: 0.730)
5. [ ] `0003-python-parser-service.md#ADR-0003: Separate Python service for document parsing` (rerank: 0.676)

### no-docling

> Why doesn't this project use docling for PDF parsing?

RR: 1.000 · Recall@1: 0.50 · Recall@3: 0.50 · Recall@5: 1.00 · Recall@10: 1.00

Expected: `0004-lightweight-parsing.md#Decision` `0004-lightweight-parsing.md#Context`

Retrieved (top 5):

1. [✓] `0004-lightweight-parsing.md#Context` (rerank: 0.898)
2. [ ] `0003-python-parser-service.md#Context` (rerank: 0.723)
3. [ ] `0004-lightweight-parsing.md#Consequences` (rerank: 0.703)
4. [✓] `0004-lightweight-parsing.md#Decision` (rerank: 0.621)
5. [ ] `0004-lightweight-parsing.md#ADR-0004: Lightweight parsing first, docling as the upgrade path` (rerank: 0.527)

### scanned-pdfs

> Does this project support scanned PDFs that have no text layer?

RR: 1.000 · Recall@1: 1.00 · Recall@3: 1.00 · Recall@5: 1.00 · Recall@10: 1.00

Expected: `0004-lightweight-parsing.md#Consequences`

Retrieved (top 5):

1. [✓] `0004-lightweight-parsing.md#Consequences` (rerank: 0.852)
2. [ ] `0004-lightweight-parsing.md#Context` (rerank: 0.500)
3. [ ] `0003-python-parser-service.md#Context` (rerank: 0.410)
4. [ ] `README.md#Architecture` (rerank: 0.387)
5. [ ] `0004-lightweight-parsing.md#Decision` (rerank: 0.385)

### migrations-tooling

> Why doesn't this project use goose or golang-migrate for database migrations?

RR: 1.000 · Recall@1: 0.50 · Recall@3: 0.50 · Recall@5: 1.00 · Recall@10: 1.00

Expected: `0005-embedded-migrations.md#Decision` `0005-embedded-migrations.md#Context`

Retrieved (top 5):

1. [✓] `0005-embedded-migrations.md#Context` (rerank: 0.820)
2. [ ] `0005-embedded-migrations.md#Consequences` (rerank: 0.695)
3. [ ] `0005-embedded-migrations.md#ADR-0005: Embedded migration runner instead of goose` (rerank: 0.605)
4. [✓] `0005-embedded-migrations.md#Decision` (rerank: 0.555)
5. [ ] `README.md#Repository layout` (rerank: 0.451)

### migrations-mechanism

> How are SQL migrations applied when the application starts?

RR: 1.000 · Recall@1: 1.00 · Recall@3: 1.00 · Recall@5: 1.00 · Recall@10: 1.00

Expected: `0005-embedded-migrations.md#Decision`

Retrieved (top 5):

1. [✓] `0005-embedded-migrations.md#Decision` (rerank: 0.809)
2. [ ] `0005-embedded-migrations.md#Consequences` (rerank: 0.621)
3. [ ] `0005-embedded-migrations.md#Context` (rerank: 0.516)
4. [ ] `README.md#Repository layout` (rerank: 0.492)
5. [ ] `0005-embedded-migrations.md#ADR-0005: Embedded migration runner instead of goose` (rerank: 0.395)

### quickstart

> How do I run this project locally for the first time?

RR: 1.000 · Recall@1: 1.00 · Recall@3: 1.00 · Recall@5: 1.00 · Recall@10: 1.00

Expected: `README.md#Quickstart`

Retrieved (top 5):

1. [✓] `README.md#Quickstart` (rerank: 0.770)
2. [ ] `README.md#Development` (rerank: 0.512)
3. [ ] `0004-lightweight-parsing.md#Context` (rerank: 0.475)
4. [ ] `0005-embedded-migrations.md#Decision` (rerank: 0.383)
5. [ ] `README.md#Repository layout` (rerank: 0.357)

### repo-layout

> What are the top-level folders in this repository and what do they contain?

RR: 1.000 · Recall@1: 1.00 · Recall@3: 1.00 · Recall@5: 1.00 · Recall@10: 1.00

Expected: `README.md#Repository layout`

Retrieved (top 5):

1. [✓] `README.md#Repository layout` (rerank: 0.832)
2. [ ] `0006-retrieval-eval-harness.md#Decision` (rerank: 0.426)
3. [ ] `README.md#Architecture` (rerank: 0.361)
4. [ ] `README.md#Quickstart` (rerank: 0.359)
5. [ ] `README.md#Development` (rerank: 0.357)

### kubernetes-support

> Does this project include Kubernetes deployment manifests?

RR: 1.000 · Recall@1: 1.00 · Recall@3: 1.00 · Recall@5: 1.00 · Recall@10: 1.00

Expected: `README.md#Repository layout`

Retrieved (top 5):

1. [✓] `README.md#Repository layout` (rerank: 0.805)
2. [ ] `0005-embedded-migrations.md#Consequences` (rerank: 0.387)
3. [ ] `README.md#Quickstart` (rerank: 0.336)
4. [ ] `README.md#Development` (rerank: 0.295)
5. [ ] `README.md#Architecture` (rerank: 0.289)

### run-tests

> How do I run the test suite for this project?

RR: 1.000 · Recall@1: 1.00 · Recall@3: 1.00 · Recall@5: 1.00 · Recall@10: 1.00

Expected: `README.md#Development`

Retrieved (top 5):

1. [✓] `README.md#Development` (rerank: 0.711)
2. [ ] `README.md#Quickstart` (rerank: 0.410)
3. [ ] `0006-retrieval-eval-harness.md#Decision` (rerank: 0.404)
4. [ ] `0005-embedded-migrations.md#Decision` (rerank: 0.350)
5. [ ] `0006-retrieval-eval-harness.md#Consequences` (rerank: 0.344)

### ci-checks

> What does continuous integration check on every push?

RR: 1.000 · Recall@1: 1.00 · Recall@3: 1.00 · Recall@5: 1.00 · Recall@10: 1.00

Expected: `README.md#Development`

Retrieved (top 5):

1. [✓] `README.md#Development` (rerank: 0.816)
2. [ ] `0005-embedded-migrations.md#Consequences` (rerank: 0.336)
3. [ ] `0005-embedded-migrations.md#Decision` (rerank: 0.285)
4. [ ] `README.md#Repository layout` (rerank: 0.285)
5. [ ] `0002-postgres-job-queue.md#Context` (rerank: 0.283)

### backend-languages

> What programming languages does the backend use and why?

RR: 1.000 · Recall@1: 1.00 · Recall@3: 1.00 · Recall@5: 1.00 · Recall@10: 1.00

Expected: `README.md#Architecture`

Retrieved (top 5):

1. [✓] `README.md#Architecture` (rerank: 0.789)
2. [ ] `README.md#Repository layout` (rerank: 0.688)
3. [ ] `0003-python-parser-service.md#Context` (rerank: 0.621)
4. [ ] `0003-python-parser-service.md#Decision` (rerank: 0.617)
5. [ ] `0001-postgres-pgvector.md#Decision` (rerank: 0.613)

### sync-vs-async

> Why does this system separate document ingestion from answering questions?

RR: 1.000 · Recall@1: 1.00 · Recall@3: 1.00 · Recall@5: 1.00 · Recall@10: 1.00

Expected: `README.md#Architecture`

Retrieved (top 5):

1. [✓] `README.md#Architecture` (rerank: 0.801)
2. [ ] `0002-postgres-job-queue.md#Context` (rerank: 0.520)
3. [ ] `README.md#Garden of Knowledge` (rerank: 0.479)
4. [ ] `0002-postgres-job-queue.md#Consequences` (rerank: 0.439)
5. [ ] `README.md#Design decisions` (rerank: 0.436)
