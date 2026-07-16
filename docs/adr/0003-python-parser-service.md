# ADR-0003: Separate Python service for document parsing

**Status:** accepted · **Date:** 2026-07

## Context
The core system is Go. Parsing PDFs (layout, tables, OCR) is the one area where
the Python ecosystem (docling, unstructured, tesseract bindings) is clearly
ahead of anything available in Go.

## Decision
Keep Go as the system language; isolate parsing in a small Python FastAPI
service with a narrow, typed contract (`POST /parse` → list of content blocks).

## Consequences
+ Each language is used where it is strongest.
+ The parser is stateless and independently scalable (parsing is the CPU-heavy step).
+ The contract boundary makes the parser replaceable (e.g. a managed parsing API).
- One more service to build, deploy and monitor — accepted, since it also
  demonstrates cross-service design in the portfolio.
