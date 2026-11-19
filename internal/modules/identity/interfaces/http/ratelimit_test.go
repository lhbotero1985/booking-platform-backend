package httpapi_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/ratelimit"
)

var rlStart = time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)

func TestAuthEndpointsShareStrictLimit(t *testing.T) {
	c := clock.NewFixed(rlStart)
	f := newFixtureWith(ratelimit.NewLimiter(c, ratelimit.LimiterConfig{PerMinute: 4, Burst: 4, MaxKeys: 100}), nil)

	f.post(t, "/api/v1/auth/register", regBody)
	f.post(t, "/api/v1/auth/verify-email", `{"token":"`+f.issueToken(t)+`"}`)
	tok := f.mobileToken(t)
	f.post(t, "/api/v1/auth/password/forgot", `{"email":"ana@example.com"}`) // 4th: last token

	for _, path := range []string{
		"/api/v1/auth/login", "/api/v1/auth/register", "/api/v1/auth/verify-email",
		"/api/v1/auth/verify-email/resend", "/api/v1/auth/password/forgot", "/api/v1/auth/password/reset",
	} {
		r := f.do(t, http.MethodPost, path, `{}`, nil)
		if r.status != http.StatusTooManyRequests || r.header.Get("Retry-After") != "15" || !strings.Contains(r.body, `"code":"RATE_LIMITED"`) {
			t.Errorf("%s: %d %q %s", path, r.status, r.header.Get("Retry-After"), r.body)
		}
	}
	if r := f.do(t, http.MethodPost, "/api/v1/me/password", `{}`, bearer(tok)); r.status != http.StatusTooManyRequests {
		t.Errorf("/me/password: %d", r.status)
	}
	// Reading endpoints are not under the strict limit.
	if r := f.do(t, http.MethodGet, "/api/v1/me", "", bearer(tok)); r.status != http.StatusOK {
		t.Errorf("/me: %d", r.status)
	}
	c.Advance(15 * time.Second)
	if r := f.post(t, "/api/v1/auth/password/forgot", `{"email":"ana@example.com"}`); r.status != http.StatusAccepted {
		t.Errorf("after refill: %d", r.status)
	}
}

// TestLoginBackoffResponseRevealsNothing: once blocked, existing and unknown accounts get the
// same 429 (body and Retry-After), and the correct password is not even checked.
func TestLoginBackoffResponseRevealsNothing(t *testing.T) {
	c := clock.NewFixed(rlStart)
	f := newFixtureWith(nil, ratelimit.NewBackoff(c, ratelimit.BackoffConfig{Threshold: 3, Base: 30 * time.Second, Max: time.Hour, MaxKeys: 100}))
	f.activeAccount(t)

	blocked := map[string]authResp{}
	for _, email := range []string{"ana@example.com", "nadie@example.com"} {
		bad := `{"email":"` + email + `","password":"clave equivocada 123","clientType":"MOBILE"}`
		for range 3 {
			if r := f.do(t, http.MethodPost, "/api/v1/auth/login", bad, nil); r.status != http.StatusUnauthorized {
				t.Fatalf("%s: %d %s", email, r.status, r.body)
			}
		}
		good := strings.Replace(bad, "clave equivocada 123", "una clave suficientemente larga", 1)
		blocked[email] = f.do(t, http.MethodPost, "/api/v1/auth/login", good, nil)
	}
	a, b := blocked["ana@example.com"], blocked["nadie@example.com"]
	if a.status != http.StatusTooManyRequests || a.header.Get("Retry-After") != "30" {
		t.Fatalf("existing: %d %q %s", a.status, a.header.Get("Retry-After"), a.body)
	}
	if b.status != a.status || b.header.Get("Retry-After") != a.header.Get("Retry-After") || stripVolatile(t, a.body) != stripVolatile(t, b.body) {
		t.Fatalf("blocked responses differ:\n%d %s\n%d %s", a.status, a.body, b.status, b.body)
	}
	c.Advance(30 * time.Second)
	if r := f.do(t, http.MethodPost, "/api/v1/auth/login", loginMobile, nil); r.status != http.StatusOK {
		t.Fatalf("after backoff: %d %s", r.status, r.body)
	}
}
