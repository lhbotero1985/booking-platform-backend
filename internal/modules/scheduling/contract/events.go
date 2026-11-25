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

// Statuses of AppointmentState.Status and AppointmentEvent.Status used by other modules.
const (
	StatusCancelled           = "CANCELLED" //nolint:misspell // stored value
	StatusPendingConfirmation = "PENDING_CONFIRMATION"
)

// AppointmentEvent is a change of an appointment just recorded in appointment_events (T7.1).
// ProfessionalID and Start are the values after the change.
type AppointmentEvent struct {
	ID, BusinessID, AppointmentID uuid.UUID
	Type                          string
	ActorRole                     string
	Status                        string // of the appointment after the change
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

// AppointmentState is the current state of an appointment, to re-check a pending notification
// and to fill its message (T7.3).
type AppointmentState struct {
	ID, BusinessID, ProfessionalID uuid.UUID
	Status                         string
	Start                          time.Time // UTC
	Version                        int
	CustomerName                   string
	ServiceName                    string // snapshot taken when booked
	CancelReason                   string // "" unless canceled with a reason
	ProfessionalName               string // display name; "" if unknown
	BusinessName                   string
	TimeZone                       string // IANA zone of the business
	// T7.7: what a reschedule page needs to ask the public availability.
	ServiceID    uuid.UUID
	BusinessSlug string
	// T7.8: times the customer moved it (the business limits them).
	CustomerReschedules int
	// T7.6: who the appointment emails go to and the contact they show.
	CustomerEmail     string    // registered by the business; "" if none
	CustomerUserID    uuid.UUID // the customer's account; uuid.Nil if booked by hand without one
	BusinessAddress   string
	BusinessPhone     string   // E.164
	BusinessLatitude  *float64 // nil without coordinates
	BusinessLongitude *float64
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

// Errors of CustomerReplies.
var (
	// ErrCutoffPassed: too close to the appointment for the customer to cancel it.
	ErrCutoffPassed = errors.New("scheduling: the cancellation cut-off passed")
	// ErrNotActive: the appointment no longer allows that change (canceled, finished…).
	ErrNotActive = errors.New("scheduling: the appointment does not allow that change")
	// ErrSlotUnavailable: the new time is not offered (taken, outside the agenda) or the
	// appointment changed meanwhile (T7.7).
	ErrSlotUnavailable = errors.New("scheduling: that time is not available")
	// ErrRescheduleLimit: the customer already moved it as many times as the business allows
	// (T7.8).
	ErrRescheduleLimit = errors.New("scheduling: the customer's reschedule limit was reached")
)

// CustomerReplies applies the answer of a customer to a message about its appointment (T7.5: the
// buttons of the WhatsApp reminder), as that customer and with the customer's rules (cancellation
// cut-off). The caller has already authenticated the answer (single-use token of the message and
// the sender's number); no other authorization. Call it inside the caller's transaction.
type CustomerReplies interface {
	// ConfirmAsCustomer and CancelAsCustomer are no-ops when already done; ErrNotActive when the
	// status does not allow it; CancelAsCustomer: ErrCutoffPassed; ErrAppointmentNotFound.
	ConfirmAsCustomer(ctx context.Context, businessID, appointmentID uuid.UUID) error
	CancelAsCustomer(ctx context.Context, businessID, appointmentID uuid.UUID) error
	// RescheduleAsCustomer (T7.7: the email's link) moves it to a start offered by the public
	// availability, same professional, before the reschedule cut-off (ErrCutoffPassed) and
	// within the business's limit of moves (ErrRescheduleLimit); ErrSlotUnavailable; ErrNotActive; ErrAppointmentNotFound (also when the business is no
	// longer public).
	RescheduleAsCustomer(ctx context.Context, businessID, appointmentID uuid.UUID, start time.Time) error
}
