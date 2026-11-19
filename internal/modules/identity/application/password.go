package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/domain"
)

// PasswordResetTokenTTL is how long a reset link stays valid (security.md §1).
const PasswordResetTokenTTL = 30 * time.Minute

// Field codes of the password endpoints.
const (
	CodeInvalidCurrentPassword = "INVALID_CURRENT_PASSWORD"
	CodePasswordUnchanged      = "PASSWORD_UNCHANGED"
)

// ForgotPassword schedules a reset email if the account exists and is active. It always
// succeeds for well-formed emails (anti-enumeration).
func (s *Service) ForgotPassword(ctx context.Context, rawEmail string) error {
	email, err := domain.ParseEmail(rawEmail)
	if err != nil {
		return &ValidationError{Fields: []FieldError{{"email", CodeInvalidEmail}}}
	}
	return s.store.WithinTx(ctx, func(tx Tx) error {
		u, err := tx.UserByEmail(ctx, email)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if u.Status() != domain.UserActive {
			return nil // pending accounts verify first; suspended ones cannot reset
		}
		return tx.EnqueuePasswordResetEmail(ctx, u.ID())
	})
}

// SendPasswordResetEmail issues a fresh reset token and emails it (worker job). The raw token is
// never persisted.
func (s *Service) SendPasswordResetEmail(ctx context.Context, userID uuid.UUID) error {
	tok, err := s.newToken(domain.TokenPrefixPasswordReset)
	if err != nil {
		return err
	}
	now := s.clock.Now()
	var recipient *domain.User
	err = s.store.WithinTx(ctx, func(tx Tx) error {
		u, err := tx.UserByID(ctx, userID)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if u.Status() != domain.UserActive {
			return nil
		}
		if err := tx.InvalidatePasswordResetTokens(ctx, userID, now); err != nil {
			return err
		}
		if err := tx.CreatePasswordResetToken(ctx, tok.Hash(), userID, now, now.Add(PasswordResetTokenTTL)); err != nil {
			return err
		}
		recipient = u
		return nil
	})
	if err != nil || recipient == nil {
		return err
	}
	link := strings.TrimRight(s.cfg.PublicAppURL, "/") + "/reset-password?token=" + url.QueryEscape(tok.Reveal())
	body := fmt.Sprintf(passwordResetEmailBody, recipient.FullName(), link, int(PasswordResetTokenTTL.Minutes()))
	if err := s.mailer.Send(ctx, recipient.Email().String(), passwordResetEmailSubject, body); err != nil {
		return fmt.Errorf("password reset email: send: %w", err)
	}
	s.logger.InfoContext(ctx, "password reset email sent", slog.String("user_id", userID.String()))
	return nil
}

// ResetPasswordInput is a reset request from the emailed link.
type ResetPasswordInput struct {
	Token       string
	NewPassword string
}

// ResetPassword sets a new password with a valid reset token, revokes every session and
// notifies the owner. The password policy is checked before the token is consumed, so a
// rejected password does not burn the link.
func (s *Service) ResetPassword(ctx context.Context, in ResetPasswordInput) error {
	tok, err := domain.ParseToken(domain.TokenPrefixPasswordReset, in.Token)
	if err != nil {
		return domain.ErrInvalidToken
	}
	password, err := domain.NewPassword(in.NewPassword)
	if err != nil {
		return &ValidationError{Fields: []FieldError{{"newPassword", passwordCode(err)}}}
	}
	hash, err := s.hasher.Hash(ctx, password)
	if err != nil {
		return fmt.Errorf("reset password: hash: %w", err)
	}
	now := s.clock.Now()
	var userID uuid.UUID
	var revoked int64
	err = s.store.WithinTx(ctx, func(tx Tx) error {
		id, err := tx.ConsumePasswordResetToken(ctx, tok.Hash(), now)
		if errors.Is(err, ErrNotFound) {
			return domain.ErrInvalidToken
		}
		if err != nil {
			return err
		}
		u, err := tx.UserByID(ctx, id)
		if err != nil {
			return err
		}
		if u.Status() != domain.UserActive {
			return domain.ErrInvalidToken
		}
		if err := tx.UpdatePasswordHash(ctx, id, hash); err != nil {
			return err
		}
		if revoked, err = tx.RevokeAllUserSessions(ctx, id, now); err != nil {
			return err
		}
		if err := tx.InvalidatePasswordResetTokens(ctx, id, now); err != nil {
			return err
		}
		userID = id
		return tx.EnqueuePasswordChangedEmail(ctx, id)
	})
	if err != nil {
		return err
	}
	s.logger.InfoContext(ctx, "password reset", slog.String("user_id", userID.String()), slog.Int64("sessions_revoked", revoked))
	return nil
}

