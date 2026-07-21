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
	"github.com/michal-kalina/garden-of-knowledge/backend/internal/conversations"
	"github.com/michal-kalina/garden-of-knowledge/backend/internal/documents"
	"github.com/michal-kalina/garden-of-knowledge/backend/internal/retrieval"
	"github.com/michal-kalina/garden-of-knowledge/backend/internal/users"
)

// fakeDocs is an in-memory DocumentService used to test handlers in isolation.
type fakeDocs struct {
	docs map[string]documents.Document
}

func (f *fakeDocs) Upload(_ context.Context, _ string, in documents.UploadInput) (documents.Document, error) {
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

func (f *fakeDocs) List(context.Context, string) ([]documents.Document, error) {
	out := []documents.Document{}
	for _, d := range f.docs {
		out = append(out, d)
	}
	return out, nil
}

func (f *fakeDocs) Get(_ context.Context, _ string, id string) (documents.Document, error) {
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

func (f *fakeSearch) Search(_ context.Context, _ string, query string, limit int) ([]retrieval.Result, error) {
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
		Conversations:  newFakeConversations(),
		Verify:         func(string) (string, error) { return "test-user", nil },
		MaxUploadBytes: 1 << 20, // 1 MiB limit for tests
		Logger:         logger,
	})
	return h, fake, search
}

// fakeChat emits one sources event and two deltas.
type fakeChat struct{ fail bool }

func (f *fakeChat) Ask(_ context.Context, _ string, question string, _ []chat.Turn,
	onSources func([]chat.Source) error, onDelta func(string) error) (string, error) {
	if err := onSources([]chat.Source{{Index: 1, ChunkID: 42, Filename: "a.md"}}); err != nil {
		return "", err
	}
	if err := onDelta("Hello "); err != nil {
		return "", err
	}
	if f.fail {
		return "", context.DeadlineExceeded
	}
	if err := onDelta("[1]"); err != nil {
		return "", err
	}
	return "Hello [1]", nil
}

// fakeConversations is an in-memory ConversationsService.
type fakeConversations struct {
	convs map[string]conversations.Conversation
	msgs  map[string][]conversations.Message
	// appended records the last AppendExchange call for assertions.
	appended *struct {
		userID, id, title, question, answer string
		sources                             []chat.Source
	}
}

func newFakeConversations() *fakeConversations {
	return &fakeConversations{
		convs: map[string]conversations.Conversation{},
		msgs:  map[string][]conversations.Message{},
	}
}

func (f *fakeConversations) Create(_ context.Context, userID string) (conversations.Conversation, error) {
	c := conversations.Conversation{ID: "conv-" + userID}
	f.convs[c.ID] = c
	return c, nil
}

func (f *fakeConversations) List(_ context.Context, _ string) ([]conversations.Conversation, error) {
	out := make([]conversations.Conversation, 0, len(f.convs))
	for _, c := range f.convs {
		out = append(out, c)
	}
	return out, nil
}

func (f *fakeConversations) Get(_ context.Context, _, id string) (conversations.Conversation, []conversations.Message, error) {
	c, ok := f.convs[id]
	if !ok {
		return conversations.Conversation{}, nil, conversations.ErrNotFound
	}
	return c, f.msgs[id], nil
}

func (f *fakeConversations) AppendExchange(_ context.Context, userID, id, title, question, answer string, sources []chat.Source) error {
	if _, ok := f.convs[id]; !ok {
		return conversations.ErrNotFound
	}
	f.appended = &struct {
		userID, id, title, question, answer string
		sources                             []chat.Source
	}{userID, id, title, question, answer, sources}
	f.msgs[id] = append(f.msgs[id],
		conversations.Message{Role: "user", Content: question},
		conversations.Message{Role: "assistant", Content: answer, Sources: sources},
	)
	return nil
}

