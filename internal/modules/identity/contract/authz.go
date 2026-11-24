package contract

import (
	"context"
	"slices"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/platform/authz"
)

// Role is a per-business role granted through a membership (ADR-0007). CUSTOMER is implicit
// (no membership) and PLATFORM_ADMIN is a user flag (Principal.IsPlatformAdmin), so neither is
// a Role.
type Role string

// Business roles.
const (
	RoleOwner        Role = "OWNER"        // person in charge of the business (several, at least one)
	RoleProfessional Role = "PROFESSIONAL" // barber or stylist
)

// Role sets for the policy matrix (docs/architecture/security.md §2). Use cases pick the set
// that matches their action instead of listing roles ad hoc.
var (
	// RolesOwner: business configuration, memberships, suspension.
	RolesOwner = []Role{RoleOwner}
	// RolesStaff: any member of the business (e.g. read its own agenda). Use cases restricted
	// to "own" resources must additionally compare the resource owner with the principal.
	RolesStaff = []Role{RoleOwner, RoleProfessional}
)

// Membership is an active role of a user in a business.
type Membership struct {
	BusinessID uuid.UUID
	Role       Role
}

// Authorization errors (aliases of the platform sentinels so every module maps them the same).
var (
	ErrForbidden = authz.ErrForbidden
	ErrNotFound  = authz.ErrNotFound
)

// Authorizer is the identity port other modules use to authorize their use cases. Memberships
// are read from the database on every call (no cached claims), so a revoked membership loses
// access on the next request.
type Authorizer interface {
	// RequireBusinessRole returns the caller's role in businessID if it is one of allowed;
	// ErrNotFound if the caller has no active membership there; ErrForbidden if it has one
	// with an insufficient role.
	RequireBusinessRole(ctx context.Context, p Principal, businessID uuid.UUID, allowed ...Role) (Role, error)
	// RequirePlatformAdmin returns ErrForbidden unless the caller is a platform administrator.
	RequirePlatformAdmin(ctx context.Context, p Principal) error
}

// CheckBusinessRole is the pure policy behind Authorizer.RequireBusinessRole. Rules:
//   - no active membership in businessID → ErrNotFound (the business's resources "do not exist"
//     for this caller; never 403, which would confirm existence);
//   - membership whose role is not in allowed → ErrForbidden;
//   - empty allowed list → ErrForbidden (fail closed).
//
// Platform administrators get no implicit business role: admin operations go through
// dedicated admin use cases guarded by CheckPlatformAdmin (least privilege, audited).
func CheckBusinessRole(memberships []Membership, businessID uuid.UUID, allowed ...Role) (Role, error) {
	if businessID == uuid.Nil {
		return "", ErrNotFound
	}
	for _, m := range memberships {
		if m.BusinessID != businessID {
			continue
		}
		if slices.Contains(allowed, m.Role) {
			return m.Role, nil
		}
		return "", ErrForbidden
	}
	return "", ErrNotFound
}

// CheckPlatformAdmin is the pure policy behind Authorizer.RequirePlatformAdmin.
func CheckPlatformAdmin(p Principal) error {
	if p.UserID == uuid.Nil || !p.IsPlatformAdmin {
		return ErrForbidden
	}
	return nil
}
