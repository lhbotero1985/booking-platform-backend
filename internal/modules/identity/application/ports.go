package application

import (
	"context"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/domain"
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
