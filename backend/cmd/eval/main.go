// Command eval runs the retrieval-quality harness against docs/eval/golden.json
// and a real database, printing a Markdown report of recall@k and MRR.
//
// It deliberately does not reuse internal/config.Load: that config demands
// AUTH_SECRET and other chat/API concerns unrelated to a read-only scoring
// script. The eval harness only needs a database and an embeddings provider,
// so it reads exactly those env vars directly — the narrower footprint is
// the point, not an oversight.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/michal-kalina/garden-of-knowledge/backend/internal/database"
	"github.com/michal-kalina/garden-of-knowledge/backend/internal/embeddings"
	"github.com/michal-kalina/garden-of-knowledge/backend/internal/eval"
	"github.com/michal-kalina/garden-of-knowledge/backend/internal/rerank"
	"github.com/michal-kalina/garden-of-knowledge/backend/internal/retrieval"
	"github.com/michal-kalina/garden-of-knowledge/backend/internal/users"
)

func main() {
	goldenPath := flag.String("golden", "docs/eval/golden.json", "path to the golden dataset JSON")
	userEmail := flag.String("user-email", "", "account that owns the ingested eval corpus (required)")
	outPath := flag.String("out", "", "write the Markdown report here in addition to stdout (optional)")
	rerankEnabled := flag.Bool("rerank", false, "wrap retrieval with a Voyage cross-encoder rerank pass "+
		"(requires VOYAGE_API_KEY regardless of -rerank-model); run once without and once with this flag "+
		"to produce a before/after comparison")
	rerankModel := flag.String("rerank-model", "rerank-2.5", "Voyage rerank model, used only with -rerank")
	flag.Parse()

	if err := run(*goldenPath, *userEmail, *outPath, *rerankEnabled, *rerankModel); err != nil {
		fmt.Fprintln(os.Stderr, "eval failed:", err)
		os.Exit(1)
	}
}

func run(goldenPath, userEmail, outPath string, rerankEnabled bool, rerankModel string) error {
	if userEmail == "" {
		return fmt.Errorf("-user-email is required (the account that owns the ingested README + ADR corpus)")
	}
	if rerankEnabled && os.Getenv("VOYAGE_API_KEY") == "" {
		return fmt.Errorf("-rerank requires VOYAGE_API_KEY (used for the reranking call regardless of EMBEDDINGS_PROVIDER)")
	}

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	db, err := database.Open(ctx, dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	provider := os.Getenv("EMBEDDINGS_PROVIDER")
	if provider == "" {
		if os.Getenv("VOYAGE_API_KEY") != "" {
			provider = "voyage"
		} else {
			provider = "fake"
		}
	}
	if provider == "fake" {
		logger.Warn("EMBEDDINGS_PROVIDER=fake — recall/MRR numbers below measure nothing meaningful; " +
			"set VOYAGE_API_KEY to evaluate real retrieval quality")
	}
	embedder := embeddings.FromProvider(provider, os.Getenv("VOYAGE_API_KEY"),
		envDefault("VOYAGE_MODEL", "voyage-4"), logger)

	rawUser, _, err := users.NewRepository(db).GetByEmail(ctx, userEmail)
	if err != nil {
		return fmt.Errorf("look up eval user %q (has this account ingested the golden corpus?): %w", userEmail, err)
	}

	golden, err := loadGolden(goldenPath)
	if err != nil {
		return err
	}
	logger.Info("loaded golden set", "cases", len(golden.Cases), "note", golden.CorpusNote)

	searcher := buildSearcher(db, embedder, rerankEnabled, rerankModel, os.Getenv("VOYAGE_API_KEY"), logger)
	search := func(query string, limit int) ([]retrieval.Result, error) {
		return searcher.Search(ctx, rawUser.ID, query, limit)
	}

	report, err := eval.Evaluate(golden.Cases, search)
	if err != nil {
		return fmt.Errorf("evaluate: %w", err)
	}

	md := renderMarkdown(report, rerankEnabled, rerankModel)
	fmt.Println(md)

	if outPath != "" {
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return fmt.Errorf("create report directory: %w", err)
		}
		if err := os.WriteFile(outPath, []byte(md), 0o644); err != nil {
			return fmt.Errorf("write report: %w", err)
		}
		logger.Info("report written", "path", outPath)
	}
	return nil
}

