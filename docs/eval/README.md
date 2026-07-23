# Retrieval evaluation

Measures hybrid-search quality (recall@k, MRR) against a hand-judged golden
set, instead of eyeballing a handful of manual queries.

## Running it

1. Ingest the eval corpus — `README.md` and every file in `docs/adr/` — via
   the normal upload flow, logged in as a dedicated account (e.g.
   `eval@yourdomain.test`). Using the project's own docs as the corpus keeps
   the eval reproducible without exposing anyone's private documents.
2. Make sure `VOYAGE_API_KEY` is set — running with the fake embedder
   produces a report, but the numbers measure nothing (see "Known findings").
3. `make eval-retrieval EVAL_USER=eval@yourdomain.test`

This writes `docs/eval/results/latest.md` and prints the same report to
stdout: an aggregate table (MRR, Recall@1/3/5/10) followed by a per-case
breakdown showing exactly which sources were retrieved and at what rank —
the detail that turns "recall@5 = 0.83" into "query X is missing source Y."

## Golden set format

`golden.json` is a list of `{id, query, relevant}` cases. `relevant` entries
are judged by **(filename, heading)**, not chunk id — ids are ingestion-order
dependent and would break on every re-ingestion; heading text is stable as
long as the source document's headings don't change. See
`internal/eval/eval.go` for the scoring implementation and
`internal/eval/eval_test.go` for it exercised against hand-built fixtures
(fast, no database required).

## Metrics

- **Recall@k** — fraction of a query's judged-relevant chunks appearing
  anywhere in the top k. Several queries here have two relevant chunks (a
  topic often lives in both the README and its ADR); recall@k rewards
  surfacing all of them, not just one.
- **MRR** (mean reciprocal rank) — punishes burying the right answer on
  page two, which matters because chat only feeds the top 6 chunks
  (`contextLimit` in `internal/chat`) to the model — a relevant chunk
  retrieved at rank 8 might as well not exist.

Answer-quality evaluation (LLM-as-judge) is tracked for a later step; this
one covers retrieval, now including an optional reranking pass.

## Reranking: before/after

A Voyage cross-encoder (`rerank-2.5`) can rerank hybrid search's candidates
before the top results are returned — see [ADR-0007](../adr/0007-reranking.md)
for why (short version: Recall@1 was the weak spot in the baseline below,
and that's exactly the failure mode a cross-encoder targets). It's off by
default; turning it on and comparing against the baseline is a two-command
before/after:

```bash
make eval-retrieval EVAL_USER=you@example.com            # baseline (already below)
make eval-retrieval-reranked EVAL_USER=you@example.com    # same corpus, reranked
```

Both write to `docs/eval/results/` (`latest.md` / `latest-reranked.md`); diff
them, or just compare the summary tables. Requires `VOYAGE_API_KEY`
regardless of which embeddings provider is configured — reranking is a
separate API call, billed separately from embeddings.

### Result: real, checked-in run (`rerank-2.5`, 2026-07-22)

| Metric | Baseline | Reranked | Delta |
|---|---|---|---|
| MRR | 0.833 | 0.866 | **+0.033** |
| Recall@1 | 0.556 | 0.639 | **+0.083** |
| Recall@3 | 0.944 | 0.806 | **−0.138** |
| Recall@5 | 0.944 | 0.944 | 0.000 |
| Recall@10 | 1.000 | 1.000 | 0.000 |

**This is not a clean win, and that's the finding worth keeping.** MRR and
Recall@1 improved as ADR-0007 predicted — the cross-encoder is genuinely
better at picking the single best chunk. But Recall@3 regressed by 14
points, driven by a repeatable pattern in the per-case detail
(`results/latest-reranked.md`): `README.md#Design decisions` and
`README.md#Architecture` — short, cross-cutting sections that each touch
several ADRs in passing — score highly against *many unrelated queries*
(`vectorstore-choice`, `no-message-broker`, `job-queue-tradeoffs`,
`parser-service-boundary` all pull one of them into the top 2). They read
as "topically relevant" to a cross-encoder trained on general semantic
match, and end up displacing the second judged-relevant chunk out of the
top 3 — call it **summary-chunk cannibalization**.

The number that actually matters for production: chat feeds the top
`contextLimit = 6` chunks to the model (`internal/chat`), which Recall@5
approximates — and **Recall@5 is unchanged, 0.944 both ways.** For this
corpus and query style, reranking costs an extra network call and money per
chat turn for a metric that, at the depth the model actually sees, doesn't
move. `RERANK_PROVIDER` stays off by default; the code stays available and
correctly wired for a corpus where it might pay off differently (denser
documents without a "table of contents"-style chunk, or a larger
`contextLimit`). Re-run both eval targets after any retrieval or chunking
change and compare — this table is the reference point.

## Baseline

The first real run (`voyage-4`, no reranking) is checked into
[`results/latest.md`](results/latest.md): **MRR 0.833, Recall@1 0.556,
Recall@3/@5 0.944, Recall@10 1.000.** Every judged source is reachable
within the top 10 — retrieval never truly *misses*; the weak spot is
Recall@1, where two-thirds of cases place the top-judged source at rank 2
instead of 1. Per-case detail shows a repeating pattern: for a query like
"what are the downsides of X", the topically-adjacent "Decision" chunk of
the same ADR often outranks the specifically-relevant "Consequences" chunk
(e.g. `job-queue-tradeoffs`, RR 0.333) — the embedding correctly finds the
right *document*, just not always the right *section* of it first. That is
exactly the failure mode a cross-encoder reranker is good at fixing, which
makes it a well-motivated next experiment rather than a reflexive addition.
Re-run after any retrieval change and diff against this file.

## Known findings

Running the harness surfaced a real characteristic of the lexical retriever,
not a golden-set quirk: `websearch_to_tsquery('simple', question)` ANDs
together every word of a full-sentence question, stopwords included ("why",
"does", "this"...). Real prose almost never contains all of those words
verbatim, so **the lexical side contributes close to nothing for natural
questions** — it earns its keep specifically for queries containing an exact
rare term (confirmed separately by the "zorbafex" case in
`internal/retrieval/integration_test.go`). This isn't a bug to silently
patch; it's evidence for why the vector side is load-bearing, not a nice-to-
have, for this corpus and query style — worth citing alongside ADR-0001.
Whether swapping in `plainto_tsquery` (OR-ish, no stopword sensitivity) or a
language-specific config measurably improves Recall@k is a good next
before/after experiment.
