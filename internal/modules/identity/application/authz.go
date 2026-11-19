package application

import (
	"context"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
)

// Service implements contract.Authorizer.
var _ contract.Authorizer = (*Service)(nil)

// RequireBusinessRole implements contract.Authorizer. Memberships are loaded on every call
// (indexed query, no cache) so revocations take effect on the next request (ADR-0007).
func (s *Service) RequireBusinessRole(ctx context.Context, p contract.Principal, businessID uuid.UUID, allowed ...contract.Role) (contract.Role, error) {
	if p.UserID == uuid.Nil {
		return "", ErrUnauthenticated
	}
	ms, err := s.activeMemberships(ctx, p.UserID)
	if err != nil {
		return "", err
	}
	return contract.CheckBusinessRole(ms, businessID, allowed...)
}

// RequirePlatformAdmin implements contract.Authorizer. Principal.IsPlatformAdmin is read from
// the user row when the session is authenticated, i.e. also per request.
func (s *Service) RequirePlatformAdmin(p contract.Principal) error {
	return contract.CheckPlatformAdmin(p)
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
