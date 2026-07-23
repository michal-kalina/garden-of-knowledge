// Package embeddings turns text into vectors. The Embedder interface hides
// the provider; the worker and (later) the query path depend only on it.
//
// Two implementations:
//   - Voyage: production, calls the Voyage AI API (voyage-3, 1024 dims —
//     matching the vector(1024) column from migration 0001).
//   - Fake: deterministic pseudo-embeddings for development without an API
//     key and for tests. Retrieval quality with Fake is meaningless by
//     design; it exists so the whole pipeline runs offline.
package embeddings

import (
	"context"
	"hash/fnv"
	"math"
)

// Dim is the embedding dimensionality, fixed by the chunks.embedding column.
const Dim = 1024

// InputType tells the provider how the text will be used. Voyage (and
// others) train separate projections for documents and queries; using the
// right one measurably improves retrieval.
type InputType string

const (
	InputDocument InputType = "document"
	InputQuery    InputType = "query"
)

// Embedder converts a batch of texts into vectors, preserving order.
type Embedder interface {
	Embed(ctx context.Context, texts []string, input InputType) ([][]float32, error)
	// Model names the model in use, for cost estimation and trace
	// attributes (internal/cost, internal/telemetry) — not used for
	// behavior.
	ModelName() string
}

// Fake produces deterministic unit-length vectors derived from a hash of the
// text. Same text, same vector — enough for exercising storage and the
// pipeline end-to-end without network access.
type Fake struct{}

// compile-time check that Fake implements Embedder
var _ Embedder = Fake{}

func (Fake) ModelName() string { return "fake" }

func (Fake) Embed(_ context.Context, texts []string, _ InputType) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i] = fakeVector(t)
	}
	return out, nil
}

func fakeVector(text string) []float32 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(text))
	state := h.Sum64()

	vec := make([]float32, Dim)
	var norm float64
	for i := range vec {
		// Simple LCG over the hash state; quality is irrelevant here,
		// determinism is the point.
		state = state*6364136223846793005 + 1442695040888963407
		// Map the top bits into (-1, 1).
		v := float64(int64(state>>11))/float64(1<<52) - 1
		vec[i] = float32(v)
		norm += v * v
	}
	// Normalize to unit length so cosine distance behaves sanely.
	n := float32(math.Sqrt(norm))
	if n > 0 {
		for i := range vec {
			vec[i] /= n
		}
	}
	return vec
}
