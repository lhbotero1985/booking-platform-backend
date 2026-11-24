//go:build integration

package postgres_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/netip"
	"net/url"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application/apptest"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/infrastructure/postgres"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/jobs"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/migrate"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/pgtest"
)

var pool *pgxpool.Pool

func TestMain(m *testing.M) {
	ctx := context.Background()
	c, err := pgtest.Start(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := func() int {
		defer c.Terminate()
		p, err := pgxpool.New(ctx, c.URL)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		defer p.Close()
		sqlDB := stdlib.OpenDBFromPool(p)
		defer sqlDB.Close()
		mg, err := migrate.New(sqlDB)
		if err == nil {
			_, err = mg.Up(ctx)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		pool = p
		return m.Run()
	}()
	os.Exit(code)
}

type env struct {
	svc    *application.Service
	mailer *apptest.Mailer
	clock  *clock.Fixed
}

func newEnv(t *testing.T) env {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	inserter, err := jobs.NewInsertClient(pool, logger)
	if err != nil {
		t.Fatal(err)
	}
	e := env{mailer: &apptest.Mailer{}, clock: clock.NewFixed(time.Now().UTC().Truncate(time.Microsecond))}
	e.svc = application.NewService(application.Deps{
		Store: postgres.NewStore(pool, inserter), Hasher: &apptest.Hasher{}, Mailer: e.mailer,
		Clock: e.clock, Config: application.Config{PublicAppURL: "https://app.example.test"}, Logger: logger,
	})
	return e
}

func uniqueEmail() string { return "user-" + uuid.NewString()[:8] + "@example.test" }

func userRow(t *testing.T, email string) (id uuid.UUID, status string, verifiedAt *time.Time) {
	t.Helper()
	err := pool.QueryRow(t.Context(), `SELECT id, status, email_verified_at FROM users WHERE email = $1`, email).Scan(&id, &status, &verifiedAt)
	if err != nil {
		t.Fatalf("user %s: %v", email, err)
	}
	return
}

func pendingJobs(t *testing.T, userID uuid.UUID) int {
	t.Helper()
	var n int
	err := pool.QueryRow(t.Context(), `SELECT count(*) FROM river_job WHERE kind = 'identity.send_verification_email' AND args->>'user_id' = $1`, userID.String()).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

var tokenRe = regexp.MustCompile(`token=([^\s]+)`)

func TestRegistrationAndVerificationOnPostgres(t *testing.T) {
	ctx := t.Context()
	e := newEnv(t)
	email := uniqueEmail()

	if err := e.svc.Register(ctx, application.RegisterInput{Email: email, Password: "una clave suficientemente larga", FullName: "Ana Pérez", Phone: "3001234567"}); err != nil {
		t.Fatal(err)
	}
	id, status, verified := userRow(t, email)
	if status != "PENDING_VERIFICATION" || verified != nil {
		t.Fatalf("after register: status %s verified %v", status, verified)
	}
	if n := pendingJobs(t, id); n != 1 {
		t.Fatalf("verification jobs = %d, want 1 (enqueued in the same transaction)", n)
	}
	// Resend within the same minute is coalesced by River's unique opts.
	if err := e.svc.ResendVerification(ctx, email); err != nil {
		t.Fatal(err)
	}
	if n := pendingJobs(t, id); n != 1 {
		t.Fatalf("jobs after immediate resend = %d, want 1 (coalesced)", n)
	}
	// The job carries only the user ID, never a token.
	var args []byte
	if err := pool.QueryRow(ctx, `SELECT args FROM river_job WHERE kind = 'identity.send_verification_email' AND args->>'user_id' = $1`, id.String()).Scan(&args); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(args, []byte("bp_ev_")) || bytes.Contains(args, []byte("token")) {
		t.Fatalf("job args contain a token: %s", args)
	}

	// Worker step.
	if err := e.svc.SendVerificationEmail(ctx, id); err != nil {
		t.Fatal(err)
	}
	m := tokenRe.FindStringSubmatch(e.mailer.Last().Body)
	raw, _ := url.QueryUnescape(m[1])
	var stored int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM email_verification_tokens WHERE user_id = $1 AND token_hash = $2`, id, []byte(raw)).Scan(&stored); err != nil || stored != 0 {
		t.Fatalf("raw token stored in DB (count %d, err %v)", stored, err)
	}
	tok, _ := domain.ParseToken(domain.TokenPrefixEmailVerification, raw)
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM email_verification_tokens WHERE user_id = $1 AND token_hash = $2 AND used_at IS NULL`, id, tok.Hash()).Scan(&stored); err != nil || stored != 1 {
		t.Fatalf("token hash not stored (count %d, err %v)", stored, err)
	}

	if err := e.svc.VerifyEmail(ctx, raw); err != nil {
		t.Fatal(err)
	}
	_, status, verified = userRow(t, email)
	if status != "ACTIVE" || verified == nil {
		t.Fatalf("after verify: status %s verified %v", status, verified)
	}
	if err := e.svc.VerifyEmail(ctx, raw); err == nil {
		t.Fatal("token reused")
	}
}

func TestDuplicateRegistrationIsSilent(t *testing.T) {
	ctx := t.Context()
	e := newEnv(t)
	email := uniqueEmail()
	in := application.RegisterInput{Email: email, Password: "una clave suficientemente larga", FullName: "Ana", Phone: "3001234567"}
	if err := e.svc.Register(ctx, in); err != nil {
		t.Fatal(err)
	}
	in.Email = "  " + email + "  "
	if err := e.svc.Register(ctx, in); err != nil {
		t.Fatalf("duplicate registration must look successful: %v", err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE email = $1`, email).Scan(&n); err != nil || n != 1 {
		t.Fatalf("users = %d, %v", n, err)
	}
}

func TestExpiredTokenRejectedOnPostgres(t *testing.T) {
	ctx := t.Context()
	e := newEnv(t)
	email := uniqueEmail()
	if err := e.svc.Register(ctx, application.RegisterInput{Email: email, Password: "una clave suficientemente larga", FullName: "Ana", Phone: "3001234567"}); err != nil {
		t.Fatal(err)
	}
	id, _, _ := userRow(t, email)
	if err := e.svc.SendVerificationEmail(ctx, id); err != nil {
		t.Fatal(err)
	}
	raw, _ := url.QueryUnescape(tokenRe.FindStringSubmatch(e.mailer.Last().Body)[1])
	e.clock.Advance(application.VerificationTokenTTL + time.Minute)
	if err := e.svc.VerifyEmail(ctx, raw); err == nil {
		t.Fatal("expired token accepted")
	}
	if _, status, _ := userRow(t, email); status != "PENDING_VERIFICATION" {
		t.Fatalf("status = %s", status)
	}
}

// activeUser registers and verifies a fresh account and returns its email.
func (e env) activeUser(t *testing.T) string {
	t.Helper()
	ctx := t.Context()
	email := uniqueEmail()
	if err := e.svc.Register(ctx, application.RegisterInput{Email: email, Password: "una clave suficientemente larga", FullName: "Ana", Phone: "3001234567"}); err != nil {
		t.Fatal(err)
	}
	id, _, _ := userRow(t, email)
	if err := e.svc.SendVerificationEmail(ctx, id); err != nil {
		t.Fatal(err)
	}
	raw, _ := url.QueryUnescape(tokenRe.FindStringSubmatch(e.mailer.Last().Body)[1])
	if err := e.svc.VerifyEmail(ctx, raw); err != nil {
		t.Fatal(err)
	}
	return email
}

func TestSessionsOnPostgres(t *testing.T) {
	ctx := t.Context()
	e := newEnv(t)
	email := e.activeUser(t)
	ip := netip.MustParseAddr("203.0.113.7")

	res, err := e.svc.Login(ctx, application.LoginInput{Email: email, Password: "una clave suficientemente larga", ClientType: "WEB", IP: &ip, UserAgent: "Mozilla/5.0 test"})
	if err != nil {
		t.Fatal(err)
	}
	raw := res.Token.Reveal()

	var storedIP netip.Addr
	var ua string
	var lastSeen time.Time
	err = pool.QueryRow(ctx, `SELECT ip, user_agent, last_seen_at FROM sessions WHERE token_hash = $1`, res.Token.Hash()).Scan(&storedIP, &ua, &lastSeen)
	if err != nil || storedIP != ip || ua != "Mozilla/5.0 test" {
		t.Fatalf("session row: ip %v ua %q err %v", storedIP, ua, err)
	}
	var rawStored int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE token_hash = $1`, []byte(raw)).Scan(&rawStored); err != nil || rawStored != 0 {
		t.Fatal("raw session token stored")
	}

	p, err := e.svc.Authenticate(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	// Throttled touch: no write within 5 minutes, write after.
	e.clock.Advance(time.Minute)
	if _, err := e.svc.Authenticate(ctx, raw); err != nil {
		t.Fatal(err)
	}
	var seen2 time.Time
	_ = pool.QueryRow(ctx, `SELECT last_seen_at FROM sessions WHERE id = $1`, p.SessionID).Scan(&seen2)
	if !seen2.Equal(lastSeen) {
		t.Fatalf("last_seen written within the throttle window: %v -> %v", lastSeen, seen2)
	}
	e.clock.Advance(10 * time.Minute)
	if _, err := e.svc.Authenticate(ctx, raw); err != nil {
		t.Fatal(err)
	}
	_ = pool.QueryRow(ctx, `SELECT last_seen_at FROM sessions WHERE id = $1`, p.SessionID).Scan(&seen2)
	if !seen2.Equal(e.clock.Now()) {
		t.Fatalf("last_seen not updated after the throttle window: %v vs %v", seen2, e.clock.Now())
	}

	// Suspension blocks the session immediately.
	if _, err := pool.Exec(ctx, `UPDATE users SET status='SUSPENDED' WHERE email=$1`, email); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Authenticate(ctx, raw); !errors.Is(err, application.ErrUnauthenticated) {
		t.Fatalf("suspended user authenticated: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET status='ACTIVE' WHERE email=$1`, email); err != nil {
		t.Fatal(err)
	}

	// Logout-all revokes every session.
	other, _ := e.svc.Login(ctx, application.LoginInput{Email: email, Password: "una clave suficientemente larga", ClientType: "MOBILE"})
	n, err := e.svc.LogoutAll(ctx, p)
	if err != nil || n != 2 {
		t.Fatalf("LogoutAll = %d, %v", n, err)
	}
	for _, tok := range []string{raw, other.Token.Reveal()} {
		if _, err := e.svc.Authenticate(ctx, tok); !errors.Is(err, application.ErrUnauthenticated) {
			t.Fatal("session survived logout-all")
		}
	}
}

func TestSessionExpiryOnPostgres(t *testing.T) {
	ctx := t.Context()
	e := newEnv(t)
	email := e.activeUser(t)
	res, err := e.svc.Login(ctx, application.LoginInput{Email: email, Password: "una clave suficientemente larga", ClientType: "WEB"})
	if err != nil {
		t.Fatal(err)
	}
	e.clock.Advance(12*time.Hour + time.Second) // web idle expiry
	if _, err := e.svc.Authenticate(ctx, res.Token.Reveal()); !errors.Is(err, application.ErrUnauthenticated) {
		t.Fatalf("idle-expired session accepted: %v", err)
	}
}

func jobCount(t *testing.T, kind string, userID uuid.UUID) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM river_job WHERE kind = $1 AND args->>'user_id' = $2`, kind, userID.String()).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestPasswordResetOnPostgres(t *testing.T) {
	ctx := t.Context()
	e := newEnv(t)
	email := e.activeUser(t)
	id, _, _ := userRow(t, email)
	login, err := e.svc.Login(ctx, application.LoginInput{Email: email, Password: "una clave suficientemente larga", ClientType: "MOBILE"})
	if err != nil {
		t.Fatal(err)
	}

	if err := e.svc.ForgotPassword(ctx, email); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.ForgotPassword(ctx, email); err != nil {
		t.Fatal(err)
	}
	if n := jobCount(t, "identity.send_password_reset_email", id); n != 1 {
		t.Fatalf("reset jobs = %d, want 1 (coalesced)", n)
	}
	if err := e.svc.SendPasswordResetEmail(ctx, id); err != nil {
		t.Fatal(err)
	}
	raw, _ := url.QueryUnescape(regexp.MustCompile(`reset-password\?token=([^\s]+)`).FindStringSubmatch(e.mailer.Last().Body)[1])
	tok, err := domain.ParseToken(domain.TokenPrefixPasswordReset, raw)
	if err != nil {
		t.Fatal(err)
	}
	var stored int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM password_reset_tokens WHERE user_id=$1 AND token_hash=$2 AND used_at IS NULL AND expires_at <= created_at + interval '30 minutes'`, id, tok.Hash()).Scan(&stored); err != nil || stored != 1 {
		t.Fatalf("reset token hash not stored with 30 min expiry (count %d, err %v)", stored, err)
	}

	var before string
	_ = pool.QueryRow(ctx, `SELECT password_hash FROM users WHERE id=$1`, id).Scan(&before)
	if err := e.svc.ResetPassword(ctx, application.ResetPasswordInput{Token: raw, NewPassword: "otra frase nueva bastante larga"}); err != nil {
		t.Fatal(err)
	}
	var after string
	_ = pool.QueryRow(ctx, `SELECT password_hash FROM users WHERE id=$1`, id).Scan(&after)
	if before == after {
		t.Fatal("password hash unchanged")
	}
	if _, err := e.svc.Authenticate(ctx, login.Token.Reveal()); !errors.Is(err, application.ErrUnauthenticated) {
		t.Fatal("session survived reset")
	}
	if n := jobCount(t, "identity.send_password_changed_email", id); n != 1 {
		t.Fatalf("password changed notices = %d", n)
	}
	if err := e.svc.ResetPassword(ctx, application.ResetPasswordInput{Token: raw, NewPassword: "una tercera clave larguisima"}); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("reset token reused: %v", err)
	}
}

func TestChangePasswordOnPostgres(t *testing.T) {
	ctx := t.Context()
	e := newEnv(t)
	email := e.activeUser(t)
	cur, _ := e.svc.Login(ctx, application.LoginInput{Email: email, Password: "una clave suficientemente larga", ClientType: "WEB"})
	other, _ := e.svc.Login(ctx, application.LoginInput{Email: email, Password: "una clave suficientemente larga", ClientType: "MOBILE"})
	p, err := e.svc.Authenticate(ctx, cur.Token.Reveal())
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.ChangePassword(ctx, p, application.ChangePasswordInput{CurrentPassword: "una clave suficientemente larga", NewPassword: "otra frase nueva bastante larga"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Authenticate(ctx, cur.Token.Reveal()); err != nil {
		t.Fatal("current session revoked")
	}
	if _, err := e.svc.Authenticate(ctx, other.Token.Reveal()); !errors.Is(err, application.ErrUnauthenticated) {
		t.Fatal("other session survived")
	}
}
