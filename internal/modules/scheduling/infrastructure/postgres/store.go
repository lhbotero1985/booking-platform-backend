// Package postgres implements the scheduling persistence with SQLC (T6.3).
package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/infrastructure/sqlcgen"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/db"
)

// codeQueryCanceled is raised by statement_timeout.
const codeQueryCanceled = "57014"

// Store is the scheduling store.
type Store struct{ pool *pgxpool.Pool }

// NewStore builds the store.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// WithinTx implements application.Store. Lock and statement timeouts become application.ErrBusy.
func (s *Store) WithinTx(ctx context.Context, fn func(ctx context.Context, tx application.Tx) error) error {
	err := db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		return fn(db.ContextWithTx(ctx, tx), &txStore{q: sqlcgen.New(tx)})
	})
	if code := db.SQLState(err); code == db.CodeLockNotAvailable || code == codeQueryCanceled {
		return errors.Join(application.ErrBusy, err)
	}
	return err
}

type txStore struct{ q *sqlcgen.Queries }

func (t *txStore) SetBookingTimeouts(ctx context.Context) error { return t.q.SetBookingTimeouts(ctx) }

func (t *txStore) ClaimIdempotencyKey(ctx context.Context, userID uuid.UUID, key, fingerprint string, now time.Time) (bool, error) {
	rows, err := t.q.ClaimIdempotencyKey(ctx, sqlcgen.ClaimIdempotencyKeyParams{UserID: userID, Key: key, Fingerprint: fingerprint, CreatedAt: now})
	return len(rows) == 1, err
}

func (t *txStore) IdempotencyKeyOf(ctx context.Context, userID uuid.UUID, key string) (string, uuid.UUID, error) {
	r, err := t.q.IdempotencyKeyOf(ctx, sqlcgen.IdempotencyKeyOfParams{UserID: userID, Key: key})
	if err != nil {
		return "", uuid.Nil, err
	}
	id := uuid.Nil
	if r.AppointmentID != nil {
		id = *r.AppointmentID
	}
	return r.Fingerprint, id, nil
}

func (t *txStore) SetIdempotencyAppointment(ctx context.Context, userID uuid.UUID, key string, appointmentID uuid.UUID) error {
	return t.q.SetIdempotencyAppointment(ctx, sqlcgen.SetIdempotencyAppointmentParams{UserID: userID, Key: key, AppointmentID: &appointmentID})
}

func (t *txStore) InsertAppointment(ctx context.Context, a domain.Appointment) error {
	var customer *uuid.UUID
	if a.Customer.UserID != uuid.Nil {
		id := a.Customer.UserID
		customer = &id
	}
	err := t.q.InsertAppointment(ctx, sqlcgen.InsertAppointmentParams{
		ID: a.ID, BusinessID: a.BusinessID, ProfessionalID: a.ProfessionalID, ServiceID: a.Service.ServiceID,
		CustomerID: optional(a.Customer.CustomerID), CustomerUserID: customer, CustomerName: a.Customer.Name, CustomerPhone: a.Customer.Phone,
		StartAt: a.Start, EndAt: a.End, BufferMinutes: int16(a.Buffer / time.Minute), OccupiedUntil: a.OccupiedUntil(), //nolint:gosec // 0–60, CHECK
		Status: string(a.Status), ServiceName: a.Service.Name, DurationMinutes: int16(a.Service.DurationMinutes), //nolint:gosec // 5–480, CHECK
		PriceMinor: a.Service.PriceMinor, Currency: a.Service.Currency, Version: int32(a.Version), //nolint:gosec // small counter
		CreatedBy: a.CreatedBy, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
		CancelledAt: optionalTime(a.CancelledAt), CancelReason: optionalText(a.CancelReason), CancelledBy: optionalText(string(a.CancelledBy)),
	})
	if db.IsExclusionViolation(err) {
		return errors.Join(application.ErrSlotUnavailable, err)
	}
	return err
}

func (t *txStore) InsertAppointmentEvent(ctx context.Context, businessID, appointmentID uuid.UUID, e domain.Event) error {
	var prevStart *time.Time
	if !e.PreviousStart.IsZero() {
		prevStart = &e.PreviousStart
	}
	return t.q.InsertAppointmentEvent(ctx, sqlcgen.InsertAppointmentEventParams{
		ID: e.ID, BusinessID: businessID, AppointmentID: appointmentID, Type: string(e.Type), Status: string(e.Status),
		ProfessionalID: e.ProfessionalID, StartAt: e.Start, ActorUserID: optional(e.Actor), OccurredAt: e.At,
		ActorRole: optionalText(string(e.ActorRole)), FromStatus: optionalText(string(e.FromStatus)), PreviousStartAt: prevStart,
		PreviousProfessionalID: optional(e.PreviousProfessionalID), Reason: optionalText(e.Reason),
	})
}

