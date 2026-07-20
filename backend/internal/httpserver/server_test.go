package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"github.com/michal-kalina/garden-of-knowledge/backend/internal/chat"
	"github.com/michal-kalina/garden-of-knowledge/backend/internal/documents"
	"github.com/michal-kalina/garden-of-knowledge/backend/internal/retrieval"
)

// fakeDocs is an in-memory DocumentService used to test handlers in isolation.
type fakeDocs struct {
	docs map[string]documents.Document
}

func (f *fakeDocs) Upload(_ context.Context, in documents.UploadInput) (documents.Document, error) {
	// Drain the reader like the real service would.
	if _, err := io.Copy(io.Discard, in.Content); err != nil {
		return documents.Document{}, err
	}
	d := documents.Document{
		ID:          "11111111-1111-1111-1111-111111111111",
		Filename:    in.Filename,
		ContentType: in.ContentType,
		SizeBytes:   in.Size,
		Status:      "pending",
	}
	f.docs[d.ID] = d
	return d, nil
}

func (f *fakeDocs) List(context.Context) ([]documents.Document, error) {
	out := []documents.Document{}
	for _, d := range f.docs {
		out = append(out, d)
	}
	return out, nil
}

func (f *fakeDocs) Get(_ context.Context, id string) (documents.Document, error) {
	d, ok := f.docs[id]
	if !ok {
		return documents.Document{}, documents.ErrNotFound
	}
	return d, nil
}

// fakeSearch returns canned results and records the last query.
type fakeSearch struct {
	lastQuery string
	lastLimit int
	results   []retrieval.Result
}

func (f *fakeSearch) Search(_ context.Context, query string, limit int) ([]retrieval.Result, error) {
	f.lastQuery, f.lastLimit = query, limit
	return f.results, nil
}

func newTestServer(t *testing.T) (http.Handler, *fakeDocs) {
	t.Helper()
	h, fake, _ := newTestServerWithSearch(t)
	return h, fake
}

func newTestServerWithSearch(t *testing.T) (http.Handler, *fakeDocs, *fakeSearch) {
	t.Helper()
	fake := &fakeDocs{docs: map[string]documents.Document{}}
	search := &fakeSearch{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := New(Deps{
		// DB is only used by /readyz, which these tests do not exercise.
		Documents:      fake,
		Search:         search,
		Chat:           &fakeChat{},
		MaxUploadBytes: 1 << 20, // 1 MiB limit for tests
		Logger:         logger,
	})
	return h, fake, search
}

// fakeChat emits one sources event and two deltas.
type fakeChat struct{ fail bool }

func (f *fakeChat) Ask(_ context.Context, question string,
	onSources func([]chat.Source) error, onDelta func(string) error) error {
	if err := onSources([]chat.Source{{Index: 1, ChunkID: 42, Filename: "a.md"}}); err != nil {
		return err
	}
	if err := onDelta("Hello "); err != nil {
		return err
	}
	if f.fail {
		return context.DeadlineExceeded
	}
	return onDelta("[1]")
}

// multipartBody builds a multipart request body with a single "file" part.
func multipartBody(t *testing.T, filename, contentType, content string) (io.Reader, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
	h.Set("Content-Type", contentType)
	part, err := w.CreatePart(h)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf, w.FormDataContentType()
}

func TestDocumentUpload(t *testing.T) {
	t.Run("accepts markdown and returns 201", func(t *testing.T) {
		srv, _ := newTestServer(t)
		body, ct := multipartBody(t, "notes.md", "text/markdown", "# hello")
		req := httptest.NewRequest(http.MethodPost, "/documents", body)
		req.Header.Set("Content-Type", ct)
		rec := httptest.NewRecorder()

		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201; body: %s", rec.Code, rec.Body)
		}
		var doc documents.Document
		if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
			t.Fatal(err)
		}
		if doc.Filename != "notes.md" || doc.Status != "pending" {
			t.Errorf("unexpected document: %+v", doc)
		}
	})

	t.Run("rejects unsupported content type with 415", func(t *testing.T) {
		srv, _ := newTestServer(t)
		body, ct := multipartBody(t, "cat.gif", "image/gif", "GIF89a")
		req := httptest.NewRequest(http.MethodPost, "/documents", body)
		req.Header.Set("Content-Type", ct)
		rec := httptest.NewRecorder()

		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnsupportedMediaType {
			t.Fatalf("status = %d, want 415", rec.Code)
		}
	})

	t.Run("rejects oversized upload with 413", func(t *testing.T) {
		srv, _ := newTestServer(t)
		big := strings.Repeat("x", 2<<20) // 2 MiB > 1 MiB test limit
		body, ct := multipartBody(t, "big.txt", "text/plain", big)
		req := httptest.NewRequest(http.MethodPost, "/documents", body)
		req.Header.Set("Content-Type", ct)
		rec := httptest.NewRecorder()

		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("status = %d, want 413", rec.Code)
		}
	})

	t.Run("rejects missing file field with 400", func(t *testing.T) {
		srv, _ := newTestServer(t)
		req := httptest.NewRequest(http.MethodPost, "/documents", strings.NewReader("nope"))
		req.Header.Set("Content-Type", "multipart/form-data; boundary=xyz")
		rec := httptest.NewRecorder()

		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
	})
}

