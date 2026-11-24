// Package contract is the only part of the subscriptions module other modules may import
// (ADR-0002, ADR-0018).
package contract

import (
	"context"
	"time"

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
	// EntitledAmong returns the subset of ids that are entitled now (one query for a page of
	// search results).
	EntitledAmong(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]bool, error)
}

// Summary is what a business's own staff sees of its subscription.
type Summary struct {
	Status      string // TRIALING, ACTIVE, PAST_DUE, EXPIRED, CANCELED ("" if none)
	TrialEndsAt *time.Time
	Entitled    bool
}

// Summaries returns a business's subscription summary. The caller authorizes the request.
type Summaries interface {
	SummaryOf(ctx context.Context, businessID uuid.UUID) (Summary, error)
}
