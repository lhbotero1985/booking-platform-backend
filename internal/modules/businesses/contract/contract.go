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

// PublicDirectory resolves the slug of a business customers can see (ACTIVE and entitled, the
// same rules as the public detail) to its id; ErrBusinessNotFound otherwise.
type PublicDirectory interface {
	PublicBusinessID(ctx context.Context, slug string) (uuid.UUID, error)
}

// BookingPolicy is what the availability engine needs from a public business (T6.2).
type BookingPolicy struct {
	BusinessID          uuid.UUID
	TimeZone            string // IANA
	SlotIntervalMinutes int
	BufferMinutes       int
	MinLeadMinutes      int
	MaxAdvanceDays      int
	// Cut-offs for the customer to cancel or reschedule (T6.6).
	CancellationCutoffHours int
	RescheduleCutoffHours   int
	// Reminder before each appointment (T7.4): on/off and how many hours before (1–72).
	RemindersEnabled    bool
	ReminderHoursBefore int
}

// PublicBooking resolves the slug of a business customers can see (same rules as
// PublicDirectory) to its booking policy; ErrBusinessNotFound otherwise.
type PublicBooking interface {
	PublicBookingPolicy(ctx context.Context, slug string) (BookingPolicy, error)
	// PublicBookingPolicyOf is the same by id (T6.3: customers book by business id).
	PublicBookingPolicyOf(ctx context.Context, businessID uuid.UUID) (BookingPolicy, error)
	// BookingPolicyOf returns the policy of any existing business, without the public
	// visibility rules (T6.4: its staff books manually). No authorization: callers check the
	// caller's role first.
	BookingPolicyOf(ctx context.Context, businessID uuid.UUID) (BookingPolicy, error)
	// BusinessSummaries returns name, slug and time zone of existing businesses (T6.5: a
	// customer's appointments in every business); unknown ids are left out.
	BusinessSummaries(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]BusinessSummary, error)
}

// BusinessSummary identifies a business to its customers.
type BusinessSummary struct {
	ID       uuid.UUID
	Name     string
	Slug     string
	TimeZone string
	// Contact shown in the appointment emails (T7.6).
	AddressLine  string
	ContactPhone string   // E.164
	Latitude     *float64 // nil when the business has no coordinates
	Longitude    *float64
}
