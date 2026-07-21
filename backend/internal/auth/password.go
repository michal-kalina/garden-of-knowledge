// Package auth provides password hashing and session tokens.
//
// Both are built on the standard library. For passwords that is a deliberate,
// documented trade-off: the usual choices (bcrypt, argon2) live in
// golang.org/x/crypto, and PBKDF2-HMAC-SHA256 — an OWASP-sanctioned KDF that
// is nothing more than iterated HMAC — is implementable in ~30 auditable
// lines verified against the published test vectors below. If the dependency
// budget ever loosens, swapping in argon2id is contained to this file thanks
// to the algorithm tag embedded in every stored hash.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
)

// hashIterations follows the OWASP recommendation for PBKDF2-HMAC-SHA256.
const (
	hashIterations = 600_000
	saltLen        = 16
	keyLen         = 32
)

// HashPassword derives a salted key and encodes it as
// "pbkdf2-sha256$<iterations>$<salt-b64>$<key-b64>". The self-describing
// format lets Verify handle old hashes after parameters (or the algorithm)
// change.
func HashPassword(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	key := pbkdf2Sha256([]byte(password), salt, hashIterations, keyLen)
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s",
		hashIterations,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// VerifyPassword reports whether the password matches the encoded hash.
// Comparison is constant-time; all parse failures return false rather than
// distinguishing malformed hashes from wrong passwords.
func VerifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter < 1 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}
	got := pbkdf2Sha256([]byte(password), salt, iter, len(want))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// pbkdf2Sha256 implements PBKDF2 (RFC 8018 §5.2) with HMAC-SHA256 as the
// PRF. Verified against the standard test vectors in password_test.go.
func pbkdf2Sha256(password, salt []byte, iterations, keyLen int) []byte {
	prf := hmac.New(sha256.New, password)
	blockCount := (keyLen + sha256.Size - 1) / sha256.Size

	out := make([]byte, 0, blockCount*sha256.Size)
	idx := make([]byte, 4)
	for block := 1; block <= blockCount; block++ {
		// U1 = PRF(password, salt || INT_32_BE(block))
		prf.Reset()
		prf.Write(salt)
		binary.BigEndian.PutUint32(idx, uint32(block))
		prf.Write(idx)
		u := prf.Sum(nil)

		t := make([]byte, len(u))
		copy(t, u)
		// U2..Uc, XOR-accumulated into T.
		for i := 1; i < iterations; i++ {
			prf.Reset()
			prf.Write(u)
			u = prf.Sum(nil)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		out = append(out, t...)
	}
	return out[:keyLen]
}
