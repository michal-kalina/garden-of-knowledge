// Package chunking splits parsed documents into retrieval-sized chunks.
//
// Two strategies live behind the Chunker interface:
//
//   - RecursiveChunker: the pragmatic baseline — merges blocks up to a token
//     budget, splitting oversized text along a separator hierarchy
//     (paragraph > line > sentence > word) with a configurable overlap.
//   - StructureChunker: document-structure-aware — groups blocks into
//     sections under their headings and prefixes every chunk with its
//     heading, which markedly improves embedding quality; tables are never
//     split.
//
// Keeping both behind one interface lets Phase 4 evals compare them on real
// retrieval metrics instead of folklore.
package chunking

import "strings"

// Block is a unit of parsed content, mirroring the parser service contract
// (POST /parse). The worker maps the parser's JSON response into this type.
type Block struct {
	// Type is e.g. "paragraph", "heading" or "table".
	Type string
	Text string
	// Page is the 1-based page number in the source document, if known.
	Page *int
}

// Chunk is a retrieval unit ready to be embedded and stored.
type Chunk struct {
	// Content is the full text to embed. For StructureChunker it already
	// includes the section heading as a prefix.
	Content string
	// Heading is the section heading this chunk belongs to ("" if none).
	// Stored separately so citations can display it without re-parsing.
	Heading string
	// StartPage/EndPage delimit the source pages this chunk spans, if known.
	StartPage *int
	EndPage   *int
}

// Chunker turns parsed blocks into chunks. Implementations must be pure:
// same input, same output, no I/O — which is what makes them trivially
// unit-testable.
type Chunker interface {
	Chunk(blocks []Block) []Chunk
}

// TokenCounter estimates the number of tokens in a string. Injectable so a
// real tokenizer can replace the approximation without touching chunkers.
type TokenCounter func(string) int

// ApproxTokens estimates tokens as ceil(bytes/4) — the usual rule of thumb
// for BPE tokenizers on English text. Polish runs denser (more tokens per
// word), so this undercounts slightly; acceptable because chunk size targets
// are soft budgets, not hard model limits.
func ApproxTokens(s string) int {
	if s == "" {
		return 0
	}
	return (len(s) + 3) / 4
}

// separators is the split hierarchy, coarsest first: paragraph break, line
// break, sentence end, word boundary.
var separators = []string{"\n\n", "\n", ". ", " "}

// splitText splits text into pieces that each fit the token target, trying
// the coarsest separator first and recursing into finer ones only for pieces
// that are still too large. Guaranteed to terminate: the final fallback
// splits by words and, in the degenerate case of a single enormous word, by
// runes.
func splitText(text string, target int, count TokenCounter) []string {
	return splitWith(text, separators, target, count)
}

func splitWith(text string, seps []string, target int, count TokenCounter) []string {
	if count(text) <= target {
		return []string{text}
	}
	if len(seps) == 0 {
		return hardSplit(text, target, count)
	}

	// SplitAfter keeps the separator attached, so re-joining pieces loses
	// no characters (a plain Split would silently eat sentence periods).
	parts := strings.SplitAfter(text, seps[0])
	if len(parts) == 1 {
		return splitWith(text, seps[1:], target, count)
	}

	var out []string
	var cur strings.Builder
	curCount := 0
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
			curCount = 0
		}
	}
	for _, p := range parts {
		pc := count(p)
		if pc > target {
			// This part alone is too big — recurse with finer separators.
			flush()
			out = append(out, splitWith(p, seps[1:], target, count)...)
			continue
		}
		if curCount+pc > target {
			flush()
		}
		cur.WriteString(p)
		curCount += pc
	}
	flush()
	return out
}

// hardSplit is the last-resort splitter for text with no usable separators.
func hardSplit(text string, target int, count TokenCounter) []string {
	words := strings.Fields(text)
	var out []string
	var cur []string
	curCount := 0
	flush := func() {
		if len(cur) > 0 {
			out = append(out, strings.Join(cur, " "))
			cur = nil
			curCount = 0
		}
	}
	for _, w := range words {
		wc := count(w)
		if wc > target {
			// A single word exceeding the budget (URLs, base64 blobs):
			// split by runes at ~4 chars per token.
			flush()
			out = append(out, runeSplit(w, target*4)...)
			continue
		}
		if curCount+wc > target {
			flush()
		}
		cur = append(cur, w)
		curCount += wc
	}
	flush()
	return out
}

// runeSplit splits s into chunks of at most maxRunes Unicode code points each.
// Using runes (not bytes) ensures multi-byte characters are never split mid-sequence.
// Example: runeSplit("żółw", 2) → ["żó", "łw"] — Polish diacritics (ó, ł) are each one rune.
func runeSplit(s string, maxRunes int) []string {
	if maxRunes < 1 {
		maxRunes = 1
	}
	runes := []rune(s)
	var out []string
	for start := 0; start < len(runes); start += maxRunes {
		end := start + maxRunes
		if end > len(runes) {
			end = len(runes)
		}
		out = append(out, string(runes[start:end]))
	}
	return out
}

// tailWords returns the trailing words of s amounting to roughly n tokens —
// used to build the overlap carried into the next chunk.
// Example: tailWords("the quick brown fox", 2, ApproxTokens) → "brown fox" (≈2 tokens).
func tailWords(s string, n int, count TokenCounter) string {
	if n <= 0 {
		return ""
	}
	words := strings.Fields(s)
	total := 0
	i := len(words)
	for i > 0 && total < n {
		i--
		total += count(words[i])
	}
	return strings.Join(words[i:], " ")
}
