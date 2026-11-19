package application

import (
	"context"
	"errors"
	"net/netip"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/domain"
)

// Repository errors returned by Store implementations.
var (
	ErrNotFound   = errors.New("identity: not found")
	ErrEmailTaken = errors.New("identity: email already registered")
)

// Authentication errors. ErrInvalidCredentials is deliberately generic: unknown email, wrong
// password and suspended account are indistinguishable to the caller.
var (
	ErrInvalidCredentials = errors.New("identity: invalid credentials")
	ErrEmailNotVerified   = errors.New("identity: email not verified")
	ErrUnauthenticated    = errors.New("identity: unauthenticated")
)

// PasswordHasher hashes and verifies passwords (implemented with Argon2id, ADR-0006).
type PasswordHasher interface {
	// Hash returns a self-describing hash of p.
	Hash(ctx context.Context, p domain.Password) (domain.PasswordHash, error)
	// Verify reports whether p matches hash in constant time. needsRehash is true when the
	// hash was produced with weaker parameters than the current ones, so the caller can
	// transparently upgrade it after a successful login.
	Verify(ctx context.Context, p domain.Password, hash domain.PasswordHash) (ok, needsRehash bool, err error)
	// VerifyDummy spends the same time as Verify without a stored hash. Login calls it when
	// the account does not exist so response timing does not reveal registered emails.
	VerifyDummy(ctx context.Context, p domain.Password)
}

// Store runs a unit of work in a database transaction.
type Store interface {
	WithinTx(ctx context.Context, fn func(tx Tx) error) error
}

// Tx is the set of persistence operations available inside a transaction. Jobs enqueued
// through it are committed atomically with the data (transactional outbox, ADR-0008).
type Tx interface {
	// CreateUser returns ErrEmailTaken if the email is already registered.
	CreateUser(ctx context.Context, u *domain.User) error
	// UserByEmail and UserByID return ErrNotFound when absent.
	UserByEmail(ctx context.Context, email domain.Email) (*domain.User, error)
	UserByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
	MarkEmailVerified(ctx context.Context, userID uuid.UUID, now time.Time) error

	InvalidateEmailVerificationTokens(ctx context.Context, userID uuid.UUID, now time.Time) error
	CreateEmailVerificationToken(ctx context.Context, tokenHash []byte, userID uuid.UUID, now, expiresAt time.Time) error
	// ConsumeEmailVerificationToken marks a valid, unused, unexpired token as used and returns
	// its user; otherwise ErrNotFound.
	ConsumeEmailVerificationToken(ctx context.Context, tokenHash []byte, now time.Time) (uuid.UUID, error)

	// EnqueueVerificationEmail schedules the verification email for userID. Duplicate
	// requests within a short window are coalesced.
	EnqueueVerificationEmail(ctx context.Context, userID uuid.UUID) error

	UpdatePasswordHash(ctx context.Context, userID uuid.UUID, hash domain.PasswordHash) error

	// CreateSession stores a session identified by the SHA-256 of its token.
	CreateSession(ctx context.Context, s *domain.Session, tokenHash []byte, ip *netip.Addr, userAgent string) error
	// ActiveSessionByTokenHash returns a non-revoked, unexpired session (at now) and its user,
	// or ErrNotFound.
	ActiveSessionByTokenHash(ctx context.Context, tokenHash []byte, now time.Time) (*domain.Session, *domain.User, error)
	TouchSession(ctx context.Context, sessionID uuid.UUID, now, idleExpiresAt time.Time) error
	RevokeSession(ctx context.Context, sessionID, userID uuid.UUID, now time.Time) error
	RevokeAllUserSessions(ctx context.Context, userID uuid.UUID, now time.Time) (int64, error)
}

// Mailer delivers a plain-text email.
type Mailer interface {
	Send(ctx context.Context, to, subject, body string) error
}
