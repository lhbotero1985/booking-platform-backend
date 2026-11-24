package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/application/catalogtest"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/domain"
	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	professionals "github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/contract"
)

func TestPublicCatalog(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	juan, ana, pedro := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	for _, id := range []uuid.UUID{juan, ana, pedro} {
		e.profs.Add(e.biz, id)
	}
	pub := &catalogtest.Public{
		Slugs: map[string]uuid.UUID{"barberia-centro": e.biz},
		// Pedro is not bookable (inactive or no longer a member).
		Bookable: map[uuid.UUID][]professionals.PublicProfessional{e.biz: {
			{ID: ana, BusinessID: e.biz, DisplayName: "Ana", SortOrder: 0},
			{ID: juan, BusinessID: e.biz, DisplayName: "Juan", SortOrder: 1},
		}},
	}
	e.svc.SetPublicDeps(application.PublicDeps{Directory: pub, Professionals: pub})

	cortes, _ := e.svc.CreateCategory(ctx, e.owner, e.biz, application.CategoryInput{Name: "Cortes"})
	oculta, _ := e.svc.CreateCategory(ctx, e.owner, e.biz, application.CategoryInput{Name: "Oculta"})
	if _, err := e.svc.UpdateCategory(ctx, e.owner, e.biz, oculta.ID, application.CategoryPatch{Status: ptr("INACTIVE")}); err != nil {
		t.Fatal(err)
	}
	mk := func(name string, cat uuid.UUID, order int) domain.Service {
		s, err := e.svc.CreateService(ctx, e.owner, e.biz, application.ServiceInput{Name: name, CategoryID: cat, DurationMinutes: ptr(30), PriceMinor: ptr(int64(2_500_000)), SortOrder: ptr(order)})
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	corte := mk("Corte", cortes.ID, 0)
	barba := mk("Corte + barba", oculta.ID, 1) // inactive category: shown uncategorized
	tinte := mk("Tinte", cortes.ID, 2)         // only Pedro (not bookable) performs it
	mk("Uñas", uuid.Nil, 3)                    // nobody performs it
	inact := mk("Keratina", uuid.Nil, 4)       // inactive service
	if _, err := e.svc.UpdateService(ctx, e.owner, e.biz, inact.ID, application.ServicePatch{Status: ptr("INACTIVE")}); err != nil {
		t.Fatal(err)
	}
	assign := func(biz, pro uuid.UUID, p identity.Principal, ids ...uuid.UUID) {
		if _, err := e.svc.ReplaceProfessionalServices(ctx, p, biz, pro, ids); err != nil {
			t.Fatal(err)
		}
	}
	assign(e.biz, juan, e.owner, barba.ID, corte.ID, inact.ID)
	assign(e.biz, ana, e.owner, corte.ID)
	assign(e.biz, pedro, e.owner, tinte.ID, corte.ID)

	got, err := e.svc.PublicCatalog(ctx, "barberia-centro")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Services) != 2 || got.Services[0].Name != "Corte" || got.Services[1].Name != "Corte + barba" {
		t.Fatalf("services = %+v", got.Services)
	}
	if ids := got.Services[0].ProfessionalIDs; len(ids) != 2 || ids[0] != ana || ids[1] != juan {
		t.Errorf("Corte professionals (Ana before Juan, no Pedro) = %v", ids)
	}
	if got.Services[1].CategoryID != uuid.Nil {
		t.Error("a service of an inactive category keeps it publicly")
	}
	if len(got.Categories) != 1 || got.Categories[0].ID != cortes.ID {
		t.Errorf("categories = %+v", got.Categories)
	}
	if len(got.Professionals) != 2 || got.Professionals[0].ID != ana || len(got.Professionals[1].ServiceIDs) != 2 || got.Professionals[1].ServiceIDs[0] != corte.ID {
		t.Errorf("professionals = %+v", got.Professionals)
	}
	if _, err := e.svc.PublicCatalog(ctx, "no-existe"); !errors.Is(err, application.ErrNotFound) {
		t.Errorf("unknown slug: %v", err)
	}

	// Offerings for the public search: another business whose service is performed only by a
	// professional who is not bookable, and one with nothing at all.
	other, empty := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	admin := identity.Principal{UserID: uuid.Must(uuid.NewV7()), IsPlatformAdmin: true}
	s2, err := e.svc.CreateService(ctx, admin, other, application.ServiceInput{Name: "Corte", DurationMinutes: ptr(30), PriceMinor: ptr(int64(0))})
	if err != nil {
		t.Fatal(err)
	}
	ghost := uuid.Must(uuid.NewV7())
	e.profs.Add(other, ghost)
	assign(other, ghost, admin, s2.ID)
	ok, err := e.svc.BookableAmong(ctx, []uuid.UUID{e.biz, other, empty})
	if err != nil || !ok[e.biz] || ok[other] || ok[empty] {
		t.Errorf("bookable = %v %v", ok, err)
	}
	if ok, err := e.svc.BookableAmong(ctx, nil); err != nil || len(ok) != 0 {
		t.Errorf("no ids: %v %v", ok, err)
	}
	// Without the public dependencies the service fails loudly instead of hiding everything.
	bare := application.NewService(catalogtest.NewStore(), e.ids, catalogtest.Currencies{}, e.profs, nil)
	if _, err := bare.BookableAmong(ctx, []uuid.UUID{e.biz}); err == nil {
		t.Error("BookableAmong without public deps")
	}
	if _, err := bare.PublicCatalog(ctx, "barberia-centro"); err == nil {
		t.Error("PublicCatalog without public deps")
	}
}
