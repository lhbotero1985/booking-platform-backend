package domain

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"log/slog"
	"strings"
)

// Token prefixes make leaked tokens recognizable by secret scanners (.gitleaks.toml) and
// prevent using one kind of token where another is expected.
const (
	TokenPrefixSession           = "bp_s_"
	TokenPrefixEmailVerification = "bp_ev_"
	TokenPrefixPasswordReset     = "bp_pr_"
)

// TokenEntropyBytes is the random part of every token (256 bits).
const TokenEntropyBytes = 32

// encodedEntropyLen is the base64url (no padding) length of TokenEntropyBytes.
var encodedEntropyLen = base64.RawURLEncoding.EncodedLen(TokenEntropyBytes)

// ErrInvalidToken covers malformed, unknown, expired and already used tokens alike, so callers
// cannot distinguish them (no oracle for attackers).
var ErrInvalidToken = errors.New("identity: invalid or expired token")

// Token is an opaque secret handed to a client. Only its SHA-256 is ever stored, and it never
// prints its value.
type Token struct {
	raw string
}

// NewToken builds a token from prefix and exactly TokenEntropyBytes of random data.
func NewToken(prefix string, entropy []byte) (Token, error) {
	if len(entropy) != TokenEntropyBytes || prefix == "" {
		return Token{}, errors.New("identity: token requires 32 bytes of entropy and a prefix")
	}
	return Token{raw: prefix + base64.RawURLEncoding.EncodeToString(entropy)}, nil
}

// ParseToken validates a client-supplied token of the expected kind.
func ParseToken(prefix, raw string) (Token, error) {
	rest, ok := strings.CutPrefix(raw, prefix)
	if !ok || len(rest) != encodedEntropyLen {
		return Token{}, ErrInvalidToken
	}
	if _, err := base64.RawURLEncoding.DecodeString(rest); err != nil {
		return Token{}, ErrInvalidToken
	}
	return Token{raw: raw}, nil
}

// Hash returns the SHA-256 digest that is persisted.
func (t Token) Hash() []byte {
	h := sha256.Sum256([]byte(t.raw))
	return h[:]
}

// Reveal returns the raw token, only to deliver it to its owner (email body, response body).
func (t Token) Reveal() string { return t.raw }

// String implements fmt.Stringer.
func (Token) String() string { return redacted }

// GoString implements fmt.GoStringer.
func (Token) GoString() string { return redacted }

// LogValue implements slog.LogValuer.
func (Token) LogValue() slog.Value { return slog.StringValue(redacted) }

// MarshalText prevents accidental serialization.
func (Token) MarshalText() ([]byte, error) { return []byte(redacted), nil }
