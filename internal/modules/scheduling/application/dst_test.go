package application_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	businesses "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/contract"
	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	professionals "github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/application/schedtest"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
)

// TestBookingAcrossDST (availability-engine.md §7): bookings on the days the clocks change, in a
// business of New York: correct UTC instants, no duplicated or phantom slots; on the night the
// clocks go back, 01:00 is two different, bookable instants.
func TestBookingAcrossDST(t *testing.T) {
	ctx := context.Background()
	biz, svc, pro := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	customer := identity.Principal{UserID: uuid.Must(uuid.NewV7())}
	ag := &fakeAgenda{
		owners: map[uuid.UUID]uuid.UUID{pro: uuid.Must(uuid.NewV7())},
		agenda: professionals.DayAgenda{},
	}
	s := application.NewService(application.Deps{
		Store: schedtest.NewStore(), Authz: fakeAuthz{},
		Businesses: fakeBiz{pol: businesses.BookingPolicy{BusinessID: biz, TimeZone: "America/New_York", SlotIntervalMinutes: 60, MaxAdvanceDays: 365}},
		Catalog:    &fakeCatalog{minutes: 60}, Professionals: ag, Bookable: ag,
		Customers: fakeCustomers{customer.UserID: {UserID: customer.UserID, FullName: "Ann", Phone: "+12125550100"}},
		Clock:     clock.NewFixed(time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)),
	})
	book := func(start time.Time, key string) error {
		_, err := s.Book(ctx, customer, application.BookingInput{BusinessID: biz, ServiceID: svc, ProfessionalID: pro, Start: start, IdempotencyKey: key})
		return err
	}
	starts := func(date string) []time.Time {
		t.Helper()
		d, _ := domain.ParseLocalDate(date)
		av, err := s.PublicAvailability(ctx, "barberia", svc, pro, d)
		if err != nil {
			t.Fatal(err)
		}
		out := make([]time.Time, 0, len(av.Slots))
		seen := map[time.Time]bool{}
		for _, sl := range av.Slots {
			if seen[sl.Start] {
				t.Errorf("%s: duplicated slot %s", date, sl.Start)
			}
			seen[sl.Start] = true
			out = append(out, sl.Start)
		}
		return out
	}

	// Spring forward (8 March 2026, 02:00 → 03:00): 01:00–05:00 local is three real hours.
	ag.agenda = professionals.DayAgenda{WorkingHours: []professionals.MinuteRange{{Start: 60, End: 300}}}
	spring := starts("2026-03-08")
	want := []time.Time{time.Date(2026, 3, 8, 6, 0, 0, 0, time.UTC), time.Date(2026, 3, 8, 7, 0, 0, 0, time.UTC), time.Date(2026, 3, 8, 8, 0, 0, 0, time.UTC)}
	if len(spring) != 3 || !spring[0].Equal(want[0]) || !spring[1].Equal(want[1]) || !spring[2].Equal(want[2]) {
		t.Fatalf("spring forward slots = %v", spring)
	}
	if err := book(spring[1], "dst-primavera-1"); err != nil { // 03:00 EDT
		t.Fatal(err)
	}
	if after := starts("2026-03-08"); len(after) != 2 {
		t.Errorf("after booking 03:00: %v", after)
	}

	// Fall back (1 November 2026, 02:00 → 01:00): 00:00–03:00 local is four real hours; 01:00 EDT
	// and 01:00 EST are two slots, both bookable.
	ag.agenda = professionals.DayAgenda{WorkingHours: []professionals.MinuteRange{{Start: 0, End: 180}}}
	fall := starts("2026-11-01")
	if len(fall) != 4 {
		t.Fatalf("fall back slots = %v", fall)
	}
	ny, _ := time.LoadLocation("America/New_York")
	if fall[1].In(ny).Format("15:04") != "01:00" || fall[2].In(ny).Format("15:04") != "01:00" || !fall[2].Equal(fall[1].Add(time.Hour)) {
		t.Fatalf("the repeated hour: %v / %v", fall[1].In(ny), fall[2].In(ny))
	}
	if err := book(fall[1], "dst-otono-edt"); err != nil {
		t.Fatal(err)
	}
	if err := book(fall[2], "dst-otono-est"); err != nil {
		t.Fatalf("the second 01:00 is another instant: %v", err)
	}
	if after := starts("2026-11-01"); len(after) != 2 {
		t.Errorf("after booking both 01:00: %v", after)
	}
}
