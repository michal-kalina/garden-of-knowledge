package chunking

import (
	"strings"
	"testing"
)

// wordCount is a deterministic TokenCounter for tests: 1 word = 1 token.
// It makes budgets in test cases readable ("target 6" = six words).
func wordCount(s string) int {
	return len(strings.Fields(s))
}

func page(n int) *int { return &n }

func para(text string, p *int) Block  { return Block{Type: "paragraph", Text: text, Page: p} }
func head(text string) Block          { return Block{Type: "heading", Text: text} }
func table(text string, p *int) Block { return Block{Type: "table", Text: text, Page: p} }

func TestApproxTokens(t *testing.T) {
	if got := ApproxTokens(""); got != 0 {
		t.Errorf("ApproxTokens(\"\") = %d, want 0", got)
	}
	// 8 bytes -> 2 tokens; 9 bytes -> 3 tokens (ceiling).
	if got := ApproxTokens("12345678"); got != 2 {
		t.Errorf("ApproxTokens(8 bytes) = %d, want 2", got)
	}
	if got := ApproxTokens("123456789"); got != 3 {
		t.Errorf("ApproxTokens(9 bytes) = %d, want 3", got)
	}
}

func TestRecursiveChunker(t *testing.T) {
	t.Run("empty and whitespace input yields no chunks", func(t *testing.T) {
		c := RecursiveChunker{TargetTokens: 10, Count: wordCount}
		if got := c.Chunk(nil); len(got) != 0 {
			t.Errorf("Chunk(nil) = %v, want empty", got)
		}
		if got := c.Chunk([]Block{para("   ", nil)}); len(got) != 0 {
			t.Errorf("Chunk(whitespace) = %v, want empty", got)
		}
	})

	t.Run("merges small blocks into one chunk", func(t *testing.T) {
		c := RecursiveChunker{TargetTokens: 10, Count: wordCount}
		got := c.Chunk([]Block{
			para("alpha beta gamma", page(1)),
			para("delta epsilon", page(2)),
		})
		if len(got) != 1 {
			t.Fatalf("got %d chunks, want 1: %#v", len(got), got)
		}
		want := "alpha beta gamma\n\ndelta epsilon"
		if got[0].Content != want {
			t.Errorf("content = %q, want %q", got[0].Content, want)
		}
		if got[0].StartPage == nil || *got[0].StartPage != 1 || got[0].EndPage == nil || *got[0].EndPage != 2 {
			t.Errorf("pages = %v..%v, want 1..2", got[0].StartPage, got[0].EndPage)
		}
	})

	t.Run("splits at block boundary and carries overlap", func(t *testing.T) {
		c := RecursiveChunker{TargetTokens: 6, OverlapTokens: 1, Count: wordCount}
		got := c.Chunk([]Block{
			para("one two three four five", page(1)),
			para("six seven eight nine ten", page(2)),
		})
		if len(got) != 2 {
			t.Fatalf("got %d chunks, want 2: %#v", len(got), got)
		}
		// The second chunk must start with the tail of the first (the overlap).
		if !strings.HasPrefix(got[1].Content, "five") {
			t.Errorf("chunk 2 = %q, want overlap prefix %q", got[1].Content, "five")
		}
		if !strings.Contains(got[1].Content, "six seven eight nine ten") {
			t.Errorf("chunk 2 = %q, missing second paragraph", got[1].Content)
		}
	})

	t.Run("splits an oversized block along sentences", func(t *testing.T) {
		c := RecursiveChunker{TargetTokens: 4, OverlapTokens: 0, Count: wordCount}
		got := c.Chunk([]Block{
			para("aa bb cc. dd ee ff. gg hh", nil),
		})
		if len(got) < 2 {
			t.Fatalf("got %d chunks, want >= 2: %#v", len(got), got)
		}
		// No characters may be lost: rejoined chunks contain every word once
		// (overlap disabled in this case).
		all := strings.Join([]string{got[0].Content, got[1].Content, strings.Join(contents(got[2:]), " ")}, " ")
		for _, w := range []string{"aa", "bb", "cc.", "dd", "ee", "ff.", "gg", "hh"} {
			if !strings.Contains(all, w) {
				t.Errorf("word %q lost in split output %q", w, all)
			}
		}
	})

	t.Run("hard-splits a single enormous token", func(t *testing.T) {
		// Char-based counter so one 100-char "word" really exceeds the budget.
		c := RecursiveChunker{TargetTokens: 5, OverlapTokens: 0, Count: ApproxTokens}
		blob := strings.Repeat("x", 100) // 25 approx tokens
		got := c.Chunk([]Block{para(blob, nil)})
		if len(got) < 2 {
			t.Fatalf("got %d chunks, want >= 2", len(got))
		}
		var rejoined strings.Builder
		for _, ch := range got {
			rejoined.WriteString(strings.ReplaceAll(ch.Content, "\n\n", ""))
		}
		if rejoined.String() != blob {
			t.Errorf("rejoined hard-split lost characters: len %d, want %d", rejoined.Len(), len(blob))
		}
	})

	t.Run("does not emit a trailing overlap-only chunk", func(t *testing.T) {
		c := RecursiveChunker{TargetTokens: 5, OverlapTokens: 2, Count: wordCount}
		got := c.Chunk([]Block{para("one two three four five", nil)})
		if len(got) != 1 {
			t.Fatalf("got %d chunks, want 1: %#v", len(got), got)
		}
	})

	t.Run("defaults are applied", func(t *testing.T) {
		got := RecursiveChunker{}.Chunk([]Block{para("hello world", nil)})
		if len(got) != 1 || got[0].Content != "hello world" {
			t.Errorf("unexpected result with defaults: %#v", got)
		}
	})
}

