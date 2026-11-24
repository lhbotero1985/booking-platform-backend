package application_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/application/proftest"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/authz"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
)

type env struct {
	svc      *application.Service
	store    *proftest.Store
	identity *proftest.Identity
	users    map[string]uuid.UUID
	biz      uuid.UUID
	owner    identity.Principal
}

// newEnv: a business whose OWNER is ana@example.com, plus verified accounts barbero@ and otro@
// and an unverified pendiente@.
func newEnv(t *testing.T) env {
	t.Helper()
	e := env{store: proftest.NewStore(), identity: proftest.NewIdentity(), users: map[string]uuid.UUID{}, biz: uuid.Must(uuid.NewV7())}
	for _, email := range []string{"ana@example.com", "barbero@example.com", "otro@example.com"} {
		e.users[email] = e.identity.AddUser(email, true)
	}
	e.users["pendiente@example.com"] = e.identity.AddUser("pendiente@example.com", false)
	e.identity.Grant(e.biz, e.users["ana@example.com"], identity.RoleOwner)
	e.owner = identity.Principal{UserID: e.users["ana@example.com"]}
	e.svc = application.NewService(e.store, e.identity, clock.NewFixed(time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)))
	return e
}

func (e env) principal(email string) identity.Principal {
	return identity.Principal{UserID: e.users[email]}
}

func ptr[T any](v T) *T { return &v }

func fieldCodes(t *testing.T, err error) map[string]string {
	t.Helper()
	var verr *application.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("err = %v, want ValidationError", err)
	}
	out := map[string]string{}
	for _, f := range verr.Fields {
		out[f.Field] = f.Code
	}
	return out
}

func TestCreateProfessional(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)

	// A new barber by email: becomes a PROFESSIONAL member and gets a profile.
	v, err := e.svc.Create(ctx, e.owner, e.biz, application.CreateInput{Email: "barbero@example.com", DisplayName: " Juan el Barbero ", Bio: "Fades y barba", SortOrder: ptr(2)})
	if err != nil {
		t.Fatal(err)
	}
	if v.DisplayName != "Juan el Barbero" || v.Status != "ACTIVE" || !v.Bookable || v.SortOrder != 2 || v.UserID != e.users["barbero@example.com"] {
		t.Errorf("view = %+v", v)
	}
	if r := e.identity.Role(e.biz, e.users["barbero@example.com"]); r != identity.RoleProfessional {
		t.Errorf("barber membership: %q", r)
	}
	// The OWNER also serves customers: profile created, keeps OWNER.
	if _, err := e.svc.Create(ctx, e.owner, e.biz, application.CreateInput{Email: "ana@example.com", DisplayName: "Ana"}); err != nil {
		t.Fatal(err)
	}
	if r := e.identity.Role(e.biz, e.owner.UserID); r != identity.RoleOwner {
		t.Errorf("owner role changed to %q", r)
	}
	// One profile per account and business.
	if _, err := e.svc.Create(ctx, e.owner, e.biz, application.CreateInput{Email: "barbero@example.com", DisplayName: "Otro"}); !errors.Is(err, application.ErrAlreadyExists) {
		t.Errorf("second profile: %v", err)
	}
	// Ineligible accounts and invalid input.
	for _, email := range []string{"pendiente@example.com", "nadie@example.com", "no-es-email"} {
		if got := fieldCodes(t, mustErr(e.svc.Create(ctx, e.owner, e.biz, application.CreateInput{Email: email, DisplayName: "Pepe"}))); got["email"] != application.CodeAccountNotEligible {
			t.Errorf("%s: %v", email, got)
		}
	}
	got := fieldCodes(t, mustErr(e.svc.Create(ctx, e.owner, e.biz, application.CreateInput{DisplayName: "x", Bio: "a\x07b", SortOrder: ptr(1001)})))
	want := map[string]string{"email": application.CodeRequired, "displayName": application.CodeInvalidDisplayName, "bio": application.CodeInvalidBio, "sortOrder": application.CodeInvalidSortOrder}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("fields = %v, want %v", got, want)
	}
	if acts := e.store.AuditActions(); len(acts) != 2 || acts[0] != application.AuditProfessionalCreated {
		t.Errorf("audit = %v", acts)
	}
}

func mustErr(_ application.View, err error) error { return err }

