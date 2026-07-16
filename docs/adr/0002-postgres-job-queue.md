# ADR-0002: Job queue in Postgres instead of a message broker

**Status:** accepted · **Date:** 2026-07

## Context
Document ingestion is asynchronous: upload returns immediately, a worker
processes the document in the background. The obvious reflex is to add
NATS/RabbitMQ/Redis.

## Decision
Implement the queue as a Postgres table consumed with
`SELECT ... FOR UPDATE SKIP LOCKED`.

## Consequences
+ Enqueue happens in the same transaction as the document insert — no
  dual-write problem, no outbox pattern needed.
+ Zero additional infrastructure to run, monitor and secure.
+ SKIP LOCKED gives safe concurrent consumption by multiple workers.
- Polling instead of push (acceptable at seconds-level latency for ingestion).
- At high throughput a broker becomes justified; the worker consumes through a
  narrow interface so the swap is localized.
