// Package chat turns a question into a streamed, citation-grounded answer:
// retrieve relevant chunks, build a prompt with numbered sources, stream the
// model's response. Stateless by design — conversation history arrives with
// Phase 3.
package chat

import (
	"context"
	"fmt"
	"strings"

	"github.com/michal-kalina/garden-of-knowledge/backend/internal/llm"
	"github.com/michal-kalina/garden-of-knowledge/backend/internal/retrieval"
)

// Retriever is what chat needs from the retrieval layer.
type Retriever interface {
	Search(ctx context.Context, query string, limit int) ([]retrieval.Result, error)
}

// Source is a context chunk as presented to both the model and the client.
// Index is the citation number the model is instructed to use — the client
// maps [n] markers in the streamed text back to these.
type Source struct {
	Index      int    `json:"index"`
	ChunkID    int64  `json:"chunk_id"`
	DocumentID string `json:"document_id"`
	Filename   string `json:"filename"`
	Heading    string `json:"heading,omitempty"`
	PageStart  *int   `json:"page_start,omitempty"`
	PageEnd    *int   `json:"page_end,omitempty"`
	Content    string `json:"content"`
}

// Service orchestrates retrieval and generation.
type Service struct {
	retriever Retriever
	llm       llm.Streamer
	// contextLimit caps how many retrieved chunks enter the prompt.
	contextLimit int
	maxTokens    int
}

func New(retriever Retriever, streamer llm.Streamer) *Service {
	return &Service{
		retriever:    retriever,
		llm:          streamer,
		contextLimit: 6,
		maxTokens:    1024,
	}
}

const systemPrompt = `You are the assistant for a private knowledge base called Garden of Knowledge.

Rules:
- Answer ONLY from the numbered sources provided. Do not use outside knowledge.
- Cite every factual claim with the source number in square brackets, e.g. [1] or [2][3].
- If the sources do not contain the answer, say so plainly instead of guessing.
- Answer in the same language as the question.
- Be concise.`

// noSourcesReply is streamed verbatim when retrieval finds nothing — no LLM
// call is made, because there is nothing to ground an answer in and the
// model would only be tempted to improvise.
const noSourcesReply = "I could not find anything in the knowledge base related to this question. " +
	"Try rephrasing it, or check whether the relevant documents have been uploaded and are marked ready."

// Ask runs the full flow. onSources fires once, before generation starts,
// so the client can render the citation panel while text is still
// streaming; onDelta fires per text fragment.
func (s *Service) Ask(ctx context.Context, question string,
	onSources func([]Source) error, onDelta func(string) error) error {

	results, err := s.retriever.Search(ctx, question, s.contextLimit)
	if err != nil {
		return fmt.Errorf("retrieve: %w", err)
	}

	sources := make([]Source, len(results))
	for i, r := range results {
		sources[i] = Source{
			Index:      i + 1,
			ChunkID:    r.ChunkID,
			DocumentID: r.DocumentID,
			Filename:   r.Filename,
			Heading:    r.Heading,
			PageStart:  r.PageStart,
			PageEnd:    r.PageEnd,
			Content:    r.Content,
		}
	}
	if err := onSources(sources); err != nil {
		return err
	}

	if len(sources) == 0 {
		return onDelta(noSourcesReply)
	}

	_, err = s.llm.Stream(ctx, systemPrompt,
		[]llm.Message{{Role: "user", Content: buildUserPrompt(question, sources)}},
		s.maxTokens, onDelta)
	if err != nil {
		return fmt.Errorf("generate: %w", err)
	}
	return nil
}

// buildUserPrompt lays out the numbered sources followed by the question.
// Each source header carries filename/heading/pages — the model tends to
// produce better-grounded citations when it can see what a source *is*, and
// it costs a handful of tokens.
func buildUserPrompt(question string, sources []Source) string {
	var b strings.Builder
	b.WriteString("Sources:\n\n")
	for _, src := range sources {
		fmt.Fprintf(&b, "[%d] %s", src.Index, src.Filename)
		if src.Heading != "" {
			fmt.Fprintf(&b, " — %s", src.Heading)
		}
		if src.PageStart != nil {
			if src.PageEnd != nil && *src.PageEnd != *src.PageStart {
				fmt.Fprintf(&b, " (pages %d–%d)", *src.PageStart, *src.PageEnd)
			} else {
				fmt.Fprintf(&b, " (page %d)", *src.PageStart)
			}
		}
		b.WriteString("\n")
		b.WriteString(src.Content)
		b.WriteString("\n\n")
	}
	b.WriteString("Question: ")
	b.WriteString(question)
	return b.String()
}
