package httpx_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/httpx"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/ratelimit"
)

func TestClientKey(t *testing.T) {
	cases := map[string]string{
		"203.0.113.7:5000":                "203.0.113.7",
		"[::ffff:203.0.113.7]:5000":       "203.0.113.7",
		"[2001:db8:1:2:aaaa::1]:443":      "2001:db8:1:2::/64",
		"[2001:db8:1:2:ffff:ffff::9]:443": "2001:db8:1:2::/64",
		"not-an-address":                  "unknown",
	}
	for remote, want := range cases {
		r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
		r.RemoteAddr = remote
		if got := httpx.ClientKey(r); got != want {
			t.Errorf("%s: %q, want %q", remote, got, want)
		}
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	c := clock.NewFixed(time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC))
	l := ratelimit.NewLimiter(c, ratelimit.LimiterConfig{PerMinute: 2, Burst: 2, MaxKeys: 10})
	h := httpx.Chain(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }),
		httpx.RequestID, httpx.RateLimit(l))
	call := func(remote string) *httptest.ResponseRecorder {
		r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil)
		r.RemoteAddr = remote
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec
	}
	call("198.51.100.1:1")
	call("198.51.100.1:2")
	rec := call("198.51.100.1:3")
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "30" ||
		rec.Header().Get("Content-Type") != httpx.ContentTypeProblem || !strings.Contains(rec.Body.String(), `"code":"RATE_LIMITED"`) {
		t.Fatalf("over limit: %d %v %s", rec.Code, rec.Header(), rec.Body.String())
	}
	if rec := call("198.51.100.2:1"); rec.Code != http.StatusNoContent {
		t.Fatalf("other client limited: %d", rec.Code)
	}
	if rec := call("[2001:db8::1]:1"); rec.Code != http.StatusNoContent {
		t.Fatal(rec.Code)
	}
	call("[2001:db8::2]:1")
	if rec := call("[2001:db8::3]:1"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("rotating addresses inside one /64 must share the bucket: %d", rec.Code)
	}

	if httpx.RateLimit(nil)(http.NotFoundHandler()) == nil {
		t.Fatal("nil limiter must pass through")
	}
}

func TestWriteRateLimitedMinimumOneSecond(t *testing.T) {
	rec := httptest.NewRecorder()
	httpx.WriteRateLimited(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil), 10*time.Millisecond)
	if rec.Header().Get("Retry-After") != "1" {
		t.Fatalf("Retry-After = %q", rec.Header().Get("Retry-After"))
	}
}
