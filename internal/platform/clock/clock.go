// Package clock abstracts the current time so domain rules and expirations are testable
// (ADR-0005: never call time.Now() directly in business code).
package clock

import (
	"sync"
	"time"
)

// Clock returns the current instant.
type Clock interface {
	Now() time.Time
}

// System is the real clock (UTC).
type System struct{}

// Now implements Clock.
func (System) Now() time.Time { return time.Now().UTC() }

// Fixed is a controllable clock for tests.
type Fixed struct {
	mu sync.Mutex
	t  time.Time
}

// NewFixed returns a clock frozen at t.
func NewFixed(t time.Time) *Fixed { return &Fixed{t: t.UTC()} }

// Now implements Clock.
func (f *Fixed) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.t
}

// Advance moves the clock forward by d.
func (f *Fixed) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.t = f.t.Add(d)
}
