# Honeycomb

Honeycomb accepts OTLP/HTTP directly — no collector required, just
environment variables. Good option if you want traces to survive a restart
without running your own Jaeger, and Honeycomb's free tier is generous
enough for a portfolio project.

## 1. Get an API key

Sign up at <https://www.honeycomb.io> (or use an existing team), then create
an **Ingest API key** under your team's environment settings. Ingest keys
are separate from management keys — make sure you copy the ingest one.

## 2. Configuration

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=api.honeycomb.io:443
OTEL_EXPORTER_OTLP_HEADERS=x-honeycomb-team=YOUR_INGEST_API_KEY
```

EU-region Honeycomb accounts use a different host:

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=api.eu1.honeycomb.io:443
```

Leave `OTEL_EXPORTER_OTLP_INSECURE` unset (Honeycomb requires TLS — the
exporter defaults to HTTPS, which is what you want) and leave
`OTEL_EXPORTER_OTLP_URL_PATH` unset — Honeycomb's traces endpoint is the
OTLP-standard `/v1/traces`, this project's default.

**Why `OTEL_EXPORTER_OTLP_HEADERS` isn't a variable this project's own code
reads:** it's a standard OpenTelemetry environment variable that the
`otlptracehttp` exporter reads on its own during setup, independent of the
endpoint/insecure/path options this project sets explicitly in
[`internal/telemetry`](../../backend/internal/telemetry/telemetry.go). Verify
this against the current `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp`
docs if traces aren't showing up — exact env-var precedence has changed
between SDK versions before.

## 3. Dataset routing

Honeycomb routes incoming spans to a dataset based on the span's
`service.name` resource attribute when no `x-honeycomb-dataset` header is
set. This project already sets `service.name` to `gok-api` and `gok-worker`
(see `telemetry.Setup`), so traces from each binary land in their own
dataset automatically — no extra header needed unless you'd rather force
both into one:

```bash
OTEL_EXPORTER_OTLP_HEADERS=x-honeycomb-team=YOUR_KEY,x-honeycomb-dataset=garden-of-knowledge
```

(Multiple headers are comma-separated `key=value` pairs in one string.)

## 4. Run and verify

```bash
docker compose up --build
```

Ask the chat something, then check Honeycomb's UI — under the `gok-api`
dataset you should see `chat.ask` traces with the same `gok.*` attributes
described in [`docs/adr/0008-observability.md`](../adr/0008-observability.md)
(source count, estimated cost, model name). Honeycomb's query builder can
group by `gok.model` or chart `gok.cost_usd_estimate` over time once you
have a few queries logged — useful for a "what did this demo cost me"
screenshot.
