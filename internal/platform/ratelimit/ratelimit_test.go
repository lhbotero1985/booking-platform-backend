package ratelimit_test

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/ratelimit"
)

var t0 = time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)

func TestLimiterBurstAndRefill(t *testing.T) {
	c := clock.NewFixed(t0)
	l := ratelimit.NewLimiter(c, ratelimit.LimiterConfig{PerMinute: 6, Burst: 3, MaxKeys: 10})
	for i := range 3 {
		if ok, _ := l.Allow("a"); !ok {
			t.Fatalf("request %d denied within burst", i)
		}
	}
	ok, wait := l.Allow("a")
	if ok || wait != 10*time.Second {
		t.Fatalf("4th request: ok=%v wait=%v, want denied 10s (6/min)", ok, wait)
	}
	if ok, _ := l.Allow("b"); !ok {
		t.Fatal("another key must have its own bucket")
	}
	c.Advance(9 * time.Second)
	if ok, wait := l.Allow("a"); ok || wait != time.Second {
		t.Fatalf("before refill: ok=%v wait=%v", ok, wait)
	}
	c.Advance(time.Second)
	if ok, _ := l.Allow("a"); !ok {
		t.Fatal("token not refilled after 10s")
	}
	c.Advance(time.Hour)
	for i := range 3 {
		if ok, _ := l.Allow("a"); !ok {
			t.Fatalf("burst not restored after idle (request %d)", i)
		}
	}
}

func TestLimiterIsBoundedAndForgetsIdleKeys(t *testing.T) {
	c := clock.NewFixed(t0)
	l := ratelimit.NewLimiter(c, ratelimit.LimiterConfig{PerMinute: 60, Burst: 2, MaxKeys: 100})
	for i := range 1000 {
		l.Allow(fmt.Sprintf("ip-%d", i))
	}
	if n := l.Len(); n > 100 {
		t.Fatalf("tracked keys = %d, want <= 100", n)
	}
	c.Advance(2 * time.Minute)
	l.Allow("trigger-sweep")
	if n := l.Len(); n != 1 {
		t.Fatalf("idle keys not swept: %d", n)
	}
}

func TestLimiterConcurrentUseIsExact(t *testing.T) {
	l := ratelimit.NewLimiter(clock.NewFixed(t0), ratelimit.LimiterConfig{PerMinute: 1, Burst: 50, MaxKeys: 10})
	var allowed atomic.Int32
	var wg sync.WaitGroup
	for range 200 {
		wg.Go(func() {
			if ok, _ := l.Allow("same"); ok {
				allowed.Add(1)
			}
		})
	}
	wg.Wait()
	if n := allowed.Load(); n != 50 {
		t.Fatalf("allowed %d concurrent requests, want exactly the burst (50)", n)
	}
}

func TestBackoffProgression(t *testing.T) {
	c := clock.NewFixed(t0)
	b := ratelimit.NewBackoff(c, ratelimit.BackoffConfig{Threshold: 3, Base: time.Second, Max: 8 * time.Second, ForgetAfter: time.Hour, MaxKeys: 10})
	for range 2 {
		b.Failure("k")
	}
	if _, blocked := b.Blocked("k"); blocked {
		t.Fatal("blocked before threshold")
	}
	want := []time.Duration{1, 2, 4, 8, 8} // seconds, capped at Max
	for i, w := range want {
		b.Failure("k")
		d, blocked := b.Blocked("k")
		if !blocked || d != w*time.Second {
			t.Fatalf("failure %d: blocked=%v d=%v, want %v", 3+i, blocked, d, w*time.Second)
		}
	}
	c.Advance(8 * time.Second)
	if _, blocked := b.Blocked("k"); blocked {
		t.Fatal("block must expire (no permanent lockout)")
	}
	if _, blocked := b.Blocked("other"); blocked {
		t.Fatal("unrelated key blocked")
	}
	b.Success("k")
	b.Failure("k")
	if _, blocked := b.Blocked("k"); blocked {
		t.Fatal("success must reset the counter")
	}
}

func TestBackoffForgetsOldFailuresAndIsBounded(t *testing.T) {
	c := clock.NewFixed(t0)
	b := ratelimit.NewBackoff(c, ratelimit.BackoffConfig{Threshold: 2, Base: time.Second, Max: time.Minute, ForgetAfter: 10 * time.Minute, MaxKeys: 50})
	b.Failure("k")
	c.Advance(11 * time.Minute)
	b.Failure("k")
	if _, blocked := b.Blocked("k"); blocked {
		t.Fatal("failure older than ForgetAfter must not count")
	}
	for i := range 500 {
		b.Failure(fmt.Sprintf("k-%d", i))
	}
	if n := b.Len(); n > 50 {
		t.Fatalf("tracked keys = %d, want <= 50", n)
	}
}

func TestBackoffDefaults(t *testing.T) {
	b := ratelimit.NewBackoff(clock.NewFixed(t0), ratelimit.BackoffConfig{})
	for range 4 {
		b.Failure("k")
	}
	if _, blocked := b.Blocked("k"); blocked {
		t.Fatal("default threshold is 5")
	}
	b.Failure("k")
	if d, blocked := b.Blocked("k"); !blocked || d != time.Second {
		t.Fatalf("default base: %v %v", d, blocked)
	}
}