func (t *txStore) LockAppointment(ctx context.Context, id uuid.UUID) (domain.Appointment, error) {
	r, err := t.q.LockAppointment(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Appointment{}, application.ErrAppointmentNotFound
	}
	if err != nil {
		return domain.Appointment{}, err
	}
	return toAppointment(r), nil
}

func (t *txStore) UpdateAppointment(ctx context.Context, a domain.Appointment, version int) error {
	n, err := t.q.UpdateAppointment(ctx, sqlcgen.UpdateAppointmentParams{
		ID: a.ID, Version: int32(version), ProfessionalID: a.ProfessionalID, StartAt: a.Start, EndAt: a.End, //nolint:gosec // small counter
		OccupiedUntil: a.OccupiedUntil(), Status: string(a.Status), CancelledAt: optionalTime(a.CancelledAt),
		CancelReason: optionalText(a.CancelReason), CancelledBy: optionalText(string(a.CancelledBy)), UpdatedAt: a.UpdatedAt,
	})
	switch {
	case db.IsExclusionViolation(err):
		return errors.Join(application.ErrSlotUnavailable, err)
	case err != nil:
		return err
	case n == 0:
		return application.ErrAppointmentChanged
	}
	return nil
}

func (t *txStore) AppointmentEvents(ctx context.Context, appointmentID uuid.UUID) ([]domain.Event, error) {
	rows, err := t.q.AppointmentEvents(ctx, appointmentID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Event, 0, len(rows))
	for _, r := range rows {
		e := domain.Event{ID: r.ID, Type: domain.EventType(r.Type), Status: domain.Status(r.Status), ProfessionalID: r.ProfessionalID,
			Start: r.StartAt.UTC(), At: r.OccurredAt.UTC()}
		if r.ActorUserID != nil {
			e.Actor = *r.ActorUserID
		}
		if r.ActorRole != nil {
			e.ActorRole = domain.ActorRole(*r.ActorRole)
		}
		if r.FromStatus != nil {
			e.FromStatus = domain.Status(*r.FromStatus)
		}
		if r.PreviousStartAt != nil {
			e.PreviousStart = r.PreviousStartAt.UTC()
		}
		if r.PreviousProfessionalID != nil {
			e.PreviousProfessionalID = *r.PreviousProfessionalID
		}
		if r.Reason != nil {
			e.Reason = *r.Reason
		}
		out = append(out, e)
	}
	return out, nil
}

func optionalTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func (t *txStore) AppointmentByID(ctx context.Context, id uuid.UUID) (domain.Appointment, error) {
	r, err := t.q.AppointmentByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Appointment{}, application.ErrAppointmentNotFound
	}
	if err != nil {
		return domain.Appointment{}, err
	}
	return toAppointment(r), nil
}

func toAppointment(r sqlcgen.Appointment) domain.Appointment {
	customer, record := uuid.Nil, uuid.Nil
	if r.CustomerUserID != nil {
		customer = *r.CustomerUserID
	}
	if r.CustomerID != nil {
		record = *r.CustomerID
	}
	return domain.Appointment{
		ID: r.ID, BusinessID: r.BusinessID, ProfessionalID: r.ProfessionalID,
		Service: domain.ServiceSnapshot{ServiceID: r.ServiceID, Name: r.ServiceName, DurationMinutes: int(r.DurationMinutes),
			PriceMinor: r.PriceMinor, Currency: r.Currency},
		Customer: domain.Customer{CustomerID: record, UserID: customer, Name: r.CustomerName, Phone: r.CustomerPhone},
		Start:    r.StartAt.UTC(), End: r.EndAt.UTC(), Buffer: time.Duration(r.BufferMinutes) * time.Minute,
		Status: domain.Status(r.Status), Version: int(r.Version), CreatedBy: r.CreatedBy,
		CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(), CancelledAt: cancelledAt(r.CancelledAt),
		CancelReason: text(r.CancelReason), CancelledBy: domain.ActorRole(text(r.CancelledBy)),
	}
}

func cancelledAt(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return t.UTC()
}

