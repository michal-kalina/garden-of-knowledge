# Jaeger (local, for demos and development)

Jaeger's all-in-one image ships a UI, storage, and an OTLP/HTTP receiver in
one container — the fastest way to actually *see* the spans this project
emits (`retrieval.search`, `chat.ask`, `worker.process_document` and its
sub-spans) without signing up for anything.

## 1. Run it

```bash
docker run -d --name jaeger \
  -p 16686:16686 \
  -p 4318:4318 \
  jaegertracing/all-in-one:latest
```

- `16686` — the web UI
- `4318` — OTLP/HTTP receiver (what this project's exporter talks to)

Confirm it's up: open <http://localhost:16686> — you should see the Jaeger
UI with an empty trace list.

## 2. Point the app at it

Add to `.env` (or export in your shell):

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4318
OTEL_EXPORTER_OTLP_INSECURE=true
OTEL_EXPORTER_OTLP_URL_PATH=
```

Notes on these two specifically, since they're easy to get wrong:

- **No scheme, no path** in `OTEL_EXPORTER_OTLP_ENDPOINT` — this project
  passes it straight to `otlptracehttp.WithEndpoint()`, which expects
  `host:port`, not a URL. `http://localhost:4318` here is wrong; it'll fail
  silently or error depending on the SDK version. Leave `OTEL_EXPORTER_OTLP_URL_PATH`
  unset — Jaeger uses the OTLP-standard `/v1/traces` path, which is this
  project's default.
- **`OTEL_EXPORTER_OTLP_INSECURE=true`** is required for a local, unencrypted
  container — without it the exporter tries HTTPS and fails to connect.

## 3. Run the app and generate a trace

```bash
docker compose up --build
```

Upload a document and ask it something through the chat UI, then reload
<http://localhost:16686>, pick **Service: gok-api** (or `gok-worker` for
ingestion traces), and click **Find Traces**.

## What you'll see

- A `chat.ask` span containing a `retrieval.search` child span, with
  attributes like `gok.source_count`, `gok.model`, and
  `gok.cost_usd_estimate` (see [`docs/adr/0008-observability.md`](../adr/0008-observability.md)
  for why that cost figure is an estimate, not a bill).
- On the worker side, `worker.process_document` broken into
  `worker.fetch` → `worker.parse` → `worker.chunk` → `worker.embed` →
  `worker.store` — exactly where ingestion time goes for a given document.

## Stopping it

```bash
docker stop jaeger && docker rm jaeger
```

Jaeger all-in-one keeps everything in memory by default — traces disappear
when the container stops. Fine for a demo; for anything persistent, use
Jaeger's production deployment (separate collector + storage backend) per
Jaeger's own docs, which is out of scope here.
