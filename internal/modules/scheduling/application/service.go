// Package application holds the scheduling use cases: public availability (T6.2) and customer
// bookings (T6.3).
package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	businesses "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/contract"
	catalog "github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/contract"
	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	professionals "github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
)

// ErrNotFound means the business is not public, or the service is not bookable with that
// professional. One error on purpose: the public API does not reveal which part is missing.
var ErrNotFound = errors.New("scheduling: not found")

// Store is the persistence of the scheduling module.
type Store interface {
	// WithinTx runs fn in a transaction; ctx carries it (db.ContextWithTx) so the contracts of
	// other modules join it.
	WithinTx(ctx context.Context, fn func(ctx context.Context, tx Tx) error) error
}

// Tx is the persistence available inside a transaction.
type Tx interface {
	// ActiveAppointments returns the agenda occupation of active appointments overlapping
	// [from, to).
	ActiveAppointments(ctx context.Context, businessID, professionalID uuid.UUID, from, to time.Time) ([]domain.Booked, error)
	BookingTx
	CustomerTx
	AgendaTx
}

// Authorizer is the part of identity's authorizer the staff use cases need.
type Authorizer interface {
	RequireBusinessRole(ctx context.Context, p identity.Principal, businessID uuid.UUID, allowed ...identity.Role) (identity.Role, error)
}

// Deps are the contracts of the other modules the scheduling use cases read.
type Deps struct {
	Store         Store
	Authz         Authorizer
	Businesses    businesses.PublicBooking
	Catalog       catalog.Booking
	Professionals professionals.Agenda
	Customers     identity.Customers
	Clock         clock.Clock
}

// Service implements the scheduling use cases.
type Service struct{ d Deps }

// NewService builds the service.
func NewService(d Deps) *Service { return &Service{d: d} }

// Availability is the result of a public availability query.
type Availability struct {
	Location        *time.Location
	Date            domain.LocalDate
	DurationMinutes int
	Slots           []domain.Slot
}

// PublicAvailability returns the start times customers can book with a professional for a
// service on a local day of the business (availability-engine.md). Informative only: the booking
// revalidates inside its transaction (ADR-0004). domain.ErrOutsideBookingWindow for a date
// before today or beyond the business's horizon.
func (s *Service) PublicAvailability(ctx context.Context, slug string, serviceID, professionalID uuid.UUID, date domain.LocalDate) (Availability, error) {
	pol, err := s.d.Businesses.PublicBookingPolicy(ctx, slug)
	if errors.Is(err, businesses.ErrBusinessNotFound) {
		return Availability{}, ErrNotFound
	}
	if err != nil {
		return Availability{}, err
	}
	loc, err := location(pol)
	if err != nil {
		return Availability{}, err
	}
	var out Availability
	err = s.d.Store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		svc, err := s.bookableService(ctx, pol.BusinessID, serviceID, professionalID)
		if err != nil {
			return err
		}
		slots, err := s.slots(ctx, tx, pol, loc, svc.DurationMinutes, professionalID, date)
		out = Availability{Location: loc, Date: date, DurationMinutes: svc.DurationMinutes, Slots: slots}
		return err
	})
	return out, err
}

func location(pol businesses.BookingPolicy) (*time.Location, error) {
	loc, err := time.LoadLocation(pol.TimeZone)
	if err != nil {
		return nil, fmt.Errorf("scheduling: time zone of business %s: %w", pol.BusinessID, err)
	}
	return loc, nil
}

func (s *Service) bookableService(ctx context.Context, businessID, serviceID, professionalID uuid.UUID) (catalog.BookableService, error) {
	svc, err := s.d.Catalog.BookableService(ctx, businessID, serviceID, professionalID)
	if errors.Is(err, catalog.ErrNotBookable) {
		return catalog.BookableService{}, ErrNotFound
	}
	return svc, err
}

// maxOccupation bounds how far an appointment can reach past the day (480 min + 60 min buffer):
// appointments starting that late the next day are still read, so the buffer of the last slot of
// the day is checked against them.
const maxOccupation = 9 * time.Hour

// slots runs the engine for one local day with the professional's agenda and active appointments.
func (s *Service) slots(ctx context.Context, tx Tx, pol businesses.BookingPolicy, loc *time.Location, minutes int, professionalID uuid.UUID, date domain.LocalDate) ([]domain.Slot, error) {
	from, to := domain.ResolveLocal(date, 0, loc), domain.ResolveLocal(date.AddDays(1), 0, loc)
	agenda, err := s.d.Professionals.DayAgenda(ctx, pol.BusinessID, professionalID, date.Weekday(), from, to)
	if errors.Is(err, professionals.ErrProfessionalNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	booked, err := tx.ActiveAppointments(ctx, pol.BusinessID, professionalID, from.Add(-maxOccupation), to.Add(maxOccupation))
	if err != nil {
		return nil, err
	}
	in := domain.AvailabilityInput{
		Location: loc,
		Policy: domain.Policy{
			SlotInterval:   time.Duration(pol.SlotIntervalMinutes) * time.Minute,
			Buffer:         time.Duration(pol.BufferMinutes) * time.Minute,
			MinLead:        time.Duration(pol.MinLeadMinutes) * time.Minute,
			MaxAdvanceDays: pol.MaxAdvanceDays,
		},
		Duration: time.Duration(minutes) * time.Minute,
		Date:     date,
		Now:      s.d.Clock.Now(),
		Booked:   booked,
	}
	for _, r := range agenda.WorkingHours {
		in.WorkingHours = append(in.WorkingHours, domain.LocalRange{Start: r.Start, End: r.End})
	}
	for _, r := range agenda.Breaks {
		in.Breaks = append(in.Breaks, domain.LocalRange{Start: r.Start, End: r.End})
	}
	for _, a := range agenda.Absences {
		in.Absences = append(in.Absences, domain.Interval{Start: a.Start, End: a.End})
	}
	return domain.Available(in)
}
