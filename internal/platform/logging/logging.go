// Package logging builds the structured JSON logger used by every process.
//
// Attributes whose key looks sensitive (passwords, tokens, cookies, secrets, phone numbers…)
// are redacted automatically as a safety net. Callers must still avoid logging such data.
package logging

import (
	"io"
	"log/slog"
	"strings"
)

// Redacted replaces the value of sensitive attributes.
const Redacted = "[REDACTED]"

var sensitiveKeyParts = []string{
	"password", "passwd", "secret", "token", "authorization", "cookie",
	"api_key", "api-key", "apikey", "phone", "signature",
}

// New returns a JSON logger writing to w at the given level.
func New(w io.Writer, level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level:       level,
		ReplaceAttr: redact,
	}))
}

func redact(_ []string, a slog.Attr) slog.Attr {
	if a.Value.Kind() == slog.KindGroup {
		return a
	}
	if IsSensitiveKey(a.Key) {
		return slog.String(a.Key, Redacted)
	}
	return a
}

// IsSensitiveKey reports whether an attribute or header name should never be logged in clear.
func IsSensitiveKey(key string) bool {
	k := strings.ToLower(key)
	for _, part := range sensitiveKeyParts {
		if strings.Contains(k, part) {
			return true
		}
	}
	return false
}