func TestProfessionalsPermissions(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	v, err := e.svc.Create(ctx, e.owner, e.biz, application.CreateInput{Email: "barbero@example.com", DisplayName: "Juan"})
	if err != nil {
		t.Fatal(err)
	}
	barber := e.principal("barbero@example.com")
	// The barber reads but cannot create or edit.
	if list, err := e.svc.List(ctx, barber, e.biz); err != nil || len(list) != 1 {
		t.Errorf("barber list: %v %v", list, err)
	}
	if _, err := e.svc.Update(ctx, barber, e.biz, v.ID, application.UpdateInput{DisplayName: ptr("Juancho")}); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("barber edits: %v", err)
	}
	if _, err := e.svc.Create(ctx, barber, e.biz, application.CreateInput{Email: "otro@example.com", DisplayName: "Otro"}); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("barber creates: %v", err)
	}
	// Another business / no membership: 404 everywhere.
	stranger := e.principal("otro@example.com")
	if _, err := e.svc.List(ctx, stranger, e.biz); !errors.Is(err, authz.ErrNotFound) {
		t.Errorf("stranger list: %v", err)
	}
	if _, err := e.svc.Get(ctx, stranger, e.biz, v.ID); !errors.Is(err, authz.ErrNotFound) {
		t.Errorf("stranger get: %v", err)
	}
	// A professional of another business is not found through this business.
	other := uuid.Must(uuid.NewV7())
	e.identity.Grant(other, stranger.UserID, identity.RoleOwner)
	if _, err := e.svc.Get(ctx, stranger, other, v.ID); !errors.Is(err, application.ErrNotFound) {
		t.Errorf("professional through another business: %v", err)
	}
	// PLATFORM_ADMIN acts on any business.
	admin := identity.Principal{UserID: uuid.Must(uuid.NewV7()), IsPlatformAdmin: true}
	if _, err := e.svc.Update(ctx, admin, e.biz, v.ID, application.UpdateInput{Bio: ptr("Revisado")}); err != nil {
		t.Errorf("admin edit: %v", err)
	}
}

func TestUpdateAndMembershipState(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	v, _ := e.svc.Create(ctx, e.owner, e.biz, application.CreateInput{Email: "barbero@example.com", DisplayName: "Juan"})

	got, err := e.svc.Update(ctx, e.owner, e.biz, v.ID, application.UpdateInput{Status: ptr("inactive"), SortOrder: ptr(5), DisplayName: ptr("Juan")})
	if err != nil || got.Status != "INACTIVE" || got.Bookable || got.SortOrder != 5 {
		t.Fatalf("update: %+v %v", got, err)
	}
	if last := e.store.Audit[len(e.store.Audit)-1]; last.Metadata["fields"] != "sortOrder,status" {
		t.Errorf("audited fields = %q (unchanged name not listed)", last.Metadata["fields"])
	}
	before := len(e.store.Audit)
	if _, err := e.svc.Update(ctx, e.owner, e.biz, v.ID, application.UpdateInput{Status: ptr("INACTIVE")}); err != nil || len(e.store.Audit) != before {
		t.Errorf("no-op update audited: %v", err)
	}
	if _, err := e.svc.Update(ctx, e.owner, e.biz, v.ID, application.UpdateInput{Status: ptr("PAUSED"), SortOrder: ptr(-1)}); !strings.Contains(fmtErr(err), "status") {
		t.Errorf("invalid update: %v", err)
	}
	if _, err := e.svc.Update(ctx, e.owner, e.biz, uuid.Must(uuid.NewV7()), application.UpdateInput{Bio: ptr("x")}); !errors.Is(err, application.ErrNotFound) {
		t.Errorf("unknown professional: %v", err)
	}

	// Reactivate, then remove the barber from the business: the profile is no longer bookable.
	if _, err := e.svc.Update(ctx, e.owner, e.biz, v.ID, application.UpdateInput{Status: ptr("ACTIVE")}); err != nil {
		t.Fatal(err)
	}
	e.identity.Remove(e.biz, v.UserID)
	got, err = e.svc.Get(ctx, e.owner, e.biz, v.ID)
	if err != nil || got.MemberActive || got.Bookable || got.Status != "ACTIVE" {
		t.Errorf("after removing the member: %+v %v", got, err)
	}
}

func fmtErr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestIsProfessionalOf(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	v, _ := e.svc.Create(ctx, e.owner, e.biz, application.CreateInput{Email: "barbero@example.com", DisplayName: "Juan"})
	if ok, err := e.svc.IsProfessionalOf(ctx, e.biz, v.ID); !ok || err != nil {
		t.Errorf("own professional: %v %v", ok, err)
	}
	if ok, err := e.svc.IsProfessionalOf(ctx, uuid.Must(uuid.NewV7()), v.ID); ok || err != nil {
		t.Errorf("another business: %v %v", ok, err)
	}
}
