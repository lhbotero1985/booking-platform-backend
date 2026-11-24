package application

// Manual appointments (T6.4, booking-lifecycle.md §3 and §6): the OWNER books for any
// professional, a PROFESSIONAL only in its own agenda, for a customer of the business. Same
// guarantees as a customer booking (lock + revalidation + exclusion constraint), without the
// minimum notice (the customer may be present) and whether or not the business is public.

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	businesses "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/contract"
	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	professionals "github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/authz"
)

// ManualInput is a manual booking by the staff.
type ManualInput struct {
	ProfessionalID, ServiceID, CustomerID uuid.UUID
	Start                                 time.Time
	// Confirmed books it CONFIRMED (the customer is present or called); otherwise
	// PENDING_CONFIRMATION.
	Confirmed      bool
	IdempotencyKey string
}

func (in ManualInput) fingerprint(businessID uuid.UUID) string {
	confirmed := "pending"
	if in.Confirmed {
		confirmed = "confirmed"
	}
	return BookingInput{BusinessID: businessID, ServiceID: in.ServiceID, ProfessionalID: in.ProfessionalID, Start: in.Start}.fingerprint() +
		"|" + in.CustomerID.String() + "|" + confirmed
}

// BookManual creates an appointment for a customer of the business. Errors: authorization
// (another business → 404, a PROFESSIONAL in another agenda → authz.ErrForbidden),
// ValidationError (unknown customer, professional or service not bookable),
// domain.ErrOutsideBookingWindow, ErrSlotUnavailable, ErrIdempotencyKeyReused,
// ErrInvalidIdempotencyKey, ErrBusy.
func (s *Service) BookManual(ctx context.Context, p identity.Principal, businessID uuid.UUID, in ManualInput) (domain.Appointment, error) {
	role, err := s.d.Authz.RequireBusinessRole(ctx, p, businessID, identity.RolesStaff...)
	if err != nil {
		return domain.Appointment{}, err
	}
	verr := &ValidationError{}
	for _, f := range []struct {
		name string
		id   uuid.UUID
	}{{"professionalId", in.ProfessionalID}, {"serviceId", in.ServiceID}, {"customerId", in.CustomerID}} {
		if f.id == uuid.Nil {
			verr.add(f.name, CodeRequired)
		}
	}
	if in.Start.IsZero() {
		verr.add("startAt", CodeRequired)
	}
	if err := verr.orNil(); err != nil {
		return domain.Appointment{}, err
	}
	if !idempotencyKey.MatchString(in.IdempotencyKey) {
		return domain.Appointment{}, ErrInvalidIdempotencyKey
	}
	pol, err := s.d.Businesses.BookingPolicyOf(ctx, businessID)
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
	pol.MinLeadMinutes = 0 // the staff may book a customer who is already there
	fingerprint := in.fingerprint(businessID)
	var out domain.Appointment
	err = s.d.Store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		if err := tx.SetBookingTimeouts(ctx); err != nil {
			return err
		}
		now := s.d.Clock.Now()
		claimed, err := tx.ClaimIdempotencyKey(ctx, p.UserID, in.IdempotencyKey, fingerprint, now)
		if err != nil {
			return err
		}
		if !claimed {
			fp, id, err := tx.IdempotencyKeyOf(ctx, p.UserID, in.IdempotencyKey)
			if err != nil {
				return err
			}
			if fp != fingerprint || id == uuid.Nil {
				return ErrIdempotencyKeyReused
			}
			out, err = tx.AppointmentByID(ctx, id)
			return err
		}
		if role != identity.RoleOwner {
			owner, err := s.d.Professionals.ProfessionalUserID(ctx, businessID, in.ProfessionalID)
			if errors.Is(err, professionals.ErrProfessionalNotFound) {
				return &ValidationError{Fields: []FieldError{{"professionalId", CodeProfessionalInvalid}}}
			}
			if err != nil {
				return err
			}
			if owner != p.UserID {
				return authz.ErrForbidden
			}
		}
		if err := s.d.Professionals.LockAgenda(ctx, businessID, in.ProfessionalID); err != nil {
			if errors.Is(err, professionals.ErrProfessionalNotFound) {
				return &ValidationError{Fields: []FieldError{{"professionalId", CodeProfessionalInvalid}}}
			}
			return err
		}
		customer, err := tx.CustomerByID(ctx, businessID, in.CustomerID)
		if errors.Is(err, ErrCustomerNotFound) {
			return &ValidationError{Fields: []FieldError{{"customerId", CodeCustomerNotFound}}}
		}
		if err != nil {
			return err
		}
		svc, err := s.bookableService(ctx, businessID, in.ServiceID, in.ProfessionalID)
		if errors.Is(err, ErrNotFound) {
			return &ValidationError{Fields: []FieldError{{"serviceId", "SERVICE_NOT_BOOKABLE"}}}
		}
		if err != nil {
			return err
		}
		slots, err := s.slots(ctx, tx, pol, loc, svc.DurationMinutes, in.ProfessionalID, domain.LocalDateOf(in.Start, loc))
		if err != nil {
			return err
		}
		if !offered(slots, in.Start) {
			return ErrSlotUnavailable
		}
		a, err := domain.NewAppointment(uuid.Must(uuid.NewV7()), businessID, in.ProfessionalID,
			domain.ServiceSnapshot{ServiceID: in.ServiceID, Name: svc.Name, DurationMinutes: svc.DurationMinutes, PriceMinor: svc.PriceMinor, Currency: svc.Currency},
			domain.Customer{CustomerID: customer.ID, UserID: customer.UserID, Name: customer.FullName, Phone: customer.Phone},
			in.Start, time.Duration(pol.BufferMinutes)*time.Minute, p.UserID, now)
		if err != nil {
			return err
		}
		if in.Confirmed {
			a.Status = domain.StatusConfirmed
		}
		if err := tx.InsertAppointment(ctx, a); err != nil {
			return err
		}
		if err := tx.InsertAppointmentEvent(ctx, uuid.Must(uuid.NewV7()), a, domain.EventCreated, p.UserID, now); err != nil {
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
