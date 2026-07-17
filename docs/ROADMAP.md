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
- [ ] Worker: fetch file → call parser → chunk → embed → store chunks → mark ready
- [ ] Real PDF/Markdown parsing in the parser service (docling)
- [ ] Chunking strategies (fixed-size with overlap first; document-structure-aware later)
- [ ] Retry with attempt limits; failed jobs surface the error on the document
- [ ] Migration tooling (goose), HNSW index on embeddings

## Phase 2 — Retrieval & chat
- [ ] `POST /chat` — SSE streaming responses
- [ ] Hybrid search: vector + full-text, fused with RRF
- [ ] Prompt assembly with retrieved context; citations mapped to chunk IDs
- [ ] Next.js frontend: upload view, chat with streaming, citation panel

## Phase 3 — Users & history
- [ ] JWT auth, per-user document isolation
- [ ] Persisted conversations

## Phase 4 — Quality & observability
- [ ] Golden dataset (20–30 Q/A pairs), retrieval recall@k, answer quality evals
- [ ] OpenTelemetry traces + Langfuse; cost per query
- [ ] Reranking experiment with before/after eval numbers

## Phase 5 — Deployment
- [ ] Kubernetes manifests / Helm chart, k3d walkthrough
- [ ] CI/CD image publishing
