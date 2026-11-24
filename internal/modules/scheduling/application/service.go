// Package application holds the scheduling use cases: public availability (T6.2) and, later,
// appointments (T6.3+).
package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	businesses "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/contract"
	catalog "github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/contract"
	professionals "github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
)

// ErrNotFound means the business is not public, or the service is not bookable with that
// professional. One error on purpose: the public API does not reveal which part is missing.
var ErrNotFound = errors.New("scheduling: not found")

// Deps are the contracts of the other modules the scheduling use cases read.
type Deps struct {
	Businesses    businesses.PublicBooking
	Catalog       catalog.Booking
	Professionals professionals.Agenda
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
	loc, err := time.LoadLocation(pol.TimeZone)
	if err != nil {
		return Availability{}, fmt.Errorf("scheduling: time zone of business %s: %w", pol.BusinessID, err)
	}
	minutes, err := s.d.Catalog.BookableService(ctx, pol.BusinessID, serviceID, professionalID)
	if errors.Is(err, catalog.ErrNotBookable) {
		return Availability{}, ErrNotFound
	}
	if err != nil {
		return Availability{}, err
	}
	from, to := domain.ResolveLocal(date, 0, loc), domain.ResolveLocal(date.AddDays(1), 0, loc)
	agenda, err := s.d.Professionals.DayAgenda(ctx, pol.BusinessID, professionalID, date.Weekday(), from, to)
	if errors.Is(err, professionals.ErrProfessionalNotFound) {
		return Availability{}, ErrNotFound
	}
	if err != nil {
		return Availability{}, err
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
		// Booked appointments arrive with T6.3.
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
	slots, err := domain.Available(in)
	if err != nil {
		return Availability{}, err
	}
	return Availability{Location: loc, Date: date, DurationMinutes: minutes, Slots: slots}, nil
}
