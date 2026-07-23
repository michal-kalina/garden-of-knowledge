package retrieval

import (
	"context"
	"fmt"

	"github.com/michal-kalina/garden-of-knowledge/backend/internal/rerank"
)

// Reranked wraps any Retriever (in practice, hybrid Searcher) with a
// cross-encoder pass. It satisfies Retriever itself, so nothing downstream
// — chat, the eval harness, the /search handler — has to know reranking
// exists; they call Search exactly as before.
//
// Why bother, concretely: the Phase 4 Step 1 baseline (docs/eval/results)
// showed Recall@10 = 1.0 but Recall@1 = 0.556 — the right *document* is
// always found, but the right *section* of it often isn't first. Embedding
// similarity is a cheap, global signal; a cross-encoder that jointly reads
// the query and each candidate is slower but exactly suited to that
// finer-grained call. See docs/adr/0007-reranking.md.
type Reranked struct {
	Base     Retriever
	Reranker rerank.Reranker
	// FetchFactor over-fetches from Base before reranking narrows back down
	// to the caller's limit — a reranker can only reorder what's already in
	// the candidate set, so that set has to be wider than the final answer.
	// Default 4 (e.g. limit=6 fetches 24 candidates to rerank).
	FetchFactor int
}

// compile-time check that Reranked implements Retriever
var _ Retriever = (*Reranked)(nil)

func (r *Reranked) fetchFactor() int {
	if r.FetchFactor > 0 {
		return r.FetchFactor
	}
	return 4
}

func (r *Reranked) Search(ctx context.Context, userID, query string, limit int) ([]Result, error) {
	if limit <= 0 {
		limit = DefaultLimit
	}

	fetch := limit * r.fetchFactor()
	if fetch > MaxLimit {
		fetch = MaxLimit
	}

	candidates, err := r.Base.Search(ctx, userID, query, fetch)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return candidates, nil
	}

	docs := make([]string, len(candidates))
	for i, c := range candidates {
		docs[i] = c.Content
	}

	topK := limit
	if topK > len(candidates) {
		topK = len(candidates)
	}
	scored, err := r.Reranker.Rerank(ctx, query, docs, topK)
	if err != nil {
		return nil, fmt.Errorf("rerank: %w", err)
	}

	out := make([]Result, 0, len(scored))
	for _, s := range scored {
		if s.Index < 0 || s.Index >= len(candidates) {
			return nil, fmt.Errorf("reranker returned out-of-range index %d for %d candidates",
				s.Index, len(candidates))
		}
		res := candidates[s.Index]
		score := s.Score
		res.RerankScore = &score
		out = append(out, res)
	}
	return out, nil
}
