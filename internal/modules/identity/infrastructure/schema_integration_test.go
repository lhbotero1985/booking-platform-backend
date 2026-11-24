//go:build integration

package infrastructure_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/infrastructure/sqlcgen"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/db"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/migrate"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/pgtest"
)

const fakeHash = "$argon2id$v=19$m=65536,t=3,p=2$c2FsdA$aGFzaA" // shape only; not a real hash

var (
	pool *pgxpool.Pool
	q    *sqlcgen.Queries
)

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
		pool, q = p, sqlcgen.New(p)
		return m.Run()
	}()
	os.Exit(code)
}

func newUser(t *testing.T, email string) sqlcgen.User {
	t.Helper()
	u, err := q.CreateUser(t.Context(), sqlcgen.CreateUserParams{
		ID: uuid.Must(uuid.NewV7()), Email: email, PasswordHash: fakeHash,
		FullName: "Test User", Status: "ACTIVE", Locale: "es",
	})
	if err != nil {
		t.Fatalf("CreateUser(%s): %v", email, err)
	}
	return u
}

func tokenHash(s string) []byte {
	h := sha256.Sum256([]byte(s))
	return h[:]
}

func uniqueEmail(prefix string) string {
	return fmt.Sprintf("%s-%s@example.test", prefix, uuid.NewString()[:8])
}

func TestUserEmailIsCaseInsensitiveAndUnique(t *testing.T) {
	email := uniqueEmail("Ana")
	u := newUser(t, email)

	got, err := q.GetUserByEmail(t.Context(), strings.ToUpper(email))
	if err != nil || got.ID != u.ID {
		t.Fatalf("lookup by upper-case email: %v (id %v)", err, got.ID)
	}
	_, err = q.CreateUser(t.Context(), sqlcgen.CreateUserParams{
		ID: uuid.Must(uuid.NewV7()), Email: strings.ToUpper(email), PasswordHash: fakeHash,
		FullName: "Dup", Status: "ACTIVE", Locale: "es",
	})
	if !db.IsUniqueViolation(err) {
		t.Fatalf("duplicate email (different case): err = %v, want unique violation", err)
	}
}

func TestUserCheckConstraints(t *testing.T) {
	bad := "+57 300"
	cases := map[string]sqlcgen.CreateUserParams{
		"unknown status":     {Email: uniqueEmail("s"), PasswordHash: fakeHash, FullName: "x", Status: "ROOT", Locale: "es"},
		"non E.164 phone":    {Email: uniqueEmail("p"), Phone: &bad, PasswordHash: fakeHash, FullName: "x", Status: "ACTIVE", Locale: "es"},
		"plaintext password": {Email: uniqueEmail("h"), PasswordHash: "hunter2", FullName: "x", Status: "ACTIVE", Locale: "es"},
		"empty name":         {Email: uniqueEmail("n"), PasswordHash: fakeHash, FullName: "", Status: "ACTIVE", Locale: "es"},
	}
	for name, p := range cases {
		t.Run(name, func(t *testing.T) {
			p.ID = uuid.Must(uuid.NewV7())
			_, err := q.CreateUser(t.Context(), p)
			if db.SQLState(err) != db.CodeCheckViolation {
				t.Fatalf("err = %v (sqlstate %q), want check violation", err, db.SQLState(err))
			}
		})
	}
}

