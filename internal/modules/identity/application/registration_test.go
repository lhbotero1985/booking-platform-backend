package application_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application/apptest"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
)

type env struct {
	svc    *application.Service
	store  *apptest.Store
	hasher *apptest.Hasher
	mailer *apptest.Mailer
	clock  *clock.Fixed
}

func newEnv() env {
	e := env{
		store: apptest.NewStore(), hasher: &apptest.Hasher{}, mailer: &apptest.Mailer{},
		clock: clock.NewFixed(time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)),
	}
	e.svc = application.NewService(application.Deps{
		Store: e.store, Hasher: e.hasher, Mailer: e.mailer, Clock: e.clock,
		Random: &apptest.FixedRandom{}, Config: application.Config{PublicAppURL: "https://app.example.test/"},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	return e
}

const goodPassword = "una clave suficientemente larga"

func (e env) register(t *testing.T, email string) {
	t.Helper()
	if err := e.svc.Register(context.Background(), application.RegisterInput{Email: email, Password: goodPassword, FullName: "Ana Pérez"}); err != nil {
		t.Fatalf("Register(%s): %v", email, err)
	}
}

var linkToken = regexp.MustCompile(`token=([^\s]+)`)

// sendAndExtract runs the worker step for the last enqueued user and returns the raw token
// from the email link.
func (e env) sendAndExtract(t *testing.T) string {
	t.Helper()
	id := e.store.Enqueued[len(e.store.Enqueued)-1]
	if err := e.svc.SendVerificationEmail(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	m := linkToken.FindStringSubmatch(e.mailer.Last().Body)
	if m == nil {
		t.Fatalf("no link in email: %q", e.mailer.Last().Body)
	}
	raw, err := url.QueryUnescape(m[1])
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestRegisterCreatesPendingUserAndSchedulesEmail(t *testing.T) {
	e := newEnv()
	e.register(t, "  Ana@Example.com ")

	u := e.store.User("ana@example.com")
	if u == nil || u.Status() != domain.UserPendingVerification || u.FullName() != "Ana Pérez" {
		t.Fatalf("user = %+v", u)
	}
	if strings.Contains(string(u.PasswordHash()), goodPassword) {
		t.Fatal("password stored in clear")
	}
	if len(e.store.Enqueued) != 1 || e.store.Enqueued[0] != u.ID() {
		t.Fatalf("enqueued = %v", e.store.Enqueued)
	}
	if len(e.mailer.Sent) != 0 {
		t.Fatal("email must be sent by the worker, not inline")
	}
}

func TestRegisterValidation(t *testing.T) {
	e := newEnv()
	err := e.svc.Register(context.Background(), application.RegisterInput{Email: "not-an-email", Password: "short", FullName: "  "})
	var verr *application.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("err = %v, want ValidationError", err)
	}
	got := map[string]string{}
	for _, f := range verr.Fields {
		got[f.Field] = f.Code
	}
	want := map[string]string{"email": application.CodeInvalidEmail, "password": application.CodePasswordTooShort, "fullName": application.CodeRequired}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("field %s = %q, want %q (all: %v)", k, got[k], v, got)
		}
	}
	if e.hasher.HashCalls != 0 || e.store.UserCount() != 0 {
		t.Error("invalid input must not hash or store anything")
	}

	err = e.svc.Register(context.Background(), application.RegisterInput{Email: "a@example.com", Password: "password123", FullName: "Ana\x00"})
	if !errors.As(err, &verr) {
		t.Fatalf("err = %v", err)
	}
	if verr.Fields[0].Code != application.CodePasswordTooCommon {
		t.Errorf("common password code = %v", verr.Fields)
	}
}

// TestRegisterDoesNotRevealExistingAccounts: same result for new and existing emails.
func TestRegisterDoesNotRevealExistingAccounts(t *testing.T) {
	e := newEnv()
	e.register(t, "ana@example.com")
	e.register(t, "ANA@example.com") // pending → re-send, no error
	if e.store.UserCount() != 1 {
		t.Fatalf("users = %d, want 1", e.store.UserCount())
	}
	if len(e.store.Enqueued) != 2 {
		t.Fatalf("pending account must get a new verification email; enqueued %d", len(e.store.Enqueued))
	}
	if e.hasher.HashCalls != 2 {
		t.Fatalf("password must be hashed on both paths (timing): %d", e.hasher.HashCalls)
	}

	raw := e.sendAndExtract(t)
	if err := e.svc.VerifyEmail(context.Background(), raw); err != nil {
		t.Fatal(err)
	}
	e.register(t, "ana@example.com") // verified → silent no-op
	if len(e.store.Enqueued) != 2 {
		t.Fatal("verified account must not receive verification emails")
	}
}

