package parserclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	t.Run("sends multipart file and maps blocks", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/parse" {
				t.Errorf("path = %q, want /parse", r.URL.Path)
			}
			file, header, err := r.FormFile("file")
			if err != nil {
				t.Fatalf("missing file part: %v", err)
			}
			defer file.Close()
			if header.Filename != "doc.md" {
				t.Errorf("filename = %q", header.Filename)
			}
			if ct := header.Header.Get("Content-Type"); ct != "text/markdown" {
				t.Errorf("part content type = %q", ct)
			}
			body, _ := io.ReadAll(file)
			if string(body) != "# hi" {
				t.Errorf("file body = %q", body)
			}

			page := 3
			_ = json.NewEncoder(w).Encode(map[string]any{
				"filename":     "doc.md",
				"content_type": "text/markdown",
				"blocks": []map[string]any{
					{"type": "heading", "text": "hi", "page": nil},
					{"type": "paragraph", "text": "body", "page": page},
				},
			})
		}))
		defer srv.Close()

		blocks, err := New(srv.URL).Parse(context.Background(),
			"doc.md", "text/markdown", strings.NewReader("# hi"))
		if err != nil {
			t.Fatal(err)
		}
		if len(blocks) != 2 {
			t.Fatalf("got %d blocks, want 2", len(blocks))
		}
		if blocks[0].Type != "heading" || blocks[0].Text != "hi" || blocks[0].Page != nil {
			t.Errorf("block 0 = %+v", blocks[0])
		}
		if blocks[1].Page == nil || *blocks[1].Page != 3 {
			t.Errorf("block 1 page = %v, want 3", blocks[1].Page)
		}
	})

	t.Run("surfaces parser errors with detail", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"detail":"failed to parse PDF: broken xref"}`))
		}))
		defer srv.Close()

		_, err := New(srv.URL).Parse(context.Background(),
			"bad.pdf", "application/pdf", strings.NewReader("not a pdf"))
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "broken xref") {
			t.Errorf("error %q lacks parser detail", err)
		}
	})
}