func TestSessionLifecycleQueries(t *testing.T) {
	ctx := t.Context()
	u := newUser(t, uniqueEmail("sess"))
	now := time.Now().UTC().Truncate(time.Microsecond)

	if _, err := q.CreateSession(ctx, sqlcgen.CreateSessionParams{
		ID: uuid.Must(uuid.NewV7()), UserID: u.ID, TokenHash: []byte("too-short"), ClientType: "MOBILE",
		IdleExpiresAt: now.Add(time.Hour), AbsoluteExpiresAt: now.Add(2 * time.Hour),
	}); db.SQLState(err) != db.CodeCheckViolation {
		t.Fatalf("non-SHA-256 token hash accepted: %v", err)
	}

	s, err := q.CreateSession(ctx, sqlcgen.CreateSessionParams{
		ID: uuid.Must(uuid.NewV7()), UserID: u.ID, TokenHash: tokenHash("tok-1"), ClientType: "WEB",
		IdleExpiresAt: now.Add(time.Hour), AbsoluteExpiresAt: now.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}

	row, err := q.GetActiveSessionByTokenHash(ctx, sqlcgen.GetActiveSessionByTokenHashParams{TokenHash: tokenHash("tok-1"), Now: now})
	if err != nil || row.Session.ID != s.ID || row.User.ID != u.ID {
		t.Fatalf("active session lookup: %v", err)
	}
	// Idle expiry reached → not active.
	if _, err := q.GetActiveSessionByTokenHash(ctx, sqlcgen.GetActiveSessionByTokenHashParams{TokenHash: tokenHash("tok-1"), Now: now.Add(61 * time.Minute)}); !db.IsNoRows(err) {
		t.Fatalf("idle-expired session still active: %v", err)
	}

	// Touch cannot extend beyond the absolute expiry.
	if err := q.TouchSession(ctx, sqlcgen.TouchSessionParams{ID: s.ID, Now: now, IdleExpiresAt: now.Add(48 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	var idle, abs time.Time
	if err := pool.QueryRow(ctx, `SELECT idle_expires_at, absolute_expires_at FROM sessions WHERE id=$1`, s.ID).Scan(&idle, &abs); err != nil {
		t.Fatal(err)
	}
	if !idle.Equal(abs) {
		t.Errorf("idle expiry %v exceeds/does not match absolute %v", idle, abs)
	}

	// Revocation.
	n, err := q.RevokeAllUserSessions(ctx, sqlcgen.RevokeAllUserSessionsParams{UserID: u.ID, Now: now})
	if err != nil || n != 1 {
		t.Fatalf("RevokeAllUserSessions = %d, %v", n, err)
	}
	if _, err := q.GetActiveSessionByTokenHash(ctx, sqlcgen.GetActiveSessionByTokenHashParams{TokenHash: tokenHash("tok-1"), Now: now}); !db.IsNoRows(err) {
		t.Fatalf("revoked session still active: %v", err)
	}
}

func TestDeletingUserCascadesSessions(t *testing.T) {
	ctx := t.Context()
	u := newUser(t, uniqueEmail("cascade"))
	now := time.Now().UTC()
	if _, err := q.CreateSession(ctx, sqlcgen.CreateSessionParams{
		ID: uuid.Must(uuid.NewV7()), UserID: u.ID, TokenHash: tokenHash("cascade"), ClientType: "MOBILE",
		IdleExpiresAt: now.Add(time.Hour), AbsoluteExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, u.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE user_id=$1`, u.ID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("sessions after user delete = %d, %v", n, err)
	}
}

// TestMembershipConstraints: several active OWNERs are allowed since migration 00008 (the "at
// least one OWNER" rule lives in the use cases); one membership per user and business; roles
// restricted by CHECK.
func TestMembershipConstraints(t *testing.T) {
	ctx := t.Context()
	business := uuid.Must(uuid.NewV7())
	if err := pgtest.InsertBusiness(ctx, pool, business); err != nil {
		t.Fatal(err)
	}
	owner, other, manager := newUser(t, uniqueEmail("own")), newUser(t, uniqueEmail("own2")), newUser(t, uniqueEmail("mgr"))

	if _, err := q.CreateMembership(ctx, sqlcgen.CreateMembershipParams{ID: uuid.Must(uuid.NewV7()), UserID: owner.ID, BusinessID: business, Role: "OWNER"}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.CreateMembership(ctx, sqlcgen.CreateMembershipParams{ID: uuid.Must(uuid.NewV7()), UserID: other.ID, BusinessID: business, Role: "OWNER"}); err != nil {
		t.Fatalf("second active OWNER rejected: %v", err)
	}
	if _, err := q.CreateMembership(ctx, sqlcgen.CreateMembershipParams{ID: uuid.Must(uuid.NewV7()), UserID: manager.ID, BusinessID: business, Role: "PROFESSIONAL"}); err != nil {
		t.Fatalf("PROFESSIONAL rejected: %v", err)
	}
	if _, err := q.CreateMembership(ctx, sqlcgen.CreateMembershipParams{ID: uuid.Must(uuid.NewV7()), UserID: manager.ID, BusinessID: business, Role: "PROFESSIONAL"}); !db.IsUniqueViolation(err) {
		t.Fatalf("duplicate membership for same user/business accepted: %v", err)
	}
	for _, role := range []string{"SUPERADMIN", "MANAGER"} { // MANAGER removed in migration 00010
		if _, err := q.CreateMembership(ctx, sqlcgen.CreateMembershipParams{ID: uuid.Must(uuid.NewV7()), UserID: other.ID, BusinessID: business, Role: role}); db.SQLState(err) != db.CodeCheckViolation {
			t.Fatalf("role %s accepted: %v", role, err)
		}
	}
	roles, err := q.ListActiveMembershipsByUser(ctx, manager.ID)
	if err != nil || len(roles) != 1 || roles[0].Role != "PROFESSIONAL" {
		t.Fatalf("memberships = %+v, %v", roles, err)
	}
}

func TestAuditEntryValidation(t *testing.T) {
	ctx := t.Context()
	ok := sqlcgen.InsertAuditEntryParams{ID: uuid.Must(uuid.NewV7()), OccurredAt: time.Now(), Action: "auth.login", Outcome: "SUCCESS", Metadata: []byte(`{}`)}
	if err := q.InsertAuditEntry(ctx, ok); err != nil {
		t.Fatal(err)
	}
	bad := sqlcgen.InsertAuditEntryParams{ID: uuid.Must(uuid.NewV7()), OccurredAt: time.Now(), Action: "Auth Login; DROP", Outcome: "SUCCESS", Metadata: []byte(`{}`)}
	if err := q.InsertAuditEntry(ctx, bad); db.SQLState(err) != db.CodeCheckViolation {
		t.Fatalf("malformed action accepted: %v", err)
	}
}
