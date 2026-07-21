// Package eval computes retrieval-quality metrics against a judged golden
// dataset. It knows nothing about HTTP, embeddings providers, or the
// database — it takes retrieval.Result slices and judgments and returns
// numbers, which is what makes it independently unit-testable with
// hand-built fixtures instead of a live corpus.
package eval

import "github.com/michal-kalina/garden-of-knowledge/backend/internal/retrieval"

// Judgment identifies a chunk judged relevant for a query, by the
// (filename, heading) pair rather than a chunk id — ids are ingestion-order
// dependent and would break every time the corpus is re-ingested; heading
// text is stable as long as the source document's headings don't change.
type Judgment struct {
	Filename string `json:"filename"`
	Heading  string `json:"heading"`
}

// GoldenCase is one query with its relevance judgments.
type GoldenCase struct {
	ID       string     `json:"id"`
	Query    string     `json:"query"`
	Relevant []Judgment `json:"relevant"`
}

// GoldenSet is the on-disk format of docs/eval/golden.json.
type GoldenSet struct {
	CorpusNote string       `json:"corpus_note"`
	Cases      []GoldenCase `json:"cases"`
}

func isRelevant(r retrieval.Result, judgments []Judgment) bool {
	for _, j := range judgments {
		if r.Filename == j.Filename && r.Heading == j.Heading {
			return true
		}
	}
	return false
}

// RecallAtK reports the fraction of judged-relevant chunks that appear
// anywhere in the top k results. With more than one relevant chunk per
// query (common here — a topic often lives in both the README and its ADR)
// this is a meaningfully different signal from "did we get any hit at all":
// it rewards a retriever that surfaces every relevant source, not just one.
func RecallAtK(results []retrieval.Result, judgments []Judgment, k int) float64 {
	if len(judgments) == 0 {
		return 0
	}
	if k > len(results) {
		k = len(results)
	}
	found := 0
	for _, j := range judgments {
		for i := 0; i < k; i++ {
			if isRelevant(results[i], []Judgment{j}) {
				found++
				break
			}
		}
	}
	return float64(found) / float64(len(judgments))
}

// ReciprocalRank is 1/rank of the first relevant result, or 0 if none of the
// results are relevant. Averaged across queries this is MRR — unlike
// recall, it punishes a retriever that finds the right answer but buries it
// on page two, which matters here because chat only feeds the top few
// chunks (contextLimit) to the model.
func ReciprocalRank(results []retrieval.Result, judgments []Judgment) float64 {
	for i, r := range results {
		if isRelevant(r, judgments) {
			return 1.0 / float64(i+1)
		}
	}
	return 0
}

// CaseResult is one query's computed metrics, kept alongside its raw
// results so a report can show not just numbers but *why* — which sources
// were retrieved and at what rank.
type CaseResult struct {
	Case       GoldenCase
	Results    []retrieval.Result
	Recall     map[int]float64 // keyed by k
	Reciprocal float64
}

// Report aggregates per-case results into means — the numbers that belong
// in a README or a "before/after reranking" comparison.
type Report struct {
	Cases      []CaseResult
	MeanRecall map[int]float64
	MeanRR     float64 // Mean Reciprocal Rank across all cases
}

// Ks are the cutoffs reported by default. 1 shows "best case", 10 shows
// coverage close to what a generous contextLimit would use; the chat
// service itself uses 6 (see internal/chat), so 5 approximates its window.
var Ks = []int{1, 3, 5, 10}

// Evaluate scores every case in the golden set against results obtained by
// calling search once per query. search is injected so the harness (which
// knows about the database and embeddings provider) stays separate from
// this package's pure scoring logic.
func Evaluate(cases []GoldenCase, search func(query string, limit int) ([]retrieval.Result, error)) (Report, error) {
	maxK := 0
	for _, k := range Ks {
		if k > maxK {
			maxK = k
		}
	}

	var report Report
	report.MeanRecall = map[int]float64{}

	for _, c := range cases {
		results, err := search(c.Query, maxK)
		if err != nil {
			return Report{}, err
		}
		cr := CaseResult{
			Case:       c,
			Results:    results,
			Recall:     map[int]float64{},
			Reciprocal: ReciprocalRank(results, c.Relevant),
		}
		for _, k := range Ks {
			cr.Recall[k] = RecallAtK(results, c.Relevant, k)
		}
		report.Cases = append(report.Cases, cr)
		report.MeanRR += cr.Reciprocal
	}

	n := float64(len(cases))
	if n > 0 {
		report.MeanRR /= n
		for _, k := range Ks {
			var sum float64
			for _, cr := range report.Cases {
				sum += cr.Recall[k]
			}
			report.MeanRecall[k] = sum / n
		}
	}
	return report, nil
}
