package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Tokens issues and verifies JWT session tokens (HS256).
//
// Hand-rolled on the standard library rather than a JWT dependency: this
// service both issues and verifies its own tokens, so the notorious JWT
// pitfalls (algorithm negotiation, "alg":"none", key confusion) do not apply
// — the algorithm is pinned, there is exactly one key, and anything else is
// rejected. What remains is base64url + HMAC + an expiry check.
type Tokens struct {
	secret []byte
	ttl    time.Duration
	// now is injectable for expiry tests.
	now func() time.Time
}

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("token expired")
)

func NewTokens(secret string, ttl time.Duration) *Tokens {
	return &Tokens{secret: []byte(secret), ttl: ttl, now: time.Now}
}

type header struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

type claims struct {
	Sub string `json:"sub"`
	Iat int64  `json:"iat"`
	Exp int64  `json:"exp"`
}

var b64 = base64.RawURLEncoding

// Issue creates a signed token carrying the user id.
func (t *Tokens) Issue(userID string) (string, error) {
	head, err := json.Marshal(header{Alg: "HS256", Typ: "JWT"})
	if err != nil {
		return "", err
	}
	now := t.now()
	body, err := json.Marshal(claims{
		Sub: userID,
		Iat: now.Unix(),
		Exp: now.Add(t.ttl).Unix(),
	})
	if err != nil {
		return "", err
	}
	signingInput := b64.EncodeToString(head) + "." + b64.EncodeToString(body)
	return signingInput + "." + b64.EncodeToString(t.sign(signingInput)), nil
}

// Verify checks the signature and expiry and returns the user id.
func (t *Tokens) Verify(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", ErrInvalidToken
	}

	// Signature first: nothing in the payload is trusted before this line.
	sig, err := b64.DecodeString(parts[2])
	if err != nil {
		return "", ErrInvalidToken
	}
	if !hmac.Equal(sig, t.sign(parts[0]+"."+parts[1])) {
		return "", ErrInvalidToken
	}

	var h header
	headBytes, err := b64.DecodeString(parts[0])
	if err != nil || json.Unmarshal(headBytes, &h) != nil {
		return "", ErrInvalidToken
	}
	// The algorithm is pinned; a valid signature with a different declared
	// alg still means someone is playing games.
	if h.Alg != "HS256" {
		return "", fmt.Errorf("%w: unexpected algorithm %q", ErrInvalidToken, h.Alg)
	}

	var c claims
	bodyBytes, err := b64.DecodeString(parts[1])
	if err != nil || json.Unmarshal(bodyBytes, &c) != nil {
		return "", ErrInvalidToken
	}
	if c.Sub == "" {
		return "", ErrInvalidToken
	}
	if t.now().Unix() >= c.Exp {
		return "", ErrExpiredToken
	}
	return c.Sub, nil
}

func (t *Tokens) sign(input string) []byte {
	mac := hmac.New(sha256.New, t.secret)
	mac.Write([]byte(input))
	return mac.Sum(nil)
}