func TestVerificationFlow(t *testing.T) {
	e := newEnv()
	e.register(t, "ana@example.com")
	raw := e.sendAndExtract(t)

	mail := e.mailer.Last()
	if mail.To != "ana@example.com" || !strings.Contains(mail.Body, "https://app.example.test/verify-email?token=") || !strings.HasPrefix(raw, "bp_ev_") {
		t.Fatalf("email = %+v", mail)
	}
	for _, h := range e.store.StoredTokenHashes() {
		if strings.Contains(raw, h) || strings.Contains(h, raw) {
			t.Fatal("raw token stored")
		}
	}

	if err := e.svc.VerifyEmail(context.Background(), raw); err != nil {
		t.Fatal(err)
	}
	u := e.store.User("ana@example.com")
	if u.Status() != domain.UserActive || u.CanAuthenticate() != nil {
		t.Fatalf("status after verify = %s", u.Status())
	}
	// Single use.
	if err := e.svc.VerifyEmail(context.Background(), raw); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("reused token: %v", err)
	}
	// No more emails once verified.
	before := len(e.mailer.Sent)
	if err := e.svc.SendVerificationEmail(context.Background(), u.ID()); err != nil || len(e.mailer.Sent) != before {
		t.Fatalf("email sent to verified user: %v", err)
	}
}

func TestVerificationTokenExpiresAndRotates(t *testing.T) {
	e := newEnv()
	e.register(t, "ana@example.com")
	first := e.sendAndExtract(t)

	// A new email invalidates the previous token.
	if err := e.svc.ResendVerification(context.Background(), "ana@example.com"); err != nil {
		t.Fatal(err)
	}
	second := e.sendAndExtract(t)
	if first == second {
		t.Fatal("tokens must differ")
	}
	if err := e.svc.VerifyEmail(context.Background(), first); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("superseded token accepted: %v", err)
	}
	u := e.store.User("ana@example.com")
	if n := e.store.ActiveTokenCount(u.ID()); n != 1 {
		t.Fatalf("active tokens = %d, want 1", n)
	}

	e.clock.Advance(application.VerificationTokenTTL + time.Second)
	if err := e.svc.VerifyEmail(context.Background(), second); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("expired token accepted: %v", err)
	}
}

func TestVerifyEmailRejectsGarbage(t *testing.T) {
	e := newEnv()
	for _, raw := range []string{"", "abc", "bp_s_" + strings.Repeat("A", 43), "bp_ev_" + strings.Repeat("A", 43), "' OR 1=1 --"} {
		if err := e.svc.VerifyEmail(context.Background(), raw); !errors.Is(err, domain.ErrInvalidToken) {
			t.Errorf("VerifyEmail(%q) = %v", raw, err)
		}
	}
}

func TestSuspendedUserCannotVerify(t *testing.T) {
	e := newEnv()
	e.register(t, "ana@example.com")
	raw := e.sendAndExtract(t)
	e.store.User("ana@example.com").Suspend(e.clock.Now())
	if err := e.svc.VerifyEmail(context.Background(), raw); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("suspended user verified: %v", err)
	}
}

func TestResendVerification(t *testing.T) {
	e := newEnv()
	if err := e.svc.ResendVerification(context.Background(), "nobody@example.com"); err != nil || len(e.store.Enqueued) != 0 {
		t.Fatalf("unknown email: err %v, enqueued %d", err, len(e.store.Enqueued))
	}
	var verr *application.ValidationError
	if err := e.svc.ResendVerification(context.Background(), "bad"); !errors.As(err, &verr) {
		t.Fatalf("malformed email: %v", err)
	}
}

func TestMailerFailureIsRetryable(t *testing.T) {
	e := newEnv()
	e.register(t, "ana@example.com")
	e.mailer.Err = errors.New("smtp down")
	if err := e.svc.SendVerificationEmail(context.Background(), e.store.Enqueued[0]); err == nil {
		t.Fatal("mailer error must be returned so River retries")
	}
	e.mailer.Err = nil
	if raw := e.sendAndExtract(t); raw == "" {
		t.Fatal("retry must issue a usable token")
	}
}

func TestStoreFailureRollsBack(t *testing.T) {
	e := newEnv()
	boom := errors.New("db down")
	e.store.FailNext = boom
	err := e.svc.Register(context.Background(), application.RegisterInput{Email: "ana@example.com", Password: goodPassword, FullName: "Ana"})
	if !errors.Is(err, boom) || e.store.UserCount() != 0 || len(e.store.Enqueued) != 0 {
		t.Fatalf("err %v users %d enqueued %d", err, e.store.UserCount(), len(e.store.Enqueued))
	}
}
