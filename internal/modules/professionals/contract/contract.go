// Package contract is the public API of the professionals module for other modules (T5.6).
package contract

import (
	"context"
	"errors"
	"time"

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

// MinuteRange is [Start, End) in minutes since local midnight.
type MinuteRange struct{ Start, End int }

// Interval is an absolute [Start, End) in UTC.
type Interval struct{ Start, End time.Time }

// DayAgenda is a professional's schedule for one weekday (local times of the business) and the
// absences overlapping a period.
type DayAgenda struct {
	WorkingHours []MinuteRange
	Breaks       []MinuteRange
	Absences     []Interval
}

// Agenda reads a professional's agenda for the availability engine (T6.2). No authorization:
// callers check that the professional is bookable. ErrProfessionalNotFound if it does not exist in
// the business.
type Agenda interface {
	DayAgenda(ctx context.Context, businessID, professionalID uuid.UUID, weekday time.Weekday, from, to time.Time) (DayAgenda, error)
}

// ErrProfessionalNotFound means there is no such professional in the business.
var ErrProfessionalNotFound = errors.New("professionals: professional not found")
