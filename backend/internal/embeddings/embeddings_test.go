package embeddings

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFake(t *testing.T) {
	f := Fake{}

	t.Run("is deterministic", func(t *testing.T) {
		a, _ := f.Embed(context.Background(), []string{"hello"}, InputDocument)
		b, _ := f.Embed(context.Background(), []string{"hello"}, InputDocument)
		for i := range a[0] {
			if a[0][i] != b[0][i] {
				t.Fatalf("vectors differ at %d for identical input", i)
			}
		}
	})

	t.Run("different texts give different vectors", func(t *testing.T) {
		vs, _ := f.Embed(context.Background(), []string{"alpha", "beta"}, InputDocument)
		same := true
		for i := range vs[0] {
			if vs[0][i] != vs[1][i] {
				same = false
				break
			}
		}
		if same {
			t.Fatal("distinct inputs produced identical vectors")
		}
	})

	t.Run("vectors have the schema dimension and unit length", func(t *testing.T) {
		vs, _ := f.Embed(context.Background(), []string{"x"}, InputDocument)
		if len(vs[0]) != Dim {
			t.Fatalf("dim = %d, want %d", len(vs[0]), Dim)
		}
		var norm float64
		for _, v := range vs[0] {
			norm += float64(v) * float64(v)
		}
		if math.Abs(math.Sqrt(norm)-1) > 1e-3 {
			t.Errorf("norm = %f, want ~1", math.Sqrt(norm))
		}
	})
}

func TestVoyage(t *testing.T) {
	t.Run("sends auth and payload, respects index order", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
				t.Errorf("auth header = %q", got)
			}
			var req voyageRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatal(err)
			}
			if req.Model != "voyage-3" || req.InputType != "document" {
				t.Errorf("unexpected request: %+v", req)
			}
			// Respond deliberately out of order; the client must re-order
			// by index.
			resp := map[string]any{"data": []map[string]any{
				{"embedding": []float32{2, 2}, "index": 1},
				{"embedding": []float32{1, 1}, "index": 0},
			}}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer srv.Close()

		v := NewVoyage("test-key")
		v.BaseURL = srv.URL
		got, err := v.Embed(context.Background(), []string{"a", "b"}, InputDocument)
		if err != nil {
			t.Fatal(err)
		}
		if got[0][0] != 1 || got[1][0] != 2 {
			t.Errorf("order not restored by index: %v", got)
		}
	})

	t.Run("surfaces provider errors with body snippet", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"detail":"rate limited"}`))
		}))
		defer srv.Close()

		v := NewVoyage("k")
		v.BaseURL = srv.URL
		_, err := v.Embed(context.Background(), []string{"a"}, InputQuery)
		if err == nil {
			t.Fatal("expected error")
		}
		if want := "rate limited"; !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	})

	t.Run("rejects mismatched embedding count", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
		}))
		defer srv.Close()

		v := NewVoyage("k")
		v.BaseURL = srv.URL
		if _, err := v.Embed(context.Background(), []string{"a"}, InputDocument); err == nil {
			t.Fatal("expected error for missing embeddings")
		}
	})
}
