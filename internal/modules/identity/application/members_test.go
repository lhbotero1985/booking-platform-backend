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
	if _, err := e.svc.AddMember(ctx, owner, biz, "staff@example.com", contract.RoleProfessional); err != nil {
		t.Fatalf("owner adds manager: %v", err)
	}
	// Rules.
	for name, tc := range map[string]struct {
		actor contract.Principal
		email string
		role  contract.Role
		want  error
	}{
		"already member":      {admin, "owner@example.com", contract.RoleProfessional, contract.ErrAlreadyMember},
		"unverified account":  {admin, "pending@example.com", contract.RoleProfessional, contract.ErrAccountNotEligible},
		"unknown account":     {admin, "nobody@example.com", contract.RoleProfessional, contract.ErrAccountNotEligible},
		"malformed email":     {admin, "not-an-email", contract.RoleProfessional, contract.ErrAccountNotEligible},
		"manager cannot add":  {contract.Principal{UserID: e.store.User("staff@example.com").ID()}, "pending@example.com", contract.RoleProfessional, contract.ErrForbidden},
		"other business user": {contract.Principal{UserID: uuid.Must(uuid.NewV7())}, "pending@example.com", contract.RoleProfessional, contract.ErrNotFound},
	} {
		if _, err := e.svc.AddMember(ctx, tc.actor, biz, tc.email, tc.role); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", name, err, tc.want)
		}
	}
	// Several OWNERs per business are allowed (decision of 2026-11-24).
	other := uuid.Must(uuid.NewV7())
	if _, err := e.svc.AddMember(ctx, admin, other, "staff@example.com", contract.RoleOwner); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.AddMember(ctx, admin, other, "owner@example.com", contract.RoleOwner); err != nil {
		t.Errorf("second OWNER of a business: %v", err)
	}
	if _, err := e.svc.AddMember(ctx, admin, other, "owner@example.com", "SUPERADMIN"); err == nil {
		t.Error("unknown role accepted")
	}
}

func TestManageMembers(t *testing.T) {
	ctx := context.Background()
	e := newEnv()
	for _, email := range []string{"ana@example.com", "bob@example.com", "carla@example.com", "otro@example.com"} {
		e.register(t, email)
		if err := e.svc.VerifyEmail(ctx, e.sendAndExtract(t)); err != nil {
			t.Fatal(err)
		}
	}
	id := func(email string) uuid.UUID { return e.store.User(email).ID() }
	p := func(email string) contract.Principal { return contract.Principal{UserID: id(email)} }
	biz := uuid.Must(uuid.NewV7())
	admin := contract.Principal{UserID: uuid.Must(uuid.NewV7()), IsPlatformAdmin: true}
	if _, err := e.svc.AddMember(ctx, admin, biz, "ana@example.com", contract.RoleOwner); err != nil {
		t.Fatal(err)
	}
	ana := p("ana@example.com")
	if _, err := e.svc.AddMember(ctx, ana, biz, "bob@example.com", contract.RoleProfessional); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.AddMember(ctx, ana, biz, "carla@example.com", contract.RoleProfessional); err != nil {
		t.Fatal(err)
	}
	e.store.GrantMembership(id("otro@example.com"), uuid.Must(uuid.NewV7()), contract.RoleOwner)

	// List: OWNER only; PROFESSIONAL 403; owner of another business 404.
	ms, err := e.svc.ListMembers(ctx, ana, biz)
	if err != nil || len(ms) != 3 || ms[0].Role != contract.RoleOwner || ms[0].Email != "ana@example.com" {
		t.Fatalf("members = %+v %v", ms, err)
	}
	if _, err := e.svc.ListMembers(ctx, p("carla@example.com"), biz); !errors.Is(err, contract.ErrForbidden) {
		t.Errorf("professional list: %v", err)
	}
	if _, err := e.svc.ListMembers(ctx, p("otro@example.com"), biz); !errors.Is(err, contract.ErrNotFound) {
		t.Errorf("other business list: %v", err)
	}

	// Only OWNERs manage members.
	if err := e.svc.ChangeMemberRole(ctx, p("bob@example.com"), biz, id("carla@example.com"), contract.RoleProfessional); !errors.Is(err, contract.ErrForbidden) {
		t.Errorf("manager changes a role: %v", err)
	}
	if _, err := e.svc.AddMember(ctx, p("bob@example.com"), biz, "otro@example.com", contract.RoleProfessional); !errors.Is(err, contract.ErrForbidden) {
		t.Errorf("manager adds a member: %v", err)
	}

	// The only OWNER can be neither demoted nor removed.
	if err := e.svc.ChangeMemberRole(ctx, ana, biz, id("ana@example.com"), contract.RoleProfessional); !errors.Is(err, contract.ErrLastOwner) {
		t.Errorf("demote the last owner: %v", err)
	}
	if err := e.svc.RemoveMember(ctx, ana, biz, id("ana@example.com")); !errors.Is(err, contract.ErrLastOwner) {
		t.Errorf("remove the last owner: %v", err)
	}

	// Change of owner: Bob becomes OWNER, then Ana steps down and leaves.
	if err := e.svc.ChangeMemberRole(ctx, ana, biz, id("bob@example.com"), contract.RoleOwner); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.ChangeMemberRole(ctx, ana, biz, id("ana@example.com"), contract.RoleProfessional); err != nil {
		t.Fatalf("demote with another owner: %v", err)
	}
	if err := e.svc.RemoveMember(ctx, p("bob@example.com"), biz, id("ana@example.com")); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.RequireBusinessRole(ctx, ana, biz, contract.RolesStaff...); !errors.Is(err, contract.ErrNotFound) {
		t.Errorf("removed member still has access: %v", err)
	}
	// Removed member can be added again (reactivated).
	if _, err := e.svc.AddMember(ctx, p("bob@example.com"), biz, "ana@example.com", contract.RoleProfessional); err != nil {
		t.Errorf("re-add a removed member: %v", err)
	}
	// Unknown member, unknown role, no-op.
	if err := e.svc.RemoveMember(ctx, p("bob@example.com"), biz, uuid.Must(uuid.NewV7())); !errors.Is(err, contract.ErrMemberNotFound) {
		t.Errorf("remove a non-member: %v", err)
	}
	if err := e.svc.ChangeMemberRole(ctx, p("bob@example.com"), biz, id("carla@example.com"), "BOSS"); err == nil {
		t.Error("unknown role accepted")
	}
	before := len(e.store.AuditActions())
	if err := e.svc.ChangeMemberRole(ctx, p("bob@example.com"), biz, id("carla@example.com"), contract.RoleProfessional); err != nil || len(e.store.AuditActions()) != before {
		t.Errorf("same role: %v, audited %d", err, len(e.store.AuditActions())-before)
	}
	var changed, revoked int
	for _, a := range e.store.AuditActions() {
		switch a {
		case application.AuditMembershipRoleChanged + ":SUCCESS":
			changed++
		case application.AuditMembershipRevoked + ":SUCCESS":
			revoked++
		}
	}
	if changed != 2 || revoked != 1 {
		t.Errorf("audit: %d role changes, %d revocations; want 2 and 1", changed, revoked)
	}
}

