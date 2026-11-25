package contract

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
)

// ErrAppointmentNotFound means there is no appointment with that id in that business (or the
// caller may not see it: the same error on purpose).
var ErrAppointmentNotFound = errors.New("scheduling: appointment not found")

// Event types of AppointmentEvent.Type (booking-lifecycle.md).
const (
	EventCreated     = "CREATED"
	EventConfirmed   = "CONFIRMED"
	EventCancelled   = "CANCELLED" //nolint:misspell // stored value
	EventRescheduled = "RESCHEDULED"
	EventReassigned  = "REASSIGNED"
	EventStarted     = "STARTED"
	EventCompleted   = "COMPLETED"
	EventNoShow      = "NO_SHOW"
)

// Actor roles of AppointmentEvent.ActorRole.
const (
	ActorCustomer = "CUSTOMER"
	ActorBusiness = "BUSINESS"
)

// StatusCancelled is the status of a canceled appointment in AppointmentState.Status.
const StatusCancelled = "CANCELLED" //nolint:misspell // stored value

// AppointmentEvent is a change of an appointment just recorded in appointment_events (T7.1).
// ProfessionalID and Start are the values after the change.
type AppointmentEvent struct {
	ID, BusinessID, AppointmentID uuid.UUID
	Type                          string
	ActorRole                     string
	ProfessionalID                uuid.UUID
	Start                         time.Time // UTC
	CustomerPhone                 string    // E.164
	CustomerHasAccount            bool      // false for a customer the business booked by hand
	At                            time.Time
}

// EventListener is told of every recorded change of an appointment, inside the same transaction
// (ctx carries it: db.ContextWithTx), so what it stores commits or rolls back with the change.
// An error aborts the change: listeners must only do local, fast work (enqueue, insert).
type EventListener interface {
	AppointmentEventRecorded(ctx context.Context, e AppointmentEvent) error
}

// AppointmentState is the current state of an appointment, to re-check a pending notification.
type AppointmentState struct {
	ID, BusinessID, ProfessionalID uuid.UUID
	Status                         string
	Start                          time.Time // UTC
	Version                        int
}

// Appointments is what other modules read about appointments.
type Appointments interface {
	// AppointmentState returns ErrAppointmentNotFound when the appointment is not in the
	// business. No authorization (background jobs).
	AppointmentState(ctx context.Context, businessID, appointmentID uuid.UUID) (AppointmentState, error)
	// AuthorizeAppointmentView succeeds when p may see the appointment as staff of the business
	// (the same rule as its history: the owner or manager any appointment, a professional its
	// own). Otherwise the identity authorization error or ErrAppointmentNotFound.
	AuthorizeAppointmentView(ctx context.Context, p identity.Principal, businessID, appointmentID uuid.UUID) error
}
