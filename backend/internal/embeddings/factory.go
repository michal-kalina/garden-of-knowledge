package embeddings

import "log/slog"

// FromProvider builds the Embedder selected by configuration. Shared by the
// api (query embedding) and the worker (document embedding) so both binaries
// pick the same provider from the same env.
func FromProvider(provider, voyageKey, voyageModel string, logger *slog.Logger) Embedder {
	switch provider {
	case "voyage":
		logger.Info("embeddings provider: voyage", "model", voyageModel)
		return NewVoyage(voyageKey, voyageModel)
	default:
		logger.Warn("embeddings provider: FAKE — vectors are deterministic noise; " +
			"set VOYAGE_API_KEY for real retrieval quality")
		return Fake{}
	}
}
