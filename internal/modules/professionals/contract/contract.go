// Package contract is the public API of the professionals module for other modules (T5.6).
package contract

import (
	"context"

	"github.com/google/uuid"
)

// Directory tells whether a professional profile exists in a business. It performs no
// authorization: callers ask for a business the principal was already authorized for.
type Directory interface {
	IsProfessionalOf(ctx context.Context, businessID, professionalID uuid.UUID) (bool, error)
}
