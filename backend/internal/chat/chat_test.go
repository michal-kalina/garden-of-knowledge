package chat

import (
	"context"
	"strings"
	"testing"

	"github.com/michal-kalina/garden-of-knowledge/backend/internal/llm"
	"github.com/michal-kalina/garden-of-knowledge/backend/internal/retrieval"
)

type fakeRetriever struct {
	results []retrieval.Result
	query   string
}

func (f *fakeRetriever) Search(_ context.Context, query string, _ int) ([]retrieval.Result, error) {
	f.query = query
	return f.results, nil
}

type fakeLLM struct {
	system string
	user   string
	called bool
	reply  string
}

func (f *fakeLLM) Stream(_ context.Context, system string, msgs []llm.Message, _ int,
	onDelta func(string) error) (string, error) {
	f.called = true
	f.system = system
	f.user = msgs[len(msgs)-1].Content
	for _, part := range strings.SplitAfter(f.reply, " ") {
		if err := onDelta(part); err != nil {
			return "", err
		}
	}
	return f.reply, nil
}

func page(n int) *int { return &n }

func TestAsk(t *testing.T) {
	t.Run("numbers sources, includes metadata in prompt, streams answer", func(t *testing.T) {
		ret := &fakeRetriever{results: []retrieval.Result{
			{ChunkID: 10, Filename: "handbook.md", Heading: "Leave",
				PageStart: page(2), PageEnd: page(3), Content: "26 days of paid leave."},
			{ChunkID: 11, Filename: "handbook.md", Content: "Remote work 3 days weekly."},
		}}
		model := &fakeLLM{reply: "You get 26 days [1]."}
		svc := New(ret, model)

		var gotSources []Source
		var streamed strings.Builder
		err := svc.Ask(context.Background(), "how many vacation days?",
			func(s []Source) error { gotSources = s; return nil },
			func(d string) error { streamed.WriteString(d); return nil })
		if err != nil {
			t.Fatal(err)
		}

		if ret.query != "how many vacation days?" {
			t.Errorf("retriever query = %q", ret.query)
		}
		if len(gotSources) != 2 || gotSources[0].Index != 1 || gotSources[1].Index != 2 {
			t.Fatalf("sources = %+v", gotSources)
		}
		if streamed.String() != "You get 26 days [1]." {
			t.Errorf("streamed = %q", streamed.String())
		}

		for _, want := range []string{
			"[1] handbook.md — Leave (pages 2–3)",
			"26 days of paid leave.",
			"[2] handbook.md",
			"Question: how many vacation days?",
		} {
			if !strings.Contains(model.user, want) {
				t.Errorf("user prompt missing %q; prompt:\n%s", want, model.user)
			}
		}
		if !strings.Contains(model.system, "Cite every factual claim") {
			t.Errorf("system prompt missing citation rule")
		}
	})

	t.Run("no results: streams fallback without calling the model", func(t *testing.T) {
		ret := &fakeRetriever{}
		model := &fakeLLM{}
		svc := New(ret, model)

		var sources []Source
		var streamed strings.Builder
		err := svc.Ask(context.Background(), "anything",
			func(s []Source) error { sources = s; return nil },
			func(d string) error { streamed.WriteString(d); return nil })
		if err != nil {
			t.Fatal(err)
		}
		if model.called {
			t.Error("LLM must not be called with zero sources")
		}
		if len(sources) != 0 {
			t.Errorf("sources = %+v, want empty", sources)
		}
		if !strings.Contains(streamed.String(), "could not find anything") {
			t.Errorf("fallback not streamed: %q", streamed.String())
		}
	})
}
