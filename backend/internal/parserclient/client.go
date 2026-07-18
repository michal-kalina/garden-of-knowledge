// Package parserclient is the Go client for the Python parsing service.
// It owns the wire format of POST /parse and translates the response into
// chunking.Block, so the worker never touches parser JSON directly.
package parserclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"time"

	"github.com/michal-kalina/garden-of-knowledge/backend/internal/chunking"
)

// Client calls the parser service.
type Client struct {
	baseURL string
	http    *http.Client
}

func New(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		// Parsing large PDFs is CPU-bound on the parser side; give it room,
		// but never wait forever — the worker's retry loop handles the rest.
		http: &http.Client{Timeout: 2 * time.Minute},
	}
}

// parseResponse mirrors the parser's ParseResponse model.
type parseResponse struct {
	Blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
		Page *int   `json:"page"`
	} `json:"blocks"`
}

// Parse streams the file to the parser service and returns typed blocks.
func (c *Client) Parse(ctx context.Context, filename, contentType string, r io.Reader) ([]chunking.Block, error) {
	// The multipart body is built in memory; documents are capped by
	// MaxUploadBytes at the API edge, so this stays bounded.
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, filename))
	h.Set("Content-Type", contentType)
	part, err := w.CreatePart(h)
	if err != nil {
		return nil, fmt.Errorf("build multipart: %w", err)
	}
	if _, err := io.Copy(part, r); err != nil {
		return nil, fmt.Errorf("copy file into request: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("finalize multipart: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/parse", &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call parser: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Include a snippet of the response body — parser errors (422 with
		// detail) are the most useful diagnostic for failed jobs.
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("parser returned %d: %s", resp.StatusCode, snippet)
	}

	var pr parseResponse
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return nil, fmt.Errorf("decode parser response: %w", err)
	}

	blocks := make([]chunking.Block, 0, len(pr.Blocks))
	for _, b := range pr.Blocks {
		blocks = append(blocks, chunking.Block{Type: b.Type, Text: b.Text, Page: b.Page})
	}
	return blocks, nil
}
