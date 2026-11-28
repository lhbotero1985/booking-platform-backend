package contract

import (
	"context"

	"github.com/google/uuid"
)

// Rating is the average of the visible reviews (one decimal; nil without reviews) and how many
// there are (2026-11-28).
type Rating struct {
	Average *float64
	Count   int
}

// Ratings is what other modules show of the reviews: the stars of businesses and of the
// professionals of a business. Public data, no authorization.
type Ratings interface {
	// BusinessRatings returns the rating of each business of ids (a business without reviews is
	// missing from the map).
	BusinessRatings(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]Rating, error)
	// ProfessionalRatings returns the rating of each professional of the business with reviews.
	ProfessionalRatings(ctx context.Context, businessID uuid.UUID) (map[uuid.UUID]Rating, error)
}
