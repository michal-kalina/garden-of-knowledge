# ADR-0007: Voyage cross-encoder rerank pass, opt-in

**Status:** accepted · **Date:** 2026-07

## Context
The Phase 4 Step 1 baseline (`docs/eval/results/latest.md`, no reranking)
measured MRR 0.833, Recall@1 0.556, Recall@10 1.0. The gap between Recall@1
and Recall@10 is the finding: the right *document* is always found
somewhere in the candidates, but the right *section* of it frequently isn't
ranked first — the recurring pattern being a "Decision" chunk outranking
the specifically relevant "Consequences" chunk for questions like "what are
the downsides of X." Embedding similarity is a cheap, global signal;
distinguishing "on-topic" from "specifically answers this" within the same
document is precisely the finer-grained comparison a cross-encoder is built
for, at the cost of one model call per candidate instead of a vector lookup.

## Decision
A `Reranker` interface (`internal/rerank`) with a Voyage implementation
(`rerank-2.5`, same API key as embeddings — one provider for both stages)
and a `Fake` for tests. Wired in as a decorator, `retrieval.Reranked`,
implementing the same `Retriever` interface as the plain hybrid `Searcher` —
chat, the eval harness, and the `/search` handler never know it exists.
Disabled by default (`RERANK_PROVIDER` empty); opt-in via config, not
inferred from `VOYAGE_API_KEY` being present, because reranking adds a
network round-trip per query on top of embeddings and is a cost/latency
trade the operator should choose explicitly.

## Consequences
+ Zero changes to `chat`, `/search`, or the eval harness's own logic —
  Go's structural typing means substituting `Reranked` for `Searcher` behind
  an interface variable is the entire integration.
+ `cmd/eval -rerank` runs the identical golden set through the identical
  production code path (`buildSearcher` in `cmd/eval` mirrors the wiring in
  `cmd/api`), so a before/after comparison measures the real system, not a
  simulated approximation of it.
+ Extra latency (one rerank call per chat turn) and cost (Voyage bills
  reranking separately from embeddings) — acceptable given it's opt-in and
  the candidate window is small (`FetchFactor` × `contextLimit`, currently
  ≤ 24 short chunks per query).

## Result (real run, `rerank-2.5`, 2026-07-22)

| Metric | Baseline | Reranked | Delta |
|---|---|---|---|
| MRR | 0.833 | 0.866 | +0.033 |
| Recall@1 | 0.556 | 0.639 | +0.083 |
| Recall@3 | 0.944 | 0.806 | **−0.138** |
| Recall@5 | 0.944 | 0.944 | 0.000 |
| Recall@10 | 1.000 | 1.000 | 0.000 |

Mixed, not a clean win. MRR/Recall@1 improved as predicted, but Recall@3
regressed — traced to a repeatable pattern (`docs/eval/results/latest-
reranked.md`): short, cross-cutting README sections ("Design decisions",
"Architecture") score highly against many unrelated queries and displace
the second judged-relevant chunk out of the top 3 — summary-chunk
cannibalization. Recall@5, the metric closest to chat's actual
`contextLimit = 6`, is **unchanged** either way. Conclusion: `RERANK_PROVIDER`
stays off by default for this corpus — the mechanism is correct and
available, but the data doesn't support enabling it here. See
`docs/eval/README.md` for the full write-up.

- The before/after numbers in this ADR's own baseline reference are only as
  good as the golden set's 18 cases; a larger set would tighten the
  confidence in any measured delta.
