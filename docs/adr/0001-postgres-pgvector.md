# ADR-0001: PostgreSQL + pgvector as the vector store

**Status:** accepted · **Date:** 2026-07

## Context
The system needs vector similarity search over document chunks, plus full-text
search for hybrid retrieval, plus relational storage for documents, users and
chat history.

## Decision
Use PostgreSQL with the pgvector extension for all of it, behind a `VectorStore`
interface in Go.

## Consequences
+ One battle-tested store instead of Postgres + Qdrant + something for FTS.
+ Transactional consistency: chunks, metadata and jobs live in one database.
+ Hybrid search is a single SQL query (vector distance + tsvector rank, fused with RRF).
- At very large scale (>10M chunks) a dedicated vector DB wins on latency and
  filtering; the interface keeps Qdrant swappable and adding a second
  implementation is a planned exercise.