func TestDocumentGet(t *testing.T) {
	srv, fake := newTestServer(t)
	fake.docs["11111111-1111-1111-1111-111111111111"] = documents.Document{
		ID: "11111111-1111-1111-1111-111111111111", Filename: "a.pdf", Status: "ready",
	}

	t.Run("returns document", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/documents/11111111-1111-1111-1111-111111111111", nil)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
	})

	t.Run("returns 404 for unknown id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/documents/22222222-2222-2222-2222-222222222222", nil)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rec.Code)
		}
	})
}

func TestSearch(t *testing.T) {
	t.Run("returns results and passes query through", func(t *testing.T) {
		srv, _, search := newTestServerWithSearch(t)
		search.results = []retrieval.Result{{ChunkID: 7, Content: "hit", Score: 0.03}}

		req := httptest.NewRequest(http.MethodPost, "/search",
			strings.NewReader(`{"query":"vacation days","limit":5}`))
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
		}
		if search.lastQuery != "vacation days" || search.lastLimit != 5 {
			t.Errorf("service got query=%q limit=%d", search.lastQuery, search.lastLimit)
		}
		var resp struct {
			Results []retrieval.Result `json:"results"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		if len(resp.Results) != 1 || resp.Results[0].ChunkID != 7 {
			t.Errorf("unexpected results: %+v", resp.Results)
		}
	})

	t.Run("rejects empty query with 400", func(t *testing.T) {
		srv, _, _ := newTestServerWithSearch(t)
		req := httptest.NewRequest(http.MethodPost, "/search", strings.NewReader(`{"query":"  "}`))
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("rejects malformed JSON with 400", func(t *testing.T) {
		srv, _, _ := newTestServerWithSearch(t)
		req := httptest.NewRequest(http.MethodPost, "/search", strings.NewReader(`{`))
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
	})
}

func TestChat(t *testing.T) {
	newChatServer := func(c ChatService) http.Handler {
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		return New(Deps{
			Documents:      &fakeDocs{docs: map[string]documents.Document{}},
			Search:         &fakeSearch{},
			Chat:           c,
			MaxUploadBytes: 1 << 20,
			Logger:         logger,
		})
	}

	t.Run("streams sources, deltas and done as SSE", func(t *testing.T) {
		srv := newChatServer(&fakeChat{})
		req := httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(`{"query":"hi"}`))
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d; body: %s", rec.Code, rec.Body)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
			t.Errorf("content type = %q", ct)
		}
		body := rec.Body.String()
		iSources := strings.Index(body, "event: sources")
		iDelta := strings.Index(body, "event: delta")
		iDone := strings.Index(body, "event: done")
		if iSources == -1 || iDelta == -1 || iDone == -1 {
			t.Fatalf("missing SSE events; body:\n%s", body)
		}
		if !(iSources < iDelta && iDelta < iDone) {
			t.Errorf("event order wrong; body:\n%s", body)
		}
		if !strings.Contains(body, `"chunk_id":42`) {
			t.Errorf("sources payload missing chunk id; body:\n%s", body)
		}
		if !strings.Contains(body, `{"text":"Hello "}`) {
			t.Errorf("delta payload missing; body:\n%s", body)
		}
	})

	t.Run("mid-stream failure arrives as an error event", func(t *testing.T) {
		srv := newChatServer(&fakeChat{fail: true})
		req := httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(`{"query":"hi"}`))
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)

		body := rec.Body.String()
		if !strings.Contains(body, "event: error") {
			t.Fatalf("no error event; body:\n%s", body)
		}
		if strings.Contains(body, "event: done") {
			t.Errorf("done must not follow an error; body:\n%s", body)
		}
	})

	t.Run("returns 503 when chat is unconfigured", func(t *testing.T) {
		srv := newChatServer(nil)
		req := httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(`{"query":"hi"}`))
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", rec.Code)
		}
	})

	t.Run("rejects empty query with 400", func(t *testing.T) {
		srv := newChatServer(&fakeChat{})
		req := httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(`{"query":""}`))
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
	})
}
