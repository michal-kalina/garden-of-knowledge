package chunking

import "strings"

// RecursiveChunker is the baseline strategy: it merges consecutive blocks
// until the token budget is reached, recursively splitting any single block
// that alone exceeds the budget. A configurable overlap is carried from each
// chunk into the next, so a thought cut at a boundary survives intact in at
// least one chunk.
//
// It ignores document structure by design — it is the control group the
// structure-aware strategy gets measured against in Phase 4 evals.
type RecursiveChunker struct {
	// TargetTokens is the soft chunk size budget. Default 500.
	TargetTokens int
	// OverlapTokens is carried from the end of one chunk into the start of
	// the next. Must be well below TargetTokens. Default 50.
	OverlapTokens int
	// Count estimates tokens; defaults to ApproxTokens.
	Count TokenCounter
}

// compile-time check that RecursiveChunker implements Chunker
var _ Chunker = RecursiveChunker{}

func (c RecursiveChunker) params() (target, overlap int, count TokenCounter) {
	target, overlap, count = c.TargetTokens, c.OverlapTokens, c.Count
	if target <= 0 {
		target = 500
	}
	if overlap < 0 {
		overlap = 0
	}
	if c.TargetTokens <= 0 && c.OverlapTokens == 0 {
		overlap = 50
	}
	// Overlap approaching the target would make chunks mostly duplicates.
	if overlap > target/4 {
		overlap = target / 4
	}
	if count == nil {
		count = ApproxTokens
	}
	return target, overlap, count
}

func (c RecursiveChunker) Chunk(blocks []Block) []Chunk {
	target, overlap, count := c.params()

	var chunks []Chunk
	var cur []string
	curCount := 0
	seedOnly := false // true while cur holds nothing but the overlap seed
	var startPage, endPage *int

	flush := func() {
		if len(cur) == 0 {
			return
		}
		content := strings.Join(cur, "\n\n")
		chunks = append(chunks, Chunk{
			Content:   content,
			StartPage: startPage,
			EndPage:   endPage,
		})
		cur = nil
		curCount = 0
		// Seed the next chunk with the overlap tail. Its page attribution
		// stays with the page the previous chunk ended on.
		if tail := tailWords(content, overlap, count); tail != "" {
			cur = []string{tail}
			curCount = count(tail)
			seedOnly = true
			startPage = endPage
		} else {
			startPage, endPage = nil, nil
		}
	}

	for _, b := range blocks {
		text := strings.TrimSpace(b.Text)
		if text == "" {
			continue
		}
		pieces := []string{text}
		if count(text) > target {
			pieces = splitText(text, target, count)
		}
		for _, p := range pieces {
			pc := count(p)
			if curCount+pc > target && len(cur) > 0 {
				flush()
			}
			cur = append(cur, p)
			curCount += pc
			seedOnly = false
			if b.Page != nil {
				if startPage == nil {
					startPage = b.Page
				}
				endPage = b.Page
			}
		}
	}

	// Final flush — but never emit a chunk that is only the overlap seed,
	// as it would be a pure duplicate of the previous chunk's tail.
	if len(cur) > 0 && !seedOnly {
		chunks = append(chunks, Chunk{
			Content:   strings.Join(cur, "\n\n"),
			StartPage: startPage,
			EndPage:   endPage,
		})
	}
	return chunks
}
