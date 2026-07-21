//go:build integration

package conversations

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/michal-kalina/garden-of-knowledge/backend/internal/chat"
	"github.com/michal-kalina/garden-of-knowledge/backend/internal/database"
	"github.com/michal-kalina/garden-of-knowledge/backend/internal/users"
)

func TestConversationsIntegration(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.ExecContext(ctx,
		"DROP TABLE IF EXISTS messages, conversations, chunks, ingestion_jobs, documents, users, schema_migrations CASCADE",
	); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, db, logger); err != nil {
		t.Fatal(err)
	}

	usersRepo := users.NewRepository(db)
	alice, err := usersRepo.Create(ctx, "alice@example.com", "x")
	if err != nil {
		t.Fatal(err)
	}
	bob, err := usersRepo.Create(ctx, "bob@example.com", "x")
	if err != nil {
		t.Fatal(err)
	}

	repo := NewRepository(db)

	t.Run("full lifecycle: create, append, list ordering, get", func(t *testing.T) {
		conv, err := repo.Create(ctx, alice.ID)
		if err != nil {
			t.Fatal(err)
		}
		if conv.Title != "" {
			t.Errorf("new conversation title = %q, want empty", conv.Title)
		}

		sources := []chat.Source{{Index: 1, ChunkID: 7, Filename: "a.md", Content: "fact"}}
		if err := repo.AppendExchange(ctx, alice.ID, conv.ID, "First question",
			"First question", "First answer [1]", sources); err != nil {
			t.Fatal(err)
		}

		got, msgs, err := repo.Get(ctx, alice.ID, conv.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Title != "First question" {
			t.Errorf("title = %q", got.Title)
		}
		if len(msgs) != 2 {
			t.Fatalf("messages = %d, want 2", len(msgs))
		}
		if msgs[0].Role != "user" || msgs[0].Content != "First question" {
			t.Errorf("msg 0 = %+v", msgs[0])
		}
		if msgs[1].Role != "assistant" || len(msgs[1].Sources) != 1 || msgs[1].Sources[0].ChunkID != 7 {
			t.Errorf("msg 1 = %+v", msgs[1])
		}

		// Second exchange: title must NOT be overwritten (empty title passed).
		if err := repo.AppendExchange(ctx, alice.ID, conv.ID, "",
			"Second question", "Second answer", nil); err != nil {
			t.Fatal(err)
		}
		got2, msgs2, err := repo.Get(ctx, alice.ID, conv.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got2.Title != "First question" {
			t.Errorf("title changed on second exchange: %q", got2.Title)
		}
		if len(msgs2) != 4 {
			t.Fatalf("messages after 2 exchanges = %d, want 4", len(msgs2))
		}
		if !got2.UpdatedAt.After(got.UpdatedAt) && got2.UpdatedAt != got.UpdatedAt {
			t.Errorf("updated_at did not advance: %v -> %v", got.UpdatedAt, got2.UpdatedAt)
		}
	})

	t.Run("list is ordered most-recently-updated first", func(t *testing.T) {
		older, err := repo.Create(ctx, bob.ID)
		if err != nil {
			t.Fatal(err)
		}
		newer, err := repo.Create(ctx, bob.ID)
		if err != nil {
			t.Fatal(err)
		}
		// Touch the older one's sibling after creation so it becomes newest.
		if err := repo.AppendExchange(ctx, bob.ID, older.ID, "T", "q", "a", nil); err != nil {
			t.Fatal(err)
		}

		list, err := repo.List(ctx, bob.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 2 || list[0].ID != older.ID {
			t.Fatalf("expected the just-touched conversation first: %+v (newer=%s)", list, newer.ID)
		}
	})

	t.Run("cross-user access is rejected", func(t *testing.T) {
		conv, err := repo.Create(ctx, alice.ID)
		if err != nil {
			t.Fatal(err)
		}

		if _, _, err := repo.Get(ctx, bob.ID, conv.ID); err != ErrNotFound {
			t.Fatalf("bob reading alice's conversation: err = %v, want ErrNotFound", err)
		}
		if err := repo.AppendExchange(ctx, bob.ID, conv.ID, "", "q", "a", nil); err != ErrNotFound {
			t.Fatalf("bob appending to alice's conversation: err = %v, want ErrNotFound", err)
		}
		if err := repo.Delete(ctx, bob.ID, conv.ID); err != ErrNotFound {
			t.Fatalf("bob deleting alice's conversation: err = %v, want ErrNotFound", err)
		}

		// Alice can still do all of the above on her own conversation.
		if err := repo.Delete(ctx, alice.ID, conv.ID); err != nil {
			t.Fatalf("owner delete failed: %v", err)
		}
		if _, _, err := repo.Get(ctx, alice.ID, conv.ID); err != ErrNotFound {
			t.Fatalf("conversation should be gone after delete: err = %v", err)
		}
	})
}