func text(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func toAppointments(rows []sqlcgen.Appointment, err error) ([]domain.Appointment, error) {
	if err != nil {
		return nil, err
	}
	out := make([]domain.Appointment, 0, len(rows))
	for _, r := range rows {
		out = append(out, toAppointment(r))
	}
	return out, nil
}

func cursorArgs(after *application.Cursor) (bool, time.Time, uuid.UUID) {
	if after == nil {
		return false, time.Time{}, uuid.Nil
	}
	return true, after.Start, after.ID
}

func (t *txStore) AgendaAppointments(ctx context.Context, businessID uuid.UUID, professionalIDs []uuid.UUID, from, to time.Time, after *application.Cursor, limit int) ([]domain.Appointment, error) {
	has, start, id := cursorArgs(after)
	return toAppointments(t.q.AgendaAppointments(ctx, sqlcgen.AgendaAppointmentsParams{
		BusinessID: businessID, AllProfessionals: professionalIDs == nil, ProfessionalIds: professionalIDs,
		FromAt: from, ToAt: to, HasCursor: has, AfterStart: start, AfterID: id, MaxRows: int32(limit), //nolint:gosec // bounded by the application
	}))
}

func (t *txStore) CustomerAppointments(ctx context.Context, userID uuid.UUID, now time.Time, upcoming bool, after *application.Cursor, limit int) ([]domain.Appointment, error) {
	has, start, id := cursorArgs(after)
	if upcoming {
		return toAppointments(t.q.CustomerUpcomingAppointments(ctx, sqlcgen.CustomerUpcomingAppointmentsParams{
			UserID: &userID, NowAt: now, HasCursor: has, AfterStart: start, AfterID: id, MaxRows: int32(limit), //nolint:gosec // bounded by the application
		}))
	}
	return toAppointments(t.q.CustomerPastAppointments(ctx, sqlcgen.CustomerPastAppointmentsParams{
		UserID: &userID, NowAt: now, HasCursor: has, AfterStart: start, AfterID: id, MaxRows: int32(limit), //nolint:gosec // bounded by the application
	}))
}

func (t *txStore) ActiveAppointments(ctx context.Context, businessID, professionalID uuid.UUID, from, to time.Time) ([]domain.Booked, error) {
	rows, err := t.q.ActiveAppointmentsOverlapping(ctx, sqlcgen.ActiveAppointmentsOverlappingParams{
		BusinessID: businessID, ProfessionalID: professionalID, FromAt: from, ToAt: to,
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.Booked, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.Booked{ID: r.ID, Start: r.StartAt, End: r.EndAt, Buffer: r.OccupiedUntil.Sub(r.EndAt)})
	}
	return out, nil
}

func optionalText(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func optional(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}

// customerPhoneTaken maps the per-business unique phone to application.ErrCustomerPhoneTaken.
func customerPhoneTaken(err error) error {
	if db.IsUniqueViolation(err) && db.ConstraintName(err) == "customers_phone_unique" {
		return errors.Join(application.ErrCustomerPhoneTaken, err)
	}
	return err
}

func (t *txStore) InsertCustomer(ctx context.Context, c domain.BusinessCustomer) error {
	return customerPhoneTaken(t.q.InsertCustomer(ctx, sqlcgen.InsertCustomerParams{
		ID: c.ID, BusinessID: c.BusinessID, FullName: c.FullName, Phone: c.Phone, Email: optionalText(c.Email), UserID: optional(c.UserID),
		CreatedBy: c.CreatedBy, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}))
}

func (t *txStore) UpdateCustomer(ctx context.Context, c domain.BusinessCustomer) error {
	return customerPhoneTaken(t.q.UpdateCustomer(ctx, sqlcgen.UpdateCustomerParams{
		BusinessID: c.BusinessID, ID: c.ID, FullName: c.FullName, Phone: c.Phone, Email: optionalText(c.Email), UserID: optional(c.UserID), UpdatedAt: c.UpdatedAt,
	}))
}

func (t *txStore) CustomerByID(ctx context.Context, businessID, id uuid.UUID) (domain.BusinessCustomer, error) {
	r, err := t.q.CustomerByID(ctx, sqlcgen.CustomerByIDParams{BusinessID: businessID, ID: id})
	return toCustomer(r, err)
}

func (t *txStore) CustomerByPhone(ctx context.Context, businessID uuid.UUID, phone string) (domain.BusinessCustomer, error) {
	r, err := t.q.CustomerByPhone(ctx, sqlcgen.CustomerByPhoneParams{BusinessID: businessID, Phone: phone})
	return toCustomer(r, err)
}

func (t *txStore) ListCustomers(ctx context.Context, businessID uuid.UUID, q string, limit int) ([]domain.BusinessCustomer, error) {
	rows, err := t.q.ListCustomers(ctx, sqlcgen.ListCustomersParams{BusinessID: businessID, Q: q, MaxRows: int32(limit)}) //nolint:gosec // bounded by the application
	if err != nil {
		return nil, err
	}
	out := make([]domain.BusinessCustomer, 0, len(rows))
	for _, r := range rows {
		c, _ := toCustomer(r, nil)
		out = append(out, c)
	}
	return out, nil
}

func toCustomer(r sqlcgen.Customer, err error) (domain.BusinessCustomer, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.BusinessCustomer{}, application.ErrCustomerNotFound
	}
	if err != nil {
		return domain.BusinessCustomer{}, err
	}
	user, email := uuid.Nil, ""
	if r.UserID != nil {
		user = *r.UserID
	}
	if r.Email != nil {
		email = *r.Email
	}
	return domain.BusinessCustomer{
		ID: r.ID, BusinessID: r.BusinessID, FullName: r.FullName, Phone: r.Phone, Email: email, UserID: user,
		CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}, nil
}
