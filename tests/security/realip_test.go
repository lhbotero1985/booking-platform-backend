package security_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lhbotero1985/booking-platform-backend/internal/app"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/config"
)

// TestClientCannotSpoofItsIP: behind the composed handler with Caddy as trusted proxy, a client
// talking directly to the API cannot escape its rate limit by sending X-Forwarded-For, while
// clients behind the proxy are limited by their real address.
func TestClientCannotSpoofItsIP(t *testing.T) {
	cfg, err := config.Load(func(k string) string {
		return map[string]string{
			"APP_ENV": "test", "DATABASE_URL": "postgres://u:p@127.0.0.1:1/db",
			"TRUSTED_PROXIES": "172.30.0.10",
		}[k]
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg.RateLimit.APIPerMinute, cfg.RateLimit.APIBurst = 10, 2
	limiters := app.NewRateLimiters(cfg.RateLimit, clock.NewFixed(time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)))
	h := app.NewHTTPHandler(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), app.Deps{APILimiter: limiters.API})

	get := func(peer, xff string) int {
		r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/nothing", nil)
		r.RemoteAddr = peer
		if xff != "" {
			r.Header.Set("X-Forwarded-For", xff)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec.Code
	}

	// Direct attacker rotating fake X-Forwarded-For values: still one bucket.
	get("203.0.113.66:1", "1.1.1.1")
	get("203.0.113.66:2", "2.2.2.2")
	if code := get("203.0.113.66:3", "3.3.3.3"); code != http.StatusTooManyRequests {
		t.Fatalf("spoofed header escaped the limit: %d", code)
	}

	// Through the proxy: each real client has its own bucket…
	for _, client := range []string{"198.51.100.1", "198.51.100.2", "198.51.100.3"} {
		if code := get("172.30.0.10:4000", client); code != http.StatusNotFound {
			t.Fatalf("client %s behind proxy limited: %d", client, code)
		}
	}
	// …and a client behind the proxy cannot spoof either (Caddy appends the real address).
	get("172.30.0.10:4000", "9.9.9.9, 198.51.100.50")
	get("172.30.0.10:4000", "8.8.8.8, 198.51.100.50")
	if code := get("172.30.0.10:4000", "7.7.7.7, 198.51.100.50"); code != http.StatusTooManyRequests {
		t.Fatalf("client prefix spoofing through the proxy: %d", code)
	}
}
