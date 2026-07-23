// Package cost estimates USD spend per operation from token counts we
// already compute ourselves (chunking.ApproxTokens), not from providers'
// billed usage figures. This is a deliberate, documented approximation:
// wiring real usage would mean parsing Anthropic's message_start/
// message_delta usage fields and adding stream_options.include_usage to
// OpenRouter's request, extending internal/llm's Streamer interface to
// return token counts — a reasonable follow-up, tracked but not done here.
// What this package gives you today is directionally useful cost visibility
// per query in traces, not a billing-accurate ledger.
//
// Prices are USD per million tokens, sourced from each provider's public
// pricing as of 2026-07 (Voyage: docs.voyageai.com/docs/pricing; Anthropic:
// anthropic's API pricing page). They drift — treat every number here as
// "approximately right at the time this was written," not current truth;
// update from the providers' own pages before relying on this for a real
// budget decision.
package cost

// USDPerMillion holds input/output prices. Embeddings and reranking have no
// meaningful "output" (Voyage bills embeddings on input tokens only, and
// rerank on a combined query×documents formula — see RerankUSD), so Output
// is 0 for those and unused.
type USDPerMillion struct {
	Input  float64
	Output float64
}

// Embedding model prices (input tokens only).
var embeddingPrices = map[string]USDPerMillion{
	"voyage-4":       {Input: 0.06},
	"voyage-4-lite":  {Input: 0.02},
	"voyage-4-large": {Input: 0.12},
}

// Rerank model prices (see RerankUSD for the token-counting formula Voyage
// actually bills on).
var rerankPrices = map[string]USDPerMillion{
	"rerank-2.5":      {Input: 0.05},
	"rerank-2.5-lite": {Input: 0.02},
}

// LLM chat model prices (input and output priced separately; output is
// typically 5x input across current-generation models).
var llmPrices = map[string]USDPerMillion{
	"claude-sonnet-4-6": {Input: 3.00, Output: 15.00},
	"claude-haiku-4-5":  {Input: 1.00, Output: 5.00},
	"claude-opus-4-8":   {Input: 5.00, Output: 25.00},
}

// EmbeddingUSD estimates the cost of embedding approxTokens input tokens.
// Returns 0 for an unrecognized model rather than an error — cost
// estimation is observability, not a billing gate; a missing price entry
// should degrade to "no number" in a trace, not break the request.
func EmbeddingUSD(model string, approxTokens int) float64 {
	p, ok := embeddingPrices[model]
	if !ok {
		return 0
	}
	return float64(approxTokens) / 1_000_000 * p.Input
}

// RerankUSD estimates Voyage's rerank cost, which is billed on
// (queryTokens × numDocuments) + sum(all document tokens) — not a simple
// per-input-token rate the way embeddings are. See docs.voyageai.com/docs/pricing.
func RerankUSD(model string, queryTokens, numDocuments, totalDocumentTokens int) float64 {
	p, ok := rerankPrices[model]
	if !ok {
		return 0
	}
	billedTokens := queryTokens*numDocuments + totalDocumentTokens
	return float64(billedTokens) / 1_000_000 * p.Input
}

// LLMUSD estimates chat generation cost from approximate input and output
// token counts.
func LLMUSD(model string, approxInputTokens, approxOutputTokens int) float64 {
	p, ok := llmPrices[model]
	if !ok {
		return 0
	}
	return float64(approxInputTokens)/1_000_000*p.Input +
		float64(approxOutputTokens)/1_000_000*p.Output
}
