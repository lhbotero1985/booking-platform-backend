// Package contract is the public API of the businesses module for other modules (T5.5).
package contract

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// ErrBusinessNotFound is returned when the business does not exist.
var ErrBusinessNotFound = errors.New("businesses: business not found")

// Currencies tells the ISO 4217 currency in which a business sets its prices. It performs no
// authorization: callers ask for a business the principal was already authorized for.
type Currencies interface {
	CurrencyOf(ctx context.Context, businessID uuid.UUID) (string, error)
}
