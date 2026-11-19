// Package ratelimit provides in-memory, per-key rate limiting (token bucket) and progressive
// backoff after failures. State lives in the process (MVP: a single API instance, see
// docs/architecture/security.md §3); it must move to a shared store before scaling out.
//
// Both structures are bounded (maxKeys) so a flood of distinct keys cannot exhaust memory, and
// clean up idle entries lazily on use (no background goroutine).
package ratelimit

import (
	"math"
	"sync"
	"time"

	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
)

// sweepInterval is the minimum time between two full scans for idle entries.
const sweepInterval = time.Minute

// epsilon absorbs floating point error in token arithmetic.
const epsilon = 1e-9

// Limiter is a token bucket per key: up to Burst requests at once, refilled at PerMinute.
type Limiter struct {
	mu        sync.Mutex
	clock     clock.Clock
	perSecond float64
	burst     float64
	maxKeys   int
	buckets   map[string]*bucket
	lastSweep time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

// LimiterConfig configures a Limiter. All values must be positive.
type LimiterConfig struct {
	PerMinute int
	Burst     int
	MaxKeys   int
}

// NewLimiter builds a Limiter. Invalid (non-positive) values are clamped to 1.
func NewLimiter(c clock.Clock, cfg LimiterConfig) *Limiter {
	return &Limiter{
		clock:     c,
		perSecond: float64(max(cfg.PerMinute, 1)) / 60,
		burst:     float64(max(cfg.Burst, 1)),
		maxKeys:   max(cfg.MaxKeys, 1),
		buckets:   map[string]*bucket{},
		lastSweep: c.Now(),
	}
}

// Allow consumes one token for key. When none is left it returns false and how long until the
// next token is available.
func (l *Limiter) Allow(key string) (bool, time.Duration) {
	now := l.clock.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.maybeSweep(now)

	b, ok := l.buckets[key]
	if !ok {
		if len(l.buckets) >= l.maxKeys {
			l.sweep(now)
			if len(l.buckets) >= l.maxKeys {
				evictOne(l.buckets)
			}
		}
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	}
	if elapsed := now.Sub(b.last).Seconds(); elapsed > 0 {
		b.tokens = math.Min(l.burst, b.tokens+elapsed*l.perSecond)
	}
	b.last = now
	if b.tokens >= 1-epsilon {
		b.tokens = math.Max(0, b.tokens-1)
		return true, 0
	}
	// Rounded up to the millisecond so float error never yields a too-short wait.
	wait := time.Duration(math.Ceil((1-b.tokens)/l.perSecond*1000)) * time.Millisecond
	return false, wait
}

// Len returns the number of tracked keys (for tests and metrics).
func (l *Limiter) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}

func (l *Limiter) maybeSweep(now time.Time) {
	if now.Sub(l.lastSweep) >= sweepInterval {
		l.sweep(now)
	}
}

// sweep drops buckets that would be full again: forgetting them changes nothing.
func (l *Limiter) sweep(now time.Time) {
	l.lastSweep = now
	refill := time.Duration(l.burst / l.perSecond * float64(time.Second))
	for k, b := range l.buckets {
		if now.Sub(b.last) >= refill {
			delete(l.buckets, k)
		}
	}
}

// evictOne removes an arbitrary entry (Go map iteration order is randomized), so an attacker
// cannot predict which key is dropped when the table is full.
func evictOne[V any](m map[string]V) {
	for k := range m {
		delete(m, k)
		return
	}
}

// Backoff blocks a key progressively after consecutive failures: from the Threshold-th failure
// on, the key is blocked for Base, 2×Base, 4×Base… up to Max. A success clears the key, and a
// key with no failure for ForgetAfter starts over. Never a permanent lockout.
type Backoff struct {
	mu        sync.Mutex
	clock     clock.Clock
	cfg       BackoffConfig
	entries   map[string]*failures
	lastSweep time.Time
}

type failures struct {
	count        int
	last         time.Time
	blockedUntil time.Time
}

// BackoffConfig configures a Backoff.
type BackoffConfig struct {
	Threshold   int
	Base        time.Duration
	Max         time.Duration
	ForgetAfter time.Duration
	MaxKeys     int
}

// NewBackoff builds a Backoff. Non-positive values are replaced by safe defaults.
func NewBackoff(c clock.Clock, cfg BackoffConfig) *Backoff {
	if cfg.Threshold <= 0 {
		cfg.Threshold = 5
	}
	if cfg.Base <= 0 {
		cfg.Base = time.Second
	}
	if cfg.Max < cfg.Base {
		cfg.Max = cfg.Base
	}
	if cfg.ForgetAfter <= cfg.Max {
		cfg.ForgetAfter = 4 * cfg.Max
	}
	cfg.MaxKeys = max(cfg.MaxKeys, 1)
	return &Backoff{clock: c, cfg: cfg, entries: map[string]*failures{}, lastSweep: c.Now()}
}

// Blocked reports whether key is currently blocked and for how long.
func (b *Backoff) Blocked(key string) (time.Duration, bool) {
	now := b.clock.Now()
	b.mu.Lock()
	defer b.mu.Unlock()
	f, ok := b.entries[key]
	if !ok || !now.Before(f.blockedUntil) {
		return 0, false
	}
	return f.blockedUntil.Sub(now), true
}

// Failure records a failed attempt for key.
func (b *Backoff) Failure(key string) {
	now := b.clock.Now()
	b.mu.Lock()
	defer b.mu.Unlock()
	if now.Sub(b.lastSweep) >= sweepInterval {
		b.sweep(now)
	}
	f, ok := b.entries[key]
	if ok && now.Sub(f.last) >= b.cfg.ForgetAfter {
		ok = false
	}
	if !ok {
		if _, exists := b.entries[key]; !exists && len(b.entries) >= b.cfg.MaxKeys {
			b.sweep(now)
			if len(b.entries) >= b.cfg.MaxKeys {
				evictOne(b.entries)
			}
		}
		f = &failures{}
		b.entries[key] = f
	}
	f.count++
	f.last = now
	if over := f.count - b.cfg.Threshold; over >= 0 {
		d := b.cfg.Max
		if over < 32 { // avoid shift overflow; anything this large is capped anyway
			d = min(b.cfg.Base<<over, b.cfg.Max)
		}
		f.blockedUntil = now.Add(d)
	}
}

// Success clears key.
func (b *Backoff) Success(key string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.entries, key)
}

// Len returns the number of tracked keys.
func (b *Backoff) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.entries)
}

func (b *Backoff) sweep(now time.Time) {
	b.lastSweep = now
	for k, f := range b.entries {
		if now.Sub(f.last) >= b.cfg.ForgetAfter && !now.Before(f.blockedUntil) {
			delete(b.entries, k)
		}
	}
}
