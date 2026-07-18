package embeddings

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// voyageMaxBatch is the provider's per-request input limit (with margin).
const voyageMaxBatch = 96

// Voyage calls the Voyage AI embeddings API.
type Voyage struct {
	APIKey  string
	Model   string
	BaseURL string
	HTTP    *http.Client
}

// compile-time check that *Voyage implements Embedder
var _ Embedder = (*Voyage)(nil)

func NewVoyage(apiKey string) *Voyage {
	return &Voyage{
		APIKey:  apiKey,
		Model:   "voyage-3",
		BaseURL: "https://api.voyageai.com",
		HTTP:    &http.Client{Timeout: 60 * time.Second},
	}
}

type voyageRequest struct {
	Input     []string `json:"input"`
	Model     string   `json:"model"`
	InputType string   `json:"input_type"`
}

type voyageResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
}

// Embed processes texts in provider-sized batches and returns vectors in the
// same order as the input.
func (v *Voyage) Embed(ctx context.Context, texts []string, input InputType) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for start := 0; start < len(texts); start += voyageMaxBatch {
		end := start + voyageMaxBatch
		if end > len(texts) {
			end = len(texts)
		}
		if err := v.embedBatch(ctx, texts[start:end], input, out[start:end]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (v *Voyage) embedBatch(ctx context.Context, texts []string, input InputType, out [][]float32) error {
	payload, err := json.Marshal(voyageRequest{
		Input:     texts,
		Model:     v.Model,
		InputType: string(input),
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		v.BaseURL+"/v1/embeddings", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+v.APIKey)

	resp, err := v.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("call voyage: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("voyage returned %d: %s", resp.StatusCode, snippet)
	}

	var vr voyageResponse
	if err := json.NewDecoder(resp.Body).Decode(&vr); err != nil {
		return fmt.Errorf("decode voyage response: %w", err)
	}
	if len(vr.Data) != len(texts) {
		return fmt.Errorf("voyage returned %d embeddings for %d inputs", len(vr.Data), len(texts))
	}

	// The API documents an index field; place by it rather than trusting
	// response order.
	for _, d := range vr.Data {
		if d.Index < 0 || d.Index >= len(out) {
			return fmt.Errorf("voyage returned out-of-range index %d", d.Index)
		}
		out[d.Index] = d.Embedding
	}
	return nil
}
