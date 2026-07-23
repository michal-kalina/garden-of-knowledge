# Telemetry backends

This project exports traces over plain OTLP/HTTP (see
[`docs/adr/0008-observability.md`](../adr/0008-observability.md)) — no
vendor SDK, no code changes to switch backends. Pick one and follow its
guide:

- [Jaeger](jaeger.md) — local, zero-signup, fastest way to see a trace today.
- [Honeycomb](honeycomb.md) — hosted, generous free tier, no collector needed.
- [Langfuse](langfuse.md) — hosted, LLM-specific views (prompts, tokens, cost).

All three (and anything else OTLP-compatible) are configured through the
same three environment variables:

| Variable | Meaning |
|---|---|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `host:port`, **no scheme, no path** |
| `OTEL_EXPORTER_OTLP_INSECURE` | `true` for plain HTTP (local collectors only) |
| `OTEL_EXPORTER_OTLP_URL_PATH` | override the ingestion path if it isn't `/v1/traces` |

Leaving `OTEL_EXPORTER_OTLP_ENDPOINT` empty disables tracing entirely — a
genuine no-op, not a disabled code path (see the ADR for why that
distinction was worth making).
