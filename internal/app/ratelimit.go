package app

import (
	"time"

	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/config"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/ratelimit"
)

// RateLimiters are the process-wide limiters (in memory: one API instance in the MVP).
type RateLimiters struct {
	API   *ratelimit.Limiter
	Auth  *ratelimit.Limiter
	Login *ratelimit.Backoff
}

// NewRateLimiters builds the limiters from configuration.
func NewRateLimiters(cfg config.RateLimit, c clock.Clock) RateLimiters {
	return RateLimiters{
		API:  ratelimit.NewLimiter(c, ratelimit.LimiterConfig{PerMinute: cfg.APIPerMinute, Burst: cfg.APIBurst, MaxKeys: cfg.MaxKeys}),
		Auth: ratelimit.NewLimiter(c, ratelimit.LimiterConfig{PerMinute: cfg.AuthPerMinute, Burst: cfg.AuthBurst, MaxKeys: cfg.MaxKeys}),
		Login: ratelimit.NewBackoff(c, ratelimit.BackoffConfig{
			Threshold: cfg.LoginBackoffThreshold, Base: time.Second, Max: cfg.LoginBackoffMax,
			ForgetAfter: 4 * cfg.LoginBackoffMax, MaxKeys: cfg.MaxKeys,
		}),
	}
}
