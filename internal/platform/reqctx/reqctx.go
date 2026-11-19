// Package reqctx carries per-request metadata (correlation ID, client address) in the context
// so that use cases can record it (audit, logs) without depending on HTTP.
package reqctx

import (
	"context"
	"net/netip"
)

type requestIDKey struct{}

type clientIPKey struct{}

// WithRequestID returns ctx carrying the request correlation ID.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestID returns the request correlation ID, or "".
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// WithClientIP returns ctx carrying the client address.
func WithClientIP(ctx context.Context, ip netip.Addr) context.Context {
	return context.WithValue(ctx, clientIPKey{}, ip)
}

// ClientIP returns the client address, if known.
func ClientIP(ctx context.Context) (netip.Addr, bool) {
	ip, ok := ctx.Value(clientIPKey{}).(netip.Addr)
	return ip, ok && ip.IsValid()
}
