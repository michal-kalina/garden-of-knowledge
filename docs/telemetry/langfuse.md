# Langfuse

Langfuse is purpose-built for LLM observability — prompt/completion
content, token usage, and cost show up as first-class fields, not just
generic span attributes. It accepts plain OTLP/HTTP, so no Langfuse SDK is
needed in this project's code; the existing OpenTelemetry instrumentation
is enough.

## 1. Get credentials

Sign up at <https://cloud.langfuse.com> (or use a self-hosted instance),
create a project, and copy its **public key** and **secret key** from the
project settings.

## 2. Build the auth header

Langfuse authenticates OTLP traffic with HTTP Basic auth: the header value
is `Basic <base64(public_key:secret_key)>`.

```bash
echo -n "pk-lf-xxxxxxxx:sk-lf-xxxxxxxx" | base64
```

## 3. Configuration

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=cloud.langfuse.com:443
OTEL_EXPORTER_OTLP_URL_PATH=/api/public/otel/v1/traces
OTEL_EXPORTER_OTLP_HEADERS=Authorization=Basic <the base64 string from step 2>,x-langfuse-ingestion-version=4
```

Self-hosted Langfuse: replace the endpoint with your own host, same path.

Three things worth being precise about, since Langfuse's ingestion path is
*not* the OTLP-standard one this project defaults to:

- **`OTEL_EXPORTER_OTLP_URL_PATH` is required here** — Langfuse ingests
  traces at `/api/public/otel/v1/traces`.
- **No scheme in `OTEL_EXPORTER_OTLP_ENDPOINT`** — same rule as the other
  guides in this folder: this project passes it straight to
  `otlptracehttp.WithEndpoint()`, which wants `host:port`. Leave
  `OTEL_EXPORTER_OTLP_INSECURE` unset — Langfuse Cloud requires HTTPS.
- **Langfuse only supports OTLP/HTTP** (JSON or protobuf), not gRPC — this
  project's exporter is already HTTP-only, so nothing to change there.

## 4. Run and verify

```bash
make up
```

Ask the chat something, then open your Langfuse project's **Traces** view.
Langfuse maps a known set of span attributes into its own data model
(trace input/output, token usage, cost); this project's `gok.*` attributes
(`gok.model`, `gok.cost_usd_estimate`, `gok.source_count`, etc. — see
[`docs/adr/0008-observability.md`](../adr/0008-observability.md)) ride
along as regular attributes even where they aren't part of that mapping, so
they're still visible on the span detail view even if they don't populate
Langfuse's dedicated cost/token columns.

## Known limitation

Langfuse's OTLP endpoint currently only maps traces, not the separate
"scores" concept from their SDK (e.g. attaching a quality rating to a
generation) — check Langfuse's current docs if you plan to add human or
automated feedback scores later, since that may still require their SDK's
dedicated ingestion path rather than pure OTLP.

## Documentation
(OpenTelemetry setup)[https://langfuse.com/integrations/native/opentelemetry]