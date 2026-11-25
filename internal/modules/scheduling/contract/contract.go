// Package contract is the public API of the scheduling module for other modules (T6.7).
package contract

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// ConflictingAppointment is an active appointment that a change of the agenda would leave out.
type ConflictingAppointment struct {
	ID           uuid.UUID
	Start, End   time.Time // UTC
	CustomerName string
	Status       string
}

// Range is [Start, End) in minutes since local midnight.
type Range struct{ Start, End int }

// Day is a weekday of a proposed weekly schedule.
type Day struct {
	WorkingHours []Range
	Breaks       []Range
}

// AgendaGuard tells which active appointments a change of a professional's agenda would break
// (P11, decision of 2026-11-24: the change is refused and these are listed). Call it inside the
// caller's transaction, after taking the professional's lock (professionals.Agenda.LockAgenda),
// so no booking can slip in between. No authorization.
type AgendaGuard interface {
	// AppointmentsDuring returns the active appointments whose service overlaps [from, to).
	AppointmentsDuring(ctx context.Context, businessID, professionalID uuid.UUID, from, to time.Time) ([]ConflictingAppointment, error)
	// AppointmentsOutside returns the active appointments not finished yet that the weekly
	// schedule week (index = time.Weekday, local times of the business) would leave outside the
	// working hours or on a break.
	AppointmentsOutside(ctx context.Context, businessID, professionalID uuid.UUID, week [7]Day) ([]ConflictingAppointment, error)
}
