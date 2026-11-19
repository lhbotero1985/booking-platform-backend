package application

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/netip"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/domain"
)

// CodeInvalidClientType is the field code for an unknown clientType.
const CodeInvalidClientType = "INVALID_CLIENT_TYPE"

// maxUserAgent matches the sessions_user_agent_length CHECK.
const maxUserAgent = 512

// LoginInput is a login attempt.
type LoginInput struct {
	Email      string
	Password   string
	ClientType string
	IP         *netip.Addr
	UserAgent  string
}

// LoginResult carries the new session token. Token must only be delivered to the client (body
// or cookie) and never logged.
type LoginResult struct {
	Token             domain.Token
	ClientType        domain.ClientType
	IdleExpiresAt     time.Time
	AbsoluteExpiresAt time.Time
}

// Login authenticates with email and password and opens a session.
//
// Unknown email, wrong password and suspended account all return ErrInvalidCredentials and take
// comparable time (VerifyDummy). ErrEmailNotVerified is only returned after a correct password.
//
// Repeated failures for the same email+IP trigger a progressive backoff (*RetryAfterError),
// checked before any password hashing so a blocked attacker costs no Argon2 work. The backoff
// is keyed by the submitted email whether or not the account exists.
func (s *Service) Login(ctx context.Context, in LoginInput) (LoginResult, error) {
	verr := &ValidationError{}
	email, emailErr := domain.ParseEmail(in.Email)
	if emailErr != nil {
		verr.add("email", CodeInvalidEmail)
	}
	if in.Password == "" {
		verr.add("password", CodeRequired)
	}
	clientType, err := domain.ParseClientType(in.ClientType)
	if err != nil {
		verr.add("clientType", CodeInvalidClientType)
	}
	if err := verr.orNil(); err != nil {
		return LoginResult{}, err
	}
	key := loginThrottleKey(email, in.IP)
	if err := s.checkThrottle(key); err != nil {
		s.auditDetached(ctx, s.newAudit(ctx, AuditLogin, OutcomeDenied).withIP(in.IP).
			meta("reason", reasonThrottled).meta("email_fp", emailFingerprint(email)).meta("client_type", string(clientType)))
		return LoginResult{}, err
	}
	var out loginOutcome
	res, err := s.login(ctx, in, email, clientType, &out)
	switch {
	case errors.Is(err, ErrInvalidCredentials):
		s.throttle.Failure(key)
	case err == nil, errors.Is(err, ErrEmailNotVerified): // correct password
		s.throttle.Success(key)
	}
	if out.reason != "" { // failures; success is audited inside the session transaction
		outcome := OutcomeFailure
		if errors.Is(err, ErrEmailNotVerified) {
			outcome = OutcomeDenied
		}
		e := s.newAudit(ctx, AuditLogin, outcome).withIP(in.IP).actor(out.userID).
			meta("reason", out.reason).meta("client_type", string(clientType))
		if out.userID == uuid.Nil {
			e.meta("email_fp", emailFingerprint(email))
		}
		s.auditDetached(ctx, e)
	}
	return res, err
}

// loginOutcome tells Login what to audit for a failed attempt.
type loginOutcome struct {
	userID uuid.UUID
	reason string
}

func (s *Service) login(ctx context.Context, in LoginInput, email domain.Email, clientType domain.ClientType, out *loginOutcome) (LoginResult, error) {
	password, err := domain.PasswordForVerification(in.Password)
	if err != nil {
		out.reason = reasonWrongPassword
		return LoginResult{}, ErrInvalidCredentials
	}

	var user *domain.User
	err = s.store.WithinTx(ctx, func(tx Tx) error {
		u, err := tx.UserByEmail(ctx, email)
		if err == nil {
			user = u
		}
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
	})
	if err != nil {
		return LoginResult{}, err
	}
	if user == nil {
		s.hasher.VerifyDummy(ctx, password)
		out.reason = reasonUnknownAccount
		return LoginResult{}, ErrInvalidCredentials
	}
	out.userID = user.ID()

	ok, needsRehash, err := s.hasher.Verify(ctx, password, user.PasswordHash())
	if err != nil {
		return LoginResult{}, fmt.Errorf("login: verify: %w", err)
	}
	if !ok {
		out.reason = reasonWrongPassword
		return LoginResult{}, ErrInvalidCredentials
	}
	switch err := user.CanAuthenticate(); {
	case errors.Is(err, domain.ErrUserNotVerified):
		out.reason = reasonEmailNotVerified
		return LoginResult{}, ErrEmailNotVerified
	case err != nil:
		out.reason = reasonAccountSuspended
		return LoginResult{}, ErrInvalidCredentials
	}

	var newHash domain.PasswordHash
	if needsRehash {
		if newHash, err = s.hasher.Hash(ctx, password); err != nil {
			return LoginResult{}, fmt.Errorf("login: rehash: %w", err)
		}
	}

	tok, err := s.newToken(domain.TokenPrefixSession)
	if err != nil {
		return LoginResult{}, err
	}
	now := s.clock.Now()
	session := domain.NewSession(uuid.Must(uuid.NewV7()), user.ID(), clientType, domain.PolicyFor(clientType, user.IsPlatformAdmin()), now)

	err = s.store.WithinTx(ctx, func(tx Tx) error {
		if newHash != "" {
			if err := tx.UpdatePasswordHash(ctx, user.ID(), newHash); err != nil {
				return err
			}
		}
		if err := tx.CreateSession(ctx, session, tok.Hash(), in.IP, truncate(in.UserAgent, maxUserAgent)); err != nil {
			return err
		}
		e := s.newAudit(ctx, AuditLogin, OutcomeSuccess).withIP(in.IP).actor(user.ID()).
			resource("session", session.ID()).meta("client_type", string(clientType))
		if newHash != "" {
			e.meta("rehashed", "true")
		}
		return tx.AppendAudit(ctx, *e)
	})
	if err != nil {
		return LoginResult{}, err
	}
	s.logger.InfoContext(ctx, "login succeeded",
		slog.String("user_id", user.ID().String()),
		slog.String("session_id", session.ID().String()),
		slog.String("client_type", string(clientType)),
		slog.Bool("rehashed", newHash != ""))
	return LoginResult{
		Token: tok, ClientType: clientType,
		IdleExpiresAt: session.IdleExpiresAt(), AbsoluteExpiresAt: session.AbsoluteExpiresAt(),
	}, nil
}