func TestEnsureMemberAndActiveMembersAmong(t *testing.T) {
	ctx := context.Background()
	e := newEnv()
	for _, email := range []string{"ana@example.com", "barbero@example.com"} {
		e.register(t, email)
		if err := e.svc.VerifyEmail(ctx, e.sendAndExtract(t)); err != nil {
			t.Fatal(err)
		}
	}
	e.register(t, "pendiente@example.com")
	biz := uuid.Must(uuid.NewV7())
	ana := contract.Principal{UserID: e.store.User("ana@example.com").ID()}
	e.store.GrantMembership(ana.UserID, biz, contract.RoleOwner)

	// A new account becomes PROFESSIONAL; an existing member keeps its role.
	barber, err := e.svc.EnsureMember(ctx, ana, biz, "barbero@example.com", contract.RoleProfessional)
	if err != nil || barber != e.store.User("barbero@example.com").ID() {
		t.Fatalf("ensure barber: %v %v", barber, err)
	}
	if id, err := e.svc.EnsureMember(ctx, ana, biz, "ana@example.com", contract.RoleProfessional); err != nil || id != ana.UserID {
		t.Fatalf("ensure owner: %v %v", id, err)
	}
	if r, _ := e.svc.RequireBusinessRole(ctx, ana, biz, contract.RolesOwner...); r != contract.RoleOwner {
		t.Errorf("owner role changed to %q", r)
	}
	if _, err := e.svc.EnsureMember(ctx, ana, biz, "barbero@example.com", contract.RoleProfessional); err != nil {
		t.Errorf("ensure again: %v", err)
	}
	for _, email := range []string{"pendiente@example.com", "nadie@example.com", "x"} {
		if _, err := e.svc.EnsureMember(ctx, ana, biz, email, contract.RoleProfessional); !errors.Is(err, contract.ErrAccountNotEligible) {
			t.Errorf("%s: %v", email, err)
		}
	}
	if _, err := e.svc.EnsureMember(ctx, contract.Principal{UserID: barber}, biz, "ana@example.com", contract.RoleProfessional); !errors.Is(err, contract.ErrForbidden) {
		t.Errorf("professional ensures: %v", err)
	}

	got, err := e.svc.ActiveMembersAmong(ctx, biz, []uuid.UUID{ana.UserID, barber, uuid.Must(uuid.NewV7())})
	if err != nil || len(got) != 2 || !got[barber] || !got[ana.UserID] {
		t.Errorf("active members = %v %v", got, err)
	}
	if err := e.svc.RemoveMember(ctx, ana, biz, barber); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.svc.ActiveMembersAmong(ctx, biz, []uuid.UUID{barber}); got[barber] {
		t.Error("removed member still active")
	}
	if got, err := e.svc.ActiveMembersAmong(ctx, biz, nil); err != nil || len(got) != 0 {
		t.Errorf("empty batch: %v %v", got, err)
	}
}
