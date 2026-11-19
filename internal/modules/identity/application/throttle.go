package application

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/domain"
)

// ErrTooManyAttempts is returned (wrapped in *RetryAfterError) while a password check is
// blocked by the progressive backoff.
var ErrTooManyAttempts = errors.New("identity: too many attempts")

// RetryAfterError carries how long the caller must wait.
type RetryAfterError struct {
	RetryAfter time.Duration
}

func (e *RetryAfterError) Error() string {
	return fmt.Sprintf("%v (retry after %s)", ErrTooManyAttempts, e.RetryAfter)
}

// Unwrap makes errors.Is(err, ErrTooManyAttempts) work.
func (e *RetryAfterError) Unwrap() error { return ErrTooManyAttempts }

// AttemptThrottle is the progressive backoff applied to password checks (implemented by
// ratelimit.Backoff). Keys are opaque strings built by the use cases.
type AttemptThrottle interface {
	Blocked(key string) (time.Duration, bool)
	Failure(key string)
	Success(key string)
}

// noThrottle disables the backoff (tests that measure timing, tools).
type noThrottle struct{}

func (noThrottle) Blocked(string) (time.Duration, bool) { return 0, false }
func (noThrottle) Failure(string)                       {}
func (noThrottle) Success(string)                       {}

// loginThrottleKey identifies account+IP (security.md §3). It is derived from the submitted
// email whether or not the account exists, so blocking reveals nothing about registration, and
// the email is hashed so the in-memory table holds no personal data in clear.
func loginThrottleKey(email domain.Email, ip *netip.Addr) string {
	sum := sha256.Sum256([]byte(email.String()))
	addr := "-"
	if ip != nil {
		addr = ip.String()
		if ip.Is6() {
			if p, err := ip.Prefix(64); err == nil {
				addr = p.String()
			}
		}
	}
	return "login|" + hex.EncodeToString(sum[:16]) + "|" + addr
}

// changePasswordThrottleKey limits guessing the current password with a stolen session.
func changePasswordThrottleKey(userID uuid.UUID) string {
	return "change-password|" + userID.String()
}

func (s *Service) checkThrottle(key string) error {
	if d, blocked := s.throttle.Blocked(key); blocked {
		return &RetryAfterError{RetryAfter: d}
	}
	return nil
}
