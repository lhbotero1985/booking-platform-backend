package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	businesses "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/contract"
	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	professionals "github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/application/schedtest"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/authz"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
)

type fakeSchedules map[uuid.UUID]professionals.WeekSchedule

func (f fakeSchedules) WeekSchedules(context.Context, uuid.UUID, time.Time, time.Time) (map[uuid.UUID]professionals.WeekSchedule, error) {
	return f, nil
}

type fakeCurrencies string

func (f fakeCurrencies) CurrencyOf(context.Context, uuid.UUID) (string, error) { return string(f), nil }

type fakeMemberships map[uuid.UUID][]identity.Membership

func (f fakeMemberships) MyMemberships(_ context.Context, p identity.Principal) ([]identity.Membership, error) {
	return f[p.UserID], nil
}

// TestStats (P-12): November 2026 in Bogotá with known appointments, schedules and absences.
func TestStats(t *testing.T) {
	ctx := context.Background()
	bog, _ := time.LoadLocation("America/Bogota")
	id := func() uuid.UUID { return uuid.Must(uuid.NewV7()) }
	biz, otherBiz, corte, barba, laura, pedro := id(), id(), id(), id(), id(), id()
	owner, barber, outsider := identity.Principal{UserID: id()}, identity.Principal{UserID: id()}, identity.Principal{UserID: id()}
	juan, boss := id(), id()
	at := func(m time.Month, d, h int) time.Time { return time.Date(2026, m, d, h, 0, 0, 0, bog) }

	store := schedtest.NewStore()
	add := func(b, pro, svc, customer uuid.UUID, start time.Time, st domain.Status, minutes int, price int64) {
		a := domain.Appointment{ID: id(), BusinessID: b, ProfessionalID: pro, Start: start, End: start.Add(time.Duration(minutes) * time.Minute), Status: st,
			Service:  domain.ServiceSnapshot{ServiceID: svc, Name: map[uuid.UUID]string{corte: "Corte", barba: "Barba"}[svc], DurationMinutes: minutes, PriceMinor: price, Currency: "COP"},
			Customer: domain.Customer{CustomerID: customer}}
		store.Appointments[a.ID] = a
	}
	add(biz, juan, corte, laura, at(10, 20, 9), domain.StatusCompleted, 30, 30_000) // Laura's first visit, before the range
	add(biz, juan, corte, laura, at(11, 3, 9), domain.StatusCompleted, 30, 30_000)
	add(biz, juan, corte, pedro, at(11, 4, 9), domain.StatusCancelled, 30, 30_000)
	add(biz, juan, corte, pedro, at(11, 5, 9), domain.StatusNoShow, 30, 30_000)
	add(biz, boss, barba, pedro, at(11, 3, 10), domain.StatusCompleted, 20, 15_000) // Pedro's first visit
	add(biz, juan, corte, laura, at(12, 1, 9), domain.StatusConfirmed, 30, 30_000)
	add(otherBiz, id(), corte, id(), at(11, 3, 9), domain.StatusCompleted, 30, 99_000)

	// Juan: Tuesdays 09:00–12:00 with a break 10:00–10:30 (150 min), absent on Tuesday 17.
	// The owner also serves: Mondays 09:00–11:00, profile created on the 15th.
	tuesday, monday := professionals.DaySchedule{WorkingHours: []professionals.MinuteRange{{Start: 540, End: 720}}, Breaks: []professionals.MinuteRange{{Start: 600, End: 630}}},
		professionals.DaySchedule{WorkingHours: []professionals.MinuteRange{{Start: 540, End: 660}}}
	js, bs := professionals.WeekSchedule{CreatedAt: at(1, 1, 0)}, professionals.WeekSchedule{CreatedAt: at(11, 15, 8)}
	js.Days[time.Tuesday], bs.Days[time.Monday] = tuesday, monday
	js.Absences = []professionals.Interval{{Start: at(11, 17, 0), End: at(11, 18, 0)}}

	roles := fakeAuthz{{biz, owner.UserID}: identity.RoleOwner, {biz, barber.UserID}: identity.RoleProfessional}
	ag := &fakeAgenda{owners: map[uuid.UUID]uuid.UUID{juan: barber.UserID, boss: owner.UserID}, names: map[uuid.UUID]string{juan: "Juan", boss: "El Dueño"}}
	svc := application.NewService(application.Deps{
		Store: store, Authz: roles, Businesses: fakeBiz{pol: businesses.BookingPolicy{BusinessID: biz, TimeZone: "America/Bogota"}},
		Professionals: ag, Clock: clock.NewFixed(time.Date(2026, 11, 27, 17, 0, 0, 0, time.UTC)),
		Schedules: fakeSchedules{juan: js, boss: bs}, Currencies: fakeCurrencies("COP"),
		Memberships: fakeMemberships{owner.UserID: {{BusinessID: biz, Role: identity.RoleOwner}}, barber.UserID: {{BusinessID: biz, Role: identity.RoleProfessional}}},
	})
	nov := func(g, pro string) application.StatsQuery {
		return application.StatsQuery{From: "2026-11-01", To: "2026-11-30", GroupBy: g, ProfessionalID: pro}
	}

	s, err := svc.BusinessStats(ctx, owner, biz, nov("month", ""))
	if err != nil {
		t.Fatal(err)
	}
	// Tuesdays 3, 10, 24 (the 17th absent): 450; Mondays 16, 23, 30: 360.
	tot := *s.Totals
	if tot.Appointments != 3 || tot.Completed != 2 || tot.Canceled != 1 || tot.NoShow != 1 || tot.RevenueMinor != 45_000 ||
		tot.AverageTicketMinor != 22_500 || tot.CustomersServed != 2 || tot.NewCustomers != 1 || tot.BookedMinutes != 50 ||
		tot.AvailableMinutes != 810 || tot.OccupancyPercent == nil || *tot.OccupancyPercent != 6 || s.Currency != "COP" {
		t.Errorf("owner totals %+v %q", tot, s.Currency)
	}
	if len(s.Series) != 1 || s.Series[0].Period != "2026-11" || s.Series[0].Figures.RevenueMinor != 45_000 {
		t.Errorf("series %+v", s.Series)
	}
	if len(s.ByProfessional) != 2 || s.ByProfessional[0].DisplayName != "El Dueño" || s.ByProfessional[0].Figures.AvailableMinutes != 360 ||
		s.ByProfessional[1].Figures.RevenueMinor != 30_000 || s.ByProfessional[1].Figures.AvailableMinutes != 450 {
		t.Errorf("by professional %+v", s.ByProfessional)
	}
	if len(s.ByService) != 2 || s.ByService[0].Name != "Barba" || s.ByService[1].Figures.Appointments != 2 || s.ByService[1].Figures.OccupancyPercent != nil {
		t.Errorf("by service %+v", s.ByService)
	}
	days, _ := svc.BusinessStats(ctx, owner, biz, nov("", ""))
	if len(days.Series) != 30 || days.Series[2].Period != "2026-11-03" || days.Series[2].Figures.RevenueMinor != 45_000 || days.Series[16].Figures.AvailableMinutes != 0 {
		t.Errorf("by day: %d periods %+v", len(days.Series), days.Series[2])
	}
	one, _ := svc.BusinessStats(ctx, owner, biz, nov("month", juan.String()))
	if one.Totals.RevenueMinor != 30_000 || one.Totals.AvailableMinutes != 450 || len(one.ByProfessional) != 1 {
		t.Errorf("owner, Juan only: %+v %+v", one.Totals, one.ByProfessional)
	}
	mine, err := svc.BusinessStats(ctx, barber, biz, nov("month", ""))
	if err != nil || mine.Totals.RevenueMinor != 30_000 || mine.Totals.NoShow != 1 || mine.ByProfessional != nil || mine.Totals.NewCustomers != 0 {
		t.Errorf("barber: %+v %+v %v", mine.Totals, mine.ByProfessional, err)
	}

	// Authorization before validation; ranges and parameters.
	for name, c := range map[string]struct {
		p    identity.Principal
		q    application.StatsQuery
		want error
	}{
		"barber asks for the owner":   {barber, nov("month", boss.String()), authz.ErrForbidden},
		"outsider":                    {outsider, nov("month", ""), authz.ErrNotFound},
		"outsider, invalid query":     {outsider, application.StatsQuery{From: "x"}, authz.ErrNotFound},
		"to before from":              {owner, application.StatsQuery{From: "2026-11-30", To: "2026-11-01"}, application.ErrInvalidRange},
		"367 days by day":             {owner, application.StatsQuery{From: "2026-01-01", To: "2027-01-02"}, application.ErrInvalidRange},
		"more than 5 years by month":  {owner, application.StatsQuery{From: "2020-01-01", To: "2025-01-03", GroupBy: "month"}, application.ErrInvalidRange},
		"5 years by year":             {owner, application.StatsQuery{From: "2021-11-28", To: "2026-11-27", GroupBy: "year"}, nil},
		"366 days by day (leap year)": {owner, application.StatsQuery{From: "2028-01-01", To: "2028-12-31"}, nil},
	} {
		if _, err := svc.BusinessStats(ctx, c.p, biz, c.q); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", name, err, c.want)
		}
	}
	var verr *application.ValidationError
	for name, q := range map[string]application.StatsQuery{
		"bad dates":            {From: "01-11-2026", To: "2026-13-01"},
		"bad grouping":         {From: "2026-11-01", To: "2026-11-30", GroupBy: "week"},
		"bad professional":     nov("month", "juan"),
		"unknown professional": nov("month", id().String()),
	} {
		if _, err := svc.BusinessStats(ctx, owner, biz, q); !errors.As(err, &verr) {
			t.Errorf("%s: %v", name, err)
		}
	}

	// Consolidated: the owner's business; the barber owns none.
	all, err := svc.MyStats(ctx, owner, nov("month", ""))
	if err != nil || all.Currency != "COP" || all.Totals.RevenueMinor != 45_000 || all.Totals.AvailableMinutes != 810 ||
		len(all.ByBusiness) != 1 || all.ByBusiness[0].Name != "Barbería Centro" || len(all.TotalsByCurrency) != 1 || all.ByProfessional != nil || len(all.ByService) != 2 {
		t.Errorf("my stats: %+v %v", all, err)
	}
	none, err := svc.MyStats(ctx, barber, nov("month", ""))
	if err != nil || none.Currency != "" || none.Totals == nil || none.Totals.Appointments != 0 || len(none.ByBusiness) != 0 || len(none.Series) != 1 {
		t.Errorf("not an owner: %+v %v", none, err)
	}
	if _, err := svc.MyStats(ctx, owner, application.StatsQuery{From: "2026-11-30", To: "2026-11-01"}); !errors.Is(err, application.ErrInvalidRange) {
		t.Errorf("my stats, reversed range: %v", err)
	}
}
