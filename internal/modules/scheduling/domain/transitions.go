package domain

// Changes of an appointment (T6.6, booking-lifecycle.md §2 and §5, decisions of 2026-11-24):
//
//	PENDING_CONFIRMATION → CONFIRMED (confirm), StatusCancelled (cancel), IN_PROGRESS (start),
//	                       NO_SHOW (no-show, from the start time)
//	CONFIRMED            → StatusCancelled, IN_PROGRESS, COMPLETED (from the start time), NO_SHOW
//	IN_PROGRESS          → COMPLETED
//
// The canceled, COMPLETED and NO_SHOW statuses are final and free the agenda. Cancel and reschedule only
// before the start time; the customer also before the business's cut-off.

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// ActorRole tells whether a change came from the customer or from the business's staff.
type ActorRole string

// Actor roles (matches the CHECKs of appointments.cancelled_by and appointment_events.actor_role).
const (
	ActorCustomer ActorRole = "CUSTOMER"
	ActorBusiness ActorRole = "BUSINESS"
)

// Event types of the changes.
const (
	EventConfirmed   EventType = "CONFIRMED"
	EventCancelled   EventType = "CANCELLED" //nolint:misspell // stored value
	EventRescheduled EventType = "RESCHEDULED"
	EventReassigned  EventType = "REASSIGNED"
	EventStarted     EventType = "STARTED"
	EventCompleted   EventType = "COMPLETED"
	EventNoShow      EventType = "NO_SHOW"
)

// StartWindow is how early the staff may start an appointment.
const StartWindow = 15 * time.Minute

// MaxCancelReason matches the appointments_cancel_reason_length CHECK.
const MaxCancelReason = 300

// Change errors.
var (
	// ErrInvalidTransition means the change is not allowed from the current status.
	ErrInvalidTransition = errors.New("scheduling: invalid status change")
	// ErrTooEarly means it is too early (start: 15 min before; complete, no-show: the start).
	ErrTooEarly = errors.New("scheduling: too early for that change")
	// ErrAlreadyStarted means the start time has passed: it can no longer be canceled or moved.
	ErrAlreadyStarted = errors.New("scheduling: the appointment time has passed")
	// ErrCutoffPassed means the customer is past the business's cut-off to cancel or move.
	ErrCutoffPassed = errors.New("scheduling: past the cut-off for the customer")
	// ErrInvalidReason means the cancel reason is too long or has control characters.
	ErrInvalidReason = errors.New("scheduling: invalid reason")
)

// Event is one change in the history of an appointment.
type Event struct {
	ID                     uuid.UUID
	Type                   EventType
	Status                 Status // after the change
	FromStatus             Status // "" for CREATED
	ProfessionalID         uuid.UUID
	Start                  time.Time
	PreviousStart          time.Time // zero unless moved
	PreviousProfessionalID uuid.UUID // uuid.Nil unless reassigned
	Reason                 string
	Actor                  uuid.UUID
	ActorRole              ActorRole
	At                     time.Time
}

// ParseReason normalizes an optional cancel reason.
func ParseReason(raw string) (string, error) {
	s := strings.Join(strings.Fields(raw), " ")
	if utf8.RuneCountInString(s) > MaxCancelReason || strings.ContainsFunc(s, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return "", ErrInvalidReason
	}
	return s, nil
}

func (a *Appointment) set(s Status, now time.Time) {
	a.Status, a.UpdatedAt = s, now.UTC()
}

// Confirm confirms a pending appointment; confirming a confirmed one changes nothing.
func (a *Appointment) Confirm(now time.Time) (bool, error) {
	switch a.Status {
	case StatusConfirmed:
		return false, nil
	case StatusPendingConfirmation:
		a.set(StatusConfirmed, now)
		return true, nil
	default:
		return false, ErrInvalidTransition
	}
}

// Cancel cancels an appointment before its start (the customer also before cutoff); canceling
// a canceled one changes nothing.
func (a *Appointment) Cancel(by ActorRole, reason string, cutoff time.Duration, now time.Time) (bool, error) {
	switch {
	case a.Status == StatusCancelled:
		return false, nil
	case a.Status != StatusPendingConfirmation && a.Status != StatusConfirmed:
		return false, ErrInvalidTransition
	case !now.Before(a.Start):
		return false, ErrAlreadyStarted
	case by == ActorCustomer && now.Add(cutoff).After(a.Start):
		return false, ErrCutoffPassed
	}
	a.set(StatusCancelled, now)
	a.CancelledAt, a.CancelReason, a.CancelledBy = now.UTC(), reason, by
	return true, nil
}

// Begin starts the service, from 15 minutes before the start time.
func (a *Appointment) Begin(now time.Time) error {
	if a.Status != StatusPendingConfirmation && a.Status != StatusConfirmed {
		return ErrInvalidTransition
	}
	if now.Before(a.Start.Add(-StartWindow)) {
		return ErrTooEarly
	}
	a.set(StatusInProgress, now)
	return nil
}

// Complete finishes the service: from IN_PROGRESS, or from CONFIRMED once the start time came.
func (a *Appointment) Complete(now time.Time) error {
	switch a.Status {
	case StatusInProgress:
	case StatusConfirmed:
		if now.Before(a.Start) {
			return ErrTooEarly
		}
	default:
		return ErrInvalidTransition
	}
	a.set(StatusCompleted, now)
	return nil
}

// MarkNoShow records that the customer did not come, from the start time; it frees the rest of
// the slot at once (a NO_SHOW does not occupy the agenda).
func (a *Appointment) MarkNoShow(now time.Time) error {
	if a.Status != StatusPendingConfirmation && a.Status != StatusConfirmed {
		return ErrInvalidTransition
	}
	if now.Before(a.Start) {
		return ErrTooEarly
	}
	a.set(StatusNoShow, now)
	return nil
}

// CanMove tells whether the appointment may be rescheduled or reassigned now by that actor.
func (a *Appointment) CanMove(by ActorRole, cutoff time.Duration, now time.Time) error {
	switch {
	case a.Status != StatusPendingConfirmation && a.Status != StatusConfirmed:
		return ErrInvalidTransition
	case !now.Before(a.Start):
		return ErrAlreadyStarted
	case by == ActorCustomer && now.Add(cutoff).After(a.Start):
		return ErrCutoffPassed
	}
	return nil
}

// MoveTo sets a new start (and professional), keeping the duration and buffer booked. The status
// after the move (decision P7 and of 2026-11-24): moved by the customer, unchanged; moved by the
// business, PENDING_CONFIRMATION if the customer has an account (it confirms in the app),
// CONFIRMED otherwise (agreed by phone or in person).
func (a *Appointment) MoveTo(professionalID uuid.UUID, start time.Time, by ActorRole, now time.Time) {
	a.ProfessionalID = professionalID
	a.Start = start.UTC()
	a.End = a.Start.Add(time.Duration(a.Service.DurationMinutes) * time.Minute)
	status := a.Status
	if by == ActorBusiness {
		status = StatusConfirmed
		if a.Customer.UserID != uuid.Nil {
			status = StatusPendingConfirmation
		}
	}
	a.set(status, now)
}
