//go:build integration

package postgres_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/pgtest"
)

func grant(t *testing.T, userID, businessID uuid.UUID, role contract.Role) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO memberships (id, user_id, business_id, role) VALUES ($1, $2, $3, $4)`,
		id, userID, businessID, string(role)); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestAuthorizationOnPostgres(t *testing.T) {
	e := newEnv(t)
	ctx := t.Context()
	ana, _, _ := userRow(t, e.activeUser(t))
	bob, _, _ := userRow(t, e.activeUser(t))
	bizA, bizB := uuid.New(), uuid.New()
	for _, b := range []uuid.UUID{bizA, bizB} {
		if err := pgtest.InsertBusiness(ctx, pool, b); err != nil {
			t.Fatal(err)
		}
	}
	anaInA := grant(t, ana, bizA, contract.RoleManager)
	grant(t, ana, bizB, contract.RoleProfessional)
	grant(t, bob, bizA, contract.RoleOwner)
	pAna := contract.Principal{UserID: ana}

	ms, err := e.svc.MyMemberships(ctx, pAna)
	if err != nil || len(ms) != 2 {
		t.Fatalf("memberships = %+v, %v", ms, err)
	}
	if r, err := e.svc.RequireBusinessRole(ctx, pAna, bizA, contract.RolesManagement...); err != nil || r != contract.RoleManager {
		t.Errorf("manager in A: %q %v", r, err)
	}
	if _, err := e.svc.RequireBusinessRole(ctx, pAna, bizA, contract.RolesOwner...); !errors.Is(err, contract.ErrForbidden) {
		t.Errorf("owner action in A: %v", err)
	}
	if _, err := e.svc.RequireBusinessRole(ctx, pAna, bizB, contract.RolesManagement...); !errors.Is(err, contract.ErrForbidden) {
		t.Errorf("management action in B as professional: %v", err)
	}
	if _, err := e.svc.RequireBusinessRole(ctx, pAna, uuid.New(), contract.RolesStaff...); !errors.Is(err, contract.ErrNotFound) {
		t.Errorf("unrelated business: %v", err)
	}

	// Revocation (status change, not delete) takes effect on the next call.
	if _, err := pool.Exec(ctx, `UPDATE memberships SET status = 'REVOKED' WHERE id = $1`, anaInA); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.RequireBusinessRole(ctx, pAna, bizA, contract.RolesStaff...); !errors.Is(err, contract.ErrNotFound) {
		t.Errorf("after revoke: %v, want ErrNotFound", err)
	}
	if ms, _ := e.svc.MyMemberships(ctx, pAna); len(ms) != 1 || ms[0].BusinessID != bizB {
		t.Errorf("after revoke: %+v", ms)
	}

	// Platform admin flag comes from the user row at authentication time.
	if _, err := pool.Exec(ctx, `UPDATE users SET is_platform_admin = true WHERE id = $1`, bob); err != nil {
		t.Fatal(err)
	}
	res, err := e.svc.Login(ctx, application.LoginInput{Email: emailOf(t, bob), Password: "una clave suficientemente larga", ClientType: "MOBILE"})
	if err != nil {
		t.Fatal(err)
	}
	pBob, err := e.svc.Authenticate(ctx, res.Token.Reveal())
	if err != nil || e.svc.RequirePlatformAdmin(ctx, pBob) != nil {
		t.Errorf("admin principal: %+v %v", pBob, err)
	}
	if e.svc.RequirePlatformAdmin(ctx, pAna) == nil {
		t.Error("non-admin passed RequirePlatformAdmin")
	}
}

func emailOf(t *testing.T, id uuid.UUID) string {
	t.Helper()
	var email string
	if err := pool.QueryRow(t.Context(), `SELECT email FROM users WHERE id = $1`, id).Scan(&email); err != nil {
		t.Fatal(err)
	}
	return email
}
