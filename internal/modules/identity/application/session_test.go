package application_test

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/domain"
)

// verifiedUser registers and verifies an account.
func (e env) verifiedUser(t *testing.T, email string) {
	t.Helper()
	e.register(t, email)
	if err := e.svc.VerifyEmail(context.Background(), e.sendAndExtract(t)); err != nil {
		t.Fatal(err)
	}
}

func (e env) login(email, password, client string) (application.LoginResult, error) {
	return e.svc.Login(context.Background(), application.LoginInput{Email: email, Password: password, ClientType: client, UserAgent: "test"})
}

func TestLoginSuccessCreatesSession(t *testing.T) {
	e := newEnv()
	e.verifiedUser(t, "ana@example.com")

	res, err := e.login("ANA@example.com", goodPassword, "MOBILE")
	if err != nil {
		t.Fatal(err)
	}
	raw := res.Token.Reveal()
	if !strings.HasPrefix(raw, domain.TokenPrefixSession) || res.ClientType != domain.ClientMobile {
		t.Fatalf("token %q client %s", raw, res.ClientType)
	}
	now := e.clock.Now()
	if !res.AbsoluteExpiresAt.Equal(now.Add(90*24*time.Hour)) || !res.IdleExpiresAt.Equal(now.Add(30*24*time.Hour)) {
		t.Fatalf("mobile expiries: %v / %v", res.IdleExpiresAt, res.AbsoluteExpiresAt)
	}
	u := e.store.User("ana@example.com")
	if e.store.SessionCount(u.ID()) != 1 {
		t.Fatal("session not stored")
	}
	for _, h := range e.store.SessionHashes() {
		if h == hex.EncodeToString([]byte(raw)) || strings.Contains(raw, h) {
			t.Fatal("raw session token stored")
		}
	}

	p, err := e.svc.Authenticate(context.Background(), raw)
	if err != nil || p.UserID != u.ID() || p.ClientType != "MOBILE" || p.IsPlatformAdmin {
		t.Fatalf("Authenticate: %+v %v", p, err)
	}
}

func TestLoginWebAndAdminPolicies(t *testing.T) {
	e := newEnv()
	e.verifiedUser(t, "ana@example.com")
	res, _ := e.login("ana@example.com", goodPassword, "WEB")
	if !res.AbsoluteExpiresAt.Equal(e.clock.Now().Add(7 * 24 * time.Hour)) {
		t.Fatalf("web absolute = %v", res.AbsoluteExpiresAt)
	}
	e.store.SetPlatformAdmin("ana@example.com")
	res, _ = e.login("ana@example.com", goodPassword, "MOBILE")
	if !res.AbsoluteExpiresAt.Equal(e.clock.Now().Add(12*time.Hour)) || !res.IdleExpiresAt.Equal(e.clock.Now().Add(time.Hour)) {
		t.Fatalf("admin policy not applied: %v / %v", res.IdleExpiresAt, res.AbsoluteExpiresAt)
	}
}

// TestLoginFailuresAreIndistinguishable: unknown email, wrong password and suspended account
// return the same error; the unknown-email path still spends a password verification.
func TestLoginFailuresAreIndistinguishable(t *testing.T) {
	e := newEnv()
	e.verifiedUser(t, "ana@example.com")
	e.verifiedUser(t, "bob@example.com")
	e.store.User("bob@example.com").Suspend(e.clock.Now())

	for name, attempt := range map[string][2]string{
		"unknown email":  {"nobody@example.com", goodPassword},
		"wrong password": {"ana@example.com", goodPassword + "x"},
		"suspended":      {"bob@example.com", goodPassword},
	} {
		if _, err := e.login(attempt[0], attempt[1], "WEB"); !errors.Is(err, application.ErrInvalidCredentials) {
			t.Errorf("%s: err = %v, want ErrInvalidCredentials", name, err)
		}
	}
	if e.hasher.DummyCalls != 1 {
		t.Errorf("VerifyDummy calls = %d, want 1 (unknown email path)", e.hasher.DummyCalls)
	}
}

func TestLoginRequiresVerifiedEmail(t *testing.T) {
	e := newEnv()
	e.register(t, "ana@example.com")
	if _, err := e.login("ana@example.com", goodPassword+"x", "WEB"); !errors.Is(err, application.ErrInvalidCredentials) {
		t.Fatalf("wrong password on unverified account must not reveal state: %v", err)
	}
	if _, err := e.login("ana@example.com", goodPassword, "WEB"); !errors.Is(err, application.ErrEmailNotVerified) {
		t.Fatalf("err = %v, want ErrEmailNotVerified", err)
	}
	if n := e.store.SessionCount(e.store.User("ana@example.com").ID()); n != 0 {
		t.Fatal("session created for unverified account")
	}
}

