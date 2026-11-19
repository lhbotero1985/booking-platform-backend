package security_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lhbotero1985/booking-platform-backend/internal/app"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application/apptest"
	identityhttp "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/interfaces/http"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/config"
)

// TestBruteForceIsStoppedWithDefaultLimits runs an online password guessing attack against the
// full HTTP stack wired exactly like cmd/api with the default configuration, and checks that
// the attacker gets very few password checks per minute.
func TestBruteForceIsStoppedWithDefaultLimits(t *testing.T) {
	cfg, err := config.Load(func(k string) string {
		return map[string]string{"APP_ENV": "test", "DATABASE_URL": "postgres://u:p@localhost:5432/test"}[k]
	})
	if err != nil {
		t.Fatal(err)
	}
	c := clock.NewFixed(time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC))
	limiters := app.NewRateLimiters(cfg.RateLimit, c)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hasher := &apptest.Hasher{}
	svc := application.NewService(application.Deps{
		Store: apptest.NewStore(), Hasher: hasher, Mailer: &apptest.Mailer{}, Clock: c,
		Random: &apptest.FixedRandom{}, Config: application.Config{PublicAppURL: "https://app.example.test"},
		Logger: logger, Throttle: limiters.Login,
	})
	h := app.NewHTTPHandler(cfg, logger, app.Deps{
		Modules:    []app.RouteRegistrar{identityhttp.New(svc, logger, identityhttp.Options{AuthLimiter: limiters.Auth})},
		APILimiter: limiters.API,
	})

	attempt := func(ip string, i int) int {
		body := `{"email":"victima@example.com","password":"intento numero ` + strings.Repeat("x", i%7) + `","clientType":"MOBILE"}`
		r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.RemoteAddr = ip + ":40000"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if rec.Code == http.StatusTooManyRequests && rec.Header().Get("Retry-After") == "" {
			t.Fatal("429 without Retry-After")
		}
		return rec.Code
	}

	// One attacker IP, 200 attempts within the same minute.
	limited := 0
	for i := range 200 {
		if attempt("198.51.100.66", i) == http.StatusTooManyRequests {
			limited++
		}
	}
	checks := hasher.VerifyCalls + hasher.DummyCalls
	t.Logf("single IP: %d/200 limited, %d password checks", limited, checks)
	if checks > cfg.RateLimit.LoginBackoffThreshold {
		t.Fatalf("attacker got %d password checks in a minute, want <= %d", checks, cfg.RateLimit.LoginBackoffThreshold)
	}

	// A legitimate user on another IP is unaffected.
	if code := attempt("203.0.113.10", 0); code != http.StatusUnauthorized {
		t.Fatalf("other client blocked: %d", code)
	}

	// After a quiet period the attacker gets a few new tries, never a burst.
	c.Advance(10 * time.Minute)
	before := hasher.VerifyCalls + hasher.DummyCalls
	for i := range 50 {
		attempt("198.51.100.66", i)
	}
	if got := hasher.VerifyCalls + hasher.DummyCalls - before; got > 1 {
		t.Fatalf("after the pause the backoff must allow at most one check at a time, got %d", got)
	}
}
