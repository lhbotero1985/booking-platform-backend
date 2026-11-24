package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
)

func TestAddMember(t *testing.T) {
	ctx := context.Background()
	e := newEnv()
	for _, email := range []string{"owner@example.com", "staff@example.com"} {
		e.register(t, email)
		if err := e.svc.VerifyEmail(ctx, e.sendAndExtract(t)); err != nil {
			t.Fatal(err)
		}
	}
	e.register(t, "pending@example.com") // never verified
	biz := uuid.Must(uuid.NewV7())
	admin := contract.Principal{UserID: uuid.Must(uuid.NewV7()), IsPlatformAdmin: true}
	owner := contract.Principal{UserID: e.store.User("owner@example.com").ID()}

	id, err := e.svc.AddMember(ctx, admin, biz, " OWNER@example.com ", contract.RoleOwner)
	if err != nil || id != owner.UserID {
		t.Fatalf("admin adds owner: %v %v", id, err)
	}
	last := e.store.LastAudit()
	if last.Action != application.AuditMembershipGranted || *last.ActorUserID != admin.UserID || *last.BusinessID != biz ||
		last.Metadata["role"] != "OWNER" || last.ResourceID != owner.UserID.String() {
		t.Errorf("audit = %+v", last)
	}
	// The OWNER may add members to their business.
	if _, err := e.svc.AddMember(ctx, owner, biz, "staff@example.com", contract.RoleManager); err != nil {
		t.Fatalf("owner adds manager: %v", err)
	}
	// Rules.
	for name, tc := range map[string]struct {
		actor contract.Principal
		email string
		role  contract.Role
		want  error
	}{
		"already member":      {admin, "owner@example.com", contract.RoleManager, contract.ErrAlreadyMember},
		"unverified account":  {admin, "pending@example.com", contract.RoleManager, contract.ErrAccountNotEligible},
		"unknown account":     {admin, "nobody@example.com", contract.RoleManager, contract.ErrAccountNotEligible},
		"malformed email":     {admin, "not-an-email", contract.RoleManager, contract.ErrAccountNotEligible},
		"manager cannot add":  {contract.Principal{UserID: e.store.User("staff@example.com").ID()}, "pending@example.com", contract.RoleProfessional, contract.ErrForbidden},
		"other business user": {contract.Principal{UserID: uuid.Must(uuid.NewV7())}, "pending@example.com", contract.RoleProfessional, contract.ErrNotFound},
	} {
		if _, err := e.svc.AddMember(ctx, tc.actor, biz, tc.email, tc.role); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", name, err, tc.want)
		}
	}
	// One active OWNER per business.
	other := uuid.Must(uuid.NewV7())
	if _, err := e.svc.AddMember(ctx, admin, other, "staff@example.com", contract.RoleOwner); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.AddMember(ctx, admin, other, "owner@example.com", contract.RoleOwner); !errors.Is(err, contract.ErrOwnerExists) {
		t.Errorf("second OWNER of a business: %v", err)
	}
	if _, err := e.svc.AddMember(ctx, admin, other, "owner@example.com", "SUPERADMIN"); err == nil {
		t.Error("unknown role accepted")
	}
}
