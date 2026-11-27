package application

// Agenda and "my appointments" (T6.5, decision of 2026-11-24): a PROFESSIONAL sees its own
// agenda; the OWNER sees every professional's (or one, its own included); a customer sees its
// appointments in every business. Day, week, month or year in the business's calendar.

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	businesses "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/contract"
	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/authz"
)

// Page sizes.
const (
	DefaultPageSize = 100
	MaxPageSize     = 500
)

// ErrAppointmentNotFound means there is no such appointment for the caller.
var ErrAppointmentNotFound = errors.New("scheduling: appointment not found")

// Cursor is a keyset position: the last (start, id) returned.
type Cursor struct {
	Start time.Time
	ID    uuid.UUID
}

// Encode returns the opaque form given to clients.
func (c Cursor) Encode() string {
	return base64.RawURLEncoding.EncodeToString([]byte(c.Start.UTC().Format(time.RFC3339Nano) + "|" + c.ID.String()))
}

// DecodeCursor parses an opaque cursor ("" = none).
func DecodeCursor(s string) (*Cursor, bool) {
	if s == "" {
		return nil, true
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, false
	}
	at, id, ok := strings.Cut(string(raw), "|")
	if !ok {
		return nil, false
	}
	t, err1 := time.Parse(time.RFC3339Nano, at)
	u, err2 := uuid.Parse(id)
	if err1 != nil || err2 != nil {
		return nil, false
	}
	return &Cursor{Start: t, ID: u}, true
}

// AgendaTx is the agenda persistence available inside a transaction.
type AgendaTx interface {
	// AgendaAppointments returns up to limit appointments of the business starting in [from, to),
	// by start, of every professional (professionalIDs nil) or of some, after the cursor.
	AgendaAppointments(ctx context.Context, businessID uuid.UUID, professionalIDs []uuid.UUID, from, to time.Time, after *Cursor, limit int) ([]domain.Appointment, error)
	// CustomerAppointments returns up to limit appointments of an account, upcoming (start >=
	// now, ascending) or past (descending), after the cursor.
	CustomerAppointments(ctx context.Context, userID uuid.UUID, now time.Time, upcoming bool, after *Cursor, limit int) ([]domain.Appointment, error)
	// BusinessCustomerAppointments returns up to limit appointments of a customer of the business
	// (P-10), of every professional (professionalIDs nil) or of some, upcoming (start >= now,
	// ascending) or past (descending), after the cursor.
	BusinessCustomerAppointments(ctx context.Context, businessID, customerID uuid.UUID, professionalIDs []uuid.UUID, now time.Time, upcoming bool, after *Cursor, limit int) ([]domain.Appointment, error)
	// AppointmentsDuring returns the active appointments of a professional whose service overlaps
	// [from, to), by start (T6.7).
	AppointmentsDuring(ctx context.Context, businessID, professionalID uuid.UUID, from, to time.Time) ([]domain.Appointment, error)
}

// AgendaQuery selects an agenda page.
type AgendaQuery struct {
	View           domain.View
	Date           domain.LocalDate // zero = today in the business's calendar
	ProfessionalID uuid.UUID        // uuid.Nil = every professional the caller may see
	Cursor         string
	Limit          int
}

// AgendaItem is an appointment with the professional's name.
type AgendaItem struct {
	domain.Appointment
	ProfessionalName string
	// NeedsReassignment: active and not finished, but its professional is no longer bookable
	// (T6.7): the OWNER should move it to another professional.
	NeedsReassignment bool
}

// AgendaPage is a page of an agenda.
type AgendaPage struct {
	Location   *time.Location
	From, To   domain.LocalDate // [From, To)
	Items      []AgendaItem
	NextCursor string
}

func pageSize(verr *ValidationError, n int) int {
	switch {
	case n == 0:
		return DefaultPageSize
	case n < 1 || n > MaxPageSize:
		verr.add("limit", "OUT_OF_RANGE")
	}
	return n
}

