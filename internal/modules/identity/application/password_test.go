package application_test

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/domain"
)

const newPassword = "otra frase nueva bastante larga"

var resetLink = regexp.MustCompile(`reset-password\?token=([^\s]+)`)

// resetToken runs the worker step for the last reset request and returns the raw token.
func (e env) resetToken(t *testing.T) string {
	t.Helper()
	id := e.store.ResetEnqueued[len(e.store.ResetEnqueued)-1]
	if err := e.svc.SendPasswordResetEmail(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	m := resetLink.FindStringSubmatch(e.mailer.Last().Body)
	if m == nil {
		t.Fatalf("no reset link in %q", e.mailer.Last().Body)
	}
	raw, _ := url.QueryUnescape(m[1])
	return raw
}

func TestForgotPasswordIsSilentAndSelective(t *testing.T) {
	e := newEnv()
	e.verifiedUser(t, "ana@example.com")
	e.register(t, "pending@example.com")
	e.verifiedUser(t, "bob@example.com")
	e.store.User("bob@example.com").Suspend(e.clock.Now())

	for _, email := range []string{"nobody@example.com", "pending@example.com", "bob@example.com", "ANA@example.com"} {
		if err := e.svc.ForgotPassword(context.Background(), email); err != nil {
			t.Fatalf("ForgotPassword(%s) = %v; must always succeed", email, err)
		}
	}
	if len(e.store.ResetEnqueued) != 1 || e.store.ResetEnqueued[0] != e.store.User("ana@example.com").ID() {
		t.Fatalf("reset emails only for active accounts; enqueued %v", e.store.ResetEnqueued)
	}
	var verr *application.ValidationError
	if err := e.svc.ForgotPassword(context.Background(), "not-an-email"); !errors.As(err, &verr) {
		t.Fatalf("malformed email: %v", err)
	}
}

func TestResetPasswordFlow(t *testing.T) {
	e := newEnv()
	e.verifiedUser(t, "ana@example.com")
	a, _ := e.login("ana@example.com", goodPassword, "MOBILE")
	b, _ := e.login("ana@example.com", goodPassword, "WEB")

	_ = e.svc.ForgotPassword(context.Background(), "ana@example.com")
	raw := e.resetToken(t)
	mail := e.mailer.Last()
	if !strings.HasPrefix(raw, domain.TokenPrefixPasswordReset) || !strings.Contains(mail.Body, "https://app.example.test/reset-password?token=") || !strings.Contains(mail.Body, "30 minutos") {
		t.Fatalf("reset email: %+v", mail)
	}
	if strings.Contains(mail.Body, "nolint") || strings.Contains(mail.Body, "// G") {
		t.Fatalf("lint directive leaked into the email body: %q", mail.Body)
	}

	if err := e.svc.ResetPassword(context.Background(), application.ResetPasswordInput{Token: raw, NewPassword: newPassword}); err != nil {
		t.Fatal(err)
	}
	// Every session revoked.
	for _, r := range []application.LoginResult{a, b} {
		if _, err := e.svc.Authenticate(context.Background(), r.Token.Reveal()); !errors.Is(err, application.ErrUnauthenticated) {
			t.Fatal("session survived password reset")
		}
	}
	// Old password no longer works, new one does.
	if _, err := e.login("ana@example.com", goodPassword, "WEB"); !errors.Is(err, application.ErrInvalidCredentials) {
		t.Fatalf("old password still valid: %v", err)
	}
	if _, err := e.login("ana@example.com", newPassword, "WEB"); err != nil {
		t.Fatalf("new password rejected: %v", err)
	}
	// Single use and "password changed" notice.
	if err := e.svc.ResetPassword(context.Background(), application.ResetPasswordInput{Token: raw, NewPassword: "una tercera clave larga"}); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("token reused: %v", err)
	}
	if len(e.store.ChangedEnqueued) != 1 {
		t.Fatalf("password changed notices = %d", len(e.store.ChangedEnqueued))
	}
	if err := e.svc.SendPasswordChangedEmail(context.Background(), e.store.ChangedEnqueued[0]); err != nil {
		t.Fatal(err)
	}
	notice := e.mailer.Last()
	if notice.Subject != "Tu contraseña fue cambiada" || strings.Contains(notice.Body, "token=") {
		t.Fatalf("notice: %+v", notice)
	}
}

