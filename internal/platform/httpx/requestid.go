package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/lhbotero1985/booking-platform-backend/internal/platform/reqctx"
)

// HeaderRequestID carries the request correlation ID.
const HeaderRequestID = "X-Request-ID"

// RequestID assigns a correlation ID to every request and records the client address in the
// context (reqctx) for use cases that audit. A client-supplied X-Request-ID is reused only if
// it is short and contains safe characters (it ends up in logs).
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(HeaderRequestID)
		if !validRequestID(id) {
			id = newRequestID()
		}
		w.Header().Set(HeaderRequestID, id)
		ctx := reqctx.WithRequestID(r.Context(), id)
		if ip, ok := ClientIP(r); ok {
			ctx = reqctx.WithClientIP(ctx, ip)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestIDFrom returns the request ID stored in ctx, or "".
func RequestIDFrom(ctx context.Context) string {
	return reqctx.RequestID(ctx)
}

func newRequestID() string {
	var b [16]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read never returns an error
	return hex.EncodeToString(b[:])
}

func validRequestID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, c := range id {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_', c == '.':
		default:
			return false
		}
	}
	return true
}