func (f *fakeConversations) Delete(_ context.Context, _, id string) error {
	if _, ok := f.convs[id]; !ok {
		return conversations.ErrNotFound
	}
	delete(f.convs, id)
	return nil
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
		req := authed(httptest.NewRequest(http.MethodPost, "/documents", body))
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
		req := authed(httptest.NewRequest(http.MethodPost, "/documents", body))
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
		req := authed(httptest.NewRequest(http.MethodPost, "/documents", body))
		req.Header.Set("Content-Type", ct)
		rec := httptest.NewRecorder()

		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("status = %d, want 413", rec.Code)
		}
	})

	t.Run("rejects missing file field with 400", func(t *testing.T) {
		srv, _ := newTestServer(t)
		req := authed(httptest.NewRequest(http.MethodPost, "/documents", strings.NewReader("nope")))
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
		req := authed(httptest.NewRequest(http.MethodGet, "/documents/11111111-1111-1111-1111-111111111111", nil))
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
	})

	t.Run("returns 404 for unknown id", func(t *testing.T) {
		req := authed(httptest.NewRequest(http.MethodGet, "/documents/22222222-2222-2222-2222-222222222222", nil))
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

		req := authed(httptest.NewRequest(http.MethodPost, "/search",
			strings.NewReader(`{"query":"vacation days","limit":5}`)))
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
		req := authed(httptest.NewRequest(http.MethodPost, "/search", strings.NewReader(`{"query":"  "}`)))
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("rejects malformed JSON with 400", func(t *testing.T) {
		srv, _, _ := newTestServerWithSearch(t)
		req := authed(httptest.NewRequest(http.MethodPost, "/search", strings.NewReader(`{`)))
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
	})
}

func TestChat(t *testing.T) {
	newChatServer := func(c ChatService) (http.Handler, *fakeConversations) {
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		fc := newFakeConversations()
		h := New(Deps{
			Documents:      &fakeDocs{docs: map[string]documents.Document{}},
			Search:         &fakeSearch{},
			Chat:           c,
			Conversations:  fc,
			Verify:         func(string) (string, error) { return "test-user", nil },
			MaxUploadBytes: 1 << 20,
			Logger:         logger,
		})
		return h, fc
	}

	t.Run("streams conversation, sources, deltas and done as SSE, then persists", func(t *testing.T) {
		srv, fc := newChatServer(&fakeChat{})
		req := authed(httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(`{"query":"hi"}`)))
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d; body: %s", rec.Code, rec.Body)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
			t.Errorf("content type = %q", ct)
		}
		body := rec.Body.String()
		iConv := strings.Index(body, "event: conversation")
		iSources := strings.Index(body, "event: sources")
		iDelta := strings.Index(body, "event: delta")
		iDone := strings.Index(body, "event: done")
		if iConv == -1 || iSources == -1 || iDelta == -1 || iDone == -1 {
			t.Fatalf("missing SSE events; body:\n%s", body)
		}
		if !(iConv < iSources && iSources < iDelta && iDelta < iDone) {
			t.Errorf("event order wrong; body:\n%s", body)
		}
		if !strings.Contains(body, `"chunk_id":42`) {
			t.Errorf("sources payload missing chunk id; body:\n%s", body)
		}
		if !strings.Contains(body, `{"text":"Hello "}`) {
			t.Errorf("delta payload missing; body:\n%s", body)
		}

		if fc.appended == nil {
			t.Fatal("exchange was not persisted")
		}
		if fc.appended.question != "hi" || fc.appended.answer != "Hello [1]" {
			t.Errorf("persisted exchange = %+v", fc.appended)
		}
		if fc.appended.title == "" {
			t.Error("title not set for a new conversation")
		}
		if len(fc.appended.sources) != 1 {
			t.Errorf("persisted sources = %+v", fc.appended.sources)
		}
	})

	t.Run("continuing a conversation loads its history and skips the title", func(t *testing.T) {
		srv, fc := newChatServer(&fakeChat{})
		fc.convs["existing"] = conversations.Conversation{ID: "existing", Title: "Old title"}
		fc.msgs["existing"] = []conversations.Message{
			{Role: "user", Content: "first question"},
			{Role: "assistant", Content: "first answer"},
		}

		req := authed(httptest.NewRequest(http.MethodPost, "/chat",
			strings.NewReader(`{"query":"follow-up","conversation_id":"existing"}`)))
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d; body: %s", rec.Code, rec.Body)
		}
		if !strings.Contains(rec.Body.String(), `event: conversation`) ||
			!strings.Contains(rec.Body.String(), `"id":"existing"`) {
			t.Errorf("conversation id not echoed; body:\n%s", rec.Body.String())
		}
		if fc.appended == nil || fc.appended.title != "" {
			t.Errorf("title must stay empty for an existing conversation: %+v", fc.appended)
		}
	})

	t.Run("unknown conversation_id yields 404 before streaming", func(t *testing.T) {
		srv, _ := newChatServer(&fakeChat{})
		req := authed(httptest.NewRequest(http.MethodPost, "/chat",
			strings.NewReader(`{"query":"hi","conversation_id":"missing"}`)))
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rec.Code)
		}
	})

	t.Run("mid-stream failure arrives as an error event", func(t *testing.T) {
		srv, fc := newChatServer(&fakeChat{fail: true})
		req := authed(httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(`{"query":"hi"}`)))
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)

		body := rec.Body.String()
		if !strings.Contains(body, "event: error") {
			t.Fatalf("no error event; body:\n%s", body)
		}
		if strings.Contains(body, "event: done") {
			t.Errorf("done must not follow an error; body:\n%s", body)
		}
		if fc.appended != nil {
			t.Error("a failed generation must not be persisted")
		}
	})

	t.Run("returns 503 when chat is unconfigured", func(t *testing.T) {
		srv, _ := newChatServer(nil)
		req := authed(httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(`{"query":"hi"}`)))
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", rec.Code)
		}
	})

	t.Run("rejects empty query with 400", func(t *testing.T) {
		srv, _ := newChatServer(&fakeChat{})
		req := authed(httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(`{"query":""}`)))
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
	})
}

