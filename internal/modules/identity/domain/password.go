package domain

import (
	"log/slog"
	"strings"
	"unicode/utf8"
)

// Password policy (NIST SP 800-63B style, docs/architecture/security.md): length limits and a
// blocklist of common passwords; no forced composition rules.
const (
	MinPasswordLength = 10
	MaxPasswordLength = 128
)

const redacted = "[REDACTED]"

// Password is a plaintext password that has passed the policy. It never prints its value:
// fmt verbs, %#v and slog all render "[REDACTED]".
type Password struct {
	value string
}

// NewPassword validates plain against the policy.
func NewPassword(plain string) (Password, error) {
	n := utf8.RuneCountInString(plain)
	switch {
	case n < MinPasswordLength:
		return Password{}, ErrPasswordTooShort
	case n > MaxPasswordLength:
		return Password{}, ErrPasswordTooLong
	case isCommonPassword(plain):
		return Password{}, ErrPasswordTooCommon
	}
	return Password{value: plain}, nil
}

// PasswordForVerification wraps a login attempt without applying the policy (old passwords
// may predate a stricter policy). Length is still capped to bound hashing cost.
func PasswordForVerification(plain string) (Password, error) {
	if utf8.RuneCountInString(plain) > MaxPasswordLength {
		return Password{}, ErrPasswordTooLong
	}
	return Password{value: plain}, nil
}

// Reveal returns the plaintext. Only the password hasher should call it.
func (p Password) Reveal() []byte { return []byte(p.value) }

// IsZero reports whether p was never set.
func (p Password) IsZero() bool { return p.value == "" }

// String implements fmt.Stringer.
func (Password) String() string { return redacted }

// GoString implements fmt.GoStringer (%#v).
func (Password) GoString() string { return redacted }

// LogValue implements slog.LogValuer.
func (Password) LogValue() slog.Value { return slog.StringValue(redacted) }

// MarshalText prevents accidental serialization (JSON, YAML…).
func (Password) MarshalText() ([]byte, error) { return []byte(redacted), nil }

// commonPasswords is a small blocklist of the most used passwords that satisfy the length
// rule; extended later with a larger list if needed. Compared case-insensitively.
var commonPasswords = map[string]struct{}{
	"1234567890": {}, "12345678910": {}, "123456789a": {}, "0123456789": {},
	"1111111111": {}, "0000000000": {}, "1q2w3e4r5t": {}, "qwertyuiop": {},
	"password12": {}, "password123": {}, "password1234": {}, "passw0rd123": {},
	"iloveyou12": {}, "qwerty1234": {}, "qwerty12345": {}, "asdfghjkl1": {},
	"abc1234567": {}, "abcdefghij": {}, "contraseña": {}, "contrasena": {},
	"contrasena1": {}, "contraseña1": {}, "contraseña123": {}, "contrasena123": {},
	"administrador": {}, "administrator": {}, "welcome123": {}, "bienvenido": {},
	"bienvenido1": {}, "bienvenido123": {}, "colombia123": {}, "mexico1234": {},
	"barberia123": {}, "peluqueria": {}, "letmein123": {}, "football123": {},
	"monkey1234": {}, "dragon1234": {}, "sunshine12": {}, "princess12": {},
}

func isCommonPassword(plain string) bool {
	p := strings.ToLower(plain)
	if _, ok := commonPasswords[p]; ok {
		return true
	}
	// A single repeated character ("aaaaaaaaaa") is as weak as a listed password.
	first, _ := utf8.DecodeRuneInString(p)
	return strings.Trim(p, string(first)) == ""
}
