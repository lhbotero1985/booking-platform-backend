package domain

// Appointment aggregate (T6.3, booking-lifecycle.md). Transitions arrive with T6.6.

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Status of an appointment (matches the appointments_status_valid CHECK).
type Status string

// Appointment states (booking-lifecycle.md §1).
const (
	StatusPendingConfirmation Status = "PENDING_CONFIRMATION"
	StatusConfirmed           Status = "CONFIRMED"
	StatusInProgress          Status = "IN_PROGRESS"
	StatusCompleted           Status = "COMPLETED"
	StatusCancelled           Status = "CANCELLED" //nolint:misspell // stored value, same spelling as the API and the database
	StatusNoShow              Status = "NO_SHOW"
)

// Occupies reports whether the status blocks the professional's agenda.
func (s Status) Occupies() bool {
	return s == StatusPendingConfirmation || s == StatusConfirmed || s == StatusInProgress
}

// EventType of an appointment_events row.
type EventType string

// Event types (matches the appointment_events_type_valid CHECK).
const (
	EventCreated EventType = "CREATED"
)

// ErrInvalidAppointment signals a programming error building an appointment.
var ErrInvalidAppointment = errors.New("scheduling: invalid appointment")

// ServiceSnapshot is the service as it was when booked: later catalog changes do not alter the
// appointment (booking-lifecycle.md §5.2).
type ServiceSnapshot struct {
	ServiceID       uuid.UUID
	Name            string
	DurationMinutes int
	PriceMinor      int64
	Currency        string
}

// Customer of an appointment, as it was when booked. CustomerID is the business's customer
// record (T6.4; uuid.Nil if none); UserID is uuid.Nil for a customer without an account.
type Customer struct {
	CustomerID uuid.UUID
	UserID     uuid.UUID
	Name       string
	Phone      string // E.164
}

// Appointment is a booking of a service with a professional.
type Appointment struct {
	ID             uuid.UUID
	BusinessID     uuid.UUID
	ProfessionalID uuid.UUID
	Service        ServiceSnapshot
	Customer       Customer
	Start, End     time.Time // UTC, [Start, End): the service
	Buffer         time.Duration
	Status         Status
	Version        int
	CreatedBy      uuid.UUID
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// OccupiedUntil is the end of the service plus the preparation time.
func (a Appointment) OccupiedUntil() time.Time { return a.End.Add(a.Buffer) }

// NewAppointment books a slot already validated by the availability engine. It starts
// PENDING_CONFIRMATION (booking-lifecycle.md §2).
func NewAppointment(id, businessID, professionalID uuid.UUID, svc ServiceSnapshot, c Customer, start time.Time,
	buffer time.Duration, createdBy uuid.UUID, now time.Time,
) (Appointment, error) {
	if id == uuid.Nil || businessID == uuid.Nil || professionalID == uuid.Nil || svc.ServiceID == uuid.Nil ||
		svc.DurationMinutes <= 0 || buffer < 0 || c.Name == "" || c.Phone == "" || createdBy == uuid.Nil {
		return Appointment{}, ErrInvalidAppointment
	}
	start = start.UTC()
	return Appointment{
		ID: id, BusinessID: businessID, ProfessionalID: professionalID, Service: svc, Customer: c,
		Start: start, End: start.Add(time.Duration(svc.DurationMinutes) * time.Minute), Buffer: buffer,
		Status: StatusPendingConfirmation, Version: 1, CreatedBy: createdBy, CreatedAt: now.UTC(), UpdatedAt: now.UTC(),
	}, nil
}
