// Package postgres implements the subscriptions application.Store (sqlc + pgx). WithinTx joins
// the caller's transaction when the context carries one (db.ContextWithTx).
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/subscriptions/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/subscriptions/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/subscriptions/infrastructure/sqlcgen"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/audit"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/db"
)

// Store implements application.Store.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore builds the store.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

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

func (t *txStore) InsertSubscription(ctx context.Context, s domain.Subscription) error {
	return t.q.InsertSubscription(ctx, sqlcgen.InsertSubscriptionParams{
		BusinessID: s.BusinessID, Status: string(s.Status), TrialEndsAt: s.TrialEndsAt, ExpiredAt: s.ExpiredAt, CreatedAt: s.CreatedAt,
	})
}

func (t *txStore) SubscriptionByBusiness(ctx context.Context, businessID uuid.UUID, forUpdate bool) (domain.Subscription, error) {
	var r sqlcgen.Subscription
	var err error
	if forUpdate {
		r, err = t.q.SubscriptionByBusinessForUpdate(ctx, businessID)
	} else {
		r, err = t.q.SubscriptionByBusiness(ctx, businessID)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Subscription{}, application.ErrNotFound
	}
	if err != nil {
		return domain.Subscription{}, err
	}
	st, err := domain.ParseStatus(r.Status)
	if err != nil {
		return domain.Subscription{}, fmt.Errorf("subscription %s: %w", businessID, err)
	}
	return domain.Subscription{
		BusinessID: r.BusinessID, Status: st, TrialEndsAt: r.TrialEndsAt, ExpiredAt: r.ExpiredAt,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}, nil
}

func (t *txStore) SubscriptionsOf(ctx context.Context, businessIDs []uuid.UUID) ([]domain.Subscription, error) {
	rows, err := t.q.SubscriptionsOf(ctx, businessIDs)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Subscription, 0, len(rows))
	for _, r := range rows {
		st, err := domain.ParseStatus(r.Status)
		if err != nil {
			return nil, fmt.Errorf("subscription %s: %w", r.BusinessID, err)
		}
		out = append(out, domain.Subscription{
			BusinessID: r.BusinessID, Status: st, TrialEndsAt: r.TrialEndsAt, ExpiredAt: r.ExpiredAt,
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		})
	}
	return out, nil
}

func (t *txStore) UpdateSubscription(ctx context.Context, s domain.Subscription) error {
	return t.q.UpdateSubscription(ctx, sqlcgen.UpdateSubscriptionParams{
		BusinessID: s.BusinessID, Status: string(s.Status), TrialEndsAt: s.TrialEndsAt, ExpiredAt: s.ExpiredAt, UpdatedAt: s.UpdatedAt,
	})
}

func (t *txStore) ExpireDueTrials(ctx context.Context, now time.Time, limit int) ([]uuid.UUID, error) {
	return t.q.ExpireDueTrials(ctx, sqlcgen.ExpireDueTrialsParams{Now: now, MaxRows: int32(limit)}) //nolint:gosec // small constant
}

func (t *txStore) AppendAudit(ctx context.Context, e *audit.Entry) error {
	return audit.Append(ctx, t.tx, e)
}
