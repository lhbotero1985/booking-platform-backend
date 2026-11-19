package httpx

import (
	"math"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"time"
)

// CodeRateLimited is returned with 429 responses.
const CodeRateLimited = "RATE_LIMITED"

// Allower is a per-key rate limiter (implemented by ratelimit.Limiter).
type Allower interface {
	Allow(key string) (ok bool, retryAfter time.Duration)
}

// ClientIP returns the TCP peer address. Behind the reverse proxy (Phase 3) the trusted
// X-Forwarded-For handling will be added here, never trusting client-supplied headers.
func ClientIP(r *http.Request) (netip.Addr, bool) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return netip.Addr{}, false
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, false
	}
	return ip.Unmap(), true
}

// ClientKey is the rate limiting key of the caller: the IPv4 address, or the IPv6 /64 prefix
// (one end user usually controls a whole /64, so per-address keys would be trivial to rotate).
func ClientKey(r *http.Request) string {
	ip, ok := ClientIP(r)
	if !ok {
		return "unknown"
	}
	if ip.Is6() {
		if p, err := ip.Prefix(64); err == nil {
			return p.String()
		}
	}
	return ip.String()
}

// RateLimit rejects requests over the limit of their client key with 429. A nil limiter
// disables the middleware.
func RateLimit(l Allower) Middleware {
	return func(next http.Handler) http.Handler {
		if l == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if ok, wait := l.Allow(ClientKey(r)); !ok {
				WriteRateLimited(w, r, wait)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// WriteRateLimited writes 429 RATE_LIMITED with Retry-After in whole seconds (at least 1).
func WriteRateLimited(w http.ResponseWriter, r *http.Request, retryAfter time.Duration) {
	secs := max(int64(math.Ceil(retryAfter.Seconds())), 1)
	w.Header().Set("Retry-After", strconv.FormatInt(secs, 10))
	WriteProblem(w, r, NewProblem(http.StatusTooManyRequests, CodeRateLimited, "too many requests, try again later"))
}