// ChangePasswordInput is an authenticated password change.
type ChangePasswordInput struct {
	CurrentPassword string
	NewPassword     string
}

// ChangePassword verifies the current password, sets the new one, revokes every other session
// (the current one stays) and notifies the owner.
func (s *Service) ChangePassword(ctx context.Context, p contract.Principal, in ChangePasswordInput) error {
	verr := &ValidationError{}
	newPassword, err := domain.NewPassword(in.NewPassword)
	if err != nil {
		verr.add("newPassword", passwordCode(err))
	}
	current, curErr := domain.PasswordForVerification(in.CurrentPassword)
	if curErr != nil || in.CurrentPassword == "" {
		verr.add("currentPassword", CodeInvalidCurrentPassword)
	}
	if err := verr.orNil(); err != nil {
		return err
	}
	if in.CurrentPassword == in.NewPassword {
		return &ValidationError{Fields: []FieldError{{"newPassword", CodePasswordUnchanged}}}
	}

	u, err := s.Me(ctx, p)
	if err != nil {
		return err
	}
	ok, _, err := s.hasher.Verify(ctx, current, u.PasswordHash())
	if err != nil {
		return fmt.Errorf("change password: verify: %w", err)
	}
	if !ok {
		return &ValidationError{Fields: []FieldError{{"currentPassword", CodeInvalidCurrentPassword}}}
	}
	hash, err := s.hasher.Hash(ctx, newPassword)
	if err != nil {
		return fmt.Errorf("change password: hash: %w", err)
	}
	now := s.clock.Now()
	var revoked int64
	err = s.store.WithinTx(ctx, func(tx Tx) error {
		if err := tx.UpdatePasswordHash(ctx, p.UserID, hash); err != nil {
			return err
		}
		if revoked, err = tx.RevokeOtherUserSessions(ctx, p.UserID, p.SessionID, now); err != nil {
			return err
		}
		if err := tx.InvalidatePasswordResetTokens(ctx, p.UserID, now); err != nil {
			return err
		}
		return tx.EnqueuePasswordChangedEmail(ctx, p.UserID)
	})
	if err != nil {
		return err
	}
	s.logger.InfoContext(ctx, "password changed", slog.String("user_id", p.UserID.String()), slog.Int64("other_sessions_revoked", revoked))
	return nil
}

// SendPasswordChangedEmail notifies the owner that the password changed (worker job). It
// contains no token or link that grants access.
func (s *Service) SendPasswordChangedEmail(ctx context.Context, userID uuid.UUID) error {
	var u *domain.User
	err := s.store.WithinTx(ctx, func(tx Tx) error {
		var err error
		u, err = tx.UserByID(ctx, userID)
		if errors.Is(err, ErrNotFound) {
			u = nil
			return nil
		}
		return err
	})
	if err != nil || u == nil {
		return err
	}
	body := fmt.Sprintf(passwordChangedEmailBody, u.FullName(), s.clock.Now().UTC().Format("2006-01-02 15:04 UTC"))
	if err := s.mailer.Send(ctx, u.Email().String(), passwordChangedEmailSubject, body); err != nil {
		return fmt.Errorf("password changed email: send: %w", err)
	}
	return nil
}

const passwordResetEmailSubject = "Restablece tu contraseña" //nolint:gosec // G101: email subject, not a credential

//nolint:gosec // G101: email template, not a credential
const passwordResetEmailBody = `Hola %s:

Recibimos una solicitud para restablecer tu contraseña. Para elegir una nueva, abre este enlace:

%s

El enlace vence en %d minutos y solo se puede usar una vez. Al cambiar la contraseña se cerrarán todas tus sesiones.

Si no solicitaste este cambio, ignora este mensaje: tu contraseña actual sigue siendo válida.
`

const passwordChangedEmailSubject = "Tu contraseña fue cambiada" //nolint:gosec // G101: email subject, not a credential

//nolint:gosec // G101: email template, not a credential
const passwordChangedEmailBody = `Hola %s:

La contraseña de tu cuenta fue cambiada el %s.

Si fuiste tú, no necesitas hacer nada. Si no reconoces este cambio, restablece tu contraseña de inmediato desde la opción "¿Olvidaste tu contraseña?" y contacta a soporte.
`
