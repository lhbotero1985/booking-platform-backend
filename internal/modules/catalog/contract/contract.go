// Package contract is the public API of the catalog module for other modules (T5.7).
package contract

import (
	"context"

	"github.com/google/uuid"
)

// Offerings tells which businesses customers can actually book: at least one ACTIVE service
// performed by at least one bookable professional. No authorization (public data).
type Offerings interface {
	BookableAmong(ctx context.Context, businessIDs []uuid.UUID) (map[uuid.UUID]bool, error)
}
