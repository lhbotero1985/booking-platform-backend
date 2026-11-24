//go:build integration

package app

import (
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

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

// TestProfessionalServicesOnPostgres (T5.6): the OWNER assigns services to a professional, only
// within the same business (application and composite FKs), and concurrent replacements never
// mix two lists.
func TestProfessionalServicesOnPostgres(t *testing.T) {
	ctx := t.Context()
	pool := migratedPool(t)
	identity := idapp.NewService(idapp.Deps{
		Store: idpg.NewStore(pool, nil), Hasher: &apptest.Hasher{}, Mailer: &apptest.Mailer{}, Clock: clock.System{},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	biz := NewBusinessesService(pool, identity, NewGeographyService(pool, identity), NewSubscriptionsService(pool, identity))
	profs := NewProfessionalsService(pool, identity)
	catalog := NewCatalogService(pool, identity, biz, profs)

	ownerID, _ := activeUser(t, pool, "ACTIVE")
	_, barberEmail := activeUser(t, pool, "ACTIVE")
	owner := identitycontract.Principal{UserID: ownerID}
	b, err := biz.CreateOwnBusiness(ctx, owner, bizapp.BusinessInput{Name: "Barbería Asignaciones", ContactPhone: "3001234567", CityID: pgtest.SeedCaliID, AddressLine: "Calle 1"})
	if err != nil {
		t.Fatal(err)
	}
	juan, err := profs.Create(ctx, owner, b.ID(), profapp.CreateInput{Email: barberEmail, DisplayName: "Juan"})
	if err != nil {
		t.Fatal(err)
	}
	const n = 8
	services := make([]uuid.UUID, n)
	for i := range n {
		s, err := catalog.CreateService(ctx, owner, b.ID(), catapp.ServiceInput{Name: "Servicio " + string(rune('A'+i)), DurationMinutes: ptrTo(30), PriceMinor: ptrTo(int64(1_000_000))})
		if err != nil {
			t.Fatal(err)
		}
		services[i] = s.ID
	}

	list, err := catalog.ReplaceProfessionalServices(ctx, owner, b.ID(), juan.ID, services[:2])
	if err != nil || len(list) != 2 || list[0].Name != "Servicio A" {
		t.Fatalf("assign: %v %v", len(list), err)
	}
	got, err := catalog.ProfessionalServices(ctx, identitycontract.Principal{UserID: juan.UserID}, b.ID(), juan.ID)
	if err != nil || len(got) != 2 {
		t.Fatalf("barber reads: %d %v", len(got), err)
	}

	// Another business: its service cannot be assigned here, nor this professional there.
	otherOwner, _ := activeUser(t, pool, "ACTIVE")
	other := identitycontract.Principal{UserID: otherOwner}
	b2, err := biz.CreateOwnBusiness(ctx, other, bizapp.BusinessInput{Name: "Otra Barbería", ContactPhone: "3001234567", CityID: pgtest.SeedCaliID, AddressLine: "Calle 2"})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := catalog.CreateService(ctx, other, b2.ID(), catapp.ServiceInput{Name: "Ajeno", DurationMinutes: ptrTo(30), PriceMinor: ptrTo(int64(0))})
	if err != nil {
		t.Fatal(err)
	}
	var verr *catapp.ValidationError
	if _, err := catalog.ReplaceProfessionalServices(ctx, owner, b.ID(), juan.ID, []uuid.UUID{foreign.ID}); !errors.As(err, &verr) {
		t.Errorf("foreign service: %v", err)
	}
	if _, err := catalog.ReplaceProfessionalServices(ctx, other, b2.ID(), juan.ID, []uuid.UUID{foreign.ID}); !errors.Is(err, catapp.ErrNotFound) {
		t.Errorf("professional through another business: %v", err)
	}
	for _, tc := range []struct {
		constraint      string
		biz, svc, profe uuid.UUID
	}{
		{"service_professionals_service_fk", b.ID(), foreign.ID, juan.ID},
		{"service_professionals_professional_fk", b2.ID(), foreign.ID, juan.ID},
	} {
		_, err := pool.Exec(ctx, `INSERT INTO service_professionals (business_id, service_id, professional_id, created_at) VALUES ($1, $2, $3, now())`, tc.biz, tc.svc, tc.profe)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.ConstraintName != tc.constraint {
			t.Errorf("%s: %v", tc.constraint, err)
		}
	}

	// Concurrency: N different lists at once; the result is exactly one of them.
	var wg sync.WaitGroup
	errs := make(chan error, n)
	lists := make([][]uuid.UUID, n)
	for i := range n {
		lists[i] = []uuid.UUID{services[i], services[(i+1)%n], services[(i+3)%n]}
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := catalog.ReplaceProfessionalServices(ctx, owner, b.ID(), juan.ID, lists[i])
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("concurrent replace: %v", err)
		}
	}
	final, err := catalog.ProfessionalServices(ctx, owner, b.ID(), juan.ID)
	if err != nil {
		t.Fatal(err)
	}
	matches := 0
	for _, l := range lists {
		set := map[uuid.UUID]bool{}
		for _, id := range l {
			set[id] = true
		}
		same := len(final) == len(set)
		for _, s := range final {
			same = same && set[s.ID]
		}
		if same {
			matches++
		}
	}
	if matches != 1 {
		t.Errorf("final list is not one of the submitted lists (%d services)", len(final))
	}
}
