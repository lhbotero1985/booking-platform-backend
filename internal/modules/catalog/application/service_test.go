package application_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/application/catalogtest"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/domain"
	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/authz"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
)

type env struct {
	svc           *application.Service
	store         *catalogtest.Store
	ids           *catalogtest.Identity
	biz           uuid.UUID
	owner, barber identity.Principal
}

func newEnv(t *testing.T) env {
	t.Helper()
	e := env{store: catalogtest.NewStore(), ids: catalogtest.NewIdentity(), biz: uuid.Must(uuid.NewV7()),
		owner: identity.Principal{UserID: uuid.Must(uuid.NewV7())}, barber: identity.Principal{UserID: uuid.Must(uuid.NewV7())}}
	e.ids.Grant(e.biz, e.owner.UserID, identity.RoleOwner)
	e.ids.Grant(e.biz, e.barber.UserID, identity.RoleProfessional)
	e.svc = application.NewService(e.store, e.ids, catalogtest.Currencies{}, clock.NewFixed(time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)))
	return e
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

func corteBarba(category uuid.UUID) application.ServiceInput {
	return application.ServiceInput{Name: " Corte + barba ", Description: "Incluye toalla caliente", CategoryID: category,
		DurationMinutes: ptr(45), PriceMinor: ptr(int64(3_500_000)), SortOrder: ptr(1)}
}

