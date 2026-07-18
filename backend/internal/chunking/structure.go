package chunking

import "strings"

// StructureChunker groups blocks into sections delimited by heading blocks.
// Every emitted chunk is prefixed with its section heading, which gives the
// embedding model topical context ("Termination penalties\n\nIn case of
// delay…" retrieves far better than the bare paragraph). Oversized sections
// fall back to the recursive splitter, but each resulting piece keeps the
// heading prefix. Table blocks are atomic: a table is never split, even if
// it alone exceeds the budget — half a table is noise for both retrieval
// and the LLM.
//
// Heading levels are not distinguished yet (the parser contract has a single
// "heading" type); the most recent heading wins.
// TODO(phase-2+): hierarchical headings once the parser reports levels.
type StructureChunker struct {
	// TargetTokens is the soft chunk size budget. Default 500.
	TargetTokens int
	// Count estimates tokens; defaults to ApproxTokens.
	Count TokenCounter
}

// compile-time check that StructureChunker implements Chunker
var _ Chunker = StructureChunker{}

func (c StructureChunker) params() (target int, count TokenCounter) {
	target = c.TargetTokens
	if target <= 0 {
		target = 500
	}
	count = c.Count
	if count == nil {
		count = ApproxTokens
	}
	return target, count
}

func (c StructureChunker) Chunk(blocks []Block) []Chunk {
	target, count := c.params()

	var chunks []Chunk
	heading := ""
	var body []string
	bodyCount := 0
	var startPage, endPage *int

	emit := func(content string, sp, ep *int) {
		content = strings.TrimSpace(content)
		if content == "" {
			return
		}
		if heading != "" {
			content = heading + "\n\n" + content
		}
		chunks = append(chunks, Chunk{
			Content:   content,
			Heading:   heading,
			StartPage: sp,
			EndPage:   ep,
		})
	}

	flush := func() {
		if len(body) == 0 {
			return
		}
		emit(strings.Join(body, "\n\n"), startPage, endPage)
		body = nil
		bodyCount = 0
		startPage, endPage = nil, nil
	}

	addPiece := func(p string, pc int, page *int) {
		if bodyCount+pc > target && len(body) > 0 {
			flush()
		}
		body = append(body, p)
		bodyCount += pc
		if page != nil {
			if startPage == nil {
				startPage = page
			}
			endPage = page
		}
	}

	for _, b := range blocks {
		text := strings.TrimSpace(b.Text)
		if text == "" {
			continue
		}
		switch b.Type {
		case "heading":
			// A heading closes the previous section and opens a new one.
			flush()
			heading = text

		case "table":
			// Atomic: flush what we have, then place the table whole — in
			// the current chunk if it fits, otherwise as its own chunk even
			// when oversized.
			tc := count(text)
			if tc > target {
				flush()
				emit(text, b.Page, b.Page)
				continue
			}
			addPiece(text, tc, b.Page)

		default:
			pieces := []string{text}
			if count(text) > target {
				pieces = splitText(text, target, count)
			}
			for _, p := range pieces {
				addPiece(p, count(p), b.Page)
			}
		}
	}
	flush()
	return chunks
}
