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

// PublicProfessional is what customers may see of a bookable professional (T5.7): no account,
// email or membership data.
type PublicProfessional struct {
	ID          uuid.UUID
	BusinessID  uuid.UUID
	DisplayName string
	Bio         string
	SortOrder   int
}

// Bookable lists, per business, the professionals customers can book: profile ACTIVE and the
// account still a member. No authorization (public data).
type Bookable interface {
	BookableProfessionals(ctx context.Context, businessIDs []uuid.UUID) (map[uuid.UUID][]PublicProfessional, error)
}