func TestCategories(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	cortes, err := e.svc.CreateCategory(ctx, e.owner, e.biz, application.CategoryInput{Name: " Cortes "})
	if err != nil || cortes.Name != "Cortes" || cortes.Status != domain.StatusActive {
		t.Fatalf("create: %+v %v", cortes, err)
	}
	if _, err := e.svc.CreateCategory(ctx, e.owner, e.biz, application.CategoryInput{Name: "Barba", SortOrder: ptr(0)}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.CreateCategory(ctx, e.owner, e.biz, application.CategoryInput{Name: "CORTES"}); !errors.Is(err, application.ErrNameTaken) {
		t.Errorf("duplicate (case-insensitive): %v", err)
	}
	if got := fieldCodes(t, mustCat(e.svc.CreateCategory(ctx, e.owner, e.biz, application.CategoryInput{Name: " ", SortOrder: ptr(-1)}))); !reflect.DeepEqual(got, map[string]string{"name": application.CodeRequired, "sortOrder": application.CodeInvalidSortOrder}) {
		t.Errorf("fields = %v", got)
	}
	list, err := e.svc.ListCategories(ctx, e.barber, e.biz)
	if err != nil || len(list) != 2 || list[0].Name != "Barba" {
		t.Errorf("barber lists: %+v %v", list, err)
	}
	upd, err := e.svc.UpdateCategory(ctx, e.owner, e.biz, cortes.ID, application.CategoryPatch{Name: ptr("Cortes de cabello"), Status: ptr("inactive"), SortOrder: ptr(cortes.SortOrder)})
	if err != nil || upd.Status != domain.StatusInactive || upd.Name != "Cortes de cabello" {
		t.Fatalf("update: %+v %v", upd, err)
	}
	if last := e.store.Audit[len(e.store.Audit)-1]; last.Metadata["fields"] != "name,status" {
		t.Errorf("audited fields = %q", last.Metadata["fields"])
	}
	before := len(e.store.Audit)
	if _, err := e.svc.UpdateCategory(ctx, e.owner, e.biz, cortes.ID, application.CategoryPatch{Status: ptr("INACTIVE")}); err != nil || len(e.store.Audit) != before {
		t.Errorf("no-op audited: %v", err)
	}
	if got := fieldCodes(t, mustCat(e.svc.UpdateCategory(ctx, e.owner, e.biz, cortes.ID, application.CategoryPatch{Name: ptr("x"), Status: ptr("PAUSED"), SortOrder: ptr(2000)}))); len(got) != 3 {
		t.Errorf("update fields = %v", got)
	}
	if _, err := e.svc.UpdateCategory(ctx, e.owner, e.biz, cortes.ID, application.CategoryPatch{Name: ptr("barba")}); !errors.Is(err, application.ErrNameTaken) {
		t.Errorf("rename to existing: %v", err)
	}
	if _, err := e.svc.UpdateCategory(ctx, e.owner, e.biz, uuid.Must(uuid.NewV7()), application.CategoryPatch{Name: ptr("Nueva")}); !errors.Is(err, application.ErrNotFound) {
		t.Errorf("unknown: %v", err)
	}

	// Deleting a category leaves its services uncategorized.
	svc, err := e.svc.CreateService(ctx, e.owner, e.biz, corteBarba(cortes.ID))
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.DeleteCategory(ctx, e.owner, e.biz, cortes.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.svc.GetService(ctx, e.barber, e.biz, svc.ID); got.CategoryID != uuid.Nil {
		t.Errorf("service still categorized: %v", got.CategoryID)
	}
	if err := e.svc.DeleteCategory(ctx, e.owner, e.biz, cortes.ID); !errors.Is(err, application.ErrNotFound) {
		t.Errorf("delete twice: %v", err)
	}
}

func mustCat(_ domain.Category, err error) error { return err }

func mustSvc(_ domain.Service, err error) error { return err }

func TestServices(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	barba, _ := e.svc.CreateCategory(ctx, e.owner, e.biz, application.CategoryInput{Name: "Barba"})

	// "Corte + barba": an ordinary service with its own price and duration, in COP.
	s, err := e.svc.CreateService(ctx, e.owner, e.biz, corteBarba(barba.ID))
	if err != nil || s.Name != "Corte + barba" || s.DurationMinutes != 45 || s.PriceMinor != 3_500_000 || s.Currency != "COP" || s.CategoryID != barba.ID || s.Status != domain.StatusActive {
		t.Fatalf("create: %+v %v", s, err)
	}
	corte := application.ServiceInput{Name: "Corte", DurationMinutes: ptr(30), PriceMinor: ptr(int64(2_500_000))}
	if _, err := e.svc.CreateService(ctx, e.owner, e.biz, corte); err != nil {
		t.Fatalf("uncategorized: %v", err)
	}
	if _, err := e.svc.CreateService(ctx, e.owner, e.biz, application.ServiceInput{Name: "corte", DurationMinutes: ptr(30), PriceMinor: ptr(int64(0))}); !errors.Is(err, application.ErrNameTaken) {
		t.Errorf("duplicate name: %v", err)
	}
	// Validation: every field reported.
	got := fieldCodes(t, mustSvc(e.svc.CreateService(ctx, e.owner, e.biz, application.ServiceInput{Description: "a\x00", SortOrder: ptr(-1)})))
	want := map[string]string{"name": application.CodeRequired, "description": application.CodeInvalidDescription, "durationMinutes": application.CodeRequired, "priceMinor": application.CodeRequired, "sortOrder": application.CodeInvalidSortOrder}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("fields = %v, want %v", got, want)
	}
	got = fieldCodes(t, mustSvc(e.svc.CreateService(ctx, e.owner, e.biz, application.ServiceInput{Name: "Tinte", DurationMinutes: ptr(42), PriceMinor: ptr(int64(2_500_050))})))
	if got["durationMinutes"] != application.CodeInvalidDuration || got["priceMinor"] != application.CodeInvalidPrice {
		t.Errorf("duration/price = %v", got)
	}
	// A category of another business (or unknown) is rejected.
	other := uuid.Must(uuid.NewV7())
	foreign, _ := e.svc.CreateCategory(ctx, identity.Principal{UserID: uuid.Must(uuid.NewV7()), IsPlatformAdmin: true}, other, application.CategoryInput{Name: "Ajena"})
	in := corteBarba(foreign.ID)
	in.Name = "Con categoría ajena"
	if got := fieldCodes(t, mustSvc(e.svc.CreateService(ctx, e.owner, e.biz, in))); got["categoryId"] != application.CodeCategoryNotFound {
		t.Errorf("foreign category: %v", got)
	}

	// The staff reads; the list includes inactive services, by sort order then name.
	list, err := e.svc.ListServices(ctx, e.barber, e.biz)
	if err != nil || len(list) != 2 || list[0].Name != "Corte" {
		t.Errorf("list = %+v %v", list, err)
	}
	// Update: price and duration change; category removed with uuid.Nil.
	upd, err := e.svc.UpdateService(ctx, e.owner, e.biz, s.ID, application.ServicePatch{
		PriceMinor: ptr(int64(4_000_000)), DurationMinutes: ptr(60), CategoryID: ptr(uuid.Nil), Status: ptr("INACTIVE"), Name: ptr("Corte + barba"),
	})
	if err != nil || upd.PriceMinor != 4_000_000 || upd.DurationMinutes != 60 || upd.CategoryID != uuid.Nil || upd.Status != domain.StatusInactive {
		t.Fatalf("update: %+v %v", upd, err)
	}
	if last := e.store.Audit[len(e.store.Audit)-1]; last.Action != application.AuditServiceUpdated || last.Metadata["fields"] != "categoryId,durationMinutes,priceMinor,status" {
		t.Errorf("audit = %+v", last)
	}
	before := len(e.store.Audit)
	if _, err := e.svc.UpdateService(ctx, e.owner, e.biz, s.ID, application.ServicePatch{PriceMinor: ptr(int64(4_000_000))}); err != nil || len(e.store.Audit) != before {
		t.Errorf("no-op audited: %v", err)
	}
	if _, err := e.svc.UpdateService(ctx, e.owner, e.biz, s.ID, application.ServicePatch{CategoryID: ptr(foreign.ID)}); fieldCodes(t, err)["categoryId"] != application.CodeCategoryNotFound {
		t.Errorf("update to foreign category: %v", err)
	}
	if got := fieldCodes(t, mustSvc(e.svc.UpdateService(ctx, e.owner, e.biz, s.ID, application.ServicePatch{
		Name: ptr(""), Description: ptr("a\x00"), Status: ptr("X"), DurationMinutes: ptr(500), PriceMinor: ptr(int64(-100)), SortOrder: ptr(5000),
	}))); len(got) != 6 {
		t.Errorf("update fields = %v", got)
	}
	if _, err := e.svc.UpdateService(ctx, e.owner, e.biz, s.ID, application.ServicePatch{Name: ptr("CORTE")}); !errors.Is(err, application.ErrNameTaken) {
		t.Errorf("rename to existing: %v", err)
	}
	if _, err := e.svc.UpdateService(ctx, e.owner, e.biz, uuid.Must(uuid.NewV7()), application.ServicePatch{}); !errors.Is(err, application.ErrNotFound) {
		t.Errorf("unknown: %v", err)
	}
}

func TestCatalogPermissions(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	s, _ := e.svc.CreateService(ctx, e.owner, e.biz, corteBarba(uuid.Nil))
	c, _ := e.svc.CreateCategory(ctx, e.owner, e.biz, application.CategoryInput{Name: "Cortes"})

	// The barber reads but does not manage the catalog.
	for name, err := range map[string]error{
		"create service":  mustSvc(e.svc.CreateService(ctx, e.barber, e.biz, corteBarba(uuid.Nil))),
		"update service":  mustSvc(e.svc.UpdateService(ctx, e.barber, e.biz, s.ID, application.ServicePatch{PriceMinor: ptr(int64(100))})),
		"create category": mustCat(e.svc.CreateCategory(ctx, e.barber, e.biz, application.CategoryInput{Name: "Uñas"})),
		"update category": mustCat(e.svc.UpdateCategory(ctx, e.barber, e.biz, c.ID, application.CategoryPatch{Name: ptr("X1")})),
		"delete category": e.svc.DeleteCategory(ctx, e.barber, e.biz, c.ID),
	} {
		if !errors.Is(err, authz.ErrForbidden) {
			t.Errorf("barber %s: %v", name, err)
		}
	}
	if _, err := e.svc.GetService(ctx, e.barber, e.biz, s.ID); err != nil {
		t.Errorf("barber reads: %v", err)
	}
	// A stranger: 404 everywhere.
	stranger := identity.Principal{UserID: uuid.Must(uuid.NewV7())}
	if _, err := e.svc.ListServices(ctx, stranger, e.biz); !errors.Is(err, authz.ErrNotFound) {
		t.Errorf("stranger list: %v", err)
	}
	if _, err := e.svc.ListCategories(ctx, stranger, e.biz); !errors.Is(err, authz.ErrNotFound) {
		t.Errorf("stranger categories: %v", err)
	}
	// A service of this business is not found through another one.
	other := uuid.Must(uuid.NewV7())
	e.ids.Grant(other, stranger.UserID, identity.RoleOwner)
	if _, err := e.svc.GetService(ctx, stranger, other, s.ID); !errors.Is(err, application.ErrNotFound) {
		t.Errorf("through another business: %v", err)
	}
	// PLATFORM_ADMIN manages any catalog.
	admin := identity.Principal{UserID: uuid.Must(uuid.NewV7()), IsPlatformAdmin: true}
	if _, err := e.svc.UpdateService(ctx, admin, e.biz, s.ID, application.ServicePatch{Description: ptr("Revisado")}); err != nil {
		t.Errorf("admin: %v", err)
	}
	// The business currency comes from the businesses contract.
	gone := uuid.Must(uuid.NewV7())
	svc := application.NewService(catalogtest.NewStore(), e.ids, catalogtest.Currencies{gone: ""}, nil)
	if _, err := svc.CreateService(ctx, admin, gone, corteBarba(uuid.Nil)); err == nil {
		t.Error("service for a missing business")
	}
}
