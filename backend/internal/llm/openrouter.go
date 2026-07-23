package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// OpenRouter streams chat completions through openrouter.ai, which fronts
// many upstream models (Anthropic, OpenAI, …) behind one OpenAI-compatible
// API. The wire format therefore differs from the Anthropic provider in
// three ways this client has to bridge:
//
//   - the system prompt travels as the first message with role "system"
//     rather than a dedicated top-level field;
//   - deltas arrive as choices[0].delta.content;
//   - the stream terminates with a literal "data: [DONE]" line.
type OpenRouter struct {
	APIKey  string
	Model   string // OpenRouter model id, e.g. "anthropic/claude-sonnet-4.5"
	BaseURL string
	HTTP    *http.Client
}

// compile-time check that *OpenRouter implements Streamer
var _ Streamer = (*OpenRouter)(nil)

func NewOpenRouter(apiKey, model string) *OpenRouter {
	return &OpenRouter{
		APIKey:  apiKey,
		Model:   model,
		BaseURL: "https://openrouter.ai/api",
		// No overall timeout: streams legitimately run for minutes.
		HTTP: &http.Client{Timeout: 0},
	}
}

type orMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type orRequest struct {
	Model     string      `json:"model"`
	Messages  []orMessage `json:"messages"`
	MaxTokens int         `json:"max_tokens"`
	Stream    bool        `json:"stream"`
}

// orChunk is a streamed completion chunk. The error field covers
// OpenRouter's mid-stream error objects (e.g. upstream provider failures),
// which arrive in-band as a data event rather than an HTTP status.
type orChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (o *OpenRouter) Stream(ctx context.Context, system string, msgs []Message, maxTokens int,
	onDelta func(string) error) (string, error) {

	orMsgs := make([]orMessage, 0, len(msgs)+1)
	if system != "" {
		orMsgs = append(orMsgs, orMessage{Role: "system", Content: system})
	}
	for _, m := range msgs {
		orMsgs = append(orMsgs, orMessage{Role: m.Role, Content: m.Content})
	}

	payload, err := json.Marshal(orRequest{
		Model:     o.Model,
		Messages:  orMsgs,
		MaxTokens: maxTokens,
		Stream:    true,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		o.BaseURL+"/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.APIKey)
	// Shown on the OpenRouter dashboard next to this app's usage.
	req.Header.Set("X-Title", "Garden of Knowledge")

	resp, err := o.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("call openrouter: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("openrouter returned %d: %s", resp.StatusCode, snippet)
	}

	// SSE comment lines (": OPENROUTER PROCESSING" keep-alives) lack the
	// data: prefix and are skipped implicitly.
	var full strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			return full.String(), nil
		}

		var chunk orChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return full.String(), fmt.Errorf("decode stream chunk: %w", err)
		}
		if chunk.Error != nil {
			return full.String(), fmt.Errorf("openrouter stream error: %s", chunk.Error.Message)
		}
		for _, c := range chunk.Choices {
			if c.Delta.Content == "" {
				continue
			}
			full.WriteString(c.Delta.Content)
			if err := onDelta(c.Delta.Content); err != nil {
				return full.String(), err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return full.String(), fmt.Errorf("read stream: %w", err)
	}
	return full.String(), nil
}
