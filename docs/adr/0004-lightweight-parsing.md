# ADR-0004: Lightweight parsing first, docling as the upgrade path

**Status:** accepted · **Date:** 2026-07

## Context
PDF parsing quality spans a huge range: from "extract the text layer" to
full layout analysis with OCR and table structure recovery (docling,
unstructured). Docling ships ML models and a torch dependency — gigabytes in
the Docker image and minutes of model downloads on first run, which hurts
the fast local iteration this project optimizes for.

## Decision
Parse Markdown natively and PDFs with PyMuPDF, detecting headings by font
size relative to the character-weighted median body size. Keep the block
contract (`heading | paragraph | table`) provider-agnostic so docling can
replace the implementation later without touching the worker or chunker.

## Consequences
+ Parser image builds in seconds; no model downloads; deterministic output
  that is easy to unit-test (fixtures generated programmatically).
+ Heading detection works on digitally-born PDFs, which covers the typical
  knowledge-base corpus.
- Scanned documents (no text layer) and complex PDF tables are not handled;
  both are explicitly docling territory and tracked as a future phase.
