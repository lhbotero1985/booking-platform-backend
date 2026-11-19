package application_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/reqctx"
)

var clientAddr = netip.MustParseAddr("198.51.100.20")

// reqCtx simulates the context the HTTP layer builds (request ID + client IP).
func reqCtx() context.Context {
	return reqctx.WithClientIP(reqctx.WithRequestID(context.Background(), "req-123"), clientAddr)
}

// lastAudit returns the last entry and checks the fields every entry must carry.
func lastAudit(t *testing.T, e env, wantAction string, wantOutcome application.AuditOutcome) application.AuditEntry {
	t.Helper()
	a := e.store.LastAudit()
	if a.Action != wantAction || a.Outcome != wantOutcome {
		t.Fatalf("last audit = %s:%s, want %s:%s (all: %v)", a.Action, a.Outcome, wantAction, wantOutcome, e.store.AuditActions())
	}
	if a.ID == uuid.Nil || !a.OccurredAt.Equal(e.clock.Now()) {
		t.Fatalf("audit id/time: %v %v", a.ID, a.OccurredAt)
	}
	return a
}

func actorIs(t *testing.T, a application.AuditEntry, id uuid.UUID) {
	t.Helper()
	if a.ActorUserID == nil || *a.ActorUserID != id {
		t.Fatalf("actor = %v, want %v", a.ActorUserID, id)
	}
}

func TestAuditRegistrationAndVerification(t *testing.T) {
	e := newEnv()
	ctx := reqCtx()
	if err := e.svc.Register(ctx, application.RegisterInput{Email: "ana@example.com", Password: goodPassword, FullName: "Ana"}); err != nil {
		t.Fatal(err)
	}
	ana := e.store.User("ana@example.com").ID()
	a := lastAudit(t, e, application.AuditRegistered, application.OutcomeSuccess)
	actorIs(t, a, ana)
	if a.IP == nil || *a.IP != clientAddr || a.Metadata["request_id"] != "req-123" {
		t.Fatalf("request context not recorded: %+v", a)
	}

	_ = e.svc.Register(ctx, application.RegisterInput{Email: "ANA@example.com", Password: goodPassword, FullName: "Otra"})
	a = lastAudit(t, e, application.AuditRegistered, application.OutcomeFailure)
	actorIs(t, a, ana)
	if a.Metadata["reason"] != "email_taken" {
		t.Fatalf("reason = %q", a.Metadata["reason"])
	}

	if err := e.svc.VerifyEmail(ctx, "bp_ev_"+strings.Repeat("A", 43)); err == nil {
		t.Fatal("bogus token accepted")
	}
	a = lastAudit(t, e, application.AuditEmailVerified, application.OutcomeFailure)
	if a.ActorUserID != nil || a.Metadata["reason"] != "invalid_token" {
		t.Fatalf("invalid token audit: %+v", a)
	}
	if err := e.svc.VerifyEmail(ctx, e.sendAndExtract(t)); err != nil {
		t.Fatal(err)
	}
	actorIs(t, lastAudit(t, e, application.AuditEmailVerified, application.OutcomeSuccess), ana)
}

func TestAuditLoginOutcomes(t *testing.T) {
	e := newEnv()
	e.verifiedUser(t, "ana@example.com")
	e.register(t, "pendiente@example.com")
	e.verifiedUser(t, "bob@example.com")
	e.store.User("bob@example.com").Suspend(e.clock.Now())
	ana := e.store.User("ana@example.com").ID()
	ctx := reqCtx()
	login := func(email, pw string) error {
		_, err := e.svc.Login(ctx, application.LoginInput{Email: email, Password: pw, ClientType: "WEB", IP: &clientAddr})
		return err
	}

	_ = login("ana@example.com", goodPassword)
	a := lastAudit(t, e, application.AuditLogin, application.OutcomeSuccess)
	actorIs(t, a, ana)
	if a.ResourceType != "session" || a.ResourceID == "" || a.Metadata["client_type"] != "WEB" || *a.IP != clientAddr {
		t.Fatalf("success audit: %+v", a)
	}

	cases := []struct {
		email, password string
		outcome         application.AuditOutcome
		reason          string
		hasActor        bool
	}{
		{"ana@example.com", "clave equivocada 123", application.OutcomeFailure, "wrong_password", true},
		{"nadie@example.com", goodPassword, application.OutcomeFailure, "unknown_account", false},
		{"pendiente@example.com", goodPassword, application.OutcomeDenied, "email_not_verified", true},
		{"bob@example.com", goodPassword, application.OutcomeFailure, "account_suspended", true},
	}
	for _, c := range cases {
		_ = login(c.email, c.password)
		a := lastAudit(t, e, application.AuditLogin, c.outcome)
		if a.Metadata["reason"] != c.reason || (a.ActorUserID != nil) != c.hasActor {
			t.Errorf("%s: %+v", c.email, a)
		}
		// Without an actor, a fingerprint (never the email) allows correlating attempts.
		if !c.hasActor && len(a.Metadata["email_fp"]) != 16 {
			t.Errorf("%s: email_fp = %q", c.email, a.Metadata["email_fp"])
		}
	}
}

