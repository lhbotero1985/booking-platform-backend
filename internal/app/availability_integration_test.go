//go:build integration

package app

import (
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	bizapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/application"
	catapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/application"
	idapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application/apptest"
	identitycontract "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	idpg "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/infrastructure/postgres"
	profapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/application"
	profdomain "github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/domain"
	schedapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/application"
	scheddomain "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/domain"
	schedpg "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/infrastructure/postgres"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/pgtest"
)

// TestPublicAvailabilityOnPostgres (T6.2): the public availability through the real modules
// and their SQL (schedule as time columns, absences by period, assignments, bookability).
func TestPublicAvailabilityOnPostgres(t *testing.T) {
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
	// Fixed "now" for the engine: Friday 1 March 2030, noon in Bogota.
	sched := schedapp.NewService(schedapp.Deps{Store: schedpg.NewStore(pool), Businesses: biz, Catalog: catalog, Professionals: profs, Customers: identity, Clock: clock.NewFixed(time.Date(2030, 3, 1, 17, 0, 0, 0, time.UTC))})

	ownerID, _ := activeUser(t, pool, "ACTIVE")
	owner := identitycontract.Principal{UserID: ownerID}
	b, err := biz.CreateOwnBusiness(ctx, owner, bizapp.BusinessInput{Name: "Barbería Agenda Yumbo", ContactPhone: "3001234567", CityID: pgtest.SeedYumboID, AddressLine: "Calle 1"})
	if err != nil {
		t.Fatal(err)
	}
	_, barberEmail := activeUser(t, pool, "ACTIVE")
	juan, err := profs.Create(ctx, owner, b.ID(), profapp.CreateInput{Email: barberEmail, DisplayName: "Juan"})
	if err != nil {
		t.Fatal(err)
	}
	r := func(s, e string) profdomain.RangeInput { return profdomain.RangeInput{Start: s, End: e} }
	if _, err := profs.ReplaceSchedule(ctx, owner, b.ID(), juan.ID, []profdomain.DayInput{
		{Weekday: "TUESDAY", WorkingHours: []profdomain.RangeInput{r("08:00", "10:00"), r("14:00", "15:00")}, Breaks: []profdomain.RangeInput{r("09:00", "09:20")}},
	}); err != nil {
		t.Fatal(err)
	}
	// Tuesday 5 March 2030, 14:15–14:45 in Bogota; another absence the next day must not count.
	for _, p := range [][2]time.Time{
		{time.Date(2030, 3, 5, 19, 15, 0, 0, time.UTC), time.Date(2030, 3, 5, 19, 45, 0, 0, time.UTC)},
		{time.Date(2030, 3, 6, 13, 0, 0, 0, time.UTC), time.Date(2030, 3, 6, 23, 0, 0, 0, time.UTC)},
	} {
		if _, err := profs.CreateAbsence(ctx, owner, b.ID(), juan.ID, profapp.AbsenceInput{Start: p[0], End: p[1], Reason: "Diligencia"}); err != nil {
			t.Fatal(err)
		}
	}
	svc, err := catalog.CreateService(ctx, owner, b.ID(), catapp.ServiceInput{Name: "Corte", DurationMinutes: ptrTo(30), PriceMinor: ptrTo(int64(2_500_000))})
	if err != nil {
		t.Fatal(err)
	}
	tuesday, _ := scheddomain.ParseLocalDate("2030-03-05")
	if _, err := sched.PublicAvailability(ctx, string(b.Slug()), svc.ID, juan.ID, tuesday); !errors.Is(err, schedapp.ErrNotFound) {
		t.Fatalf("service not assigned yet: %v", err)
	}
	if _, err := catalog.ReplaceProfessionalServices(ctx, owner, b.ID(), juan.ID, []uuid.UUID{svc.ID}); err != nil {
		t.Fatal(err)
	}
	got, err := sched.PublicAvailability(ctx, string(b.Slug()), svc.ID, juan.ID, tuesday)
	if err != nil {
		t.Fatal(err)
	}
	var hours []string
	for _, s := range got.Slots {
		hours = append(hours, s.Start.In(got.Location).Format("15:04"))
	}
	// 08:00–09:00 and 09:20–10:00 (break), 14:00–14:15 is too short, 14:45–15:00 too short.
	if want := "08:00 08:10 08:20 08:30 09:20 09:30"; strings.Join(hours, " ") != want || got.Location.String() != "America/Bogota" {
		t.Errorf("slots = %v (%s), want %s", hours, got.Location, want)
	}

	// The barber leaves the business: no longer bookable.
	if err := identity.RemoveMember(ctx, owner, b.ID(), juan.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := sched.PublicAvailability(ctx, string(b.Slug()), svc.ID, juan.ID, tuesday); !errors.Is(err, schedapp.ErrNotFound) {
		t.Errorf("after the barber left: %v", err)
	}
}
