package contract

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// Errors of MemberAdder.
var (
	// ErrAccountNotEligible: no active (verified, not suspended) account has that email.
	ErrAccountNotEligible = errors.New("identity: no active account with that email")
	// ErrAlreadyMember: the account already has a membership in the business.
	ErrAlreadyMember = errors.New("identity: already a member of the business")
	// ErrOwnerExists: the business already has an active OWNER.
	ErrOwnerExists = errors.New("identity: the business already has an owner")
)

// MemberAdder grants a role in a business to an existing account, found by email. It is
// authorized for PLATFORM_ADMIN and for the business OWNER, and audited (membership.granted).
// Called with a context carrying the caller's transaction (db.ContextWithTx), the membership
// is written atomically with the caller's changes.
type MemberAdder interface {
	AddMember(ctx context.Context, actor Principal, businessID uuid.UUID, email string, role Role) (userID uuid.UUID, err error)
}
