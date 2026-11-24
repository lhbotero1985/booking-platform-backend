package contract

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Errors of MemberAdder.
var (
	// ErrAccountNotEligible: no active (verified, not suspended) account has that email.
	ErrAccountNotEligible = errors.New("identity: no active account with that email")
	// ErrAlreadyMember: the account already has a membership in the business.
	ErrAlreadyMember = errors.New("identity: already a member of the business")
	// ErrLastOwner: the change would leave the business without an active OWNER (there must
	// always be one person in charge, decision of 2026-11-24).
	ErrLastOwner = errors.New("identity: a business must keep at least one owner")
	// ErrMemberNotFound: the user is not an active member of the business.
	ErrMemberNotFound = errors.New("identity: not a member of the business")
	// ErrAlreadyOwner: the account already owns a business; another one (another demo) must be
	// requested from support (ADR-0018: one demo per account).
	ErrAlreadyOwner = errors.New("identity: the account already owns a business")
)

// OwnershipClaimer makes the authenticated account the OWNER of a business it has just created
// (self-service onboarding, ADR-0018). Refused with ErrAlreadyOwner if the account already owns
// a business. Called with the caller's transaction in ctx; audited (membership.granted).
type OwnershipClaimer interface {
	ClaimOwnership(ctx context.Context, actor Principal, businessID uuid.UUID) error
}

// MemberAdder grants a role in a business to an existing account, found by email. It is
// authorized for PLATFORM_ADMIN and for the business OWNER, and audited (membership.granted).
// Called with a context carrying the caller's transaction (db.ContextWithTx), the membership
// is written atomically with the caller's changes.
type MemberAdder interface {
	AddMember(ctx context.Context, actor Principal, businessID uuid.UUID, email string, role Role) (userID uuid.UUID, err error)
}

// Member is an active member of a business as its OWNERs see it.
type Member struct {
	UserID   uuid.UUID
	FullName string
	Email    string
	Role     Role
	Since    time.Time
}
