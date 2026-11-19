package application_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/netip"
	"testing"
	"time"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application/apptest"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/ratelimit"
)

// newThrottledEnv uses the real Backoff: 3 failures, then 1s, 2s, 4s… up to 1 minute.
func newThrottledEnv() env {
	e := env{
		store: apptest.NewStore(), hasher: &apptest.Hasher{}, mailer: &apptest.Mailer{},
		clock: clock.NewFixed(time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)),
	}
	e.svc = application.NewService(application.Deps{
		Store: e.store, Hasher: e.hasher, Mailer: e.mailer, Clock: e.clock,
		Random: &apptest.FixedRandom{}, Config: application.Config{PublicAppURL: "https://app.example.test/"},
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Throttle: ratelimit.NewBackoff(e.clock, ratelimit.BackoffConfig{Threshold: 3, Base: time.Second, Max: time.Minute, MaxKeys: 100}),
	})
	return e
}

func (e env) loginFrom(ip, email, password string) error {
	addr := netip.MustParseAddr(ip)
	_, err := e.svc.Login(context.Background(), application.LoginInput{Email: email, Password: password, ClientType: "MOBILE", IP: &addr})
	return err
}

func retryAfter(err error) (time.Duration, bool) {
	var r *application.RetryAfterError
	if errors.As(err, &r) && errors.Is(err, application.ErrTooManyAttempts) {
		return r.RetryAfter, true
	}
	return 0, false
}

func TestLoginBackoffSameForExistingAndUnknownAccounts(t *testing.T) {
	for _, email := range []string{"ana@example.com", "nadie@example.com"} {
		t.Run(email, func(t *testing.T) {
			e := newThrottledEnv()
			e.verifiedUser(t, "ana@example.com")
			for i := range 3 {
				if err := e.loginFrom("198.51.100.7", email, "clave equivocada 123"); !errors.Is(err, application.ErrInvalidCredentials) {
					t.Fatalf("attempt %d: %v", i+1, err)
				}
			}
			calls := e.hasher.VerifyCalls + e.hasher.DummyCalls
			err := e.loginFrom("198.51.100.7", email, goodPassword)
			if d, ok := retryAfter(err); !ok || d != time.Second {
				t.Fatalf("4th attempt: %v, want RetryAfter 1s", err)
			}
			if e.hasher.VerifyCalls+e.hasher.DummyCalls != calls {
				t.Fatal("a blocked attempt must not spend password hashing")
			}
			// Another client IP is not affected by this IP's failures.
			if err := e.loginFrom("203.0.113.9", email, "clave equivocada 123"); !errors.Is(err, application.ErrInvalidCredentials) {
				t.Fatalf("other IP: %v", err)
			}
			// Case-insensitive email: same key.
			if _, ok := retryAfter(e.loginFrom("198.51.100.7", "  "+upper(email), "x")); !ok {
				t.Fatal("email variants must share the backoff")
			}
		})
	}
}

func upper(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'a' && c <= 'z' {
			b[i] = c - 32
		}
	}
	return string(b)
}

func TestLoginBackoffGrowsAndExpires(t *testing.T) {
	e := newThrottledEnv()
	e.verifiedUser(t, "ana@example.com")
	const ip = "198.51.100.7"
	for range 3 {
		_ = e.loginFrom(ip, "ana@example.com", "clave equivocada 123")
	}
	e.clock.Advance(time.Second)
	_ = e.loginFrom(ip, "ana@example.com", "clave equivocada 123") // 4th failure → 2s
	if d, ok := retryAfter(e.loginFrom(ip, "ana@example.com", goodPassword)); !ok || d != 2*time.Second {
		t.Fatalf("after 4 failures: %v %v", d, ok)
	}
	e.clock.Advance(2 * time.Second)
	if err := e.loginFrom(ip, "ana@example.com", goodPassword); err != nil {
		t.Fatalf("correct password after the block expires: %v", err)
	}
	// Success cleared the counter: one new failure does not block.
	_ = e.loginFrom(ip, "ana@example.com", "clave equivocada 123")
	if err := e.loginFrom(ip, "ana@example.com", goodPassword); err != nil {
		t.Fatalf("counter not reset by success: %v", err)
	}
}

func TestLoginBackoffIgnoresValidationErrors(t *testing.T) {
	e := newThrottledEnv()
	for range 10 {
		var verr *application.ValidationError
		if err := e.loginFrom("198.51.100.7", "not-an-email", "x"); !errors.As(err, &verr) {
			t.Fatalf("validation: %v", err)
		}
	}
}

func TestChangePasswordBackoff(t *testing.T) {
	e := newThrottledEnv()
	e.verifiedUser(t, "ana@example.com")
	res, err := e.login("ana@example.com", goodPassword, "MOBILE")
	if err != nil {
		t.Fatal(err)
	}
	p, _ := e.svc.Authenticate(context.Background(), res.Token.Reveal())
	change := func(current string) error {
		return e.svc.ChangePassword(context.Background(), p, application.ChangePasswordInput{CurrentPassword: current, NewPassword: "otra frase nueva bastante larga"})
	}
	for range 3 {
		var verr *application.ValidationError
		if err := change("no es la actual 123"); !errors.As(err, &verr) {
			t.Fatalf("wrong current: %v", err)
		}
	}
	if _, ok := retryAfter(change(goodPassword)); !ok {
		t.Fatal("guessing the current password must be throttled")
	}
	e.clock.Advance(time.Second)
	if err := change(goodPassword); err != nil {
		t.Fatalf("after block: %v", err)
	}
}
