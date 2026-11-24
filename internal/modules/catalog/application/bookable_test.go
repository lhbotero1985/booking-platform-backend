package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/application/catalogtest"
	catalog "github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/contract"
	professionals "github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/contract"
)

// TestBookableService (T6.2): the duration customers can book, only for an ACTIVE service the
// professional performs while bookable.
func TestBookableService(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	juan, pedro := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	e.profs.Add(e.biz, juan)
	e.profs.Add(e.biz, pedro)
	pub := &catalogtest.Public{Bookable: map[uuid.UUID][]professionals.PublicProfessional{e.biz: {{ID: juan, BusinessID: e.biz, DisplayName: "Juan"}}}}
	e.svc.SetPublicDeps(application.PublicDeps{Directory: pub, Professionals: pub})
	mk := func(name string) uuid.UUID {
		s, err := e.svc.CreateService(ctx, e.owner, e.biz, application.ServiceInput{Name: name, DurationMinutes: ptr(45), PriceMinor: ptr(int64(3_500_000))})
		if err != nil {
			t.Fatal(err)
		}
		return s.ID
	}
	corte, barba, tinte := mk("Corte + barba"), mk("Barba"), mk("Tinte")
	if _, err := e.svc.ReplaceProfessionalServices(ctx, e.owner, e.biz, juan, []uuid.UUID{corte, tinte}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.ReplaceProfessionalServices(ctx, e.owner, e.biz, pedro, []uuid.UUID{corte}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.UpdateService(ctx, e.owner, e.biz, tinte, application.ServicePatch{Status: ptr("INACTIVE")}); err != nil {
		t.Fatal(err)
	}
	if m, err := e.svc.BookableService(ctx, e.biz, corte, juan); err != nil || m != (catalog.BookableService{Name: "Corte + barba", DurationMinutes: 45, PriceMinor: 3_500_000, Currency: "COP"}) {
		t.Errorf("bookable: %+v %v", m, err)
	}
	for name, c := range map[string][3]uuid.UUID{
		"not performed by him":    {e.biz, barba, juan},
		"inactive service":        {e.biz, tinte, juan},
		"professional not public": {e.biz, corte, pedro},
		"unknown service":         {e.biz, uuid.Must(uuid.NewV7()), juan},
		"another business":        {uuid.Must(uuid.NewV7()), corte, juan},
	} {
		if _, err := e.svc.BookableService(ctx, c[0], c[1], c[2]); !errors.Is(err, catalog.ErrNotBookable) {
			t.Errorf("%s: %v", name, err)
		}
	}
	bare := application.NewService(catalogtest.NewStore(), e.ids, catalogtest.Currencies{}, e.profs, nil)
	if _, err := bare.BookableService(ctx, e.biz, corte, juan); err == nil || errors.Is(err, catalog.ErrNotBookable) {
		t.Errorf("without public deps: %v", err)
	}
}
