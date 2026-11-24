// Package postgres implements the catalog application.Store (sqlc + pgx).
package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/infrastructure/sqlcgen"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/audit"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/db"
)

// Store implements application.Store.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore builds the store.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// WithinTx implements application.Store (fn gets a context carrying the transaction).
func (s *Store) WithinTx(ctx context.Context, fn func(ctx context.Context, tx application.Tx) error) error {
	return db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		return fn(db.ContextWithTx(ctx, tx), &txStore{tx: tx, q: sqlcgen.New(tx)})
	})
}

type txStore struct {
	tx pgx.Tx
	q  *sqlcgen.Queries
}

// nameTaken maps the case-insensitive unique name indexes to application.ErrNameTaken.
func nameTaken(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" &&
		(pgErr.ConstraintName == "service_categories_name_unique" || pgErr.ConstraintName == "services_name_unique") {
		return application.ErrNameTaken
	}
	return err
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return application.ErrNotFound
	}
	return err
}

func (t *txStore) InsertCategory(ctx context.Context, c domain.Category) error {
	return nameTaken(t.q.InsertCategory(ctx, sqlcgen.InsertCategoryParams{
		ID: c.ID, BusinessID: c.BusinessID, Name: c.Name, Status: string(c.Status), SortOrder: int32(c.SortOrder), CreatedAt: c.CreatedAt, //nolint:gosec // validated range
	}))
}

func (t *txStore) UpdateCategory(ctx context.Context, c domain.Category) error {
	return nameTaken(t.q.UpdateCategory(ctx, sqlcgen.UpdateCategoryParams{
		BusinessID: c.BusinessID, ID: c.ID, Name: c.Name, Status: string(c.Status), SortOrder: int32(c.SortOrder), UpdatedAt: c.UpdatedAt, //nolint:gosec // validated range
	}))
}

func (t *txStore) DeleteCategory(ctx context.Context, businessID, id uuid.UUID) error {
	return t.q.DeleteCategory(ctx, sqlcgen.DeleteCategoryParams{BusinessID: businessID, ID: id})
}

func (t *txStore) CategoryByID(ctx context.Context, businessID, id uuid.UUID) (domain.Category, error) {
	r, err := t.q.CategoryByID(ctx, sqlcgen.CategoryByIDParams{BusinessID: businessID, ID: id})
	if err != nil {
		return domain.Category{}, notFound(err)
	}
	return categoryToDomain(r)
}

func (t *txStore) ListCategories(ctx context.Context, businessID uuid.UUID) ([]domain.Category, error) {
	rows, err := t.q.ListCategories(ctx, businessID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Category, 0, len(rows))
	for _, r := range rows {
		c, err := categoryToDomain(r)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

func categoryID(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}

func (t *txStore) InsertService(ctx context.Context, s domain.Service) error {
	return nameTaken(t.q.InsertService(ctx, sqlcgen.InsertServiceParams{
		ID: s.ID, BusinessID: s.BusinessID, CategoryID: categoryID(s.CategoryID), Name: s.Name, Description: s.Description,
		DurationMinutes: int32(s.DurationMinutes), PriceMinor: s.PriceMinor, Currency: s.Currency, Status: string(s.Status), //nolint:gosec // validated range
		SortOrder: int32(s.SortOrder), CreatedAt: s.CreatedAt, //nolint:gosec // validated range
	}))
}

func (t *txStore) UpdateService(ctx context.Context, s domain.Service) error {
	return nameTaken(t.q.UpdateService(ctx, sqlcgen.UpdateServiceParams{
		BusinessID: s.BusinessID, ID: s.ID, CategoryID: categoryID(s.CategoryID), Name: s.Name, Description: s.Description,
		DurationMinutes: int32(s.DurationMinutes), PriceMinor: s.PriceMinor, Status: string(s.Status), //nolint:gosec // validated range
		SortOrder: int32(s.SortOrder), UpdatedAt: s.UpdatedAt, //nolint:gosec // validated range
	}))
}

func (t *txStore) ServiceByID(ctx context.Context, businessID, id uuid.UUID) (domain.Service, error) {
	r, err := t.q.ServiceByID(ctx, sqlcgen.ServiceByIDParams{BusinessID: businessID, ID: id})
	if err != nil {
		return domain.Service{}, notFound(err)
	}
	return serviceToDomain(r)
}

func (t *txStore) ListServices(ctx context.Context, businessID uuid.UUID) ([]domain.Service, error) {
	rows, err := t.q.ListServices(ctx, businessID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Service, 0, len(rows))
	for _, r := range rows {
		s, err := serviceToDomain(r)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func (t *txStore) AppendAudit(ctx context.Context, e *audit.Entry) error {
	return audit.Append(ctx, t.tx, e)
}

func categoryToDomain(r sqlcgen.ServiceCategory) (domain.Category, error) {
	st, err := domain.ParseStatus(r.Status)
	if err != nil {
		return domain.Category{}, fmt.Errorf("category %s: %w", r.ID, err)
	}
	return domain.Category{ID: r.ID, BusinessID: r.BusinessID, Name: r.Name, Status: st, SortOrder: int(r.SortOrder), CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}, nil
}

func serviceToDomain(r sqlcgen.Service) (domain.Service, error) {
	st, err := domain.ParseStatus(r.Status)
	if err != nil {
		return domain.Service{}, fmt.Errorf("service %s: %w", r.ID, err)
	}
	s := domain.Service{
		ID: r.ID, BusinessID: r.BusinessID, Name: r.Name, Description: r.Description, DurationMinutes: int(r.DurationMinutes),
		PriceMinor: r.PriceMinor, Currency: strings.TrimSpace(r.Currency), Status: st, SortOrder: int(r.SortOrder),
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
	if r.CategoryID != nil {
		s.CategoryID = *r.CategoryID
	}
	return s, nil
}
