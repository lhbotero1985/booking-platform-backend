//go:build integration

package postgres_test

import (
	"testing"
)

func TestSetPlatformAdminOnPostgres(t *testing.T) {
	e := newEnv(t)
	ctx := t.Context()
	email := e.activeUser(t)
	id, _, _ := userRow(t, email)

	flag := func() (admin bool, audits int) {
		t.Helper()
		if err := pool.QueryRow(ctx, `SELECT is_platform_admin FROM users WHERE id = $1`, id).Scan(&admin); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE resource_id = $1 AND action LIKE 'admin.platform_admin_%'
			AND metadata->>'source' = 'cli' AND actor_user_id IS NULL`, id.String()).Scan(&audits); err != nil {
			t.Fatal(err)
		}
		return
	}
	if err := e.svc.SetPlatformAdmin(ctx, email, true); err != nil {
		t.Fatal(err)
	}
	if admin, n := flag(); !admin || n != 1 {
		t.Fatalf("after grant: admin=%v audits=%d", admin, n)
	}
	if err := e.svc.SetPlatformAdmin(ctx, email, false); err != nil {
		t.Fatal(err)
	}
	if admin, n := flag(); admin || n != 2 {
		t.Fatalf("after revoke: admin=%v audits=%d", admin, n)
	}
}
