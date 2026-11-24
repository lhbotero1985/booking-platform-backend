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
	})
	if db.IsExclusionViolation(err) {
		return errors.Join(application.ErrSlotUnavailable, err)
	}
	return err
}

func (t *txStore) InsertAppointmentEvent(ctx context.Context, id uuid.UUID, a domain.Appointment, typ domain.EventType, actor uuid.UUID, at time.Time) error {
	var actorID *uuid.UUID
	if actor != uuid.Nil {
		actorID = &actor
	}
	return t.q.InsertAppointmentEvent(ctx, sqlcgen.InsertAppointmentEventParams{
		ID: id, BusinessID: a.BusinessID, AppointmentID: a.ID, Type: string(typ), Status: string(a.Status),
		ProfessionalID: a.ProfessionalID, StartAt: a.Start, ActorUserID: actorID, OccurredAt: at,
	})
}

func (t *txStore) AppointmentByID(ctx context.Context, id uuid.UUID) (domain.Appointment, error) {
	r, err := t.q.AppointmentByID(ctx, id)
	if err != nil {
		return domain.Appointment{}, err
	}
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
		CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}, nil
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
		out = append(out, domain.Booked{Start: r.StartAt, End: r.EndAt, Buffer: r.OccupiedUntil.Sub(r.EndAt)})
	}
	return out, nil
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
		ID: c.ID, BusinessID: c.BusinessID, FullName: c.FullName, Phone: c.Phone, UserID: optional(c.UserID),
		CreatedBy: c.CreatedBy, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}))
}

func (t *txStore) UpdateCustomer(ctx context.Context, c domain.BusinessCustomer) error {
	return customerPhoneTaken(t.q.UpdateCustomer(ctx, sqlcgen.UpdateCustomerParams{
		BusinessID: c.BusinessID, ID: c.ID, FullName: c.FullName, Phone: c.Phone, UserID: optional(c.UserID), UpdatedAt: c.UpdatedAt,
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
	user := uuid.Nil
	if r.UserID != nil {
		user = *r.UserID
	}
	return domain.BusinessCustomer{
		ID: r.ID, BusinessID: r.BusinessID, FullName: r.FullName, Phone: r.Phone, UserID: user,
		CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}, nil
}
