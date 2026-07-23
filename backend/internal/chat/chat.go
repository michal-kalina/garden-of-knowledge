// Package chat turns a question into a streamed, citation-grounded answer:
// retrieve relevant chunks, build a prompt with numbered sources, stream the
// model's response.
//
// Retrieval is still per-question only (the current question's text is what
// gets embedded and searched) — a known limitation of naive RAG: a follow-up
// like "and on what page?" carries little retrievable signal on its own.
// Prior turns are given to the model so it can maintain continuity in its
// answer, but do not yet rewrite the retrieval query. Query condensation
// (folding history into the search query) is tracked as a Phase 4 candidate,
// alongside the evals that would demonstrate whether it actually helps.
package chat

import (
	"context"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/michal-kalina/garden-of-knowledge/backend/internal/chunking"
	"github.com/michal-kalina/garden-of-knowledge/backend/internal/cost"
	"github.com/michal-kalina/garden-of-knowledge/backend/internal/llm"
	"github.com/michal-kalina/garden-of-knowledge/backend/internal/retrieval"
)

// Retriever is what chat needs from the retrieval layer.
type Retriever interface {
	Search(ctx context.Context, userID, query string, limit int) ([]retrieval.Result, error)
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

// Turn is one past exchange, given to the model as conversation context.
type Turn struct {
	Role    string // "user" | "assistant"
	Content string
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
- Be concise.
- Earlier turns are given for conversational continuity (e.g. resolving "it" or "that"),
  but every new factual claim must still be grounded in THIS turn's numbered sources —
  a source from an earlier turn is not automatically valid for a new question.`

// noSourcesReply is streamed verbatim when retrieval finds nothing — no LLM
// call is made, because there is nothing to ground an answer in and the
// model would only be tempted to improvise.
const noSourcesReply = "I could not find anything in the knowledge base related to this question. " +
	"Try rephrasing it, or check whether the relevant documents have been uploaded and are marked ready."

var tracer = otel.Tracer("gok/chat")

// Ask runs the full flow and returns the complete answer text (identical to
// what was streamed via onDelta) so the caller can persist it without
// re-accumulating deltas itself. onSources fires once, before generation
// starts, so the client can render the citation panel while text is still
// streaming; onDelta fires per text fragment. history is prior turns of the
// same conversation, oldest first; pass nil for a fresh conversation.
func (s *Service) Ask(ctx context.Context, userID, question string, history []Turn,
	onSources func([]Source) error, onDelta func(string) error) (string, error) {

	ctx, span := tracer.Start(ctx, "chat.ask")
	defer span.End()
	span.SetAttributes(
		attribute.Int("gok.question_chars", len(question)),
		attribute.Int("gok.history_turns", len(history)),
	)

	results, err := s.retriever.Search(ctx, userID, question, s.contextLimit)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "retrieve failed")
		return "", fmt.Errorf("retrieve: %w", err)
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
	span.SetAttributes(attribute.Int("gok.source_count", len(sources)))
	if err := onSources(sources); err != nil {
		return "", err
	}

	if len(sources) == 0 {
		if err := onDelta(noSourcesReply); err != nil {
			return "", err
		}
		return noSourcesReply, nil
	}

	messages := make([]llm.Message, 0, len(history)+1)
	for _, t := range history {
		messages = append(messages, llm.Message{Role: t.Role, Content: t.Content})
	}
	userPrompt := buildUserPrompt(question, sources)
	messages = append(messages, llm.Message{Role: "user", Content: userPrompt})

	answer, err := s.llm.Stream(ctx, systemPrompt, messages, s.maxTokens, onDelta)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "generate failed")
		return "", fmt.Errorf("generate: %w", err)
	}

	// Cost is estimated from our own token counting, not the provider's
	// billed usage — see internal/cost's package doc for why (short
	// version: wiring real usage requires extending Streamer, tracked as a
	// follow-up). Directionally useful in a trace, not a billing ledger.
	model := s.llm.ModelName()
	inputTokens := chunking.ApproxTokens(systemPrompt) + chunking.ApproxTokens(userPrompt)
	for _, t := range history {
		inputTokens += chunking.ApproxTokens(t.Content)
	}
	outputTokens := chunking.ApproxTokens(answer)
	span.SetAttributes(
		attribute.String("gok.model", model),
		attribute.Int("gok.answer_chars", len(answer)),
		attribute.Float64("gok.cost_usd_estimate", cost.LLMUSD(model, inputTokens, outputTokens)),
	)
	return answer, nil
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
