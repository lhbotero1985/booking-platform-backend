// Package postgres implements the identity application.Store on PostgreSQL (sqlc + pgx) and
// enqueues jobs with River in the same transaction (ADR-0008).
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/infrastructure/sqlcgen"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/infrastructure/worker"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/db"
)

// JobInserter is the subset of the River client used to enqueue jobs transactionally.
type JobInserter interface {
	InsertTx(ctx context.Context, tx pgx.Tx, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

// Store implements application.Store.
type Store struct {
	pool *pgxpool.Pool
	jobs JobInserter
}

// NewStore builds the store.
func NewStore(pool *pgxpool.Pool, jobs JobInserter) *Store {
	return &Store{pool: pool, jobs: jobs}
}

// WithinTx implements application.Store.
func (s *Store) WithinTx(ctx context.Context, fn func(tx application.Tx) error) error {
	return db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		return fn(&txStore{tx: tx, q: sqlcgen.New(tx), jobs: s.jobs})
	})
}

type txStore struct {
	tx   pgx.Tx
	q    *sqlcgen.Queries
	jobs JobInserter
}

func (t *txStore) CreateUser(ctx context.Context, u *domain.User) error {
	_, err := t.q.CreateUser(ctx, sqlcgen.CreateUserParams{
		ID: u.ID(), Email: u.Email().String(), PasswordHash: string(u.PasswordHash()),
		FullName: u.FullName(), Status: string(u.Status()), Locale: "es",
	})
	if db.IsUniqueViolation(err) && db.ConstraintName(err) == "users_email_unique" {
		return application.ErrEmailTaken
	}
	return err
}

func (t *txStore) UserByEmail(ctx context.Context, email domain.Email) (*domain.User, error) {
	row, err := t.q.GetUserByEmail(ctx, email.String())
	if err != nil {
		return nil, notFound(err)
	}
	return toDomain(row)
}

func (t *txStore) UserByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	row, err := t.q.GetUserByID(ctx, id)
	if err != nil {
		return nil, notFound(err)
	}
	return toDomain(row)
}

func (t *txStore) MarkEmailVerified(ctx context.Context, userID uuid.UUID, now time.Time) error {
	return t.q.MarkUserEmailVerified(ctx, sqlcgen.MarkUserEmailVerifiedParams{ID: userID, Now: now})
}

func (t *txStore) InvalidateEmailVerificationTokens(ctx context.Context, userID uuid.UUID, now time.Time) error {
	return t.q.InvalidateEmailVerificationTokens(ctx, sqlcgen.InvalidateEmailVerificationTokensParams{UserID: userID, Now: now})
}

func (t *txStore) CreateEmailVerificationToken(ctx context.Context, hash []byte, userID uuid.UUID, now, expiresAt time.Time) error {
	return t.q.CreateEmailVerificationToken(ctx, sqlcgen.CreateEmailVerificationTokenParams{
		TokenHash: hash, UserID: userID, Now: now, ExpiresAt: expiresAt,
	})
}

func (t *txStore) ConsumeEmailVerificationToken(ctx context.Context, hash []byte, now time.Time) (uuid.UUID, error) {
	id, err := t.q.ConsumeEmailVerificationToken(ctx, sqlcgen.ConsumeEmailVerificationTokenParams{TokenHash: hash, Now: now})
	if err != nil {
		return uuid.Nil, notFound(err)
	}
	return id, nil
}

func (t *txStore) EnqueueVerificationEmail(ctx context.Context, userID uuid.UUID) error {
	if _, err := t.jobs.InsertTx(ctx, t.tx, worker.SendVerificationEmailArgs{UserID: userID}, nil); err != nil {
		return fmt.Errorf("enqueue verification email: %w", err)
	}
	return nil
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return application.ErrNotFound
	}
	return err
}

func toDomain(r sqlcgen.User) (*domain.User, error) {
	status, err := domain.ParseUserStatus(r.Status)
	if err != nil {
		return nil, err
	}
	return domain.RehydrateUser(r.ID, domain.Email(r.Email), r.FullName, domain.PasswordHash(r.PasswordHash),
		status, r.IsPlatformAdmin, r.EmailVerifiedAt, r.CreatedAt, r.UpdatedAt), nil
}