func TestAuditThrottledLogin(t *testing.T) {
	e := newThrottledEnv()
	e.verifiedUser(t, "ana@example.com")
	for range 3 {
		_ = e.loginFrom("198.51.100.7", "ana@example.com", "clave equivocada 123")
	}
	_ = e.loginFrom("198.51.100.7", "ana@example.com", goodPassword)
	a := lastAudit(t, e, application.AuditLogin, application.OutcomeDenied)
	if a.Metadata["reason"] != "throttled" || a.IP == nil || a.IP.String() != "198.51.100.7" {
		t.Fatalf("throttled audit: %+v", a)
	}
}

func TestAuditSessionsAndPasswords(t *testing.T) {
	e := newEnv()
	e.verifiedUser(t, "ana@example.com")
	ana := e.store.User("ana@example.com").ID()
	ctx := reqCtx()
	res, _ := e.login("ana@example.com", goodPassword, "MOBILE")
	p, _ := e.svc.Authenticate(ctx, res.Token.Reveal())

	change := func(current string) error {
		return e.svc.ChangePassword(ctx, p, application.ChangePasswordInput{CurrentPassword: current, NewPassword: newPassword})
	}
	_ = change("no es la actual 123")
	a := lastAudit(t, e, application.AuditPasswordChanged, application.OutcomeFailure)
	actorIs(t, a, ana)
	if err := change(goodPassword); err != nil {
		t.Fatal(err)
	}
	a = lastAudit(t, e, application.AuditPasswordChanged, application.OutcomeSuccess)
	if a.Metadata["other_sessions_revoked"] != "0" || a.ResourceID != p.SessionID.String() {
		t.Fatalf("change audit: %+v", a)
	}

	_ = e.svc.ForgotPassword(ctx, "nadie@example.com")
	if a := lastAudit(t, e, application.AuditPasswordResetRequested, application.OutcomeFailure); a.Metadata["reason"] != "unknown_account" || a.ActorUserID != nil {
		t.Fatalf("forgot unknown: %+v", a)
	}
	_ = e.svc.ForgotPassword(ctx, "ana@example.com")
	actorIs(t, lastAudit(t, e, application.AuditPasswordResetRequested, application.OutcomeSuccess), ana)
	raw := e.resetToken(t)
	_ = e.svc.ResetPassword(ctx, application.ResetPasswordInput{Token: "bp_pr_" + strings.Repeat("B", 43), NewPassword: "una tercera clave larguisima"})
	lastAudit(t, e, application.AuditPasswordReset, application.OutcomeFailure)
	if err := e.svc.ResetPassword(ctx, application.ResetPasswordInput{Token: raw, NewPassword: "una tercera clave larguisima"}); err != nil {
		t.Fatal(err)
	}
	if a := lastAudit(t, e, application.AuditPasswordReset, application.OutcomeSuccess); a.Metadata["sessions_revoked"] != "1" {
		t.Fatalf("reset audit: %+v", a)
	}

	res, _ = e.login("ana@example.com", "una tercera clave larguisima", "MOBILE")
	p, _ = e.svc.Authenticate(ctx, res.Token.Reveal())
	_, _ = e.login("ana@example.com", "una tercera clave larguisima", "WEB")
	if err := e.svc.Logout(ctx, p); err != nil {
		t.Fatal(err)
	}
	if a := lastAudit(t, e, application.AuditLogout, application.OutcomeSuccess); a.ResourceID != p.SessionID.String() {
		t.Fatalf("logout audit: %+v", a)
	}
	res, _ = e.login("ana@example.com", "una tercera clave larguisima", "MOBILE")
	p, _ = e.svc.Authenticate(ctx, res.Token.Reveal())
	n, _ := e.svc.LogoutAll(ctx, p)
	if a := lastAudit(t, e, application.AuditLogoutAll, application.OutcomeSuccess); a.Metadata["sessions_revoked"] != "2" || n != 2 {
		t.Fatalf("logout-all audit: %+v (n=%d)", a, n)
	}
}

