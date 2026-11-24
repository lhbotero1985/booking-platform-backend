// Package contract is the public API of the catalog module for other modules (T5.7).
package contract

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// Offerings tells which businesses customers can actually book: at least one ACTIVE service
// performed by at least one bookable professional. No authorization (public data).
type Offerings interface {
	BookableAmong(ctx context.Context, businessIDs []uuid.UUID) (map[uuid.UUID]bool, error)
}

// ErrNotBookable means the service is not ACTIVE in the business, the professional does not perform
// it, or the professional is not bookable.
var ErrNotBookable = errors.New("catalog: service not bookable with that professional")

// Booking tells whether customers can book a service with a professional and for how long it
// runs (T6.2). No authorization (public data).
type Booking interface {
	BookableService(ctx context.Context, businessID, serviceID, professionalID uuid.UUID) (durationMinutes int, err error)
}