func TestResetPasswordPolicyDoesNotBurnToken(t *testing.T) {
	e := newEnv()
	e.verifiedUser(t, "ana@example.com")
	_ = e.svc.ForgotPassword(context.Background(), "ana@example.com")
	raw := e.resetToken(t)

	var verr *application.ValidationError
	if err := e.svc.ResetPassword(context.Background(), application.ResetPasswordInput{Token: raw, NewPassword: "corta"}); !errors.As(err, &verr) || verr.Fields[0].Code != application.CodePasswordTooShort {
		t.Fatalf("weak password: %v", err)
	}
	if err := e.svc.ResetPassword(context.Background(), application.ResetPasswordInput{Token: raw, NewPassword: newPassword}); err != nil {
		t.Fatalf("token must still be usable after a policy rejection: %v", err)
	}
}

func TestResetTokenExpiryRotationAndGarbage(t *testing.T) {
	e := newEnv()
	e.verifiedUser(t, "ana@example.com")
	_ = e.svc.ForgotPassword(context.Background(), "ana@example.com")
	first := e.resetToken(t)
	_ = e.svc.ForgotPassword(context.Background(), "ana@example.com")
	second := e.resetToken(t)

	if err := e.svc.ResetPassword(context.Background(), application.ResetPasswordInput{Token: first, NewPassword: newPassword}); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("superseded token accepted: %v", err)
	}
	if n := e.store.ActiveResetTokenCount(e.store.User("ana@example.com").ID()); n != 1 {
		t.Fatalf("active reset tokens = %d", n)
	}
	e.clock.Advance(application.PasswordResetTokenTTL + time.Second)
	if err := e.svc.ResetPassword(context.Background(), application.ResetPasswordInput{Token: second, NewPassword: newPassword}); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("expired token accepted: %v", err)
	}
	// A verification token cannot be used as a reset token.
	for _, tok := range []string{"", "x", domain.TokenPrefixEmailVerification + strings.Repeat("A", 43)} {
		if err := e.svc.ResetPassword(context.Background(), application.ResetPasswordInput{Token: tok, NewPassword: newPassword}); !errors.Is(err, domain.ErrInvalidToken) {
			t.Errorf("token %q: %v", tok, err)
		}
	}
}

func TestResetPasswordRejectedForSuspendedAccount(t *testing.T) {
	e := newEnv()
	e.verifiedUser(t, "ana@example.com")
	_ = e.svc.ForgotPassword(context.Background(), "ana@example.com")
	raw := e.resetToken(t)
	e.store.User("ana@example.com").Suspend(e.clock.Now())
	if err := e.svc.ResetPassword(context.Background(), application.ResetPasswordInput{Token: raw, NewPassword: newPassword}); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("suspended account reset: %v", err)
	}
	// And the worker does not mail suspended accounts.
	before := len(e.mailer.Sent)
	if err := e.svc.SendPasswordResetEmail(context.Background(), e.store.User("ana@example.com").ID()); err != nil || len(e.mailer.Sent) != before {
		t.Fatalf("reset email to suspended account: %v", err)
	}
}

func TestChangePassword(t *testing.T) {
	e := newEnv()
	e.verifiedUser(t, "ana@example.com")
	cur, _ := e.login("ana@example.com", goodPassword, "WEB")
	other, _ := e.login("ana@example.com", goodPassword, "MOBILE")
	p, _ := e.svc.Authenticate(context.Background(), cur.Token.Reveal())

	var verr *application.ValidationError
	cases := map[string]application.ChangePasswordInput{
		"wrong current": {CurrentPassword: "no es la actual 123", NewPassword: newPassword},
		"empty current": {CurrentPassword: "", NewPassword: newPassword},
		"weak new":      {CurrentPassword: goodPassword, NewPassword: "corta"},
		"unchanged":     {CurrentPassword: goodPassword, NewPassword: goodPassword},
	}
	for name, in := range cases {
		if err := e.svc.ChangePassword(context.Background(), p, in); !errors.As(err, &verr) {
			t.Errorf("%s: %v", name, err)
		}
	}

	if err := e.svc.ChangePassword(context.Background(), p, application.ChangePasswordInput{CurrentPassword: goodPassword, NewPassword: newPassword}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Authenticate(context.Background(), cur.Token.Reveal()); err != nil {
		t.Fatal("current session must survive a password change")
	}
	if _, err := e.svc.Authenticate(context.Background(), other.Token.Reveal()); !errors.Is(err, application.ErrUnauthenticated) {
		t.Fatal("other sessions must be revoked")
	}
	if _, err := e.login("ana@example.com", newPassword, "WEB"); err != nil {
		t.Fatal("new password rejected")
	}
	if len(e.store.ChangedEnqueued) != 1 {
		t.Fatal("password changed notice not scheduled")
	}
}
