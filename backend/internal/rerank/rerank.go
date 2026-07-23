// Package rerank scores (query, document) pairs with a cross-encoder —
// a model that jointly processes both texts, unlike the embedding models in
// internal/embeddings which encode them separately. Cross-encoders are
// slower (one inference per candidate, not a single vector lookup) but
// measurably better at judging fine-grained relevance, which is exactly the
// gap the Phase 4 Step 1 baseline surfaced: retrieval finds the right
// *document* reliably (Recall@10 = 1.0) but not always the right *section*
// of it first (Recall@1 = 0.556). See docs/adr/0007-reranking.md.
package rerank

import "context"

// Result is one document's rerank score, keyed by its position in the
// slice passed to Rerank — the caller maps Index back to its own richer
// type (retrieval.Result carries content the reranker never needs to see
// twice).
type Result struct {
	Index int
	Score float64
}

// Reranker scores documents against a query, returning results sorted by
// descending relevance. topK <= 0 means "return all of them scored."
type Reranker interface {
	Rerank(ctx context.Context, query string, documents []string, topK int) ([]Result, error)
	// ModelName names the model in use, for cost estimation and trace
	// attributes — not used for behavior.
	ModelName() string
}

// Fake preserves input order, assigning strictly decreasing scores. It
// exists so retrieval.Reranked can be unit-tested without network access —
// note that this makes Fake a no-op reranker (same order in, same order
// out), which is the right behavior for testing the decorator's plumbing,
// not for testing rerank quality.
type Fake struct{}

// compile-time check that Fake implements Rerankervar _ Reranker = Fake{}

func (Fake) ModelName() string { return "fake" }

func (Fake) Rerank(_ context.Context, _ string, documents []string, topK int) ([]Result, error) {
	out := make([]Result, len(documents))
	for i := range documents {
		out[i] = Result{Index: i, Score: float64(len(documents) - i)}
	}
	if topK > 0 && topK < len(out) {
		out = out[:topK]
	}
	return out, nil
}
