package contract_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
)

// TestCheckBusinessRoleMatrix covers role × action × same/other business (ADR-0007 policy
// matrix, docs/architecture/security.md §2).
func TestCheckBusinessRoleMatrix(t *testing.T) {
	bizA, bizB := uuid.New(), uuid.New()
	actions := map[string][]contract.Role{
		"owner-only (settings, memberships)": contract.RolesOwner,
		"management (catalog, any agenda)":   contract.RolesManagement,
		"staff (own agenda)":                 contract.RolesStaff,
	}
	// want[action][role] for the caller's own business.
	allowed := map[string]map[contract.Role]bool{
		"owner-only (settings, memberships)": {contract.RoleOwner: true},
		"management (catalog, any agenda)":   {contract.RoleOwner: true, contract.RoleManager: true},
		"staff (own agenda)":                 {contract.RoleOwner: true, contract.RoleManager: true, contract.RoleProfessional: true},
	}
	for action, roles := range actions {
		for _, role := range []contract.Role{contract.RoleOwner, contract.RoleManager, contract.RoleProfessional} {
			ms := []contract.Membership{{BusinessID: bizA, Role: role}}

			got, err := contract.CheckBusinessRole(ms, bizA, roles...)
			switch {
			case allowed[action][role] && (err != nil || got != role):
				t.Errorf("%s / %s / same business: got (%q, %v), want allowed", action, role, got, err)
			case !allowed[action][role] && !errors.Is(err, contract.ErrForbidden):
				t.Errorf("%s / %s / same business: got (%q, %v), want ErrForbidden", action, role, got, err)
			}

			// Another business is always "not found", whatever the role: never 403.
			if _, err := contract.CheckBusinessRole(ms, bizB, roles...); !errors.Is(err, contract.ErrNotFound) {
				t.Errorf("%s / %s / other business: %v, want ErrNotFound", action, role, err)
			}
		}
		// Customers (no membership) never get business access.
		if _, err := contract.CheckBusinessRole(nil, bizA, roles...); !errors.Is(err, contract.ErrNotFound) {
			t.Errorf("%s / customer: %v, want ErrNotFound", action, err)
		}
	}
}

func TestCheckBusinessRoleFailsClosed(t *testing.T) {
	biz := uuid.New()
	ms := []contract.Membership{{BusinessID: biz, Role: contract.RoleOwner}}
	if _, err := contract.CheckBusinessRole(ms, biz); !errors.Is(err, contract.ErrForbidden) {
		t.Errorf("empty allowed list: %v, want ErrForbidden", err)
	}
	if _, err := contract.CheckBusinessRole(ms, uuid.Nil, contract.RolesStaff...); !errors.Is(err, contract.ErrNotFound) {
		t.Errorf("nil business: %v, want ErrNotFound", err)
	}
	if _, err := contract.CheckBusinessRole(ms, biz, contract.Role("SUPERUSER")); !errors.Is(err, contract.ErrForbidden) {
		t.Errorf("unknown role in allowed: %v", err)
	}
}

func TestMultipleBusinessesAreIndependent(t *testing.T) {
	bizA, bizB := uuid.New(), uuid.New()
	ms := []contract.Membership{{BusinessID: bizA, Role: contract.RoleProfessional}, {BusinessID: bizB, Role: contract.RoleOwner}}
	if _, err := contract.CheckBusinessRole(ms, bizA, contract.RolesOwner...); !errors.Is(err, contract.ErrForbidden) {
		t.Errorf("owner of B must not be owner of A: %v", err)
	}
	if r, err := contract.CheckBusinessRole(ms, bizB, contract.RolesOwner...); err != nil || r != contract.RoleOwner {
		t.Errorf("owner of B: %q %v", r, err)
	}
}

// The pure membership policy ignores the admin flag; the Authorizer grants PLATFORM_ADMIN on
// top of it (ADR-0018, see application.TestPlatformAdminActsOnAnyBusiness).
func TestPlatformAdminFlagIsNotAMembership(t *testing.T) {
	admin := contract.Principal{UserID: uuid.New(), IsPlatformAdmin: true}
	if err := contract.CheckPlatformAdmin(admin); err != nil {
		t.Fatalf("admin: %v", err)
	}
	// The admin flag is not a membership: business policies still answer "not found".
	if _, err := contract.CheckBusinessRole(nil, uuid.New(), contract.RolesStaff...); !errors.Is(err, contract.ErrNotFound) {
		t.Errorf("admin without membership: %v", err)
	}
	for name, p := range map[string]contract.Principal{
		"regular user":      {UserID: uuid.New()},
		"zero principal":    {},
		"admin flag no uid": {IsPlatformAdmin: true},
	} {
		if err := contract.CheckPlatformAdmin(p); !errors.Is(err, contract.ErrForbidden) {
			t.Errorf("%s: %v, want ErrForbidden", name, err)
		}
	}
}
