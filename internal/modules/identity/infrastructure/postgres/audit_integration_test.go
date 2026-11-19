//go:build integration

package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/reqctx"
)

type auditRow struct {
	occurredAt   time.Time
	actor        *uuid.UUID
	business     *uuid.UUID
	action       string
	resourceType *string
	resourceID   *string
	ip           *netip.Addr
	outcome      string
	metadata     map[string]string
}

// auditRows returns the entries carrying requestID, oldest first.
func auditRows(t *testing.T, requestID string) []auditRow {
	t.Helper()
	rows, err := pool.Query(t.Context(), `SELECT occurred_at, actor_user_id, business_id, action, resource_type, resource_id, ip, outcome, metadata
		FROM audit_log WHERE metadata->>'request_id' = $1 ORDER BY occurred_at, id`, requestID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []auditRow
	for rows.Next() {
		var r auditRow
		var meta []byte
		if err := rows.Scan(&r.occurredAt, &r.actor, &r.business, &r.action, &r.resourceType, &r.resourceID, &r.ip, &r.outcome, &meta); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(meta, &r.metadata); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestAuditOnPostgres(t *testing.T) {
	e := newEnv(t)
	email := e.activeUser(t)
	userID, _, _ := userRow(t, email)
	ip := netip.MustParseAddr("2001:db8::7")
	reqID := "audit-" + uuid.NewString()[:8]
	ctx := reqctx.WithClientIP(reqctx.WithRequestID(t.Context(), reqID), ip)

	if _, err := e.svc.Login(ctx, application.LoginInput{Email: email, Password: "clave equivocada 123", ClientType: "MOBILE", IP: &ip}); !errors.Is(err, application.ErrInvalidCredentials) {
		t.Fatal(err)
	}
	res, err := e.svc.Login(ctx, application.LoginInput{Email: email, Password: "una clave suficientemente larga", ClientType: "WEB", IP: &ip})
	if err != nil {
		t.Fatal(err)
	}
	p, err := e.svc.Authenticate(ctx, res.Token.Reveal())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.RequireBusinessRole(ctx, p, uuid.New(), contract.RolesOwner...); !errors.Is(err, contract.ErrNotFound) {
		t.Fatal(err)
	}
	if err := e.svc.Logout(ctx, p); err != nil {
		t.Fatal(err)
	}

	rows := auditRows(t, reqID)
	want := []struct{ action, outcome string }{
		{"auth.login", "FAILURE"}, {"auth.login", "SUCCESS"}, {"authz.denied", "DENIED"}, {"auth.logout", "SUCCESS"},
	}
	if len(rows) != len(want) {
		t.Fatalf("rows = %+v", rows)
	}
	for i, w := range want {
		r := rows[i]
		if r.action != w.action || r.outcome != w.outcome || r.actor == nil || *r.actor != userID {
			t.Errorf("row %d = %s:%s actor %v, want %s:%s", i, r.action, r.outcome, r.actor, w.action, w.outcome)
		}
		if r.ip == nil || *r.ip != ip {
			t.Errorf("row %d ip = %v", i, r.ip)
		}
		if !r.occurredAt.Equal(e.clock.Now()) {
			t.Errorf("row %d occurred_at = %v, want application clock %v", i, r.occurredAt, e.clock.Now())
		}
	}
	if rows[0].metadata["reason"] != "wrong_password" {
		t.Errorf("failure reason: %v", rows[0].metadata)
	}
	if rows[1].resourceType == nil || *rows[1].resourceType != "session" || *rows[1].resourceID != p.SessionID.String() || rows[1].metadata["client_type"] != "WEB" {
		t.Errorf("login success row: %+v", rows[1])
	}
	if rows[2].business == nil || rows[2].metadata["reason"] != "no_membership" {
		t.Errorf("authz row: %+v", rows[2])
	}
}

// TestAuditRollsBackWithTheChange: an audit row that violates its CHECK aborts the whole
// transaction (here: a password change), so no state change exists without its trace.
func TestAuditRollsBackWithTheChange(t *testing.T) {
	e := newEnv(t)
	email := e.activeUser(t)
	userID, _, _ := userRow(t, email)
	// Simulate an audit failure: reject new auth.login rows (NOT VALID keeps existing ones).
	if _, err := pool.Exec(t.Context(), `ALTER TABLE audit_log ADD CONSTRAINT audit_test_block CHECK (action <> 'auth.login') NOT VALID`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `ALTER TABLE audit_log DROP CONSTRAINT IF EXISTS audit_test_block`)
	})
	if _, err := e.svc.Login(t.Context(), application.LoginInput{Email: email, Password: "una clave suficientemente larga", ClientType: "MOBILE"}); err == nil {
		t.Fatal("login committed although its audit row was rejected")
	}
	var sessions int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM sessions WHERE user_id = $1`, userID).Scan(&sessions); err != nil {
		t.Fatal(err)
	}
	if sessions != 0 {
		t.Fatalf("session created without audit row (%d)", sessions)
	}
}
