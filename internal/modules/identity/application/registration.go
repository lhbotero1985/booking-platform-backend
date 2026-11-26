package application

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
)

// VerificationTokenTTL is how long an email verification link stays valid.
const VerificationTokenTTL = 24 * time.Hour

// Config holds use-case settings.
type Config struct {
	// PublicAppURL is the base URL of the client app that hosts /verify-email.
	PublicAppURL string
}

// Service implements the Identity & Access use cases.
type Service struct {
	store  Store
	hasher PasswordHasher
	mailer Mailer
	clock  clock.Clock
	random io.Reader
	cfg    Config
	logger *slog.Logger
	// throttle applies progressive backoff to password checks (login, change password).
	throttle AttemptThrottle
}

// Deps are the Service collaborators. Random defaults to crypto/rand, Clock to the system clock
// and Logger to a discarding logger.
type Deps struct {
	Store  Store
	Hasher PasswordHasher
	Mailer Mailer
	Clock  clock.Clock
	Random io.Reader
	Config Config
	Logger *slog.Logger
	// Throttle is the password-check backoff; nil disables it.
	Throttle AttemptThrottle
}

// NewService builds the Service.
func NewService(d Deps) *Service {
	if d.Random == nil {
		d.Random = rand.Reader
	}
	if d.Clock == nil {
		d.Clock = clock.System{}
	}
	if d.Logger == nil {
		d.Logger = slog.New(slog.DiscardHandler)
	}
	if d.Throttle == nil {
		d.Throttle = noThrottle{}
	}
	return &Service{store: d.Store, hasher: d.Hasher, mailer: d.Mailer, clock: d.Clock, random: d.Random, cfg: d.Config, logger: d.Logger, throttle: d.Throttle}
}

// RegisterInput is the self-registration request.
type RegisterInput struct {
	Email    string
	Password string
	FullName string
	Phone    string // mobile, mandatory (decision of 2026-11-24); normalized to E.164
}

// Register creates a PENDING_VERIFICATION account and schedules the verification email.
//
// Anti-enumeration: when the email is already registered it returns nil exactly like a new
// registration (and re-sends the verification email if the account is still pending). The
// password is hashed before checking existence so both paths take the same time.
func (s *Service) Register(ctx context.Context, in RegisterInput) error {
	verr := &ValidationError{}
	email, err := domain.ParseEmail(in.Email)
	if err != nil {
		verr.add("email", CodeInvalidEmail)
	}
	password, err := domain.NewPassword(in.Password)
	if err != nil {
		verr.add("password", passwordCode(err))
	}
	if strings.TrimSpace(in.FullName) == "" {
		verr.add("fullName", CodeRequired)
	}
	phone, err := domain.ParsePhone(in.Phone)
	switch {
	case strings.TrimSpace(in.Phone) == "":
		verr.add("phone", CodeRequired)
	case err != nil:
		verr.add("phone", CodeInvalidPhone)
	}
	if err := verr.orNil(); err != nil {
		return err
	}

	hash, err := s.hasher.Hash(ctx, password)
	if err != nil {
		return fmt.Errorf("register: hash: %w", err)
	}
	now := s.clock.Now()
	user, err := domain.NewUser(uuid.Must(uuid.NewV7()), email, in.FullName, hash, now)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidFullName) {
			verr.add("fullName", CodeInvalidFullName)
			return verr
		}
		return fmt.Errorf("register: %w", err)
	}
	user.WithPhone(phone)

	return s.store.WithinTx(ctx, func(tx Tx) error {
		existing, err := tx.UserByEmail(ctx, email)
		switch {
		case err == nil:
			e := s.newAudit(ctx, AuditRegistered, OutcomeFailure).actor(existing.ID()).meta("reason", reasonEmailTaken)
			if err := tx.AppendAudit(ctx, *e); err != nil {
				return err
			}
			return s.resendIfPending(ctx, tx, existing)
		case !errors.Is(err, ErrNotFound):
			return err
		}
		if err := tx.CreateUser(ctx, user); err != nil {
			if errors.Is(err, ErrEmailTaken) { // concurrent registration of the same email
				return nil
			}
			return err
		}
		if err := tx.AppendAudit(ctx, *s.newAudit(ctx, AuditRegistered, OutcomeSuccess).actor(user.ID())); err != nil {
			return err
		}
		return tx.EnqueueVerificationEmail(ctx, user.ID())
	})
}