func TestLoginValidation(t *testing.T) {
	e := newEnv()
	_, err := e.login("bad", "", "TABLET")
	var verr *application.ValidationError
	if !errors.As(err, &verr) || len(verr.Fields) != 3 {
		t.Fatalf("err = %v", err)
	}
	if _, err := e.login("ana@example.com", strings.Repeat("x", domain.MaxPasswordLength+1), "WEB"); !errors.Is(err, application.ErrInvalidCredentials) {
		t.Fatalf("oversized password: %v", err)
	}
}

func TestAuthenticateRejections(t *testing.T) {
	e := newEnv()
	e.verifiedUser(t, "ana@example.com")
	res, _ := e.login("ana@example.com", goodPassword, "WEB")
	raw := res.Token.Reveal()

	for name, tok := range map[string]string{
		"empty":           "",
		"malformed":       "Bearer abc",
		"wrong prefix":    domain.TokenPrefixEmailVerification + raw[len(domain.TokenPrefixSession):],
		"unknown session": domain.TokenPrefixSession + strings.Repeat("A", 43),
	} {
		if _, err := e.svc.Authenticate(context.Background(), tok); !errors.Is(err, application.ErrUnauthenticated) {
			t.Errorf("%s: %v", name, err)
		}
	}

	// Idle expiry (web: 12 h).
	e.clock.Advance(12*time.Hour + time.Second)
	if _, err := e.svc.Authenticate(context.Background(), raw); !errors.Is(err, application.ErrUnauthenticated) {
		t.Fatalf("idle-expired session accepted: %v", err)
	}
}

func TestAuthenticateSlidesIdleAndHitsAbsolute(t *testing.T) {
	e := newEnv()
	e.verifiedUser(t, "ana@example.com")
	res, _ := e.login("ana@example.com", goodPassword, "WEB")
	raw := res.Token.Reveal()

	// Activity every 11 h keeps the session alive past the 12 h idle window…
	for range 20 { // 20 × 11 h = 220 h > 168 h (7-day absolute expiry)
		e.clock.Advance(11 * time.Hour)
		if _, err := e.svc.Authenticate(context.Background(), raw); err != nil {
			if e.clock.Now().Before(res.AbsoluteExpiresAt) {
				t.Fatalf("active session rejected at %v: %v", e.clock.Now(), err)
			}
			return // …but never beyond the 7-day absolute expiry.
		}
	}
	t.Fatal("session survived beyond its absolute expiry")
}

func TestSuspensionBlocksExistingSessions(t *testing.T) {
	e := newEnv()
	e.verifiedUser(t, "ana@example.com")
	res, _ := e.login("ana@example.com", goodPassword, "MOBILE")
	e.store.User("ana@example.com").Suspend(e.clock.Now())
	if _, err := e.svc.Authenticate(context.Background(), res.Token.Reveal()); !errors.Is(err, application.ErrUnauthenticated) {
		t.Fatalf("suspended user authenticated: %v", err)
	}
}

func TestLogoutAndLogoutAll(t *testing.T) {
	e := newEnv()
	e.verifiedUser(t, "ana@example.com")
	a, _ := e.login("ana@example.com", goodPassword, "MOBILE")
	b, _ := e.login("ana@example.com", goodPassword, "WEB")
	c, _ := e.login("ana@example.com", goodPassword, "WEB")

	pa, _ := e.svc.Authenticate(context.Background(), a.Token.Reveal())
	if err := e.svc.Logout(context.Background(), pa); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Authenticate(context.Background(), a.Token.Reveal()); !errors.Is(err, application.ErrUnauthenticated) {
		t.Fatal("logged-out session still valid")
	}
	if _, err := e.svc.Authenticate(context.Background(), b.Token.Reveal()); err != nil {
		t.Fatal("logout must only revoke the current session")
	}

	pb, _ := e.svc.Authenticate(context.Background(), b.Token.Reveal())
	n, err := e.svc.LogoutAll(context.Background(), pb)
	if err != nil || n != 2 {
		t.Fatalf("LogoutAll = %d, %v; want 2", n, err)
	}
	for _, r := range []application.LoginResult{b, c} {
		if _, err := e.svc.Authenticate(context.Background(), r.Token.Reveal()); !errors.Is(err, application.ErrUnauthenticated) {
			t.Fatal("session survived logout-all")
		}
	}
}

func TestMe(t *testing.T) {
	e := newEnv()
	e.verifiedUser(t, "ana@example.com")
	res, _ := e.login("ana@example.com", goodPassword, "WEB")
	p, _ := e.svc.Authenticate(context.Background(), res.Token.Reveal())
	u, err := e.svc.Me(context.Background(), p)
	if err != nil || u.Email().String() != "ana@example.com" || u.EmailVerifiedAt() == nil {
		t.Fatalf("Me: %+v %v", u, err)
	}
}
