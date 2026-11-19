package reqctx_test

import (
	"context"
	"net/netip"
	"testing"

	"github.com/lhbotero1985/booking-platform-backend/internal/platform/reqctx"
)

func TestRoundTrip(t *testing.T) {
	ctx := context.Background()
	if reqctx.RequestID(ctx) != "" {
		t.Fatal("empty context has a request ID")
	}
	if _, ok := reqctx.ClientIP(ctx); ok {
		t.Fatal("empty context has a client IP")
	}
	ip := netip.MustParseAddr("203.0.113.5")
	ctx = reqctx.WithClientIP(reqctx.WithRequestID(ctx, "abc"), ip)
	if got, ok := reqctx.ClientIP(ctx); !ok || got != ip || reqctx.RequestID(ctx) != "abc" {
		t.Fatalf("got %v %v %q", got, ok, reqctx.RequestID(ctx))
	}
	if _, ok := reqctx.ClientIP(reqctx.WithClientIP(context.Background(), netip.Addr{})); ok {
		t.Fatal("zero address reported as known")
	}
}