// authed attaches a Bearer token accepted by the test verifier.
func authed(req *http.Request) *http.Request {
	req.Header.Set("Authorization", "Bearer test-token")
	return req
}

type fakeUsers struct{ err error }

func (f *fakeUsers) Register(_ context.Context, email, _ string) (users.User, string, error) {
	if f.err != nil {
		return users.User{}, "", f.err
	}
	return users.User{ID: "u1", Email: email}, "tok", nil
}
func (f *fakeUsers) Login(_ context.Context, email, _ string) (users.User, string, error) {
	if f.err != nil {
		return users.User{}, "", f.err
	}
	return users.User{ID: "u1", Email: email}, "tok", nil
}

func TestAuth(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	newServer := func(u UserService) http.Handler {
		return New(Deps{
			Documents: &fakeDocs{docs: map[string]documents.Document{}},
			Search:    &fakeSearch{},
			Users:     u,
			Verify: func(tok string) (string, error) {
				if tok == "test-token" {
					return "test-user", nil
				}
				return "", users.ErrInvalidCredentials
			},
			MaxUploadBytes: 1 << 20,
			Logger:         logger,
		})
	}

	t.Run("register returns token", func(t *testing.T) {
		srv := newServer(&fakeUsers{})
		req := httptest.NewRequest(http.MethodPost, "/auth/register",
			strings.NewReader(`{"email":"a@b.c","password":"longenough"}`))
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d; body: %s", rec.Code, rec.Body)
		}
		if !strings.Contains(rec.Body.String(), `"token":"tok"`) {
			t.Errorf("body = %s", rec.Body)
		}
	})

	t.Run("duplicate email yields 409", func(t *testing.T) {
		srv := newServer(&fakeUsers{err: users.ErrEmailTaken})
		req := httptest.NewRequest(http.MethodPost, "/auth/register",
			strings.NewReader(`{"email":"a@b.c","password":"longenough"}`))
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d", rec.Code)
		}
	})

	t.Run("bad login yields 401", func(t *testing.T) {
		srv := newServer(&fakeUsers{err: users.ErrInvalidCredentials})
		req := httptest.NewRequest(http.MethodPost, "/auth/login",
			strings.NewReader(`{"email":"a@b.c","password":"nope"}`))
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d", rec.Code)
		}
	})

	t.Run("protected routes reject missing and bad tokens", func(t *testing.T) {
		srv := newServer(&fakeUsers{})
		for _, header := range []string{"", "Bearer wrong", "NotBearer test-token"} {
			req := httptest.NewRequest(http.MethodGet, "/documents", nil)
			if header != "" {
				req.Header.Set("Authorization", header)
			}
			rec := httptest.NewRecorder()
			srv.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("header %q: status = %d, want 401", header, rec.Code)
			}
		}
	})
}
