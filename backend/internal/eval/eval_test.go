package eval

import (
	"errors"
	"testing"

	"github.com/michal-kalina/garden-of-knowledge/backend/internal/retrieval"
)

func r(filename, heading string) retrieval.Result {
	return retrieval.Result{Filename: filename, Heading: heading}
}

func TestRecallAtK(t *testing.T) {
	results := []retrieval.Result{
		r("a.md", "Intro"),
		r("b.md", "Setup"),
		r("a.md", "Decision"),
		r("c.md", "Other"),
	}

	t.Run("single relevant chunk found within k", func(t *testing.T) {
		got := RecallAtK(results, []Judgment{{Filename: "a.md", Heading: "Decision"}}, 3)
		if got != 1.0 {
			t.Errorf("recall@3 = %v, want 1.0", got)
		}
	})

	t.Run("relevant chunk outside k is not counted", func(t *testing.T) {
		got := RecallAtK(results, []Judgment{{Filename: "a.md", Heading: "Decision"}}, 2)
		if got != 0.0 {
			t.Errorf("recall@2 = %v, want 0.0", got)
		}
	})

	t.Run("partial recall with two judgments, one found", func(t *testing.T) {
		judgments := []Judgment{
			{Filename: "a.md", Heading: "Decision"}, // rank 3, within k=3
			{Filename: "z.md", Heading: "Missing"},  // not present at all
		}
		got := RecallAtK(results, judgments, 3)
		if got != 0.5 {
			t.Errorf("recall@3 = %v, want 0.5", got)
		}
	})

	t.Run("both judgments found gives full recall", func(t *testing.T) {
		judgments := []Judgment{
			{Filename: "a.md", Heading: "Intro"},
			{Filename: "b.md", Heading: "Setup"},
		}
		if got := RecallAtK(results, judgments, 4); got != 1.0 {
			t.Errorf("recall@4 = %v, want 1.0", got)
		}
	})

	t.Run("no judgments is defined as zero, not divide-by-zero", func(t *testing.T) {
		if got := RecallAtK(results, nil, 5); got != 0 {
			t.Errorf("recall with no judgments = %v, want 0", got)
		}
	})

	t.Run("k larger than results list is clamped", func(t *testing.T) {
		got := RecallAtK(results, []Judgment{{Filename: "c.md", Heading: "Other"}}, 100)
		if got != 1.0 {
			t.Errorf("recall@100 (clamped) = %v, want 1.0", got)
		}
	})
}

func TestReciprocalRank(t *testing.T) {
	results := []retrieval.Result{
		r("a.md", "Intro"),
		r("b.md", "Setup"),
		r("a.md", "Decision"),
	}

	t.Run("relevant result at rank 2 gives RR 0.5", func(t *testing.T) {
		got := ReciprocalRank(results, []Judgment{{Filename: "b.md", Heading: "Setup"}})
		if got != 0.5 {
			t.Errorf("RR = %v, want 0.5", got)
		}
	})

	t.Run("relevant result at rank 1 gives RR 1.0", func(t *testing.T) {
		got := ReciprocalRank(results, []Judgment{{Filename: "a.md", Heading: "Intro"}})
		if got != 1.0 {
			t.Errorf("RR = %v, want 1.0", got)
		}
	})

	t.Run("no relevant result gives RR 0", func(t *testing.T) {
		got := ReciprocalRank(results, []Judgment{{Filename: "z.md", Heading: "Nope"}})
		if got != 0 {
			t.Errorf("RR = %v, want 0", got)
		}
	})

	t.Run("first matching judgment among several wins", func(t *testing.T) {
		judgments := []Judgment{
			{Filename: "a.md", Heading: "Decision"}, // rank 3
			{Filename: "b.md", Heading: "Setup"},     // rank 2 — should win
		}
		got := ReciprocalRank(results, judgments)
		if got != 0.5 {
			t.Errorf("RR = %v, want 0.5 (best-ranked judgment)", got)
		}
	})
}

func TestEvaluate(t *testing.T) {
	cases := []GoldenCase{
		{
			ID:       "q1",
			Query:    "setup",
			Relevant: []Judgment{{Filename: "b.md", Heading: "Setup"}},
		},
		{
			ID:       "q2",
			Query:    "missing",
			Relevant: []Judgment{{Filename: "z.md", Heading: "Nope"}},
		},
	}

	fakeResults := []retrieval.Result{r("a.md", "Intro"), r("b.md", "Setup")}
	search := func(query string, limit int) ([]retrieval.Result, error) {
		if limit < len(fakeResults) {
			return fakeResults[:limit], nil
		}
		return fakeResults, nil
	}

	report, err := Evaluate(cases, search)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Cases) != 2 {
		t.Fatalf("cases = %d, want 2", len(report.Cases))
	}
	// q1 finds its relevant chunk at rank 2 (RR 0.5, recall@3 = 1.0);
	// q2 finds nothing (RR 0, recall@3 = 0). Mean RR = 0.25.
	if report.MeanRR != 0.25 {
		t.Errorf("MeanRR = %v, want 0.25", report.MeanRR)
	}
	if report.MeanRecall[3] != 0.5 {
		t.Errorf("MeanRecall[3] = %v, want 0.5", report.MeanRecall[3])
	}

	t.Run("propagates search errors", func(t *testing.T) {
		boom := errors.New("boom")
		_, err := Evaluate(cases, func(string, int) ([]retrieval.Result, error) { return nil, boom })
		if !errors.Is(err, boom) {
			t.Fatalf("err = %v, want %v", err, boom)
		}
	})

	t.Run("empty case list yields zeroed report without panicking", func(t *testing.T) {
		report, err := Evaluate(nil, search)
		if err != nil {
			t.Fatal(err)
		}
		if report.MeanRR != 0 || len(report.Cases) != 0 {
			t.Errorf("report = %+v, want zero value", report)
		}
	})
}
