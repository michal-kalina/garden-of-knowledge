# ADR-0006: Golden-set retrieval eval instead of ad-hoc manual queries

**Status:** accepted · **Date:** 2026-07

## Context
Retrieval quality had only been judged by typing a handful of questions and
eyeballing the results (see the manual `/search` and `/chat` debugging in
earlier sessions). That doesn't scale, isn't repeatable, and gives no way to
tell whether a future change (reranking, a different embedding model, a
different tsvector config) actually helped.

## Decision
A golden dataset (`docs/eval/golden.json`) of judged (query, relevant-chunk)
pairs, scored by a small pure-logic package (`internal/eval`, recall@k and
MRR) driven by a CLI harness (`cmd/eval`) that runs the *real* hybrid
retriever against a *real* database. The corpus is the project's own
README and ADRs — reproducible by anyone who clones the repo, no private
data required. Judgments key on (filename, heading), not chunk id, so the
eval survives re-chunking and re-ingestion.

## Consequences
+ Retrieval changes are now measurable: "recall@5 went from 0.61 to 0.78"
  is a claim you can make in a portfolio README, not a feeling.
+ The harness already surfaced a real, previously-undocumented
  characteristic of the lexical retriever (see `docs/eval/README.md`,
  "Known findings") — the kind of thing a manual smoke test would never
  catch because it only shows up in aggregate across many queries.
+ 18 judged cases is enough to catch regressions, not enough to be
  statistically rigorous; growing the set over time is expected, and the
  format was chosen specifically so growing it is a JSON edit, not a
  schema migration.
- Judging (filename, heading) as the relevance unit is coarser than judging
  individual chunk text — a chunk under the right heading that doesn't
  actually answer the question would count as a false "hit." Acceptable at
  this corpus size, where headings map cleanly to a single topic each.
