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

	"github.com/michal-kalina/garden-of-knowledge/backend/internal/documents"
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

func newTestServer(t *testing.T) (http.Handler, *fakeDocs) {
	t.Helper()
	fake := &fakeDocs{docs: map[string]documents.Document{}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	// db is only used by /readyz, which these tests do not exercise.
	return New(nil, fake, 1<<20 /* 1 MiB limit for tests */, logger), fake
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
