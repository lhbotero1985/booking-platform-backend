// Package postgres implements the professionals application.Store (sqlc + pgx).
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/infrastructure/sqlcgen"
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

func (t *txStore) InsertProfessional(ctx context.Context, p domain.Professional) error {
	err := t.q.InsertProfessional(ctx, sqlcgen.InsertProfessionalParams{
		ID: p.ID, BusinessID: p.BusinessID, UserID: p.UserID, DisplayName: p.DisplayName, Bio: p.Bio,
		Status: string(p.Status), SortOrder: int32(p.SortOrder), CreatedAt: p.CreatedAt, //nolint:gosec // validated range
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "professionals_business_user_unique" {
		return application.ErrAlreadyExists
	}
	return err
}

func (t *txStore) UpdateProfessional(ctx context.Context, p domain.Professional) error {
	return t.q.UpdateProfessional(ctx, sqlcgen.UpdateProfessionalParams{
		BusinessID: p.BusinessID, ID: p.ID, DisplayName: p.DisplayName, Bio: p.Bio, Status: string(p.Status),
		SortOrder: int32(p.SortOrder), UpdatedAt: p.UpdatedAt, //nolint:gosec // validated range
	})
}

func (t *txStore) ProfessionalByID(ctx context.Context, businessID, id uuid.UUID) (domain.Professional, error) {
	r, err := t.q.ProfessionalByID(ctx, sqlcgen.ProfessionalByIDParams{BusinessID: businessID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Professional{}, application.ErrNotFound
	}
	if err != nil {
		return domain.Professional{}, err
	}
	return toDomain(r)
}

func (t *txStore) ListProfessionals(ctx context.Context, businessID uuid.UUID) ([]domain.Professional, error) {
	rows, err := t.q.ListProfessionals(ctx, businessID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Professional, 0, len(rows))
	for _, r := range rows {
		p, err := toDomain(r)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func (t *txStore) AppendAudit(ctx context.Context, e *audit.Entry) error {
	return audit.Append(ctx, t.tx, e)
}

func toDomain(r sqlcgen.Professional) (domain.Professional, error) {
	st, err := domain.ParseStatus(r.Status)
	if err != nil {
		return domain.Professional{}, fmt.Errorf("professional %s: %w", r.ID, err)
	}
	return domain.Professional{
		ID: r.ID, BusinessID: r.BusinessID, UserID: r.UserID, DisplayName: r.DisplayName, Bio: r.Bio,
		Status: st, SortOrder: int(r.SortOrder), CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}, nil
}

func (t *txStore) ActiveProfessionalsIn(ctx context.Context, businessIDs []uuid.UUID) ([]domain.Professional, error) {
	rows, err := t.q.ActiveProfessionalsIn(ctx, businessIDs)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Professional, 0, len(rows))
	for _, r := range rows {
		p, err := toDomain(r)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}
