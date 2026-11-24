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

// ClaimOwnership implements contract.OwnershipClaimer: the actor becomes OWNER of businessID,
// which the caller has just created in the same transaction. One owned business per account
// (ADR-0018); the user row is locked so two concurrent requests cannot both succeed.
func (s *Service) ClaimOwnership(ctx context.Context, actor contract.Principal, businessID uuid.UUID) error {
	if actor.UserID == uuid.Nil {
		return ErrUnauthenticated
	}
	return s.store.WithinTx(ctx, func(tx Tx) error {
		if err := tx.LockUser(ctx, actor.UserID); err != nil {
			return err
		}
		ms, err := tx.ActiveMembershipsByUser(ctx, actor.UserID)
		if err != nil {
			return err
		}
		for _, m := range ms {
			if m.Role == contract.RoleOwner {
				return contract.ErrAlreadyOwner
			}
		}
		if err := tx.InsertMembership(ctx, uuid.Must(uuid.NewV7()), actor.UserID, businessID, contract.RoleOwner, s.clock.Now()); err != nil {
			return err
		}
		return tx.AppendAudit(ctx, *s.newAudit(ctx, AuditMembershipGranted, OutcomeSuccess).
			actor(actor.UserID).business(businessID).resource("user", actor.UserID).
			meta("role", string(contract.RoleOwner)).meta("via", "self_service"))
	})
}

// ListMembers returns the active members of a business to its OWNERs and MANAGERs (and
// PLATFORM_ADMIN). Another business answers ErrNotFound.
func (s *Service) ListMembers(ctx context.Context, actor contract.Principal, businessID uuid.UUID) ([]contract.Member, error) {
	if _, err := s.RequireBusinessRole(ctx, actor, businessID, contract.RolesManagement...); err != nil {
		return nil, err
	}
	var out []contract.Member
	err := s.store.WithinTx(ctx, func(tx Tx) (err error) {
		out, err = tx.ActiveMembers(ctx, businessID)
		return err
	})
	return out, err
}

// ChangeMemberRole changes the role of an active member (OWNER or PLATFORM_ADMIN). A business
// always keeps at least one OWNER (ErrLastOwner). Audited with the old and new role.
func (s *Service) ChangeMemberRole(ctx context.Context, actor contract.Principal, businessID, userID uuid.UUID, role contract.Role) error {
	if _, err := s.RequireBusinessRole(ctx, actor, businessID, contract.RolesOwner...); err != nil {
		return err
	}
	if _, err := domain.ParseRole(string(role)); err != nil {
		return err
	}
	return s.store.WithinTx(ctx, func(tx Tx) error {
		owners, err := tx.LockActiveOwners(ctx, businessID)
		if err != nil {
			return err
		}
		current, err := tx.ActiveRole(ctx, businessID, userID)
		if err != nil {
			return err
		}
		if current == role {
			return nil
		}
		if current == contract.RoleOwner && len(owners) <= 1 {
			return contract.ErrLastOwner
		}
		if err := tx.UpdateMembershipRole(ctx, businessID, userID, role, s.clock.Now()); err != nil {
			return err
		}
		return tx.AppendAudit(ctx, *s.newAudit(ctx, AuditMembershipRoleChanged, OutcomeSuccess).
			actor(actor.UserID).business(businessID).resource("user", userID).
			meta("from", string(current)).meta("to", string(role)))
	})
}

// RemoveMember revokes an active membership (OWNER or PLATFORM_ADMIN; an OWNER may remove
// itself while another OWNER remains). The member loses access on its next request.
func (s *Service) RemoveMember(ctx context.Context, actor contract.Principal, businessID, userID uuid.UUID) error {
	if _, err := s.RequireBusinessRole(ctx, actor, businessID, contract.RolesOwner...); err != nil {
		return err
	}
	return s.store.WithinTx(ctx, func(tx Tx) error {
		owners, err := tx.LockActiveOwners(ctx, businessID)
		if err != nil {
			return err
		}
		current, err := tx.ActiveRole(ctx, businessID, userID)
		if err != nil {
			return err
		}
		if current == contract.RoleOwner && len(owners) <= 1 {
			return contract.ErrLastOwner
		}
		if err := tx.RevokeMembership(ctx, businessID, userID, s.clock.Now()); err != nil {
			return err
		}
		return tx.AppendAudit(ctx, *s.newAudit(ctx, AuditMembershipRevoked, OutcomeSuccess).
			actor(actor.UserID).business(businessID).resource("user", userID).meta("role", string(current)))
	})
}