// buildSearcher wraps hybrid retrieval with a reranking pass when requested.
// Kept separate from run() so the "which searcher am I actually testing"
// decision is a single, obvious call site — the same shape as the
// production wiring in cmd/api/main.go, deliberately, so the eval measures
// the same code path that serves real chat requests.
func buildSearcher(db *sql.DB, embedder embeddings.Embedder, rerankEnabled bool, rerankModel, voyageKey string,
	logger *slog.Logger) retrieval.Retriever {
	var searcher retrieval.Retriever = retrieval.New(db, embedder)
	if rerankEnabled {
		searcher = &retrieval.Reranked{
			Base:     searcher,
			Reranker: rerank.NewVoyage(voyageKey, rerankModel),
		}
		logger.Info("reranking enabled for this eval run", "model", rerankModel)
	}
	return searcher
}

func loadGolden(path string) (eval.GoldenSet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return eval.GoldenSet{}, fmt.Errorf("read golden set: %w", err)
	}
	var g eval.GoldenSet
	if err := json.Unmarshal(data, &g); err != nil {
		return eval.GoldenSet{}, fmt.Errorf("parse golden set: %w", err)
	}
	if len(g.Cases) == 0 {
		return eval.GoldenSet{}, fmt.Errorf("golden set has no cases")
	}
	return g, nil
}

func envDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// renderMarkdown produces a self-contained report: a summary table plus a
// per-case breakdown showing exactly which sources were retrieved at what
// rank — the detail that turns "recall@5 = 0.83" into something you can act
// on (a specific query that's missing a specific source).
func renderMarkdown(r eval.Report, rerankEnabled bool, rerankModel string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Retrieval eval — %d cases", len(r.Cases))
	if rerankEnabled {
		fmt.Fprintf(&b, " (reranked: %s)", rerankModel)
	} else {
		b.WriteString(" (no reranking)")
	}
	b.WriteString("\n\n")

	ks := make([]int, 0, len(r.MeanRecall))
	for k := range r.MeanRecall {
		ks = append(ks, k)
	}
	sort.Ints(ks)

	b.WriteString("| Metric | Value |\n|---|---|\n")
	fmt.Fprintf(&b, "| MRR | %.3f |\n", r.MeanRR)
	for _, k := range ks {
		fmt.Fprintf(&b, "| Recall@%d | %.3f |\n", k, r.MeanRecall[k])
	}

	b.WriteString("\n## Per-case detail\n\n")
	for _, cr := range r.Cases {
		fmt.Fprintf(&b, "### %s\n\n> %s\n\n", cr.Case.ID, cr.Case.Query)
		fmt.Fprintf(&b, "RR: %.3f", cr.Reciprocal)
		for _, k := range ks {
			fmt.Fprintf(&b, " · Recall@%d: %.2f", k, cr.Recall[k])
		}
		b.WriteString("\n\n")

		b.WriteString("Expected:")
		for _, j := range cr.Case.Relevant {
			fmt.Fprintf(&b, " `%s#%s`", j.Filename, j.Heading)
		}
		b.WriteString("\n\nRetrieved (top 5):\n\n")
		for i, res := range cr.Results {
			if i >= 5 {
				break
			}
			mark := " "
			for _, j := range cr.Case.Relevant {
				if res.Filename == j.Filename && res.Heading == j.Heading {
					mark = "✓"
				}
			}
			if res.RerankScore != nil {
				fmt.Fprintf(&b, "%d. [%s] `%s#%s` (rerank: %.3f)\n", i+1, mark, res.Filename, res.Heading, *res.RerankScore)
			} else {
				fmt.Fprintf(&b, "%d. [%s] `%s#%s`\n", i+1, mark, res.Filename, res.Heading)
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}
