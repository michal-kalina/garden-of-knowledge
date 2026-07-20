package llm

import "fmt"

// FromProvider builds the Streamer selected by configuration. Adding a
// provider means one new file implementing Streamer plus a case here — the
// chat layer and HTTP handlers never change.
func FromProvider(provider, model, apiKey string) (Streamer, error) {
	switch provider {
	case "anthropic":
		return NewAnthropic(apiKey, model), nil
	case "openrouter":
		return NewOpenRouter(apiKey, model), nil
	default:
		return nil, fmt.Errorf("unknown LLM provider %q (supported: anthropic, openrouter)", provider)
	}
}
