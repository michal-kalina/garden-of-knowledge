# Roadmap

## Phase 0 — Foundation ✅
- [x] Monorepo layout, docker-compose dev environment
- [x] Go API skeleton: config, structured logging, liveness/readiness, graceful shutdown
- [x] Worker skeleton: Postgres job queue via `FOR UPDATE SKIP LOCKED`
- [x] Parser service skeleton (FastAPI) with a defined `/parse` contract
- [x] Initial schema: documents, ingestion_jobs, chunks (vector + tsvector)
- [x] CI: gofmt, vet, tests (-race), ruff, Docker builds
- [x] ADRs 0001–0003

## Phase 1 — Ingestion pipeline
- [x] `POST /documents` — upload to MinIO, insert document + enqueue job in one transaction
- [x] `GET /documents` / `GET /documents/{id}` — status tracking
- [x] Parser client and embeddings client (Voyage + offline fake) in Go
- [x] Worker: fetch file → call parser → chunk → embed → store chunks → mark ready
- [x] Real PDF/Markdown parsing (PyMuPDF + native Markdown; docling documented as the OCR upgrade path — ADR-0004)
- [x] Chunking strategies: recursive (baseline with overlap) and structure-aware (heading inheritance, atomic tables) behind a common `Chunker` interface
- [x] Retry with attempt limits and stale-job takeover; failed jobs surface the error on the document
- [x] Embedded migration runner (ADR-0005) applied at startup; HNSW index on embeddings
- [x] Integration test of the full pipeline against real Postgres+pgvector (CI service container)

## Phase 2 — Retrieval & chat
- [x] `POST /chat` — SSE streaming (sources → deltas → done), stateless
- [x] Multi-provider LLM layer: Anthropic and OpenRouter behind the `Streamer` interface, selected by config
- [x] Embeddings on `voyage-4` (200M free tokens) with the output dimension pinned to the schema
- [x] Hybrid search: vector (HNSW, cosine) + full-text (websearch_to_tsquery), fused with RRF; exposed as `POST /search` with per-retriever ranks for debuggability
- [x] Prompt assembly with numbered sources and metadata; [n] citations mapped to chunk IDs via the sources event
- [x] Next.js frontend (Tailwind + shadcn-style components): upload with live statuses, streaming chat, [n] citations flashing their source card

## Phase 3 — Users & history
- [x] Auth (register/login, HS256 session tokens, PBKDF2-hashed passwords with RFC test vectors) and per-user isolation of documents, search and chat — enforced in SQL, verified by an integration test
- [x] Persisted conversations: conversations/messages tables, `POST /chat` accepts `conversation_id` (creates or continues), history threaded into the prompt, exchange persisted only after a successful answer — verified by a cross-user isolation integration test

## Phase 4 — Quality & observability
- [x] Golden dataset (18 judged cases, `docs/eval/golden.json`) + retrieval eval harness (`cmd/eval`): recall@k and MRR against the real hybrid retriever, `make eval-retrieval` — see [ADR-0006](docs/adr/0006-retrieval-eval-harness.md) and [docs/eval/README.md](docs/eval/README.md) for a documented finding on lexical-retriever behavior
- [ ] OpenTelemetry traces + Langfuse; cost per query
- [x] Reranking: Voyage cross-encoder behind a `Retriever`-shaped decorator (`retrieval.Reranked`), opt-in via `RERANK_PROVIDER`, `make eval-retrieval-reranked` for a before/after comparison against the Step 1 baseline — see [ADR-0007](docs/adr/0007-reranking.md)

## Phase 5 — Deployment
- [ ] Kubernetes manifests / Helm chart, k3d walkthrough
- [ ] CI/CD image publishing
