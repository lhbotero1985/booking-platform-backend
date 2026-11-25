package application

// Customer bookings (T6.3, availability-engine.md §6, ADR-0004): one transaction that claims the
// Idempotency-Key, takes the professional's row lock, revalidates the slot with the engine and
// inserts the appointment; the exclusion constraint is the safety net under the lock.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"time"

	"github.com/google/uuid"

	businesses "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/contract"
	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	professionals "github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/domain"
)

// Booking errors.
var (
	// ErrSlotUnavailable means the start time is not bookable (taken, outside the agenda, too
	// soon, off the grid) or another booking won the race.
	ErrSlotUnavailable = errors.New("scheduling: slot unavailable")
	// ErrPhoneRequired means the customer's account has no mobile number.
	ErrPhoneRequired = errors.New("scheduling: phone required to book")
	// ErrIdempotencyKeyReused means the key was used for a different booking request.
	ErrIdempotencyKeyReused = errors.New("scheduling: idempotency key reused with another request")
	// ErrInvalidIdempotencyKey means the Idempotency-Key header is missing or malformed.
	ErrInvalidIdempotencyKey = errors.New("scheduling: invalid idempotency key")
	// ErrBusy means the professional's agenda stayed locked too long (lock timeout): retryable.
	ErrBusy = errors.New("scheduling: agenda busy, retry")
)

// BookingTx is the booking persistence available inside a transaction.
type BookingTx interface {
	// SetBookingTimeouts bounds the wait for locks in this transaction (ADR-0004).
	SetBookingTimeouts(ctx context.Context) error
	// ClaimIdempotencyKey inserts the key; false if it already existed (it waits for a
	// concurrent transaction holding the same key to finish).
	ClaimIdempotencyKey(ctx context.Context, userID uuid.UUID, key, fingerprint string, now time.Time) (bool, error)
	// IdempotencyKeyOf returns the fingerprint and appointment of an existing key.
	IdempotencyKeyOf(ctx context.Context, userID uuid.UUID, key string) (fingerprint string, appointmentID uuid.UUID, err error)
	SetIdempotencyAppointment(ctx context.Context, userID uuid.UUID, key string, appointmentID uuid.UUID) error
	// InsertAppointment returns ErrSlotUnavailable when the exclusion constraint rejects it.
	InsertAppointment(ctx context.Context, a domain.Appointment) error
	InsertAppointmentEvent(ctx context.Context, businessID, appointmentID uuid.UUID, e domain.Event) error
	AppointmentByID(ctx context.Context, id uuid.UUID) (domain.Appointment, error)
	// LockAppointment is AppointmentByID taking a row lock (T6.6).
	LockAppointment(ctx context.Context, id uuid.UUID) (domain.Appointment, error)
	// UpdateAppointment writes a change if the stored version is still version
	// (ErrAppointmentChanged otherwise; ErrSlotUnavailable if the new time overlaps).
	UpdateAppointment(ctx context.Context, a domain.Appointment, version int) error
	AppointmentEvents(ctx context.Context, appointmentID uuid.UUID) ([]domain.Event, error)
}

var idempotencyKey = regexp.MustCompile(`^[A-Za-z0-9_-]{8,100}$`)

// BookingInput is a customer's booking request.
type BookingInput struct {
	BusinessID, ServiceID, ProfessionalID uuid.UUID
	Start                                 time.Time
	IdempotencyKey                        string
}

func (in BookingInput) fingerprint() string {
	sum := sha256.Sum256([]byte(in.BusinessID.String() + "|" + in.ServiceID.String() + "|" +
		in.ProfessionalID.String() + "|" + in.Start.UTC().Format(time.RFC3339Nano)))
	return hex.EncodeToString(sum[:])
}

