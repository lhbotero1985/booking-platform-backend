package httpx

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// RealIP replaces r.RemoteAddr with the original client address when the TCP peer is a trusted
// reverse proxy (Caddy). Everything downstream (ClientIP, rate limiting, audit) then sees the
// real client.
//
// X-Forwarded-For is read right to left, skipping trusted proxies; the first untrusted address
// is the client. Requests that do not come from a trusted proxy keep their TCP peer address:
// a client can never choose its own IP by sending the header directly. With no trusted
// proxies the middleware does nothing.
func RealIP(trusted []netip.Prefix) Middleware {
	return func(next http.Handler) http.Handler {
		if len(trusted) == 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			peer, ok := ClientIP(r)
			if ok && isTrusted(peer, trusted) {
				if client, found := clientFromForwarded(r.Header.Values("X-Forwarded-For"), trusted); found {
					r = r.WithContext(r.Context()) // shallow copy: never mutate the caller's request
					r.RemoteAddr = net.JoinHostPort(client.String(), "0")
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// clientFromForwarded returns the rightmost address that is not a trusted proxy. Any malformed
// entry stops the walk: the header is then ignored rather than partially trusted.
func clientFromForwarded(values []string, trusted []netip.Prefix) (netip.Addr, bool) {
	var hops []string
	for _, v := range values {
		hops = append(hops, strings.Split(v, ",")...)
	}
	for i := len(hops) - 1; i >= 0; i-- {
		ip, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			return netip.Addr{}, false
		}
		ip = ip.Unmap()
		if !isTrusted(ip, trusted) {
			return ip, true
		}
	}
	return netip.Addr{}, false
}

func isTrusted(ip netip.Addr, trusted []netip.Prefix) bool {
	for _, p := range trusted {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}
