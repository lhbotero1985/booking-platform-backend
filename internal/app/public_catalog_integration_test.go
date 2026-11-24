//go:build integration

package app

import (
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"

	bizapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/application"
	catapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/application"
	idapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application/apptest"
	identitycontract "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	idpg "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/infrastructure/postgres"
	profapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/pgtest"
)

// TestPublicCatalogOnPostgres (T5.7): the public search lists only businesses with an ACTIVE
// service performed by a bookable professional, through the real modules and contracts; the
// public catalog shows that offer; removing the barber hides the business again.
func TestPublicCatalogOnPostgres(t *testing.T) {
	ctx := t.Context()
	pool := migratedPool(t)
	identity := idapp.NewService(idapp.Deps{
		Store: idpg.NewStore(pool, nil), Hasher: &apptest.Hasher{}, Mailer: &apptest.Mailer{}, Clock: clock.System{},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	biz := NewBusinessesService(pool, identity, NewGeographyService(pool, identity), NewSubscriptionsService(pool, identity))
	profs := NewProfessionalsService(pool, identity)
	catalog := NewCatalogService(pool, identity, biz, profs)
	LinkPublicCatalog(biz, catalog, profs, biz)

	city := pgtest.SeedYumboID // a city no other test fills with bookable businesses
	mk := func(name string) (identitycontract.Principal, uuid.UUID, string) {
		ownerID, _ := activeUser(t, pool, "ACTIVE")
		p := identitycontract.Principal{UserID: ownerID}
		b, err := biz.CreateOwnBusiness(ctx, p, bizapp.BusinessInput{Name: name, ContactPhone: "3001234567", CityID: city, AddressLine: "Calle 1"})
		if err != nil {
			t.Fatal(err)
		}
		return p, b.ID(), string(b.Slug())
	}
	search := func() []string {
		items, _, err := biz.SearchPublic(ctx, bizapp.SearchQuery{CityID: city, Limit: 50})
		if err != nil {
			t.Fatal(err)
		}
		out := make([]string, len(items))
		for i, b := range items {
			out[i] = b.Name()
		}
		return out
	}

	owner, b1, slug := mk("Barbería Con Oferta Yumbo")
	_, _, _ = mk("Barbería Vacía Yumbo")
	if got := search(); len(got) != 0 {
		t.Fatalf("nothing is bookable yet, search = %v", got)
	}

	_, barberEmail := activeUser(t, pool, "ACTIVE")
	juan, err := profs.Create(ctx, owner, b1, profapp.CreateInput{Email: barberEmail, DisplayName: "Juan", Bio: "Fades"})
	if err != nil {
		t.Fatal(err)
	}
	s, err := catalog.CreateService(ctx, owner, b1, catapp.ServiceInput{Name: "Corte + barba", DurationMinutes: ptrTo(45), PriceMinor: ptrTo(int64(3_500_000))})
	if err != nil {
		t.Fatal(err)
	}
	if got := search(); len(got) != 0 {
		t.Fatalf("service not performed by anyone, search = %v", got)
	}
	if _, err := catalog.ReplaceProfessionalServices(ctx, owner, b1, juan.ID, []uuid.UUID{s.ID}); err != nil {
		t.Fatal(err)
	}
	if got := search(); len(got) != 1 || got[0] != "Barbería Con Oferta Yumbo" {
		t.Fatalf("search = %v", got)
	}
	pc, err := catalog.PublicCatalog(ctx, slug)
	if err != nil || len(pc.Services) != 1 || pc.Services[0].PriceMinor != 3_500_000 || len(pc.Professionals) != 1 || pc.Professionals[0].DisplayName != "Juan" {
		t.Fatalf("public catalog: %+v %v", pc, err)
	}
	if _, err := catalog.PublicCatalog(ctx, "no-existe-en-yumbo"); !errors.Is(err, catapp.ErrNotFound) {
		t.Errorf("unknown slug: %v", err)
	}

	// The barber leaves the business: nothing bookable, hidden from the search again.
	if err := identity.RemoveMember(ctx, owner, b1, juan.UserID); err != nil {
		t.Fatal(err)
	}
	if got := search(); len(got) != 0 {
		t.Errorf("after the barber left, search = %v", got)
	}
	if pc, err := catalog.PublicCatalog(ctx, slug); err != nil || len(pc.Services) != 0 || len(pc.Professionals) != 0 {
		t.Errorf("public catalog after the barber left: %+v %v", pc, err)
	}
}
