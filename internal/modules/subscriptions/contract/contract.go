// Package contract is the only part of the subscriptions module other modules may import
// (ADR-0002, ADR-0018).
package contract

import (
	"context"

	"github.com/google/uuid"
)

// Trials starts the 7-day demo of a new business. Called with a context carrying the caller's
// transaction (db.ContextWithTx) it is atomic with the business creation.
type Trials interface {
	StartTrial(ctx context.Context, businessID uuid.UUID) error
}

// Entitlements answers whether a business may be visible and take bookings (demo running or
// paid). Used by search (T4.6) and bookings (Phase 6).
type Entitlements interface {
	Entitled(ctx context.Context, businessID uuid.UUID) (bool, error)
}
