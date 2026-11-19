package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestNewRedactsSensitiveAttributes(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf, slog.LevelInfo)

	logger.Info("login",
		slog.String("email", "ana@example.com"),
		slog.String("password", "hunter2-secret"),
		slog.String("sessionToken", "bp_s_abc"),
		slog.String("Authorization", "Bearer xyz"),
		slog.Group("customer", slog.String("phone", "+573001234567")),
	)

	out := buf.String()
	for _, leaked := range []string{"hunter2-secret", "bp_s_abc", "Bearer xyz", "+573001234567"} {
		if strings.Contains(out, leaked) {
			t.Errorf("log output leaked %q: %s", leaked, out)
		}
	}
	if !strings.Contains(out, "ana@example.com") {
		t.Errorf("non-sensitive attribute missing: %s", out)
	}

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}
	if rec["password"] != Redacted {
		t.Errorf("password = %v, want %s", rec["password"], Redacted)
	}
}

func TestNewRespectsLevel(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf, slog.LevelWarn)

	logger.Info("hidden")
	if buf.Len() != 0 {
		t.Errorf("info logged at warn level: %s", buf.String())
	}
	logger.Warn("shown")
	if !strings.Contains(buf.String(), "shown") {
		t.Errorf("warn not logged: %s", buf.String())
	}
}

func TestIsSensitiveKey(t *testing.T) {
	for key, want := range map[string]bool{
		"password": true, "X-Api-Key": true, "Cookie": true, "refresh_token": true,
		"phone_number": true, "email": false, "status": false, "request_id": false,
	} {
		if got := IsSensitiveKey(key); got != want {
			t.Errorf("IsSensitiveKey(%q) = %v, want %v", key, got, want)
		}
	}
}
