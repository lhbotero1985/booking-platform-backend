// Package postgres implements the identity application.Store on PostgreSQL (sqlc + pgx) and
// enqueues jobs with River in the same transaction (ADR-0008).
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
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

func (t *txStore) UpdatePasswordHash(ctx context.Context, userID uuid.UUID, hash domain.PasswordHash) error {
	return t.q.UpdateUserPasswordHash(ctx, sqlcgen.UpdateUserPasswordHashParams{ID: userID, PasswordHash: string(hash)})
}

func (t *txStore) CreateSession(ctx context.Context, s *domain.Session, tokenHash []byte, ip *netip.Addr, userAgent string) error {
	var ua *string
	if userAgent != "" {
		ua = &userAgent
	}
	_, err := t.q.CreateSession(ctx, sqlcgen.CreateSessionParams{
		ID: s.ID(), UserID: s.UserID(), TokenHash: tokenHash, ClientType: string(s.ClientType()),
		IdleExpiresAt: s.IdleExpiresAt(), AbsoluteExpiresAt: s.AbsoluteExpiresAt(), Ip: ip, UserAgent: ua,
	})
	return err
}

func (t *txStore) ActiveSessionByTokenHash(ctx context.Context, tokenHash []byte, now time.Time) (*domain.Session, *domain.User, error) {
	row, err := t.q.GetActiveSessionByTokenHash(ctx, sqlcgen.GetActiveSessionByTokenHashParams{TokenHash: tokenHash, Now: now})
	if err != nil {
		return nil, nil, notFound(err)
	}
	ct, err := domain.ParseClientType(row.Session.ClientType)
	if err != nil {
		return nil, nil, err
	}
	u, err := toDomain(row.User)
	if err != nil {
		return nil, nil, err
	}
	s := row.Session
	return domain.RehydrateSession(s.ID, s.UserID, ct, s.CreatedAt, s.LastSeenAt, s.IdleExpiresAt, s.AbsoluteExpiresAt), u, nil
}

func (t *txStore) TouchSession(ctx context.Context, sessionID uuid.UUID, now, idleExpiresAt time.Time) error {
	return t.q.TouchSession(ctx, sqlcgen.TouchSessionParams{ID: sessionID, Now: now, IdleExpiresAt: idleExpiresAt})
}

func (t *txStore) RevokeSession(ctx context.Context, sessionID, userID uuid.UUID, now time.Time) error {
	_, err := t.q.RevokeSession(ctx, sqlcgen.RevokeSessionParams{ID: sessionID, UserID: userID, Now: now})
	return err
}

func (t *txStore) RevokeAllUserSessions(ctx context.Context, userID uuid.UUID, now time.Time) (int64, error) {
	return t.q.RevokeAllUserSessions(ctx, sqlcgen.RevokeAllUserSessionsParams{UserID: userID, Now: now})
}

func (t *txStore) RevokeOtherUserSessions(ctx context.Context, userID, keepSessionID uuid.UUID, now time.Time) (int64, error) {
	return t.q.RevokeOtherUserSessions(ctx, sqlcgen.RevokeOtherUserSessionsParams{UserID: userID, KeepSessionID: keepSessionID, Now: now})
}

func (t *txStore) InvalidatePasswordResetTokens(ctx context.Context, userID uuid.UUID, now time.Time) error {
	return t.q.InvalidatePasswordResetTokens(ctx, sqlcgen.InvalidatePasswordResetTokensParams{UserID: userID, Now: now})
}

func (t *txStore) CreatePasswordResetToken(ctx context.Context, hash []byte, userID uuid.UUID, now, expiresAt time.Time) error {
	return t.q.CreatePasswordResetToken(ctx, sqlcgen.CreatePasswordResetTokenParams{TokenHash: hash, UserID: userID, Now: now, ExpiresAt: expiresAt})
}

func (t *txStore) ConsumePasswordResetToken(ctx context.Context, hash []byte, now time.Time) (uuid.UUID, error) {
	id, err := t.q.ConsumePasswordResetToken(ctx, sqlcgen.ConsumePasswordResetTokenParams{TokenHash: hash, Now: now})
	if err != nil {
		return uuid.Nil, notFound(err)
	}
	return id, nil
}

func (t *txStore) EnqueuePasswordResetEmail(ctx context.Context, userID uuid.UUID) error {
	if _, err := t.jobs.InsertTx(ctx, t.tx, worker.SendPasswordResetEmailArgs{UserID: userID}, nil); err != nil {
		return fmt.Errorf("enqueue password reset email: %w", err)
	}
	return nil
}

func (t *txStore) EnqueuePasswordChangedEmail(ctx context.Context, userID uuid.UUID) error {
	if _, err := t.jobs.InsertTx(ctx, t.tx, worker.SendPasswordChangedEmailArgs{UserID: userID}, nil); err != nil {
		return fmt.Errorf("enqueue password changed email: %w", err)
	}
	return nil
}

func (t *txStore) ActiveMembershipsByUser(ctx context.Context, userID uuid.UUID) ([]contract.Membership, error) {
	rows, err := t.q.ListActiveMembershipsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]contract.Membership, 0, len(rows))
	for _, r := range rows {
		// The CHECK constraint already restricts roles; parsing again fails closed if the
		// schema and the code ever drift.
		role, err := domain.ParseRole(r.Role)
		if err != nil {
			return nil, fmt.Errorf("membership role %q: %w", r.Role, err)
		}
		out = append(out, contract.Membership{BusinessID: r.BusinessID, Role: contract.Role(role)})
	}
	return out, nil
}

func (t *txStore) AppendAudit(ctx context.Context, e application.AuditEntry) error {
	meta, err := json.Marshal(e.Metadata)
	if err != nil {
		return fmt.Errorf("audit metadata: %w", err)
	}
	if e.Metadata == nil {
		meta = []byte("{}")
	}
	return t.q.InsertAuditEntry(ctx, sqlcgen.InsertAuditEntryParams{
		ID: e.ID, OccurredAt: e.OccurredAt, ActorUserID: e.ActorUserID, BusinessID: e.BusinessID,
		Action: e.Action, ResourceType: optional(e.ResourceType), ResourceID: optional(e.ResourceID),
		Ip: e.IP, Outcome: string(e.Outcome), Metadata: meta,
	})
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