// Book creates a PENDING_CONFIRMATION appointment for the authenticated customer (verified
// email, mandatory phone). Repeating the request with the same Idempotency-Key returns the same
// appointment. Errors: ErrNotFound (business, service or professional not bookable),
// domain.ErrOutsideBookingWindow, ErrSlotUnavailable, ErrPhoneRequired, ErrIdempotencyKeyReused,
// ErrInvalidIdempotencyKey, ErrBusy.
func (s *Service) Book(ctx context.Context, p identity.Principal, in BookingInput) (domain.Appointment, error) {
	if !idempotencyKey.MatchString(in.IdempotencyKey) {
		return domain.Appointment{}, ErrInvalidIdempotencyKey
	}
	customer, err := s.d.Customers.CustomerProfile(ctx, p.UserID)
	if errors.Is(err, identity.ErrPhoneRequired) {
		return domain.Appointment{}, ErrPhoneRequired
	}
	if err != nil {
		return domain.Appointment{}, err
	}
	pol, err := s.d.Businesses.PublicBookingPolicyOf(ctx, in.BusinessID)
	if errors.Is(err, businesses.ErrBusinessNotFound) {
		return domain.Appointment{}, ErrNotFound
	}
	if err != nil {
		return domain.Appointment{}, err
	}
	loc, err := location(pol)
	if err != nil {
		return domain.Appointment{}, err
	}
	var out domain.Appointment
	err = s.d.Store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		if err := tx.SetBookingTimeouts(ctx); err != nil {
			return err
		}
		now := s.d.Clock.Now()
		claimed, err := tx.ClaimIdempotencyKey(ctx, p.UserID, in.IdempotencyKey, in.fingerprint(), now)
		if err != nil {
			return err
		}
		if !claimed { // a previous request with this key committed: replay it
			fp, id, err := tx.IdempotencyKeyOf(ctx, p.UserID, in.IdempotencyKey)
			if err != nil {
				return err
			}
			if fp != in.fingerprint() || id == uuid.Nil {
				return ErrIdempotencyKeyReused
			}
			out, err = tx.AppointmentByID(ctx, id)
			return err
		}
		// Serializes every write to this professional's agenda until commit (ADR-0004).
		if err := s.d.Professionals.LockAgenda(ctx, pol.BusinessID, in.ProfessionalID); err != nil {
			return mapNotFound(err)
		}
		svc, err := s.bookableService(ctx, pol.BusinessID, in.ServiceID, in.ProfessionalID)
		if err != nil {
			return err
		}
		date := domain.LocalDateOf(in.Start, loc)
		slots, err := s.slots(ctx, tx, pol, loc, svc.DurationMinutes, in.ProfessionalID, date)
		if err != nil {
			return err
		}
		if !offered(slots, in.Start) {
			return ErrSlotUnavailable
		}
		record, err := customerForBooking(ctx, tx, pol.BusinessID, customer, now)
		if err != nil {
			return err
		}
		a, err := domain.NewAppointment(uuid.Must(uuid.NewV7()), pol.BusinessID, in.ProfessionalID,
			domain.ServiceSnapshot{ServiceID: in.ServiceID, Name: svc.Name, DurationMinutes: svc.DurationMinutes, PriceMinor: svc.PriceMinor, Currency: svc.Currency},
			domain.Customer{CustomerID: record, UserID: p.UserID, Name: customer.FullName, Phone: customer.Phone},
			in.Start, time.Duration(pol.BufferMinutes)*time.Minute, p.UserID, now)
		if err != nil {
			return err
		}
		if err := tx.InsertAppointment(ctx, a); err != nil {
			return err
		}
		if err := s.record(ctx, tx, a, createdEvent(a, p.UserID, domain.ActorCustomer, now)); err != nil {
			return err
		}
		if err := tx.SetIdempotencyAppointment(ctx, p.UserID, in.IdempotencyKey, a.ID); err != nil {
			return err
		}
		out = a
		return nil
	})
	return out, err
}

func offered(slots []domain.Slot, start time.Time) bool {
	for _, sl := range slots {
		if sl.Start.Equal(start) {
			return true
		}
	}
	return false
}

func mapNotFound(err error) error {
	if errors.Is(err, professionals.ErrProfessionalNotFound) {
		return ErrNotFound
	}
	return err
}

func createdEvent(a domain.Appointment, actor uuid.UUID, role domain.ActorRole, now time.Time) domain.Event {
	return domain.Event{ID: uuid.Must(uuid.NewV7()), Type: domain.EventCreated, Status: a.Status, ProfessionalID: a.ProfessionalID,
		Start: a.Start, Actor: actor, ActorRole: role, At: now}
}
