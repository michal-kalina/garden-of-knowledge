package cost

import "testing"

func TestEmbeddingUSD(t *testing.T) {
	// 1,000,000 tokens at $0.06/M = $0.06 exactly.
	if got := EmbeddingUSD("voyage-4", 1_000_000); got != 0.06 {
		t.Errorf("EmbeddingUSD = %v, want 0.06", got)
	}
	if got := EmbeddingUSD("voyage-4", 500); got != 0.00003 {
		t.Errorf("EmbeddingUSD(500 tokens) = %v, want 0.00003", got)
	}
	t.Run("unknown model returns zero, not an error", func(t *testing.T) {
		if got := EmbeddingUSD("some-future-model", 1000); got != 0 {
			t.Errorf("got = %v, want 0", got)
		}
	})
}

func TestRerankUSD(t *testing.T) {
	// Voyage's own worked example: 100 documents, query+doc tokens summing
	// to 500 total (i.e. the formula's raw inputs, not a literal 500-token
	// query) — here using simple round numbers to verify the formula shape:
	// billed = queryTokens*numDocuments + totalDocumentTokens.
	got := RerankUSD("rerank-2.5", 10, 100, 5000) // billed = 10*100 + 5000 = 6000
	want := 6000.0 / 1_000_000 * 0.05
	if diff := got - want; diff > 1e-12 || diff < -1e-12 {
		t.Errorf("RerankUSD = %v, want %v", got, want)
	}

	t.Run("unknown model returns zero", func(t *testing.T) {
		if got := RerankUSD("unknown", 10, 100, 5000); got != 0 {
			t.Errorf("got = %v, want 0", got)
		}
	})
}

func TestLLMUSD(t *testing.T) {
	got := LLMUSD("claude-sonnet-4-6", 1_000_000, 1_000_000)
	if want := 3.00 + 15.00; got != want {
		t.Errorf("LLMUSD = %v, want %v", got, want)
	}

	t.Run("output is priced independently from input", func(t *testing.T) {
		inputOnly := LLMUSD("claude-sonnet-4-6", 1_000_000, 0)
		outputOnly := LLMUSD("claude-sonnet-4-6", 0, 1_000_000)
		if inputOnly != 3.00 || outputOnly != 15.00 {
			t.Errorf("inputOnly=%v outputOnly=%v, want 3.00/15.00", inputOnly, outputOnly)
		}
	})

	t.Run("unknown model returns zero", func(t *testing.T) {
		if got := LLMUSD("gpt-unknown", 1000, 1000); got != 0 {
			t.Errorf("got = %v, want 0", got)
		}
	})
}