// Authenticate resolves a presented session token to its principal and slides the idle expiry
// (throttled). Every failure is ErrUnauthenticated.
func (s *Service) Authenticate(ctx context.Context, rawToken string) (contract.Principal, error) {
	tok, err := domain.ParseToken(domain.TokenPrefixSession, rawToken)
	if err != nil {
		return contract.Principal{}, ErrUnauthenticated
	}
	now := s.clock.Now()
	var p contract.Principal
	err = s.store.WithinTx(ctx, func(tx Tx) error {
		sess, user, err := tx.ActiveSessionByTokenHash(ctx, tok.Hash(), now)
		if errors.Is(err, ErrNotFound) {
			return ErrUnauthenticated
		}
		if err != nil {
			return err
		}
		if user.CanAuthenticate() != nil {
			return ErrUnauthenticated
		}
		if sess.Touch(now, domain.PolicyFor(sess.ClientType(), user.IsPlatformAdmin())) {
			if err := tx.TouchSession(ctx, sess.ID(), now, sess.IdleExpiresAt()); err != nil {
				return err
			}
		}
		p = contract.Principal{
			UserID: user.ID(), SessionID: sess.ID(),
			IsPlatformAdmin: user.IsPlatformAdmin(), ClientType: string(sess.ClientType()),
		}
		return nil
	})
	return p, err
}

// Logout revokes the caller's current session.
func (s *Service) Logout(ctx context.Context, p contract.Principal) error {
	now := s.clock.Now()
	return s.store.WithinTx(ctx, func(tx Tx) error {
		if err := tx.RevokeSession(ctx, p.SessionID, p.UserID, now); err != nil {
			return err
		}
		return tx.AppendAudit(ctx, *s.newAudit(ctx, AuditLogout, OutcomeSuccess).actor(p.UserID).resource("session", p.SessionID))
	})
}

// LogoutAll revokes every session of the caller (including the current one).
func (s *Service) LogoutAll(ctx context.Context, p contract.Principal) (int64, error) {
	now := s.clock.Now()
	var n int64
	err := s.store.WithinTx(ctx, func(tx Tx) error {
		var err error
		if n, err = tx.RevokeAllUserSessions(ctx, p.UserID, now); err != nil {
			return err
		}
		return tx.AppendAudit(ctx, *s.newAudit(ctx, AuditLogoutAll, OutcomeSuccess).actor(p.UserID).
			resource("session", p.SessionID).meta("sessions_revoked", strconv.FormatInt(n, 10)))
	})
	return n, err
}

// Me returns the caller's account.
func (s *Service) Me(ctx context.Context, p contract.Principal) (*domain.User, error) {
	var u *domain.User
	err := s.store.WithinTx(ctx, func(tx Tx) error {
		var err error
		u, err = tx.UserByID(ctx, p.UserID)
		return err
	})
	return u, err
}

func (s *Service) newToken(prefix string) (domain.Token, error) {
	entropy := make([]byte, domain.TokenEntropyBytes)
	if _, err := io.ReadFull(s.random, entropy); err != nil {
		return domain.Token{}, fmt.Errorf("token entropy: %w", err)
	}
	return domain.NewToken(prefix, entropy)
}

func truncate(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	return string([]rune(s)[:maxRunes])
}