func (s *Service) resendIfPending(ctx context.Context, tx Tx, u *domain.User) error {
	if u.Status() != domain.UserPendingVerification {
		return nil
	}
	return tx.EnqueueVerificationEmail(ctx, u.ID())
}

// ResendVerification schedules a new verification email if the account exists and is pending.
// It always succeeds for well-formed emails (anti-enumeration).
func (s *Service) ResendVerification(ctx context.Context, rawEmail string) error {
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
		return s.resendIfPending(ctx, tx, u)
	})
}

// VerifyEmail consumes a verification token and activates the account. Unknown, malformed,
// expired and used tokens all return domain.ErrInvalidToken.
func (s *Service) VerifyEmail(ctx context.Context, rawToken string) error {
	tok, err := domain.ParseToken(domain.TokenPrefixEmailVerification, rawToken)
	if err != nil {
		return domain.ErrInvalidToken
	}
	now := s.clock.Now()
	var notEligible uuid.UUID
	err = s.store.WithinTx(ctx, func(tx Tx) error {
		userID, err := tx.ConsumeEmailVerificationToken(ctx, tok.Hash(), now)
		if errors.Is(err, ErrNotFound) {
			return domain.ErrInvalidToken
		}
		if err != nil {
			return err
		}
		u, err := tx.UserByID(ctx, userID)
		if err != nil {
			return err
		}
		if err := u.VerifyEmail(now); err != nil {
			notEligible = userID
			return domain.ErrInvalidToken // suspended accounts cannot be activated by email
		}
		if err := tx.MarkEmailVerified(ctx, userID, now); err != nil {
			return err
		}
		return tx.AppendAudit(ctx, *s.newAudit(ctx, AuditEmailVerified, OutcomeSuccess).actor(userID))
	})
	if errors.Is(err, domain.ErrInvalidToken) {
		reason := reasonInvalidToken
		if notEligible != uuid.Nil {
			reason = reasonNotEligible
		}
		s.auditDetached(ctx, s.newAudit(ctx, AuditEmailVerified, OutcomeFailure).actor(notEligible).meta("reason", reason))
	}
	return err
}

// SendVerificationEmail issues a fresh verification token and emails it. It runs in the worker
// (River job) so the raw token is never persisted anywhere, not even in the job queue.
func (s *Service) SendVerificationEmail(ctx context.Context, userID uuid.UUID) error {
	tok, err := s.newToken(domain.TokenPrefixEmailVerification)
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
		if u.Status() != domain.UserPendingVerification {
			return nil // already verified or suspended: nothing to send
		}
		if err := tx.InvalidateEmailVerificationTokens(ctx, userID, now); err != nil {
			return err
		}
		if err := tx.CreateEmailVerificationToken(ctx, tok.Hash(), userID, now, now.Add(VerificationTokenTTL)); err != nil {
			return err
		}
		recipient = u
		return nil
	})
	if err != nil || recipient == nil {
		return err
	}

	link := strings.TrimRight(s.cfg.PublicAppURL, "/") + "/verify-email?token=" + url.QueryEscape(tok.Reveal())
	if err := s.sendBranded(ctx, recipient.Email().String(), verificationEmailSubject, verificationEmail(recipient.FullName(), link, VerificationTokenTTL)); err != nil {
		return fmt.Errorf("verification email: send: %w", err)
	}
	s.logger.InfoContext(ctx, "verification email sent", slog.String("user_id", userID.String()))
	return nil
}
