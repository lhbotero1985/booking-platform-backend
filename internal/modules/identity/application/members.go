package application

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/domain"
)

// AddMember implements contract.MemberAdder: grants role in businessID to the active account
// with email. PLATFORM_ADMIN may do it for any business; otherwise the actor must be the
// business OWNER (denials are audited by the authorizer). Pass a context carrying the caller's
// transaction to make it atomic with the caller's changes.
func (s *Service) AddMember(ctx context.Context, actor contract.Principal, businessID uuid.UUID, rawEmail string, role contract.Role) (uuid.UUID, error) {
	if contract.CheckPlatformAdmin(actor) != nil {
		if _, err := s.RequireBusinessRole(ctx, actor, businessID, contract.RolesOwner...); err != nil {
			return uuid.Nil, err
		}
	}
	if _, err := domain.ParseRole(string(role)); err != nil {
		return uuid.Nil, err
	}
	email, err := domain.ParseEmail(rawEmail)
	if err != nil {
		return uuid.Nil, contract.ErrAccountNotEligible // no account can have a malformed email
	}
	var userID uuid.UUID
	err = s.store.WithinTx(ctx, func(tx Tx) error {
		u, err := tx.UserByEmail(ctx, email)
		if errors.Is(err, ErrNotFound) {
			return contract.ErrAccountNotEligible
		}
		if err != nil {
			return err
		}
		if u.Status() != domain.UserActive {
			return contract.ErrAccountNotEligible
		}
		now := s.clock.Now()
		if err := tx.InsertMembership(ctx, uuid.Must(uuid.NewV7()), u.ID(), businessID, role, now); err != nil {
			return err
		}
		userID = u.ID()
		return tx.AppendAudit(ctx, *s.newAudit(ctx, AuditMembershipGranted, OutcomeSuccess).
			actor(actor.UserID).business(businessID).resource("user", u.ID()).meta("role", string(role)))
	})
	return userID, err
}
