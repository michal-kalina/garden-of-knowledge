package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAnthropicStream(t *testing.T) {
	t.Run("streams text deltas and returns full text", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/messages" {
				t.Errorf("path = %q", r.URL.Path)
			}
			if got := r.Header.Get("x-api-key"); got != "sk-test" {
				t.Errorf("x-api-key = %q", got)
			}
			if got := r.Header.Get("anthropic-version"); got != "2023-06-01" {
				t.Errorf("anthropic-version = %q", got)
			}
			var req map[string]any
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatal(err)
			}
			if req["stream"] != true || req["model"] != "test-model" {
				t.Errorf("unexpected request: %v", req)
			}
			if req["system"] != "be terse" {
				t.Errorf("system = %v", req["system"])
			}

			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte(
				"event: message_start\n" +
					`data: {"type":"message_start"}` + "\n\n" +
					"event: ping\n" +
					`data: {"type":"ping"}` + "\n\n" +
					"event: content_block_delta\n" +
					`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"Hel"}}` + "\n\n" +
					"event: content_block_delta\n" +
					`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"lo [1]"}}` + "\n\n" +
					"event: message_stop\n" +
					`data: {"type":"message_stop"}` + "\n\n"))
		}))
		defer srv.Close()

		c := NewAnthropic("sk-test", "test-model")
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
		if len(deltas) != 2 || deltas[0] != "Hel" {
			t.Errorf("deltas = %v", deltas)
		}
	})

	t.Run("surfaces stream error events", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(
				`data: {"type":"error","error":{"type":"overloaded_error","message":"try later"}}` + "\n\n"))
		}))
		defer srv.Close()

		c := NewAnthropic("k", "m")
		c.BaseURL = srv.URL
		_, err := c.Stream(context.Background(), "", []Message{{Role: "user", Content: "x"}}, 10,
			func(string) error { return nil })
		if err == nil || !strings.Contains(err.Error(), "try later") {
			t.Fatalf("err = %v, want overloaded message", err)
		}
	})

	t.Run("surfaces HTTP errors with body", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"message":"invalid x-api-key"}}`))
		}))
		defer srv.Close()

		c := NewAnthropic("bad", "m")
		c.BaseURL = srv.URL
		_, err := c.Stream(context.Background(), "", []Message{{Role: "user", Content: "x"}}, 10,
			func(string) error { return nil })
		if err == nil || !strings.Contains(err.Error(), "invalid x-api-key") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("stops when the delta consumer fails", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(
				`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"a"}}` + "\n\n" +
					`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"b"}}` + "\n\n"))
		}))
		defer srv.Close()

		c := NewAnthropic("k", "m")
		c.BaseURL = srv.URL
		calls := 0
		_, err := c.Stream(context.Background(), "", []Message{{Role: "user", Content: "x"}}, 10,
			func(string) error {
				calls++
				return context.Canceled
			})
		if err == nil {
			t.Fatal("expected consumer error to propagate")
		}
		if calls != 1 {
			t.Errorf("consumer called %d times after failing, want 1", calls)
		}
	})
}
