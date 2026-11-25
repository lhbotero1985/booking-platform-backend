package application

// Customer phone (T6.3): mandatory to book (decision of 2026-11-24); WhatsApp in Phase 7.

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/domain"
)

// AuditPhoneChanged is audited when an account sets its phone (the number is not logged).
const AuditPhoneChanged = "auth.phone_changed"

// CodeInvalidPhone means the number is not E.164 nor a 10-digit Colombian mobile.
const CodeInvalidPhone = "INVALID_PHONE"

// SetPhone sets the caller's mobile number (normalized to E.164) and returns the account.
func (s *Service) SetPhone(ctx context.Context, p contract.Principal, raw string) (*domain.User, error) {
	phone, err := domain.ParsePhone(raw)
	if err != nil {
		return nil, &ValidationError{Fields: []FieldError{{"phone", CodeInvalidPhone}}}
	}
	var u *domain.User
	err = s.store.WithinTx(ctx, func(tx Tx) error {
		if u, err = tx.UserByID(ctx, p.UserID); err != nil {
			return err
		}
		if u.Phone() == phone {
			return nil
		}
		if err := tx.UpdatePhone(ctx, p.UserID, phone); err != nil {
			return err
		}
		u.ChangePhone(phone, s.clock.Now())
		return tx.AppendAudit(ctx, *s.newAudit(ctx, AuditPhoneChanged, OutcomeSuccess).actor(p.UserID))
	})
	if err != nil {
		return nil, err
	}
	s.logger.InfoContext(ctx, "phone changed", slog.String("user_id", p.UserID.String()))
	return u, nil
}

// CustomerProfile implements contract.Customers.
func (s *Service) CustomerProfile(ctx context.Context, userID uuid.UUID) (contract.Customer, error) {
	var u *domain.User
	err := s.store.WithinTx(ctx, func(tx Tx) (err error) {
		u, err = tx.UserByID(ctx, userID)
		return err
	})
	if err != nil {
		return contract.Customer{}, err
	}
	if err := u.CanAuthenticate(); err != nil {
		return contract.Customer{}, err
	}
	if u.Phone() == "" {
		return contract.Customer{}, contract.ErrPhoneRequired
	}
	return contract.Customer{UserID: u.ID(), FullName: u.FullName(), Phone: u.Phone(), Email: u.Email().String()}, nil
}

// AccountEmail implements contract.Accounts.
func (s *Service) AccountEmail(ctx context.Context, userID uuid.UUID) (string, error) {
	var u *domain.User
	err := s.store.WithinTx(ctx, func(tx Tx) (err error) {
		u, err = tx.UserByID(ctx, userID)
		return err
	})
	if errors.Is(err, ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if u.CanAuthenticate() != nil {
		return "", nil //nolint:nilerr // a disabled account gets no email
	}
	return u.Email().String(), nil
}
