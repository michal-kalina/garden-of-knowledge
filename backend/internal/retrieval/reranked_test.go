package retrieval

import (
	"context"
	"errors"
	"testing"

	"github.com/michal-kalina/garden-of-knowledge/backend/internal/rerank"
)

// fakeBase is a Retriever stand-in that records the limit it was called
// with and returns a fixed candidate list (up to that limit).
type fakeBase struct {
	results    []Result
	calledWith int
	err        error
}

func (f *fakeBase) Search(_ context.Context, _, _ string, limit int) ([]Result, error) {
	f.calledWith = limit
	if f.err != nil {
		return nil, f.err
	}
	if limit < len(f.results) {
		return f.results[:limit], nil
	}
	return f.results, nil
}

// fixedOrder is a rerank.Reranker stand-in that returns a specified
// permutation of indices — lets a test assert the decorator actually
// reorders by what the reranker says, not by the base retriever's order.
type fixedOrder struct {
	order []int
	err   error
}

func (f fixedOrder) ModelName() string { return "fixed-order-fake" }

func (f fixedOrder) Rerank(_ context.Context, _ string, documents []string, topK int) ([]rerank.Result, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make([]rerank.Result, 0, len(f.order))
	for i, idx := range f.order {
		out = append(out, rerank.Result{Index: idx, Score: float64(len(f.order) - i)})
	}
	if topK > 0 && topK < len(out) {
		out = out[:topK]
	}
	return out, nil
}

func withContent(chunkID int64, content string) Result {
	return Result{ChunkID: chunkID, Content: content}
}

func TestRerankedSearch(t *testing.T) {
	t.Run("over-fetches from the base retriever by FetchFactor", func(t *testing.T) {
		base := &fakeBase{results: make([]Result, 30)}
		for i := range base.results {
			base.results[i] = withContent(int64(i), "doc")
		}
		r := &Reranked{Base: base, Reranker: rerank.Fake{}, FetchFactor: 4}

		if _, err := r.Search(context.Background(), "u", "q", 5); err != nil {
			t.Fatal(err)
		}
		if base.calledWith != 20 {
			t.Errorf("base called with limit=%d, want 20 (5 * 4)", base.calledWith)
		}
	})

	t.Run("reorders results according to the reranker, not base order", func(t *testing.T) {
		base := &fakeBase{results: []Result{
			withContent(1, "alpha"),
			withContent(2, "beta"),
			withContent(3, "gamma"),
		}}
		// Reranker says gamma > alpha > beta, reversing the base order.
		r := &Reranked{Base: base, Reranker: fixedOrder{order: []int{2, 0, 1}}}

		got, err := r.Search(context.Background(), "u", "q", 3)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 3 || got[0].ChunkID != 3 || got[1].ChunkID != 1 || got[2].ChunkID != 2 {
			t.Fatalf("got = %+v", got)
		}
		if got[0].RerankScore == nil || *got[0].RerankScore != 3 {
			t.Errorf("RerankScore = %v, want 3", got[0].RerankScore)
		}
	})

	t.Run("truncates to the reranker's topK", func(t *testing.T) {
		base := &fakeBase{results: []Result{
			withContent(1, "a"), withContent(2, "b"), withContent(3, "c"), withContent(4, "d"),
		}}
		r := &Reranked{Base: base, Reranker: rerank.Fake{}}

		got, err := r.Search(context.Background(), "u", "q", 2)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2", len(got))
		}
	})

	t.Run("empty base results skip the reranker entirely", func(t *testing.T) {
		base := &fakeBase{results: nil}
		r := &Reranked{Base: base, Reranker: fixedOrder{err: errors.New("must not be called")}}

		got, err := r.Search(context.Background(), "u", "q", 5)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 0 {
			t.Errorf("got = %+v, want empty", got)
		}
	})

	t.Run("propagates base retriever errors", func(t *testing.T) {
		boom := errors.New("boom")
		base := &fakeBase{err: boom}
		r := &Reranked{Base: base, Reranker: rerank.Fake{}}
		if _, err := r.Search(context.Background(), "u", "q", 5); !errors.Is(err, boom) {
			t.Fatalf("err = %v, want %v", err, boom)
		}
	})

	t.Run("propagates reranker errors", func(t *testing.T) {
		boom := errors.New("boom")
		base := &fakeBase{results: []Result{withContent(1, "a")}}
		r := &Reranked{Base: base, Reranker: fixedOrder{err: boom}}
		if _, err := r.Search(context.Background(), "u", "q", 5); !errors.Is(err, boom) {
			t.Fatalf("err = %v, want %v", err, boom)
		}
	})

	t.Run("rejects an out-of-range index from the reranker", func(t *testing.T) {
		base := &fakeBase{results: []Result{withContent(1, "a")}}
		r := &Reranked{Base: base, Reranker: fixedOrder{order: []int{99}}}
		if _, err := r.Search(context.Background(), "u", "q", 5); err == nil {
			t.Fatal("expected error for out-of-range index")
		}
	})

	t.Run("defaults FetchFactor and limit when unset", func(t *testing.T) {
		base := &fakeBase{results: make([]Result, 10)}
		r := &Reranked{Base: base, Reranker: rerank.Fake{}}
		if _, err := r.Search(context.Background(), "u", "q", 0); err != nil {
			t.Fatal(err)
		}
		if base.calledWith != DefaultLimit*4 {
			t.Errorf("calledWith = %d, want %d", base.calledWith, DefaultLimit*4)
		}
	})
}
