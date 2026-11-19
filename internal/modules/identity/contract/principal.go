package contract

import (
	"context"

	"github.com/google/uuid"
)

// Principal is the authenticated caller of a request. Other modules read it from the request
// context to authorize their use cases (ADR-0007); authorization decisions themselves (roles,
// memberships) are looked up per request, never cached in the session.
type Principal struct {
	UserID          uuid.UUID
	SessionID       uuid.UUID
	IsPlatformAdmin bool
	ClientType      string // "MOBILE" or "WEB"
}

type principalKey struct{}

// WithPrincipal returns a context carrying p.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// PrincipalFrom returns the authenticated principal, if any.
func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}
