package application

// Coherence of the agenda (T6.7, P11): the professionals module asks, before saving an absence or
// a weekly schedule, which active appointments the change would break; the change is refused and
// those appointments listed. Professionals no longer bookable leave their future appointments to
// reassign.

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	businesses "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/contract"
	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/domain"
)

// horizon bounds the future appointments a schedule change is checked against (longer than any
// business's booking horizon, 365 days).
const horizon = 400 * 24 * time.Hour

// AppointmentsDuring implements the scheduling contract AgendaGuard.
func (s *Service) AppointmentsDuring(ctx context.Context, businessID, professionalID uuid.UUID, from, to time.Time) ([]contract.ConflictingAppointment, error) {
	var rows []domain.Appointment
	err := s.d.Store.WithinTx(ctx, func(ctx context.Context, tx Tx) (err error) {
		rows, err = tx.AppointmentsDuring(ctx, businessID, professionalID, from, to)
		return err
	})
	return toConflicts(rows, func(domain.Appointment) bool { return true }), err
}

// AppointmentsOutside implements the scheduling contract AgendaGuard: the active appointments not
// finished yet that would fall outside the working hours of their local weekday (in the business's
// time zone) or overlap a break.
func (s *Service) AppointmentsOutside(ctx context.Context, businessID, professionalID uuid.UUID, week [7]contract.Day) ([]contract.ConflictingAppointment, error) {
	pol, err := s.d.Businesses.BookingPolicyOf(ctx, businessID)
	if errors.Is(err, businesses.ErrBusinessNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	loc, err := location(pol)
	if err != nil {
		return nil, err
	}
	now := s.d.Clock.Now()
	var rows []domain.Appointment
	err = s.d.Store.WithinTx(ctx, func(ctx context.Context, tx Tx) (err error) {
		rows, err = tx.AppointmentsDuring(ctx, businessID, professionalID, now, now.Add(horizon))
		return err
	})
	return toConflicts(rows, func(a domain.Appointment) bool { return !fits(a, week, loc) }), err
}

// fits reports whether the service lies inside one working range of its local day and off every
// break.
func fits(a domain.Appointment, week [7]contract.Day, loc *time.Location) bool {
	start, end := a.Start.In(loc), a.End.In(loc)
	if domain.LocalDateOf(a.Start, loc) != domain.LocalDateOf(a.End.Add(-time.Nanosecond), loc) {
		return false // crosses midnight: no working range does
	}
	from := start.Hour()*60 + start.Minute()
	to := end.Hour()*60 + end.Minute()
	if to == 0 {
		to = 24 * 60
	}
	day := week[start.Weekday()]
	inside := false
	for _, r := range day.WorkingHours {
		inside = inside || (r.Start <= from && to <= r.End)
	}
	for _, b := range day.Breaks {
		if from < b.End && b.Start < to {
			return false
		}
	}
	return inside
}

func toConflicts(rows []domain.Appointment, keep func(domain.Appointment) bool) []contract.ConflictingAppointment {
	out := []contract.ConflictingAppointment{}
	for _, a := range rows {
		if keep(a) {
			out = append(out, contract.ConflictingAppointment{ID: a.ID, Start: a.Start, End: a.End, CustomerName: a.Customer.Name, Status: string(a.Status)})
		}
	}
	return out
}

// ToReassign returns the active appointments not finished yet whose professional is no longer
// bookable (inactive profile or no longer a member), for the OWNER to move them to another
// professional (decision of 2026-11-24). Up to MaxPageSize, by start.
func (s *Service) ToReassign(ctx context.Context, p identity.Principal, businessID uuid.UUID) (AgendaPage, error) {
	if _, err := s.d.Authz.RequireBusinessRole(ctx, p, businessID, identity.RolesOwner...); err != nil {
		return AgendaPage{}, err
	}
	pol, err := s.d.Businesses.BookingPolicyOf(ctx, businessID)
	if errors.Is(err, businesses.ErrBusinessNotFound) {
		return AgendaPage{}, ErrNotFound
	}
	if err != nil {
		return AgendaPage{}, err
	}
	loc, err := location(pol)
	if err != nil {
		return AgendaPage{}, err
	}
	stranded, names, err := s.stranded(ctx, businessID)
	if err != nil {
		return AgendaPage{}, err
	}
	now := s.d.Clock.Now()
	page := AgendaPage{Location: loc, From: domain.LocalDateOf(now, loc), To: domain.LocalDateOf(now.Add(horizon), loc), Items: []AgendaItem{}}
	if len(stranded) == 0 {
		return page, nil
	}
	var rows []domain.Appointment
	err = s.d.Store.WithinTx(ctx, func(ctx context.Context, tx Tx) (err error) {
		// From a day back so the ones in progress are listed too (only the ones not started can be
		// moved).
		rows, err = tx.AgendaAppointments(ctx, businessID, stranded, now.Add(-24*time.Hour), now.Add(horizon), nil, MaxPageSize)
		return err
	})
	if err != nil {
		return AgendaPage{}, err
	}
	for _, a := range rows {
		if a.Status.Occupies() && a.End.After(now) {
			page.Items = append(page.Items, AgendaItem{Appointment: a, ProfessionalName: names[a.ProfessionalID], NeedsReassignment: true})
		}
	}
	return page, nil
}

// stranded returns the professional profiles of the business that are not bookable, and every
// profile's name.
func (s *Service) stranded(ctx context.Context, businessID uuid.UUID) ([]uuid.UUID, map[uuid.UUID]string, error) {
	roster, err := s.d.Professionals.Roster(ctx, businessID)
	if err != nil {
		return nil, nil, err
	}
	bookable, err := s.d.Bookable.BookableProfessionals(ctx, []uuid.UUID{businessID})
	if err != nil {
		return nil, nil, err
	}
	ok := map[uuid.UUID]bool{}
	for _, p := range bookable[businessID] {
		ok[p.ID] = true
	}
	names := map[uuid.UUID]string{}
	stranded := []uuid.UUID{}
	for _, r := range roster {
		names[r.ID] = r.DisplayName
		if !ok[r.ID] {
			stranded = append(stranded, r.ID)
		}
	}
	return stranded, names, nil
}
