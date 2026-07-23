# ADR-0008: OpenTelemetry (OTLP) tracing + estimated cost, not a vendor SDK

**Status:** accepted · **Date:** 2026-07

## Context
Up to this point, understanding a slow or expensive query meant reading
structured logs and guessing. The roadmap asked for "OpenTelemetry traces +
Langfuse; cost per query" — two things that turn out to compose better as
one generic mechanism than as a direct Langfuse integration.

## Decision
Instrument the request paths that matter (`retrieval.Search`, `chat.Ask`,
the worker's per-document pipeline) with OpenTelemetry spans, exported via
the standard OTLP/HTTP exporter to whatever `OTEL_EXPORTER_OTLP_ENDPOINT`
points at — a local Jaeger for demos, Langfuse's OTLP ingestion endpoint,
Honeycomb, or nothing at all (tracing is then a genuine no-op, not a
disabled code path with branches everywhere). This is the same interface-
over-vendor pattern as `Retriever`, `Streamer`, and `Embedder` elsewhere:
depend on a protocol, not a specific backend's SDK.

Cost is estimated from token counts this project already computes
(`chunking.ApproxTokens`), not from providers' billed usage — attached as
span attributes (`gok.cost_usd_estimate`) and computed by a small,
versioned pricing table (`internal/cost`, prices as of 2026-07). Wiring
real billed usage would mean extending `llm.Streamer` to surface
Anthropic's `usage` fields from `message_start`/`message_delta` events and
adding `stream_options.include_usage` to OpenRouter's request — a
reasonable follow-up, not done here.

## Consequences
+ Zero new dependency on any specific observability vendor; swapping
  Jaeger for Langfuse for Honeycomb is an environment variable, not a code
  change or a redeploy of different instrumentation.
+ Tracing has genuinely zero behavioral branches: `OTEL_EXPORTER_OTLP_ENDPOINT`
  unset leaves OpenTelemetry's own no-op `TracerProvider` in place, so every
  `Start`/`End` call in the instrumented code paths is a cheap no-op rather
  than something the code has to check for.
+ Spans exist at the boundaries that actually explain "why was this slow or
  expensive": embedding a query, each retriever, the LLM call, and each
  stage of document ingestion (fetch/parse/chunk/embed/store) — someone
  debugging a slow chat turn can see exactly which stage cost the time.
- Cost figures are estimates, not a billing-accurate ledger — real provider
  usage would be tighter and is a known, tracked gap.
- No auto-instrumentation library (e.g. `otelhttp`) was added; spans are
  placed explicitly at meaningful boundaries instead, consistent with this
  project's general preference for a handful of clear span points over a
  library that auto-wraps every HTTP handler and DB call, plus it's a
  dependency this sandbox couldn't verify — see the PR/commit notes on the
  network limitation encountered building this feature.
