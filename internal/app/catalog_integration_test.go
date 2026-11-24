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
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/pgtest"
)

// TestCatalogOnPostgres (T5.5): services priced in the business currency, names unique per
// business ignoring case (also under concurrency), categories of the same business only, and
// deleting a category leaves its services uncategorized.
func TestCatalogOnPostgres(t *testing.T) {
	ctx := t.Context()
	pool := migratedPool(t)
	identity := idapp.NewService(idapp.Deps{
		Store: idpg.NewStore(pool, nil), Hasher: &apptest.Hasher{}, Mailer: &apptest.Mailer{}, Clock: clock.System{},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	biz := NewBusinessesService(pool, identity, NewGeographyService(pool, identity), NewSubscriptionsService(pool, identity))
	catalog := NewCatalogService(pool, identity, biz)

	ownerID, _ := activeUser(t, pool, "ACTIVE")
	owner := identitycontract.Principal{UserID: ownerID}
	b, err := biz.CreateOwnBusiness(ctx, owner, bizapp.BusinessInput{Name: "Barbería Catálogo", ContactPhone: "3001234567", CityID: pgtest.SeedCaliID, AddressLine: "Calle 1"})
	if err != nil {
		t.Fatal(err)
	}
	barba, err := catalog.CreateCategory(ctx, owner, b.ID(), catapp.CategoryInput{Name: "Barba"})
	if err != nil {
		t.Fatal(err)
	}
	s, err := catalog.CreateService(ctx, owner, b.ID(), catapp.ServiceInput{
		Name: "Corte + barba", CategoryID: barba.ID, DurationMinutes: ptrTo(45), PriceMinor: ptrTo(int64(3_500_000)),
	})
	if err != nil || s.Currency != "COP" {
		t.Fatalf("create service: %+v %v", s, err)
	}
	got, err := catalog.GetService(ctx, owner, b.ID(), s.ID)
	if err != nil || got.Currency != "COP" || got.CategoryID != barba.ID || got.PriceMinor != 3_500_000 || got.DurationMinutes != 45 {
		t.Fatalf("read back: %+v %v", got, err)
	}
	if _, err := catalog.CreateCategory(ctx, owner, b.ID(), catapp.CategoryInput{Name: "BARBA"}); !errors.Is(err, catapp.ErrNameTaken) {
		t.Errorf("category name ignoring case: %v", err)
	}
	if _, err := catalog.UpdateService(ctx, owner, b.ID(), s.ID, catapp.ServicePatch{Name: ptrTo("Corte + barba"), PriceMinor: ptrTo(int64(4_000_000))}); err != nil {
		t.Errorf("update: %v", err)
	}

	// Another business may reuse the names; its category is not usable here.
	otherOwner, _ := activeUser(t, pool, "ACTIVE")
	other := identitycontract.Principal{UserID: otherOwner}
	b2, err := biz.CreateOwnBusiness(ctx, other, bizapp.BusinessInput{Name: "Otra Barbería", ContactPhone: "3001234567", CityID: pgtest.SeedCaliID, AddressLine: "Calle 2"})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := catalog.CreateCategory(ctx, other, b2.ID(), catapp.CategoryInput{Name: "Barba"})
	if err != nil {
		t.Fatalf("same category name in another business: %v", err)
	}
	if _, err := catalog.CreateService(ctx, other, b2.ID(), catapp.ServiceInput{Name: "Corte + barba", DurationMinutes: ptrTo(30), PriceMinor: ptrTo(int64(0))}); err != nil {
		t.Errorf("same service name in another business: %v", err)
	}
	var verr *catapp.ValidationError
	if _, err := catalog.UpdateService(ctx, owner, b.ID(), s.ID, catapp.ServicePatch{CategoryID: &foreign.ID}); !errors.As(err, &verr) {
		t.Errorf("foreign category: %v", err)
	}

	// Schema: composite FK on the category, duration and price rules, currency format.
	for _, tc := range []struct {
		constraint string
		category   *uuid.UUID
		duration   int
		price      int64
	}{
		{"services_category_fk", &foreign.ID, 30, 0},
		{"services_duration_valid", nil, 42, 0},
		{"services_price_valid", nil, 30, 2_500_050},
	} {
		_, err := pool.Exec(ctx, `INSERT INTO services (id, business_id, category_id, name, duration_minutes, price_minor, currency, status, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, 'COP', 'ACTIVE', now(), now())`, uuid.Must(uuid.NewV7()), b.ID(), tc.category, "Prueba "+tc.constraint, tc.duration, tc.price)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.ConstraintName != tc.constraint {
			t.Errorf("%s: %v", tc.constraint, err)
		}
	}

	// Deleting the category keeps the service, uncategorized (ON DELETE SET NULL (category_id)).
	if _, err := catalog.UpdateService(ctx, owner, b.ID(), s.ID, catapp.ServicePatch{CategoryID: &barba.ID}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.DeleteCategory(ctx, owner, b.ID(), barba.ID); err != nil {
		t.Fatal(err)
	}
	if got, err := catalog.GetService(ctx, owner, b.ID(), s.ID); err != nil || got.CategoryID != uuid.Nil {
		t.Errorf("after deleting the category: %+v %v", got, err)
	}

	// Concurrency: the same new name created at once → exactly one service.
	const n = 6
	var wg sync.WaitGroup
	results := make(chan error, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			name := "Afeitado clásico"
			if i%2 == 1 {
				name = "AFEITADO CLÁSICO"
			}
			_, err := catalog.CreateService(ctx, owner, b.ID(), catapp.ServiceInput{Name: name, DurationMinutes: ptrTo(30), PriceMinor: ptrTo(int64(2_000_000))})
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	ok := 0
	for err := range results {
		switch {
		case err == nil:
			ok++
		case !errors.Is(err, catapp.ErrNameTaken):
			t.Errorf("concurrent create: %v", err)
		}
	}
	if ok != 1 {
		t.Errorf("concurrent creates stored %d services", ok)
	}
	if n := count(t, pool, `SELECT count(*) FROM audit_log WHERE business_id = $1 AND action LIKE 'catalog.%'`, b.ID()); n != 5 { // category, service, price, delete, one concurrent create (no-ops and failures are not audited)
		t.Errorf("catalog audit entries = %d, want 5", n)
	}
}
