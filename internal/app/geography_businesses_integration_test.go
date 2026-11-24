//go:build integration

package app

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	bizapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/domain"
	bizpg "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/infrastructure/postgres"
	geoapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/application/geotest"
	identitycontract "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/migrate"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/pgtest"
)

var valle = pgtest.SeedValleID

func migratedPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	c, err := pgtest.Start(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Terminate)
	pool, err := pgxpool.New(t.Context(), c.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	sqlDB := stdlib.OpenDBFromPool(pool)
	t.Cleanup(func() { _ = sqlDB.Close() })
	m, err := migrate.New(sqlDB)
	if err == nil {
		_, err = m.Up(t.Context())
	}
	if err != nil {
		t.Fatal(err)
	}
	return pool
}

func testBusiness(t *testing.T, city uuid.UUID) *domain.Business {
	t.Helper()
	b, err := domain.NewBusiness(domain.NewBusinessParams{
		ID: uuid.Must(uuid.NewV7()), Name: "Barbería Palmira", Slug: "barberia-palmira", Status: domain.StatusActive,
		ContactPhone: "+573001234567",
		Location:     domain.Location{CountryCode: "CO", DivisionID: valle, CityID: city, AddressLine: "Calle 1"},
		TimeZone:     "America/Bogota", Currency: "COP", Policy: domain.DefaultBookingPolicy(), Reminders: domain.DefaultReminders(),
		Now: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestGeographyCannotDeactivateAreasInUse: the composition root wires the businesses store into
// geography, so an area with active businesses cannot be deactivated.
func TestGeographyCannotDeactivateAreasInUse(t *testing.T) {
	ctx := t.Context()
	pool := migratedPool(t)
	biz := bizpg.NewStore(pool)
	geo := NewGeographyService(pool, geotest.Authorizer{})
	admin := identitycontract.Principal{UserID: uuid.Must(uuid.NewV7()), IsPlatformAdmin: true}

	// A fresh city with one active business.
	city, err := geo.CreateCity(ctx, admin, geoapp.CityInput{DivisionID: valle, Name: "Palmira " + uuid.NewString()[:6], TimeZone: "America/Bogota"})
	if err != nil {
		t.Fatal(err)
	}
	b := testBusiness(t, city.ID)
	if err := biz.WithinTx(ctx, func(tx bizapp.Tx) error { return tx.InsertBusiness(ctx, b) }); err != nil {
		t.Fatal(err)
	}
	if n, err := biz.ActiveBusinessesInCity(ctx, city.ID); err != nil || n != 1 {
		t.Fatalf("count = %d, %v", n, err)
	}
	off := false
	if _, err := geo.UpdateCity(ctx, admin, city.ID, geoapp.CityPatch{Active: &off}); !errors.Is(err, geoapp.ErrInUse) {
		t.Errorf("deactivating a city with an active business: %v, want ErrInUse", err)
	}
	if _, err := geo.UpdateDivision(ctx, admin, valle, geoapp.DivisionPatch{Active: &off}); !errors.Is(err, geoapp.ErrInUse) {
		t.Errorf("deactivating its division: %v", err)
	}
	if _, err := geo.UpdateCountry(ctx, admin, "CO", geoapp.CountryPatch{Active: &off}); !errors.Is(err, geoapp.ErrInUse) {
		t.Errorf("deactivating its country: %v", err)
	}

	// Once the business is suspended the city can be deactivated.
	b.Suspend(time.Now().UTC())
	if err := biz.WithinTx(ctx, func(tx bizapp.Tx) error { return tx.UpdateBusiness(ctx, b) }); err != nil {
		t.Fatal(err)
	}
	if _, err := geo.UpdateCity(ctx, admin, city.ID, geoapp.CityPatch{Active: &off}); err != nil {
		t.Errorf("deactivating a city with only suspended businesses: %v", err)
	}
	p, err := geo.PlaceOfCity(ctx, city.ID)
	if err != nil || p.Active || p.DivisionID != valle || p.Currency != "COP" {
		t.Errorf("PlaceOfCity = %+v, %v", p, err)
	}
}