func TestStructureChunker(t *testing.T) {
	t.Run("prefixes chunks with their section heading", func(t *testing.T) {
		c := StructureChunker{TargetTokens: 20, Count: wordCount}
		got := c.Chunk([]Block{
			head("Termination penalties"),
			para("Late delivery costs five percent.", page(3)),
		})
		if len(got) != 1 {
			t.Fatalf("got %d chunks, want 1: %#v", len(got), got)
		}
		want := "Termination penalties\n\nLate delivery costs five percent."
		if got[0].Content != want {
			t.Errorf("content = %q, want %q", got[0].Content, want)
		}
		if got[0].Heading != "Termination penalties" {
			t.Errorf("heading = %q", got[0].Heading)
		}
	})

	t.Run("a new heading starts a new chunk", func(t *testing.T) {
		c := StructureChunker{TargetTokens: 50, Count: wordCount}
		got := c.Chunk([]Block{
			head("Payments"),
			para("Invoices are due monthly.", nil),
			head("Liability"),
			para("Liability is capped.", nil),
		})
		if len(got) != 2 {
			t.Fatalf("got %d chunks, want 2: %#v", len(got), got)
		}
		if got[0].Heading != "Payments" || got[1].Heading != "Liability" {
			t.Errorf("headings = %q, %q", got[0].Heading, got[1].Heading)
		}
	})

	t.Run("an oversized section splits but every piece keeps the heading", func(t *testing.T) {
		c := StructureChunker{TargetTokens: 6, Count: wordCount}
		got := c.Chunk([]Block{
			head("Rules"),
			para("one two three four five", nil),
			para("six seven eight nine ten", nil),
		})
		if len(got) != 2 {
			t.Fatalf("got %d chunks, want 2: %#v", len(got), got)
		}
		for i, ch := range got {
			if !strings.HasPrefix(ch.Content, "Rules\n\n") {
				t.Errorf("chunk %d = %q, missing heading prefix", i, ch.Content)
			}
			if ch.Heading != "Rules" {
				t.Errorf("chunk %d heading = %q", i, ch.Heading)
			}
		}
	})

	t.Run("tables are never split even when oversized", func(t *testing.T) {
		c := StructureChunker{TargetTokens: 4, Count: wordCount}
		bigTable := "r1c1 r1c2 r2c1 r2c2 r3c1 r3c2 r4c1 r4c2" // 8 tokens > 4
		got := c.Chunk([]Block{
			head("Pricing"),
			para("Intro line.", nil),
			table(bigTable, page(7)),
		})
		var tableChunk *Chunk
		for i := range got {
			if strings.Contains(got[i].Content, "r4c2") {
				tableChunk = &got[i]
			}
		}
		if tableChunk == nil {
			t.Fatalf("table chunk not found in %#v", got)
		}
		if !strings.Contains(tableChunk.Content, bigTable) {
			t.Errorf("table was split: %q", tableChunk.Content)
		}
		if tableChunk.StartPage == nil || *tableChunk.StartPage != 7 {
			t.Errorf("table page = %v, want 7", tableChunk.StartPage)
		}
	})

	t.Run("documents without headings still chunk", func(t *testing.T) {
		c := StructureChunker{TargetTokens: 10, Count: wordCount}
		got := c.Chunk([]Block{para("plain text with no structure", nil)})
		if len(got) != 1 || got[0].Heading != "" {
			t.Fatalf("unexpected: %#v", got)
		}
		if got[0].Content != "plain text with no structure" {
			t.Errorf("content = %q", got[0].Content)
		}
	})
}

func contents(chunks []Chunk) []string {
	out := make([]string, len(chunks))
	for i, c := range chunks {
		out[i] = c.Content
	}
	return out
}
