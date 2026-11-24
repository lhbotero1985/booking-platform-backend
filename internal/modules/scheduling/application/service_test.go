package application_test

import (
	"context"
	"errors"
	"testing"
	"time"
	_ "time/tzdata" // same IANA database as the binaries (ADR-0005)

	"github.com/google/uuid"

	businesses "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/contract"
	catalog "github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/contract"
	professionals "github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/application/schedtest"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
)

type fakeBiz struct {
	pol businesses.BookingPolicy
	err error
}

func (f fakeBiz) PublicBookingPolicy(_ context.Context, slug string) (businesses.BookingPolicy, error) {
	if f.err != nil {
		return businesses.BookingPolicy{}, f.err
	}
	if slug != "barberia" {
		return businesses.BookingPolicy{}, businesses.ErrBusinessNotFound
	}
	return f.pol, nil
}

func (f fakeBiz) BookingPolicyOf(_ context.Context, id uuid.UUID) (businesses.BookingPolicy, error) {
	if id != f.pol.BusinessID {
		return businesses.BookingPolicy{}, businesses.ErrBusinessNotFound
	}
	return f.pol, f.err
}

func (f fakeBiz) PublicBookingPolicyOf(_ context.Context, id uuid.UUID) (businesses.BookingPolicy, error) {
	if f.err != nil {
		return businesses.BookingPolicy{}, f.err
	}
	if id != f.pol.BusinessID {
		return businesses.BookingPolicy{}, businesses.ErrBusinessNotFound
	}
	return f.pol, nil
}

type fakeCatalog struct {
	minutes int
	err     error
	got     [3]uuid.UUID
}

func (f *fakeCatalog) BookableService(_ context.Context, biz, svc, pro uuid.UUID) (catalog.BookableService, error) {
	f.got = [3]uuid.UUID{biz, svc, pro}
	return catalog.BookableService{Name: "Corte", DurationMinutes: f.minutes, PriceMinor: 2_500_000, Currency: "COP"}, f.err
}

type fakeAgenda struct {
	owners   map[uuid.UUID]uuid.UUID // professional → account
	locks    int
	lockErr  error
	agenda   professionals.DayAgenda
	err      error
	weekday  time.Weekday
	from, to time.Time
}

func (f *fakeAgenda) ProfessionalUserID(_ context.Context, _, professionalID uuid.UUID) (uuid.UUID, error) {
	u, ok := f.owners[professionalID]
	if !ok {
		return uuid.Nil, professionals.ErrProfessionalNotFound
	}
	return u, nil
}

func (f *fakeAgenda) LockAgenda(context.Context, uuid.UUID, uuid.UUID) error {
	f.locks++
	return f.lockErr
}

func (f *fakeAgenda) DayAgenda(_ context.Context, _, _ uuid.UUID, wd time.Weekday, from, to time.Time) (professionals.DayAgenda, error) {
	f.weekday, f.from, f.to = wd, from, to
	return f.agenda, f.err
}

func TestPublicAvailability(t *testing.T) {
	ctx := context.Background()
	biz, svc, pro := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	pol := businesses.BookingPolicy{BusinessID: biz, TimeZone: "America/New_York", SlotIntervalMinutes: 30, BufferMinutes: 0, MinLeadMinutes: 0, MaxAdvanceDays: 30}
	cat := &fakeCatalog{minutes: 30}
	ag := &fakeAgenda{agenda: professionals.DayAgenda{
		WorkingHours: []professionals.MinuteRange{{Start: 60, End: 240}},                                                                                // 01:00–04:00
		Absences:     []professionals.Interval{{Start: time.Date(2026, 3, 8, 7, 0, 0, 0, time.UTC), End: time.Date(2026, 3, 8, 7, 30, 0, 0, time.UTC)}}, // 03:00–03:30 EDT
	}}
	svcUnderTest := func(b fakeBiz) *application.Service {
		return application.NewService(application.Deps{Store: schedtest.NewStore(), Businesses: b, Catalog: cat, Professionals: ag, Clock: clock.NewFixed(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))})
	}
	s := svcUnderTest(fakeBiz{pol: pol})
	day, _ := domain.ParseLocalDate("2026-03-08") // DST starts in New York
	got, err := s.PublicAvailability(ctx, "barberia", svc, pro, day)
	if err != nil {
		t.Fatal(err)
	}
	if cat.got != [3]uuid.UUID{biz, svc, pro} || ag.weekday != time.Sunday {
		t.Errorf("contract calls: catalog %v, weekday %v", cat.got, ag.weekday)
	}
	// The absences are read for the whole local day: midnight EST to midnight EDT (23 h).
	if !ag.from.Equal(time.Date(2026, 3, 8, 5, 0, 0, 0, time.UTC)) || !ag.to.Equal(time.Date(2026, 3, 9, 4, 0, 0, 0, time.UTC)) {
		t.Errorf("absence period %v – %v", ag.from, ag.to)
	}
	var local []string
	for _, sl := range got.Slots {
		local = append(local, sl.Start.In(got.Location).Format("15:04"))
	}
	if len(local) != 3 || local[0] != "01:00" || local[1] != "01:30" || local[2] != "03:30" || got.DurationMinutes != 30 || got.Location.String() != "America/New_York" {
		t.Errorf("availability = %v %+v", local, got)
	}

	other := domain.LocalDate{Year: 2026, Month: 3, Day: 2}
	boom := errors.New("boom")
	for name, c := range map[string]struct {
		biz      fakeBiz
		slug     string
		catErr   error
		agErr    error
		date     domain.LocalDate
		want     error
		wantSame bool
	}{
		"unknown business":           {biz: fakeBiz{pol: pol}, slug: "otra", date: other, want: application.ErrNotFound},
		"service not bookable":       {biz: fakeBiz{pol: pol}, slug: "barberia", catErr: catalog.ErrNotBookable, date: other, want: application.ErrNotFound},
		"professional gone":          {biz: fakeBiz{pol: pol}, slug: "barberia", agErr: professionals.ErrProfessionalNotFound, date: other, want: application.ErrNotFound},
		"outside the booking window": {biz: fakeBiz{pol: pol}, slug: "barberia", date: domain.LocalDate{Year: 2026, Month: 2, Day: 28}, want: domain.ErrOutsideBookingWindow},
		"businesses fails":           {biz: fakeBiz{err: boom}, slug: "barberia", date: other, want: boom},
		"catalog fails":              {biz: fakeBiz{pol: pol}, slug: "barberia", catErr: boom, date: other, want: boom},
		"agenda fails":               {biz: fakeBiz{pol: pol}, slug: "barberia", agErr: boom, date: other, want: boom},
	} {
		cat.err, ag.err = c.catErr, c.agErr
		if _, err := svcUnderTest(c.biz).PublicAvailability(ctx, c.slug, svc, pro, c.date); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", name, err, c.want)
		}
	}
	cat.err, ag.err = nil, nil
	bad := pol
	bad.TimeZone = "Mars/Olympus"
	if _, err := svcUnderTest(fakeBiz{pol: bad}).PublicAvailability(ctx, "barberia", svc, pro, other); err == nil || errors.Is(err, application.ErrNotFound) {
		t.Errorf("invalid time zone: %v", err)
	}
}
