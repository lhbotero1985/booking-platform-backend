package domain

import (
	"net/mail"
	"strings"
)

// MaxEmailLength is the practical limit of an address (RFC 5321 path length).
const MaxEmailLength = 254

// Email is a normalized (trimmed, lower-case) e-mail address.
type Email string

// ParseEmail validates and normalizes an address. Only a bare addr-spec is accepted
// ("ana@example.com"); display names, comments and multiple addresses are rejected.
func ParseEmail(raw string) (Email, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" || len(s) > MaxEmailLength {
		return "", ErrInvalidEmail
	}
	addr, err := mail.ParseAddress(s)
	if err != nil || addr.Name != "" || addr.Address != s {
		return "", ErrInvalidEmail
	}
	local, domain, ok := strings.Cut(s, "@")
	if !ok || local == "" || len(local) > 64 || !validDomain(domain) {
		return "", ErrInvalidEmail
	}
	return Email(s), nil
}

func validDomain(d string) bool {
	if len(d) < 3 || len(d) > 253 || !strings.Contains(d, ".") ||
		strings.HasPrefix(d, ".") || strings.HasSuffix(d, ".") || strings.Contains(d, "..") {
		return false
	}
	for _, label := range strings.Split(d, ".") {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, c := range label {
			if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
				return false
			}
		}
	}
	return true
}

// String returns the normalized address.
func (e Email) String() string { return string(e) }
