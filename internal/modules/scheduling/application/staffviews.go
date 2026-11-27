package application

// Views for the staff of a business (frontend proposals P-9 and P-10, 2026-11-26): the free
// times the staff may book, and the appointments of one customer of the business.

import (
	"context"
	"errors"

	"github.com/google/uuid"

	businesses "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/contract"
	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/authz"
)

// StaffAvailabilityQuery selects the free times of a professional for a service on a local day,
// as received: it is validated after the caller is authorized (a business of someone else is a
// 404 whatever the parameters).
type StaffAvailabilityQuery struct {
	ServiceID      string
	ProfessionalID string
	Date           string // YYYY-MM-DD in the business's calendar
	// ExcludeAppointmentID, when set, does not count that appointment as busy (the one being
	// moved), as the move itself does.
	ExcludeAppointmentID string
}

type staffAvailabilityInput struct {
	serviceID, professionalID, except uuid.UUID
	date                              domain.LocalDate
}

func (q StaffAvailabilityQuery) parse() (staffAvailabilityInput, error) {
	var in staffAvailabilityInput
	verr := &ValidationError{}
	var err error
	if in.serviceID, err = uuid.Parse(q.ServiceID); err != nil {
		verr.add("serviceId", "INVALID_ID")
	}
	if in.professionalID, err = uuid.Parse(q.ProfessionalID); err != nil {
		verr.add("professionalId", "INVALID_ID")
	}
	var ok bool
	if in.date, ok = domain.ParseLocalDate(q.Date); !ok {
		verr.add("date", "INVALID_DATE")
	}
	if q.ExcludeAppointmentID != "" {
		if in.except, err = uuid.Parse(q.ExcludeAppointmentID); err != nil {
			verr.add("excludeAppointmentId", "INVALID_ID")
		}
	}
	return in, verr.orNil()
}

// StaffAvailability returns the start times the staff can book or move an appointment to (P-9):
// the same rules as a manual booking or a move by the staff — working hours, breaks, absences,
// appointments and the booking horizon, without the minimum notice and whether or not the
// business is published. A PROFESSIONAL only asks for its own agenda (another →
// authz.ErrForbidden). Informative: the booking revalidates in its transaction (ADR-0004).
func (s *Service) StaffAvailability(ctx context.Context, p identity.Principal, businessID uuid.UUID, q StaffAvailabilityQuery) (Availability, error) {
	role, err := s.d.Authz.RequireBusinessRole(ctx, p, businessID, identity.RolesStaff...)
	if err != nil {
		return Availability{}, err
	}
	in, err := q.parse()
	if err != nil {
		return Availability{}, err
	}
	if role != identity.RoleOwner {
		own, err := s.ownProfiles(ctx, businessID, p.UserID)
		if err != nil {
			return Availability{}, err
		}
		if !contains(own, in.professionalID) {
			return Availability{}, authz.ErrForbidden
		}
	}
	pol, err := s.d.Businesses.BookingPolicyOf(ctx, businessID)
	if errors.Is(err, businesses.ErrBusinessNotFound) {
		return Availability{}, ErrNotFound
	}
	if err != nil {
		return Availability{}, err
	}
	pol.MinLeadMinutes = 0 // as manual.go and policyFor: the staff may serve a customer already there
	loc, err := location(pol)
	if err != nil {
		return Availability{}, err
	}
	var out Availability
	err = s.d.Store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		svc, err := s.bookableService(ctx, businessID, in.serviceID, in.professionalID)
		if err != nil {
			return err
		}
		slots, err := s.slotsExcept(ctx, tx, pol, loc, svc.DurationMinutes, in.professionalID, in.date, in.except)
		out = Availability{Location: loc, Date: in.date, DurationMinutes: svc.DurationMinutes, Slots: slots}
		return err
	})
	return out, err
}

// ownProfiles returns the professional profiles of an account in the business.
func (s *Service) ownProfiles(ctx context.Context, businessID, userID uuid.UUID) ([]uuid.UUID, error) {
	roster, err := s.d.Professionals.Roster(ctx, businessID)
	if err != nil {
		return nil, err
	}
	var own []uuid.UUID
	for _, r := range roster {
		if r.UserID == userID {
			own = append(own, r.ID)
		}
	}
	return own, nil
}

// CustomerAppointmentsQuery selects a page of the appointments of one customer of the business.
type CustomerAppointmentsQuery struct {
	Upcoming bool // true: from now on, ascending; false: past, newest first
	Cursor   string
	Limit    int
}

// CustomerHistory is a page of the appointments of a customer of the business.
type CustomerHistory = AgendaPage

// CustomerAppointments returns the appointments (every status) of a customer of the business
// (P-10): the OWNER sees all of them, a PROFESSIONAL only those of its agenda. A customer of
// another business is ErrCustomerNotFound, the same as a missing one.
func (s *Service) CustomerAppointments(ctx context.Context, p identity.Principal, businessID, customerID uuid.UUID, q CustomerAppointmentsQuery) (CustomerHistory, error) {
	role, err := s.d.Authz.RequireBusinessRole(ctx, p, businessID, identity.RolesStaff...)
	if err != nil {
		return CustomerHistory{}, err
	}
	verr := &ValidationError{}
	limit := pageSize(verr, q.Limit)
	after, ok := DecodeCursor(q.Cursor)
	if !ok {
		verr.add("cursor", "INVALID_CURSOR")
	}
	if err := verr.orNil(); err != nil {
		return CustomerHistory{}, err
	}
	pol, err := s.d.Businesses.BookingPolicyOf(ctx, businessID)
	if errors.Is(err, businesses.ErrBusinessNotFound) {
		return CustomerHistory{}, ErrNotFound
	}
	if err != nil {
		return CustomerHistory{}, err
	}
	loc, err := location(pol)
	if err != nil {
		return CustomerHistory{}, err
	}
	roster, err := s.d.Professionals.Roster(ctx, businessID)
	if err != nil {
		return CustomerHistory{}, err
	}
	names := map[uuid.UUID]string{}
	var filter []uuid.UUID // nil = every professional (OWNER)
	if role != identity.RoleOwner {
		filter = []uuid.UUID{}
	}
	for _, r := range roster {
		names[r.ID] = r.DisplayName
		if role != identity.RoleOwner && r.UserID == p.UserID {
			filter = append(filter, r.ID)
		}
	}
	now := s.d.Clock.Now()
	page := CustomerHistory{Location: loc, Items: []AgendaItem{}}
	var rows []domain.Appointment
	err = s.d.Store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		if _, err := tx.CustomerByID(ctx, businessID, customerID); err != nil {
			return err
		}
		if filter != nil && len(filter) == 0 {
			return nil // a PROFESSIONAL without a profile: nothing of its agenda
		}
		rows, err = tx.BusinessCustomerAppointments(ctx, businessID, customerID, filter, now, q.Upcoming, after, limit+1)
		return err
	})
	if err != nil {
		return CustomerHistory{}, err
	}
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[limit-1]
		page.NextCursor = Cursor{Start: last.Start, ID: last.ID}.Encode()
	}
	stranded, _, err := s.stranded(ctx, businessID)
	if err != nil {
		return CustomerHistory{}, err
	}
	for _, a := range rows {
		page.Items = append(page.Items, AgendaItem{Appointment: a, ProfessionalName: names[a.ProfessionalID],
			NeedsReassignment: a.Status.Occupies() && a.End.After(now) && contains(stranded, a.ProfessionalID)})
	}
	return page, nil
}
