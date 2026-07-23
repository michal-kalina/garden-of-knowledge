// Package llm is a minimal client for the Anthropic Messages API with
// streaming. Hand-rolled on the standard library rather than the SDK — the
// project uses one endpoint with one content type, and owning the ~150 lines
// keeps the dependency budget flat (the same reasoning as ADR-0005) while
// making the SSE mechanics visible instead of hidden behind a helper.
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

// Message is a single conversation turn.
type Message struct {
	Role    string `json:"role"` // "user" | "assistant"
	Content string `json:"content"`
}

// Streamer is the interface the chat layer depends on.
type Streamer interface {
	// Stream sends the prompt and invokes onDelta for every text fragment
	// as it arrives, returning the full concatenated response text.
	Stream(ctx context.Context, system string, msgs []Message, maxTokens int,
		onDelta func(text string) error) (string, error)
}

// Anthropic calls the Anthropic Messages API.
type Anthropic struct {
	APIKey  string
	Model   string
	BaseURL string
	HTTP    *http.Client
}

// compile-time check that *Anthropic implements Streamer
var _ Streamer = (*Anthropic)(nil)

func NewAnthropic(apiKey, model string) *Anthropic {
	return &Anthropic{
		APIKey:  apiKey,
		Model:   model,
		BaseURL: "https://api.anthropic.com",
		// No overall timeout: streams legitimately run for minutes. The
		// caller's context cancels abandoned requests.
		HTTP: &http.Client{Timeout: 0},
	}
}

type messagesRequest struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	System    string    `json:"system,omitempty"`
	Messages  []Message `json:"messages"`
	Stream    bool      `json:"stream"`
}

// streamEvent covers the union of SSE payloads we care about; everything
// else (ping, message_start, content_block_start/stop) is skipped by type.
type streamEvent struct {
	Type  string `json:"type"`
	Delta struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"delta"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func (a *Anthropic) Stream(ctx context.Context, system string, msgs []Message, maxTokens int,
	onDelta func(string) error) (string, error) {

	payload, err := json.Marshal(messagesRequest{
		Model:     a.Model,
		MaxTokens: maxTokens,
		System:    system,
		Messages:  msgs,
		Stream:    true,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		a.BaseURL+"/v1/messages", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", a.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := a.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("call anthropic: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("anthropic returned %d: %s", resp.StatusCode, snippet)
	}

	// SSE framing: lines of "event: <name>" and "data: <json>", events
	// separated by blank lines. We only need the data lines — every payload
	// repeats its type in the JSON, so the event: line is redundant.
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

		var ev streamEvent
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			return full.String(), fmt.Errorf("decode stream event: %w", err)
		}
		switch ev.Type {
		case "content_block_delta":
			if ev.Delta.Type == "text_delta" && ev.Delta.Text != "" {
				full.WriteString(ev.Delta.Text)
				if err := onDelta(ev.Delta.Text); err != nil {
					// The consumer (e.g. a disconnected browser) refused
					// the delta — stop reading and let the deferred Close
					// abort the upstream request.
					return full.String(), err
				}
			}
		case "error":
			return full.String(), fmt.Errorf("anthropic stream error (%s): %s",
				ev.Error.Type, ev.Error.Message)
		case "message_stop":
			return full.String(), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return full.String(), fmt.Errorf("read stream: %w", err)
	}
	// Stream ended without message_stop — treat what we have as complete.
	return full.String(), nil
}
