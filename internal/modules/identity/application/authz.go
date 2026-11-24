package application

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
)

// Service implements contract.Authorizer.
var _ contract.Authorizer = (*Service)(nil)

// RequireBusinessRole implements contract.Authorizer. Memberships are loaded on every call
// (indexed query, no cache) so revocations take effect on the next request (ADR-0007).
//
// PLATFORM_ADMIN passes for any business and acts with OWNER powers (ADR-0018); its changes
// are audited by each use case with the administrator as actor.
func (s *Service) RequireBusinessRole(ctx context.Context, p contract.Principal, businessID uuid.UUID, allowed ...contract.Role) (contract.Role, error) {
	if p.UserID == uuid.Nil {
		return "", ErrUnauthenticated
	}
	if contract.CheckPlatformAdmin(p) == nil && businessID != uuid.Nil {
		return contract.RoleOwner, nil
	}
	ms, err := s.activeMemberships(ctx, p.UserID)
	if err != nil {
		return "", err
	}
	role, err := contract.CheckBusinessRole(ms, businessID, allowed...)
	if err != nil {
		reason := reasonInsufficientRole
		if errors.Is(err, contract.ErrNotFound) {
			reason = reasonNoMembership // includes cross-tenant probing (IDOR attempts)
		}
		s.auditDetached(ctx, s.newAudit(ctx, AuditAuthzDenied, OutcomeDenied).actor(p.UserID).business(businessID).
			meta("reason", reason).meta("required_roles", joinRoles(allowed)))
	}
	return role, err
}

// RequirePlatformAdmin implements contract.Authorizer. Principal.IsPlatformAdmin is read from
// the user row when the session is authenticated, i.e. also per request.
func (s *Service) RequirePlatformAdmin(ctx context.Context, p contract.Principal) error {
	err := contract.CheckPlatformAdmin(p)
	if err != nil {
		s.auditDetached(ctx, s.newAudit(ctx, AuditAuthzDenied, OutcomeDenied).actor(p.UserID).
			meta("reason", reasonInsufficientRole).meta("required_roles", "PLATFORM_ADMIN"))
	}
	return err
}

func joinRoles(roles []contract.Role) string {
	out := make([]string, len(roles))
	for i, r := range roles {
		out[i] = string(r)
	}
	return strings.Join(out, ",")
}

// MyMemberships lists the caller's active memberships, sorted by business ID for stable output.
func (s *Service) MyMemberships(ctx context.Context, p contract.Principal) ([]contract.Membership, error) {
	if p.UserID == uuid.Nil {
		return nil, ErrUnauthenticated
	}
	ms, err := s.activeMemberships(ctx, p.UserID)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(ms, func(a, b contract.Membership) int {
		return strings.Compare(a.BusinessID.String(), b.BusinessID.String())
	})
	return ms, nil
}

func (s *Service) activeMemberships(ctx context.Context, userID uuid.UUID) ([]contract.Membership, error) {
	var ms []contract.Membership
	err := s.store.WithinTx(ctx, func(tx Tx) error {
		var err error
		ms, err = tx.ActiveMembershipsByUser(ctx, userID)
		return err
	})
	return ms, err
}