// Agenda returns the appointments (every status) of a view of the business's calendar. A
// PROFESSIONAL sees only its own agenda (another professional → authz.ErrForbidden); the OWNER
// every professional's or one.
func (s *Service) Agenda(ctx context.Context, p identity.Principal, businessID uuid.UUID, q AgendaQuery) (AgendaPage, error) {
	role, err := s.d.Authz.RequireBusinessRole(ctx, p, businessID, identity.RolesStaff...)
	if err != nil {
		return AgendaPage{}, err
	}
	verr := &ValidationError{}
	limit := pageSize(verr, q.Limit)
	after, ok := DecodeCursor(q.Cursor)
	if !ok {
		verr.add("cursor", "INVALID_CURSOR")
	}
	if err := verr.orNil(); err != nil {
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
	roster, err := s.d.Professionals.Roster(ctx, businessID)
	if err != nil {
		return AgendaPage{}, err
	}
	names := map[uuid.UUID]string{}
	var own []uuid.UUID
	for _, r := range roster {
		names[r.ID] = r.DisplayName
		if r.UserID == p.UserID {
			own = append(own, r.ID)
		}
	}
	_, known := names[q.ProfessionalID]
	var filter []uuid.UUID // nil = every professional
	switch {
	case q.ProfessionalID != uuid.Nil && !known:
		return AgendaPage{}, &ValidationError{Fields: []FieldError{{"professionalId", "PROFESSIONAL_NOT_FOUND"}}}
	case role == identity.RoleOwner && q.ProfessionalID != uuid.Nil:
		filter = []uuid.UUID{q.ProfessionalID}
	case role == identity.RoleOwner:
	case q.ProfessionalID != uuid.Nil && !contains(own, q.ProfessionalID):
		return AgendaPage{}, authz.ErrForbidden
	default: // a PROFESSIONAL: its own profile (none → empty agenda)
		filter = append([]uuid.UUID{}, own...)
	}
	if q.Date == (domain.LocalDate{}) {
		q.Date = domain.LocalDateOf(s.d.Clock.Now(), loc) // today in the business's calendar
	}
	if q.View == "" {
		q.View = domain.ViewDay
	}
	fromDay, toDay := q.View.Range(q.Date)
	page := AgendaPage{Location: loc, From: fromDay, To: toDay, Items: []AgendaItem{}}
	if filter != nil && len(filter) == 0 {
		return page, nil
	}
	var rows []domain.Appointment
	err = s.d.Store.WithinTx(ctx, func(ctx context.Context, tx Tx) (err error) {
		rows, err = tx.AgendaAppointments(ctx, businessID, filter, domain.ResolveLocal(fromDay, 0, loc), domain.ResolveLocal(toDay, 0, loc), after, limit+1)
		return err
	})
	if err != nil {
		return AgendaPage{}, err
	}
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[limit-1]
		page.NextCursor = Cursor{Start: last.Start, ID: last.ID}.Encode()
	}
	stranded, _, err := s.stranded(ctx, businessID)
	if err != nil {
		return AgendaPage{}, err
	}
	now := s.d.Clock.Now()
	for _, a := range rows {
		page.Items = append(page.Items, AgendaItem{Appointment: a, ProfessionalName: names[a.ProfessionalID],
			NeedsReassignment: a.Status.Occupies() && a.End.After(now) && contains(stranded, a.ProfessionalID)})
	}
	return page, nil
}

func contains(ids []uuid.UUID, id uuid.UUID) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

// MyAppointment is a customer's appointment with the business and professional it is with.
type MyAppointment struct {
	domain.Appointment
	Business         businesses.BusinessSummary
	ProfessionalName string
}

// MyAppointmentsQuery selects a page of the caller's appointments.
type MyAppointmentsQuery struct {
	Upcoming bool // true: from now on, ascending; false: past, newest first
	Cursor   string
	Limit    int
}

// MyAppointments returns the caller's appointments in every business.
func (s *Service) MyAppointments(ctx context.Context, p identity.Principal, q MyAppointmentsQuery) ([]MyAppointment, string, error) {
	verr := &ValidationError{}
	limit := pageSize(verr, q.Limit)
	after, ok := DecodeCursor(q.Cursor)
	if !ok {
		verr.add("cursor", "INVALID_CURSOR")
	}
	if err := verr.orNil(); err != nil {
		return nil, "", err
	}
	var rows []domain.Appointment
	err := s.d.Store.WithinTx(ctx, func(ctx context.Context, tx Tx) (err error) {
		rows, err = tx.CustomerAppointments(ctx, p.UserID, s.d.Clock.Now(), q.Upcoming, after, limit+1)
		return err
	})
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[limit-1]
		next = Cursor{Start: last.Start, ID: last.ID}.Encode()
	}
	out, err := s.describe(ctx, rows)
	return out, next, err
}

// MyAppointmentDetail returns one of the caller's appointments; ErrAppointmentNotFound for
// anyone else's (indistinguishable from a missing one).
func (s *Service) MyAppointmentDetail(ctx context.Context, p identity.Principal, id uuid.UUID) (MyAppointment, error) {
	var a domain.Appointment
	err := s.d.Store.WithinTx(ctx, func(ctx context.Context, tx Tx) (err error) {
		a, err = tx.AppointmentByID(ctx, id)
		return err
	})
	if err == nil && a.Customer.UserID != p.UserID {
		err = ErrAppointmentNotFound
	}
	if err != nil {
		return MyAppointment{}, err
	}
	out, err := s.describe(ctx, []domain.Appointment{a})
	if err != nil {
		return MyAppointment{}, err
	}
	return out[0], nil
}

// describe adds the business and professional names to appointments of any businesses.
func (s *Service) describe(ctx context.Context, rows []domain.Appointment) ([]MyAppointment, error) {
	var ids []uuid.UUID
	seen := map[uuid.UUID]bool{}
	for _, a := range rows {
		if !seen[a.BusinessID] {
			seen[a.BusinessID] = true
			ids = append(ids, a.BusinessID)
		}
	}
	summaries, err := s.d.Businesses.BusinessSummaries(ctx, ids)
	if err != nil {
		return nil, err
	}
	names := map[uuid.UUID]string{}
	for _, id := range ids {
		roster, err := s.d.Professionals.Roster(ctx, id)
		if err != nil {
			return nil, err
		}
		for _, r := range roster {
			names[r.ID] = r.DisplayName
		}
	}
	out := make([]MyAppointment, 0, len(rows))
	for _, a := range rows {
		out = append(out, MyAppointment{Appointment: a, Business: summaries[a.BusinessID], ProfessionalName: names[a.ProfessionalID]})
	}
	return out, nil
}
