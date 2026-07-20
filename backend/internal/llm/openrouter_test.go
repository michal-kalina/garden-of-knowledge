package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenRouterStream(t *testing.T) {
	t.Run("streams deltas, honors system message and [DONE]", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/chat/completions" {
				t.Errorf("path = %q", r.URL.Path)
			}
			if got := r.Header.Get("Authorization"); got != "Bearer or-test" {
				t.Errorf("authorization = %q", got)
			}
			var req orRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatal(err)
			}
			if req.Model != "anthropic/claude-sonnet-4.5" || !req.Stream {
				t.Errorf("unexpected request: %+v", req)
			}
			if len(req.Messages) != 2 || req.Messages[0].Role != "system" ||
				req.Messages[0].Content != "be terse" || req.Messages[1].Role != "user" {
				t.Errorf("messages = %+v", req.Messages)
			}

			_, _ = w.Write([]byte(
				": OPENROUTER PROCESSING\n\n" +
					`data: {"choices":[{"delta":{"content":"Hel"},"finish_reason":null}]}` + "\n\n" +
					`data: {"choices":[{"delta":{"content":"lo [1]"},"finish_reason":null}]}` + "\n\n" +
					`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}` + "\n\n" +
					"data: [DONE]\n\n"))
		}))
		defer srv.Close()

		c := NewOpenRouter("or-test", "anthropic/claude-sonnet-4.5")
		c.BaseURL = srv.URL

		var deltas []string
		full, err := c.Stream(context.Background(), "be terse",
			[]Message{{Role: "user", Content: "hi"}}, 100,
			func(text string) error {
				deltas = append(deltas, text)
				return nil
			})
		if err != nil {
			t.Fatal(err)
		}
		if full != "Hello [1]" {
			t.Errorf("full = %q", full)
		}
		if len(deltas) != 2 {
			t.Errorf("deltas = %v", deltas)
		}
	})

	t.Run("surfaces mid-stream error objects", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(
				`data: {"error":{"message":"upstream provider unavailable"}}` + "\n\n"))
		}))
		defer srv.Close()

		c := NewOpenRouter("k", "m")
		c.BaseURL = srv.URL
		_, err := c.Stream(context.Background(), "", []Message{{Role: "user", Content: "x"}}, 10,
			func(string) error { return nil })
		if err == nil || !strings.Contains(err.Error(), "upstream provider unavailable") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("surfaces HTTP errors with body", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusPaymentRequired)
			_, _ = w.Write([]byte(`{"error":{"message":"insufficient credits"}}`))
		}))
		defer srv.Close()

		c := NewOpenRouter("k", "m")
		c.BaseURL = srv.URL
		_, err := c.Stream(context.Background(), "", []Message{{Role: "user", Content: "x"}}, 10,
			func(string) error { return nil })
		if err == nil || !strings.Contains(err.Error(), "insufficient credits") {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestFromProvider(t *testing.T) {
	if s, err := FromProvider("anthropic", "m", "k"); err != nil {
		t.Fatal(err)
	} else if _, ok := s.(*Anthropic); !ok {
		t.Errorf("provider anthropic -> %T", s)
	}
	if s, err := FromProvider("openrouter", "m", "k"); err != nil {
		t.Fatal(err)
	} else if _, ok := s.(*OpenRouter); !ok {
		t.Errorf("provider openrouter -> %T", s)
	}
	if _, err := FromProvider("quantum", "m", "k"); err == nil {
		t.Error("expected error for unknown provider")
	}
}
