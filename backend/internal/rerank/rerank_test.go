package rerank

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFake(t *testing.T) {
	docs := []string{"a", "b", "c"}
	out, err := Fake{}.Rerank(context.Background(), "q", docs, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 3 {
		t.Fatalf("len = %d, want 3", len(out))
	}
	for i, r := range out {
		if r.Index != i {
			t.Errorf("out[%d].Index = %d, want %d (Fake preserves order)", i, r.Index, i)
		}
	}

	t.Run("respects topK", func(t *testing.T) {
		out, err := Fake{}.Rerank(context.Background(), "q", docs, 2)
		if err != nil {
			t.Fatal(err)
		}
		if len(out) != 2 {
			t.Fatalf("len = %d, want 2", len(out))
		}
	})
}

func TestVoyageRerank(t *testing.T) {
	t.Run("sends the request shape Voyage documents and parses scores", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/rerank" {
				t.Errorf("path = %q", r.URL.Path)
			}
			if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
				t.Errorf("auth header = %q", got)
			}
			var req voyageRerankRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatal(err)
			}
			if req.Model != "rerank-2.5" || req.Query != "why pgvector" || req.TopK != 2 {
				t.Errorf("unexpected request: %+v", req)
			}
			if len(req.Documents) != 3 {
				t.Errorf("documents = %v", req.Documents)
			}
			// Response order deliberately differs from input order — the
			// whole point of reranking is permuting it.
			_ = json.NewEncoder(w).Encode(voyageRerankResponse{
				Data: []struct {
					Index          int     `json:"index"`
					RelevanceScore float64 `json:"relevance_score"`
				}{
					{Index: 2, RelevanceScore: 0.91},
					{Index: 0, RelevanceScore: 0.44},
				},
			})
		}))
		defer srv.Close()

		v := NewVoyage("test-key", "rerank-2.5")
		v.BaseURL = srv.URL
		got, err := v.Rerank(context.Background(), "why pgvector",
			[]string{"doc a", "doc b", "doc c"}, 2)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 || got[0].Index != 2 || got[1].Index != 0 {
			t.Fatalf("got = %+v", got)
		}
		if got[0].Score != 0.91 {
			t.Errorf("score = %v, want 0.91", got[0].Score)
		}
	})

	t.Run("empty document list short-circuits without a request", func(t *testing.T) {
		v := NewVoyage("k", "rerank-2.5")
		v.BaseURL = "http://unused.invalid"
		got, err := v.Rerank(context.Background(), "q", nil, 0)
		if err != nil || got != nil {
			t.Fatalf("got=%v err=%v, want nil,nil", got, err)
		}
	})

	t.Run("surfaces HTTP errors with body", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"detail":"rate limited"}`))
		}))
		defer srv.Close()

		v := NewVoyage("k", "rerank-2.5")
		v.BaseURL = srv.URL
		_, err := v.Rerank(context.Background(), "q", []string{"a"}, 0)
		if err == nil || !strings.Contains(err.Error(), "rate limited") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("rejects an out-of-range index", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{{"index": 99, "relevance_score": 0.5}},
			})
		}))
		defer srv.Close()

		v := NewVoyage("k", "rerank-2.5")
		v.BaseURL = srv.URL
		_, err := v.Rerank(context.Background(), "q", []string{"a"}, 0)
		if err == nil {
			t.Fatal("expected error for out-of-range index")
		}
	})
}
