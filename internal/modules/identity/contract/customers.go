package contract

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// ErrPhoneRequired means the customer has no mobile number on the account; booking requires one
// (decision of 2026-11-24: verified email and mandatory phone).
var ErrPhoneRequired = errors.New("identity: the customer has no phone")

// Customer is what a business may know about a customer who books (T6.3).
type Customer struct {
	UserID   uuid.UUID
	FullName string
	Phone    string // E.164
}

// Customers returns the profile of an ACTIVE (verified) account for a booking: ErrPhoneRequired
// if it has no phone. No authorization: the caller is that account.
type Customers interface {
	CustomerProfile(ctx context.Context, userID uuid.UUID) (Customer, error)
}
