// Package postgres implements the geography application.Store on PostgreSQL (sqlc + pgx).
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/infrastructure/sqlcgen"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/audit"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/db"
)

// Store implements application.Store.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore builds the store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// WithinTx implements application.Store.
func (s *Store) WithinTx(ctx context.Context, fn func(tx application.Tx) error) error {
	return db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		return fn(&txStore{tx: tx, q: sqlcgen.New(tx)})
	})
}

type txStore struct {
	tx pgx.Tx
	q  *sqlcgen.Queries
}

// mapErr turns driver errors into the application's repository errors.
func mapErr(err error) error {
	var pgErr *pgconn.PgError
	switch {
	case err == nil:
		return nil
	case errors.Is(err, pgx.ErrNoRows):
		return application.ErrNotFound
	case errors.As(err, &pgErr) && pgErr.Code == "23505": // unique_violation
		return application.ErrConflict
	default:
		return err
	}
}

func (t *txStore) ListActiveCountries(ctx context.Context) ([]domain.Country, error) {
	rows, err := t.q.ListActiveCountries(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Country, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.Country{
			Code: domain.CountryCode(r.Code), Name: r.Name, PhoneCode: r.PhoneCode,
			DefaultCurrency: r.DefaultCurrency, Active: r.Active,
		})
	}
	return out, nil
}

func (t *txStore) CountryByCode(ctx context.Context, code domain.CountryCode) (domain.Country, error) {
	r, err := t.q.CountryByCode(ctx, string(code))
	if err != nil {
		return domain.Country{}, mapErr(err)
	}
	return domain.Country{
		Code: domain.CountryCode(r.Code), Name: r.Name, PhoneCode: r.PhoneCode,
		DefaultCurrency: r.DefaultCurrency, Active: r.Active,
	}, nil
}

func (t *txStore) InsertCountry(ctx context.Context, c domain.Country, now time.Time) error {
	return mapErr(t.q.InsertCountry(ctx, sqlcgen.InsertCountryParams{
		Code: string(c.Code), Name: c.Name, PhoneCode: c.PhoneCode,
		DefaultCurrency: c.DefaultCurrency, Active: c.Active, CreatedAt: now,
	}))
}

func (t *txStore) UpdateCountry(ctx context.Context, c domain.Country, now time.Time) error {
	return mapErr(t.q.UpdateCountry(ctx, sqlcgen.UpdateCountryParams{
		Code: string(c.Code), Name: c.Name, PhoneCode: c.PhoneCode,
		DefaultCurrency: c.DefaultCurrency, Active: c.Active, UpdatedAt: now,
	}))
}

func toDivision(id uuid.UUID, country string, code *string, name, typ string, active bool) (domain.Division, error) {
	t, err := domain.ParseDivisionType(typ)
	if err != nil {
		return domain.Division{}, fmt.Errorf("division %s type %q: %w", id, typ, err)
	}
	d := domain.Division{ID: id, CountryCode: domain.CountryCode(country), Name: name, Type: t, Active: active}
	if code != nil {
		d.Code = *code
	}
	return d, nil
}

func (t *txStore) ListActiveDivisions(ctx context.Context, country domain.CountryCode) ([]domain.Division, error) {
	rows, err := t.q.ListActiveDivisionsByCountry(ctx, string(country))
	if err != nil {
		return nil, err
	}
	out := make([]domain.Division, 0, len(rows))
	for _, r := range rows {
		d, err := toDivision(r.ID, r.CountryCode, r.Code, r.Name, r.Type, r.Active)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

func (t *txStore) DivisionByID(ctx context.Context, id uuid.UUID) (domain.Division, error) {
	r, err := t.q.DivisionByID(ctx, id)
	if err != nil {
		return domain.Division{}, mapErr(err)
	}
	return toDivision(r.ID, r.CountryCode, r.Code, r.Name, r.Type, r.Active)
}

func (t *txStore) InsertDivision(ctx context.Context, d domain.Division, now time.Time) error {
	return mapErr(t.q.InsertDivision(ctx, sqlcgen.InsertDivisionParams{
		ID: d.ID, CountryCode: string(d.CountryCode), Code: optional(d.Code), Name: d.Name,
		Type: string(d.Type), Active: d.Active, CreatedAt: now,
	}))
}

func (t *txStore) UpdateDivision(ctx context.Context, d domain.Division, now time.Time) error {
	return mapErr(t.q.UpdateDivision(ctx, sqlcgen.UpdateDivisionParams{
		ID: d.ID, Code: optional(d.Code), Name: d.Name, Type: string(d.Type), Active: d.Active, UpdatedAt: now,
	}))
}

func (t *txStore) ListActiveCities(ctx context.Context, divisionID uuid.UUID) ([]domain.City, error) {
	rows, err := t.q.ListActiveCitiesByDivision(ctx, divisionID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.City, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.City{ID: r.ID, DivisionID: r.DivisionID, Name: r.Name, TimeZone: r.TimeZone, Active: r.Active})
	}
	return out, nil
}

func (t *txStore) CityByID(ctx context.Context, id uuid.UUID) (domain.City, error) {
	r, err := t.q.CityByID(ctx, id)
	if err != nil {
		return domain.City{}, mapErr(err)
	}
	return domain.City{ID: r.ID, DivisionID: r.DivisionID, Name: r.Name, TimeZone: r.TimeZone, Active: r.Active}, nil
}

func (t *txStore) InsertCity(ctx context.Context, c domain.City, now time.Time) error {
	return mapErr(t.q.InsertCity(ctx, sqlcgen.InsertCityParams{
		ID: c.ID, DivisionID: c.DivisionID, Name: c.Name, TimeZone: c.TimeZone, Active: c.Active, CreatedAt: now,
	}))
}

func (t *txStore) UpdateCity(ctx context.Context, c domain.City, now time.Time) error {
	return mapErr(t.q.UpdateCity(ctx, sqlcgen.UpdateCityParams{
		ID: c.ID, Name: c.Name, TimeZone: c.TimeZone, Active: c.Active, UpdatedAt: now,
	}))
}

func (t *txStore) AppendAudit(ctx context.Context, e *audit.Entry) error {
	return audit.Append(ctx, t.tx, e)
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
