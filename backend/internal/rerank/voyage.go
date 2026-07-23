package rerank

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Voyage calls the Voyage AI reranking endpoint (POST /v1/rerank). Same
// account and API key as internal/embeddings' Voyage client — one provider
// for both stages of retrieval, which is the whole point of picking it.
type Voyage struct {
	APIKey  string
	Model   string
	BaseURL string
	HTTP    *http.Client
}

// compile-time check that *Voyage implements Reranker
var _ Reranker = (*Voyage)(nil)

// NewVoyage builds a client for the given model. "rerank-2.5" is Voyage's
// current recommendation over the legacy rerank-2 family.
func NewVoyage(apiKey, model string) *Voyage {
	return &Voyage{
		APIKey:  apiKey,
		Model:   model,
		BaseURL: "https://api.voyageai.com",
		HTTP:    &http.Client{Timeout: 30 * time.Second},
	}
}

type voyageRerankRequest struct {
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
	Model     string   `json:"model"`
	TopK      int      `json:"top_k,omitempty"`
}

type voyageRerankResponse struct {
	Data []struct {
		Index          int     `json:"index"`
		RelevanceScore float64 `json:"relevance_score"`
	} `json:"data"`
}

func (v *Voyage) ModelName() string { return v.Model }

func (v *Voyage) Rerank(ctx context.Context, query string, documents []string, topK int) ([]Result, error) {
	if len(documents) == 0 {
		return nil, nil
	}

	payload, err := json.Marshal(voyageRerankRequest{
		Query:     query,
		Documents: documents,
		Model:     v.Model,
		TopK:      topK,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		v.BaseURL+"/v1/rerank", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+v.APIKey)

	resp, err := v.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call voyage rerank: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("voyage rerank returned %d: %s", resp.StatusCode, snippet)
	}

	var vr voyageRerankResponse
	if err := json.NewDecoder(resp.Body).Decode(&vr); err != nil {
		return nil, fmt.Errorf("decode voyage rerank response: %w", err)
	}

	// The API documents its response as already sorted by descending
	// relevance — we preserve that order rather than re-sorting, so a
	// future scoring quirk on Voyage's side doesn't get silently masked by
	// us re-deriving an order they didn't intend.
	out := make([]Result, 0, len(vr.Data))
	for _, d := range vr.Data {
		if d.Index < 0 || d.Index >= len(documents) {
			return nil, fmt.Errorf("voyage rerank returned out-of-range index %d for %d documents",
				d.Index, len(documents))
		}
		out = append(out, Result{Index: d.Index, Score: d.RelevanceScore})
	}
	return out, nil
}
