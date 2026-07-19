# ADR-0005: Embedded migration runner instead of goose

**Status:** accepted · **Date:** 2026-07

## Context
Schema changes outgrew docker-entrypoint-initdb.d, which only runs on a
fresh volume. The default reflex is goose or golang-migrate — both solid,
both pulling a dependency tree (multi-dialect drivers) far larger than the
problem being solved here.

## Decision
An ~80-line runner in `internal/database/migrate.go`: migrations embedded
into the binary with go:embed, a `schema_migrations` version table, a
Postgres advisory lock so concurrent startups (api + worker, or replicas)
serialize, and one transaction per migration file. Both binaries migrate at
startup.

## Consequences
+ Zero new dependencies; the entire mechanism is readable on one screen and
  is itself covered by the integration test.
+ Startup-time migration removes the "run migrations first" deployment step
  (relevant for Phase 5 Kubernetes rollouts).
- Forward-only: no down migrations. Accepted — rolling forward is the usual
  production stance, and dev databases are disposable.
- No checksums or out-of-order detection; if migration needs ever grow past
  this, swapping goose in behind the same startup call is a contained change.
