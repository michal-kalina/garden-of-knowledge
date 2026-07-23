// Package telemetry wires distributed tracing via OpenTelemetry's OTLP/HTTP
// exporter — deliberately generic, not tied to any specific backend. Point
// OTEL_EXPORTER_OTLP_ENDPOINT at a local Jaeger, Honeycomb, Langfuse's OTLP
// ingestion endpoint, or anything else OTLP-compatible; the instrumentation
// code (internal/chat, internal/retrieval, internal/llm, internal/worker)
// never imports or knows about any of them. Same philosophy as the
// Retriever/Streamer/Embedder interfaces elsewhere in this codebase: depend
// on a protocol, not a vendor.
//
// If OTEL_EXPORTER_OTLP_ENDPOINT is unset, Setup does nothing and leaves
// OpenTelemetry's default no-op TracerProvider in place — every Start/End
// call elsewhere in the codebase becomes a cheap no-op, so instrumentation
// code never needs an "is tracing enabled" branch.
package telemetry

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Shutdown flushes any pending spans and releases the exporter. Always call
// it (deferred) regardless of whether tracing was actually configured —
// the no-op case returns a Shutdown that does nothing.
type Shutdown func(context.Context) error

// Setup configures global tracing from environment variables:
//
//	OTEL_EXPORTER_OTLP_ENDPOINT   host:port or full URL of the OTLP/HTTP
//	                              collector (e.g. "localhost:4318" for a
//	                              local Jaeger, or a hosted backend's OTLP
//	                              ingestion endpoint — check that backend's
//	                              current docs for the exact path/port, as
//	                              these can change between providers).
//	OTEL_EXPORTER_OTLP_INSECURE  "true" to use http:// instead of https://
//	                              (set this for a local collector).
//	OTEL_EXPORTER_OTLP_URL_PATH  override the ingestion path if a backend
//	                              doesn't use the OTLP default.
//	OTEL_EXPORTER_OTLP_HEADERS   comma-separated key=value pairs sent as
//	                              extra request headers (e.g. an
//	                              Authorization header for a backend that
//	                              requires auth).
//
// serviceName tags every span so a shared backend can separate the api and
// worker binaries' traces.
func Setup(ctx context.Context, serviceName string) (Shutdown, error) {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		return func(context.Context) error { return nil }, nil
	}

	opts := []otlptracehttp.Option{otlptracehttp.WithEndpoint(endpoint)}
	if insecure, _ := strconv.ParseBool(os.Getenv("OTEL_EXPORTER_OTLP_INSECURE")); insecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}
	if path := os.Getenv("OTEL_EXPORTER_OTLP_URL_PATH"); path != "" {
		opts = append(opts, otlptracehttp.WithURLPath(path))
	}
	if headers := os.Getenv("OTEL_EXPORTER_OTLP_HEADERS"); headers != "" {
		opts = append(opts, otlptracehttp.WithHeaders(parseHeaders(headers)))
	}

	exporter, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create OTLP exporter: %w", err)
	}

	res, err := resource.New(ctx, resource.WithAttributes(
		attribute.String("service.name", serviceName),
	))
	if err != nil {
		return nil, fmt.Errorf("build resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	return tp.Shutdown, nil
}

// parseHeaders parses the standard OTLP env var format: comma-separated
// key=value pairs (e.g. "Authorization=Basic xyz,X-Other=abc"). Values may
// themselves contain "=" (base64 padding does); only the first "=" splits
// each pair.
func parseHeaders(s string) map[string]string {
	headers := make(map[string]string)
	for _, pair := range strings.Split(s, ",") {
		key, value, ok := strings.Cut(pair, "=")
		if !ok {
			continue
		}
		headers[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return headers
}
