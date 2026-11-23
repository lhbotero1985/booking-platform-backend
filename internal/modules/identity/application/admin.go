package application

import (
	"context"
	"errors"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/domain"
)

// ErrAccountNotActive is returned when granting PLATFORM_ADMIN to an unverified or suspended
// account.
var ErrAccountNotActive = errors.New("identity: account is not active")

// SetPlatformAdmin grants or revokes PLATFORM_ADMIN for the account with email. It is reachable
// only from the operator command "api admin" (server access), never over HTTP, and is audited
// with source "cli" (no actor: the operator is identified by the server access logs). The
// change applies on the user's next request: principals are loaded per request.
func (s *Service) SetPlatformAdmin(ctx context.Context, rawEmail string, admin bool) error {
	email, err := domain.ParseEmail(rawEmail)
	if err != nil {
		return &ValidationError{Fields: []FieldError{{"email", CodeInvalidEmail}}}
	}
	return s.store.WithinTx(ctx, func(tx Tx) error {
		u, err := tx.UserByEmail(ctx, email)
		if err != nil {
			return err
		}
		if admin && u.Status() != domain.UserActive {
			return ErrAccountNotActive
		}
		if u.IsPlatformAdmin() == admin {
			return nil
		}
		if err := tx.SetPlatformAdmin(ctx, u.ID(), admin); err != nil {
			return err
		}
		action := AuditPlatformAdminGranted
		if !admin {
			action = AuditPlatformAdminRevoked
		}
		return tx.AppendAudit(ctx, *s.newAudit(ctx, action, OutcomeSuccess).
			resource("user", u.ID()).meta("source", "cli"))
	})
}
