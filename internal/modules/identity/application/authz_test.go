package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application/apptest"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
)

func newAuthzService() (*application.Service, *apptest.Store) {
	store := apptest.NewStore()
	return application.NewService(application.Deps{Store: store, Hasher: &apptest.Hasher{}, Mailer: &apptest.Mailer{}}), store
}

func TestRevokedMembershipLosesAccessOnNextCall(t *testing.T) {
	svc, store := newAuthzService()
	ctx := context.Background()
	p := contract.Principal{UserID: uuid.New()}
	biz := uuid.New()
	store.GrantMembership(p.UserID, biz, contract.RoleManager)

	if r, err := svc.RequireBusinessRole(ctx, p, biz, contract.RolesManagement...); err != nil || r != contract.RoleManager {
		t.Fatalf("granted: %q %v", r, err)
	}
	store.RevokeMembership(p.UserID, biz)
	if _, err := svc.RequireBusinessRole(ctx, p, biz, contract.RolesManagement...); !errors.Is(err, contract.ErrNotFound) {
		t.Fatalf("after revoke: %v, want ErrNotFound", err)
	}
}

func TestRequireBusinessRoleErrors(t *testing.T) {
	svc, store := newAuthzService()
	ctx := context.Background()
	if _, err := svc.RequireBusinessRole(ctx, contract.Principal{}, uuid.New(), contract.RolesStaff...); !errors.Is(err, application.ErrUnauthenticated) {
		t.Errorf("anonymous: %v", err)
	}
	store.FailNext = errors.New("db down")
	if _, err := svc.RequireBusinessRole(ctx, contract.Principal{UserID: uuid.New()}, uuid.New(), contract.RolesStaff...); err == nil ||
		errors.Is(err, contract.ErrNotFound) || errors.Is(err, contract.ErrForbidden) {
		t.Errorf("store failure must propagate, not become a policy decision: %v", err)
	}
}

func TestMyMembershipsSortedAndScopedToCaller(t *testing.T) {
	svc, store := newAuthzService()
	me, other := uuid.New(), uuid.New()
	a, b := uuid.MustParse("00000000-0000-7000-8000-00000000000a"), uuid.MustParse("00000000-0000-7000-8000-00000000000b")
	store.GrantMembership(me, b, contract.RoleProfessional)
	store.GrantMembership(me, a, contract.RoleOwner)
	store.GrantMembership(other, uuid.New(), contract.RoleOwner)

	ms, err := svc.MyMemberships(context.Background(), contract.Principal{UserID: me})
	if err != nil || len(ms) != 2 || ms[0].BusinessID != a || ms[1].BusinessID != b {
		t.Fatalf("memberships = %+v, %v", ms, err)
	}
	if _, err := svc.MyMemberships(context.Background(), contract.Principal{}); !errors.Is(err, application.ErrUnauthenticated) {
		t.Errorf("anonymous: %v", err)
	}
}

func TestRequirePlatformAdminUsesPrincipalFlag(t *testing.T) {
	svc, _ := newAuthzService()
	if err := svc.RequirePlatformAdmin(contract.Principal{UserID: uuid.New(), IsPlatformAdmin: true}); err != nil {
		t.Errorf("admin: %v", err)
	}
	if err := svc.RequirePlatformAdmin(contract.Principal{UserID: uuid.New()}); !errors.Is(err, contract.ErrForbidden) {
		t.Errorf("non admin: %v", err)
	}
}
