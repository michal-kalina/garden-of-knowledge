// Package retrieval finds the chunks most relevant to a query using hybrid
// search: dense vector similarity and Postgres full-text search, fused with
// Reciprocal Rank Fusion.
//
// Why hybrid: the two retrievers fail differently. Vector search understands
// paraphrase and synonyms ("okres wypowiedzenia" ~ "termination notice") but
// is weak on exact identifiers, product codes, names and rare terms — which
// is precisely where lexical full-text search is unbeatable. Fusing both
// covers each one's blind spots.
//
// Why RRF: vector distance and ts_rank live on incomparable scales, so any
// weighted sum of raw scores is arbitrary. RRF sidesteps calibration by
// discarding scores entirely and combining rank positions:
//
//	score(d) = Σ over lists 1 / (k + rank_i(d))
//
// with k = 60 (rrfK below). Higher k flattens the curve between ranks — the
// gap between rank 1 and rank 2 shrinks relative to the gap between being in
// a list at all versus absent from it — so a document ranked well in both
// lists beats one ranked first in only one. k = 60 is not tuned for this
// dataset; it's the constant from Cormack, Clarke & Büttcher, "Reciprocal
// Rank Fusion Outperforms Condorcet and Individual Rank Learning Methods"
// (SIGIR 2009), and has since become the de facto default (e.g. Elasticsearch
// uses the same value for its built-in RRF).
package retrieval

import (
	"context"
	"database/sql"
	"fmt"
	"sort"

	"github.com/michal-kalina/garden-of-knowledge/backend/internal/database"
	"github.com/michal-kalina/garden-of-knowledge/backend/internal/embeddings"
)

const (
	// rrfK is the RRF damping constant.
	rrfK = 60
	// candidateLimit is how deep each retriever's list goes before fusion.
	// Deeper than the final limit on purpose: a chunk ranked #15 by both
	// retrievers should beat one ranked #1 by a single retriever only.
	candidateLimit = 30
	// DefaultLimit is the number of fused results returned when the caller
	// does not specify one.
	DefaultLimit = 8
	// MaxLimit caps the response size.
	MaxLimit = 50
)

// Result is a retrieved chunk with its fusion score and per-retriever ranks
// (1-based; 0 means the chunk did not appear in that retriever's list —
// surfaced deliberately so /search responses show *why* something ranked).
type Result struct {
	ChunkID    int64   `json:"chunk_id"`
	DocumentID string  `json:"document_id"`
	Filename   string  `json:"filename"`
	Ordinal    int     `json:"ordinal"`
	Content    string  `json:"content"`
	Heading    string  `json:"heading,omitempty"`
	PageStart  *int    `json:"page_start,omitempty"`
	PageEnd    *int    `json:"page_end,omitempty"`
	Score      float64 `json:"score"`
	VectorRank int     `json:"vector_rank,omitempty"`
	TextRank   int     `json:"text_rank,omitempty"`
}

// Searcher runs hybrid retrieval. Only ready documents are searched.
type Searcher struct {
	db       *sql.DB
	embedder embeddings.Embedder
}

func New(db *sql.DB, embedder embeddings.Embedder) *Searcher {
	return &Searcher{db: db, embedder: embedder}
}

// Search embeds the query, runs both retrievers over the user's documents
// and fuses their rankings. Tenancy lives in SQL, not post-filtering: a
// candidate list fetched globally and filtered afterwards could end up
// empty for a user whose documents rank below others'.
func (s *Searcher) Search(ctx context.Context, userID, query string, limit int) ([]Result, error) {
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}

	// Note InputQuery, not InputDocument: providers train separate
	// projections for the two sides of retrieval.
	vecs, err := s.embedder.Embed(ctx, []string{query}, embeddings.InputQuery)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	byID := map[int64]*Result{}

	vectorRows, err := s.vectorSearch(ctx, userID, database.VectorLiteral(vecs[0]))
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}
	for rank, r := range vectorRows {
		res := upsert(byID, r)
		res.VectorRank = rank + 1
		res.Score += 1.0 / float64(rrfK+rank+1)
	}

	textRows, err := s.textSearch(ctx, userID, query)
	if err != nil {
		return nil, fmt.Errorf("text search: %w", err)
	}
	for rank, r := range textRows {
		res := upsert(byID, r)
		res.TextRank = rank + 1
		res.Score += 1.0 / float64(rrfK+rank+1)
	}

	fused := make([]Result, 0, len(byID))
	for _, r := range byID {
		fused = append(fused, *r)
	}
	sort.Slice(fused, func(i, j int) bool {
		if fused[i].Score != fused[j].Score {
			return fused[i].Score > fused[j].Score
		}
		return fused[i].ChunkID < fused[j].ChunkID // deterministic ties
	})
	if len(fused) > limit {
		fused = fused[:limit]
	}
	return fused, nil
}

func upsert(m map[int64]*Result, r Result) *Result {
	if existing, ok := m[r.ChunkID]; ok {
		return existing
	}
	cp := r
	m[r.ChunkID] = &cp
	return &cp
}

const resultColumns = `
	c.id, c.document_id, d.filename, c.ordinal, c.content, c.heading,
	c.page_start, c.page_end`

func (s *Searcher) vectorSearch(ctx context.Context, userID, queryVec string) ([]Result, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+resultColumns+`
		FROM chunks c
		JOIN documents d ON d.id = c.document_id
		WHERE c.embedding IS NOT NULL AND d.status = 'ready' AND d.user_id = $3
		ORDER BY c.embedding <=> $1::vector
		LIMIT $2
	`, queryVec, candidateLimit, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanResults(rows)
}

func (s *Searcher) textSearch(ctx context.Context, userID, query string) ([]Result, error) {
	// websearch_to_tsquery accepts raw user input safely (quotes, OR, -).
	// The 'simple' configuration must match the one used for the generated
	// tsv column in migration 0001 — mixed configurations silently return
	// nothing.
	//
	// ORDER BY breaks ts_rank ties on c.id: without it, tied rows could
	// reorder between runs, silently reshuffling TextRank and thus the RRF
	// score for otherwise-identical searches.
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+resultColumns+`
		FROM chunks c
		JOIN documents d ON d.id = c.document_id,
		     websearch_to_tsquery('simple', $1) q
		WHERE c.tsv @@ q AND d.status = 'ready' AND d.user_id = $3
		ORDER BY ts_rank(c.tsv, q) DESC, c.id
		LIMIT $2
	`, query, candidateLimit, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanResults(rows)
}

func scanResults(rows *sql.Rows) ([]Result, error) {
	var out []Result
	for rows.Next() {
		var r Result
		if err := rows.Scan(&r.ChunkID, &r.DocumentID, &r.Filename, &r.Ordinal,
			&r.Content, &r.Heading, &r.PageStart, &r.PageEnd); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