func TestAuditAuthorizationDenials(t *testing.T) {
	e := newEnv()
	ctx := reqCtx()
	user, biz, other := uuid.New(), uuid.New(), uuid.New()
	e.store.GrantMembership(user, biz, contract.RoleProfessional)
	p := contract.Principal{UserID: user}

	before := len(e.store.Audit)
	if _, err := e.svc.RequireBusinessRole(ctx, p, biz, contract.RolesStaff...); err != nil {
		t.Fatal(err)
	}
	if len(e.store.Audit) != before {
		t.Fatal("allowed access must not be audited (volume)")
	}
	_, _ = e.svc.RequireBusinessRole(ctx, p, biz, contract.RolesOwner...)
	a := lastAudit(t, e, application.AuditAuthzDenied, application.OutcomeDenied)
	if a.Metadata["reason"] != "insufficient_role" || a.Metadata["required_roles"] != "OWNER" || a.BusinessID == nil || *a.BusinessID != biz {
		t.Fatalf("forbidden audit: %+v", a)
	}
	_, _ = e.svc.RequireBusinessRole(ctx, p, other, contract.RolesManagement...)
	a = lastAudit(t, e, application.AuditAuthzDenied, application.OutcomeDenied)
	if a.Metadata["reason"] != "no_membership" || *a.BusinessID != other || a.Metadata["required_roles"] != "OWNER,MANAGER" {
		t.Fatalf("cross-tenant audit: %+v", a)
	}
	_ = e.svc.RequirePlatformAdmin(ctx, p)
	if a := lastAudit(t, e, application.AuditAuthzDenied, application.OutcomeDenied); a.Metadata["required_roles"] != "PLATFORM_ADMIN" {
		t.Fatalf("admin audit: %+v", a)
	}
}

// TestAuditIsAtomicWithTheChange: if the audit row cannot be written, a state-changing
// operation (login → new session) fails as a whole; a detached event (failed login) does not
// change the response.
func TestAuditIsAtomicWithTheChange(t *testing.T) {
	e := newEnv()
	e.verifiedUser(t, "ana@example.com")
	ana := e.store.User("ana@example.com").ID()
	e.store.FailAudit = errors.New("audit table unavailable")

	if _, err := e.login("ana@example.com", goodPassword, "MOBILE"); err == nil {
		t.Fatal("login succeeded without its audit row")
	}
	if e.store.SessionCount(ana) != 0 {
		t.Fatal("session committed without its audit row")
	}
	if _, err := e.login("ana@example.com", "clave equivocada 123", "MOBILE"); !errors.Is(err, application.ErrInvalidCredentials) {
		t.Fatalf("failed login with audit outage: %v", err)
	}
}

// TestAuditNeverStoresSecrets scans every entry produced by a full flow.
func TestAuditNeverStoresSecrets(t *testing.T) {
	e := newEnv()
	ctx := reqCtx()
	_ = e.svc.Register(ctx, application.RegisterInput{Email: "ana@example.com", Password: goodPassword, FullName: "Ana Pérez"})
	verifyTok := e.sendAndExtract(t)
	_ = e.svc.VerifyEmail(ctx, verifyTok)
	res, _ := e.svc.Login(ctx, application.LoginInput{Email: "ana@example.com", Password: goodPassword, ClientType: "MOBILE", UserAgent: "Mozilla/5.0 secreto-ua"})
	_, _ = e.svc.Login(ctx, application.LoginInput{Email: "nadie@example.com", Password: "otra clave de prueba", ClientType: "MOBILE"})
	_ = e.svc.ForgotPassword(ctx, "ana@example.com")
	resetTok := e.resetToken(t)
	_ = e.svc.ResetPassword(ctx, application.ResetPasswordInput{Token: resetTok, NewPassword: newPassword})

	blob, _ := json.Marshal(e.store.Audit)
	for _, secret := range []string{
		goodPassword, newPassword, "otra clave de prueba", verifyTok, resetTok, res.Token.Reveal(),
		"ana@example.com", "nadie@example.com", "Ana Pérez", "secreto-ua",
	} {
		if strings.Contains(string(blob), secret) {
			t.Errorf("audit contains %q", secret)
		}
	}
	want := []string{"auth.registered:SUCCESS", "auth.email_verified:SUCCESS", "auth.login:SUCCESS", "auth.login:FAILURE",
		"auth.password_reset_requested:SUCCESS", "auth.password_reset:SUCCESS"}
	if got := e.store.AuditActions(); !slices.Equal(got, want) {
		t.Errorf("events = %v, want %v", got, want)
	}
}
