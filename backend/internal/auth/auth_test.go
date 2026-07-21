package auth

import (
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

// TestPBKDF2Vectors checks the implementation against the widely published
// PBKDF2-HMAC-SHA256 test vectors (P="password", S="salt").
func TestPBKDF2Vectors(t *testing.T) {
	cases := []struct {
		iter int
		want string
	}{
		{1, "120fb6cffcf8b32c43e7225256c4f837a86548c92ccc35480805987cb70be17b"},
		{2, "ae4d0c95af6b46d32d0adff928f06dd02a303f8ef3c251dfd6e2d85a95474c43"},
		{4096, "c5e478d59288c841aa530db6845c4c8d962893a001ce4e11a4963873aa98134a"},
	}
	for _, c := range cases {
		got := hex.EncodeToString(pbkdf2Sha256([]byte("password"), []byte("salt"), c.iter, 32))
		if got != c.want {
			t.Errorf("iter=%d: got %s, want %s", c.iter, got, c.want)
		}
	}
}

func TestPasswordHashing(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "pbkdf2-sha256$600000$") {
		t.Errorf("unexpected format: %s", hash)
	}
	if !VerifyPassword("correct horse battery staple", hash) {
		t.Error("correct password rejected")
	}
	if VerifyPassword("wrong password", hash) {
		t.Error("wrong password accepted")
	}
	if VerifyPassword("anything", "garbage") {
		t.Error("malformed hash accepted")
	}

	// Two hashes of the same password must differ (random salt).
	hash2, _ := HashPassword("correct horse battery staple")
	if hash == hash2 {
		t.Error("salt not randomized")
	}
}

func TestTokens(t *testing.T) {
	tokens := NewTokens("test-secret", time.Hour)

	t.Run("roundtrip", func(t *testing.T) {
		tok, err := tokens.Issue("user-123")
		if err != nil {
			t.Fatal(err)
		}
		got, err := tokens.Verify(tok)
		if err != nil {
			t.Fatal(err)
		}
		if got != "user-123" {
			t.Errorf("sub = %q", got)
		}
	})

	t.Run("rejects wrong secret", func(t *testing.T) {
		tok, _ := tokens.Issue("u")
		other := NewTokens("different-secret", time.Hour)
		if _, err := other.Verify(tok); err == nil {
			t.Fatal("token accepted with wrong secret")
		}
	})

	t.Run("rejects tampered payload", func(t *testing.T) {
		tok, _ := tokens.Issue("u")
		parts := strings.Split(tok, ".")
		// Flip the payload while keeping the original signature.
		forged := parts[0] + "." + b64.EncodeToString([]byte(`{"sub":"admin","exp":99999999999}`)) + "." + parts[2]
		if _, err := tokens.Verify(forged); err == nil {
			t.Fatal("forged payload accepted")
		}
	})

	t.Run("rejects expired token", func(t *testing.T) {
		past := NewTokens("test-secret", time.Hour)
		past.now = func() time.Time { return time.Now().Add(-2 * time.Hour) }
		tok, _ := past.Issue("u")
		if _, err := tokens.Verify(tok); err != ErrExpiredToken {
			t.Fatalf("err = %v, want ErrExpiredToken", err)
		}
	})

	t.Run("rejects malformed tokens", func(t *testing.T) {
		for _, bad := range []string{"", "a.b", "a.b.c.d", "!!!.???.###"} {
			if _, err := tokens.Verify(bad); err == nil {
				t.Errorf("accepted %q", bad)
			}
		}
	})
}
