package application

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/netip"
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
	password, err := domain.PasswordForVerification(in.Password)
	if err != nil {
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
		return LoginResult{}, ErrInvalidCredentials
	}

	ok, needsRehash, err := s.hasher.Verify(ctx, password, user.PasswordHash())
	if err != nil {
		return LoginResult{}, fmt.Errorf("login: verify: %w", err)
	}
	if !ok {
		return LoginResult{}, ErrInvalidCredentials
	}
	switch err := user.CanAuthenticate(); {
	case errors.Is(err, domain.ErrUserNotVerified):
		return LoginResult{}, ErrEmailNotVerified
	case err != nil:
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
		return tx.CreateSession(ctx, session, tok.Hash(), in.IP, truncate(in.UserAgent, maxUserAgent))
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
		return tx.RevokeSession(ctx, p.SessionID, p.UserID, now)
	})
}

// LogoutAll revokes every session of the caller (including the current one).
func (s *Service) LogoutAll(ctx context.Context, p contract.Principal) (int64, error) {
	now := s.clock.Now()
	var n int64
	err := s.store.WithinTx(ctx, func(tx Tx) error {
		var err error
		n, err = tx.RevokeAllUserSessions(ctx, p.UserID, now)
		return err
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
