package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application"
)

func TestSetPlatformAdmin(t *testing.T) {
	ctx := context.Background()
	e := newEnv()

	// Unverified account: refused, nothing audited.
	e.register(t, "ana@example.com")
	if err := e.svc.SetPlatformAdmin(ctx, "ana@example.com", true); !errors.Is(err, application.ErrAccountNotActive) {
		t.Fatalf("unverified account: err = %v, want ErrAccountNotActive", err)
	}
	if err := e.svc.VerifyEmail(ctx, e.sendAndExtract(t)); err != nil {
		t.Fatal(err)
	}

	before := len(e.store.AuditActions())
	if err := e.svc.SetPlatformAdmin(ctx, " ANA@example.com ", true); err != nil {
		t.Fatal(err)
	}
	if !e.store.User("ana@example.com").IsPlatformAdmin() {
		t.Fatal("flag not set")
	}
	// Idempotent: granting again changes and audits nothing.
	if err := e.svc.SetPlatformAdmin(ctx, "ana@example.com", true); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.SetPlatformAdmin(ctx, "ana@example.com", false); err != nil {
		t.Fatal(err)
	}
	if e.store.User("ana@example.com").IsPlatformAdmin() {
		t.Fatal("flag not cleared")
	}
	got := e.store.AuditActions()[before:]
	want := []string{application.AuditPlatformAdminGranted + ":SUCCESS", application.AuditPlatformAdminRevoked + ":SUCCESS"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("audit = %v, want %v", got, want)
	}
	last := e.store.LastAudit()
	if last.ResourceType != "user" || last.Metadata["source"] != "cli" || last.ActorUserID != nil {
		t.Errorf("audit entry = %+v: want resource user, source cli, no actor", last)
	}

	// Unknown account and invalid email.
	if err := e.svc.SetPlatformAdmin(ctx, "nobody@example.com", true); !errors.Is(err, application.ErrNotFound) {
		t.Errorf("unknown account: err = %v", err)
	}
	var verr *application.ValidationError
	if err := e.svc.SetPlatformAdmin(ctx, "not-an-email", true); !errors.As(err, &verr) {
		t.Errorf("invalid email: err = %v", err)
	}
}
