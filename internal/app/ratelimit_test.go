package app

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/config"
)

func TestGeneralRateLimitExemptsProbes(t *testing.T) {
	cfg, err := config.Load(func(k string) string {
		return map[string]string{"APP_ENV": "test", "DATABASE_URL": "postgres://u:p@localhost:5432/test"}[k]
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg.RateLimit.APIPerMinute, cfg.RateLimit.APIBurst = 10, 2
	limiters := NewRateLimiters(cfg.RateLimit, clock.NewFixed(time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)))
	h := NewHTTPHandler(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), Deps{APILimiter: limiters.API})

	get := func(path string) *httptest.ResponseRecorder {
		r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec
	}
	for range 20 {
		if rec := get("/healthz"); rec.Code != http.StatusOK {
			t.Fatalf("/healthz limited: %d", rec.Code)
		}
		get("/readyz")
	}
	get("/api/v1/unknown")
	get("/api/v1/unknown")
	rec := get("/api/v1/unknown")
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "6" {
		t.Fatalf("over general limit: %d %v", rec.Code, rec.Header())
	}
	// Limited responses still carry the security headers and request ID.
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" || rec.Header().Get("X-Request-ID") == "" {
		t.Fatalf("headers: %v", rec.Header())
	}
}
