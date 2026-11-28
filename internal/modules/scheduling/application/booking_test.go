package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	businesses "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/contract"
	catalog "github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/contract"
	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	professionals "github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/application/schedtest"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/authz"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
)

type fakeCustomers map[uuid.UUID]identity.Customer

func (f fakeCustomers) CustomerProfile(_ context.Context, id uuid.UUID) (identity.Customer, error) {
	c, ok := f[id]
	if !ok {
		return identity.Customer{}, identity.ErrPhoneRequired
	}
	return c, nil
}

// fakeAuthz grants roles per (business, account); anything else is authz.ErrNotFound.
type fakeAuthz map[[2]uuid.UUID]identity.Role

func (f fakeAuthz) RequireBusinessRole(_ context.Context, p identity.Principal, biz uuid.UUID, allowed ...identity.Role) (identity.Role, error) {
	role, ok := f[[2]uuid.UUID{biz, p.UserID}]
	if !ok {
		return "", authz.ErrNotFound
	}
	for _, a := range allowed {
		if a == role {
			return role, nil
		}
	}
	return "", authz.ErrForbidden
}

// fakeAdmins allows the principals flagged IsPlatformAdmin.
type fakeAdmins struct{}

func (fakeAdmins) RequirePlatformAdmin(_ context.Context, p identity.Principal) error {
	if !p.IsPlatformAdmin {
		return authz.ErrForbidden
	}
	return nil
}

type bookingEnv struct {
	clk      *clock.Fixed
	roles    fakeAuthz
	svc      *application.Service
	store    *schedtest.Store
	cat      *fakeCatalog
	ag       *fakeAgenda
	biz, pro uuid.UUID
	svcID    uuid.UUID
	ana, bob identity.Principal
	day      func(h, m int) time.Time
}

// Bogota, Tuesday 2030-03-05 09:00–12:00; "now" is Friday 2030-03-01 noon. Slots every 10 min,
// 10 min buffer, Corte 30 min.
func newBookingEnv(t *testing.T) bookingEnv {
	t.Helper()
	return newBookingEnvWith(t, nil)
}

// newBookingEnvWith lets a test change the business's policy.
func newBookingEnvWith(t *testing.T, policy func(*businesses.BookingPolicy)) bookingEnv {
	t.Helper()
	bog, _ := time.LoadLocation("America/Bogota")
	e := bookingEnv{
		store: schedtest.NewStore(), cat: &fakeCatalog{minutes: 30},
		ag:  &fakeAgenda{agenda: professionals.DayAgenda{WorkingHours: []professionals.MinuteRange{{Start: 540, End: 720}}}},
		biz: uuid.Must(uuid.NewV7()), pro: uuid.Must(uuid.NewV7()), svcID: uuid.Must(uuid.NewV7()),
		ana: identity.Principal{UserID: uuid.Must(uuid.NewV7())}, bob: identity.Principal{UserID: uuid.Must(uuid.NewV7())},
		day: func(h, m int) time.Time { return time.Date(2030, 3, 5, h, m, 0, 0, bog) },
	}
	pol := businesses.BookingPolicy{BusinessID: e.biz, TimeZone: "America/Bogota", SlotIntervalMinutes: 10, BufferMinutes: 10, MinLeadMinutes: 60, MaxAdvanceDays: 60,
		CancellationCutoffHours: 2, RescheduleCutoffHours: 2}
	if policy != nil {
		policy(&pol)
	}
	e.roles = fakeAuthz{}
	e.clk = clock.NewFixed(time.Date(2030, 3, 1, 17, 0, 0, 0, time.UTC))
	e.svc = application.NewService(application.Deps{
		Store: e.store, Authz: e.roles, Businesses: fakeBiz{pol: pol}, Catalog: e.cat, Professionals: e.ag, Bookable: e.ag,
		Customers: fakeCustomers{
			e.ana.UserID: {UserID: e.ana.UserID, FullName: "Ana Cliente", Phone: "+573001112233"},
			e.bob.UserID: {UserID: e.bob.UserID, FullName: "Bob Cliente", Phone: "+573004445566"},
		},
		Clock: e.clk, Admins: fakeAdmins{},
	})
	return e
}

func (e bookingEnv) in(start time.Time, key string) application.BookingInput {
	return application.BookingInput{BusinessID: e.biz, ServiceID: e.svcID, ProfessionalID: e.pro, Start: start, IdempotencyKey: key}
}

func TestBook(t *testing.T) {
	ctx := context.Background()
	e := newBookingEnv(t)
	a, err := e.svc.Book(ctx, e.ana, e.in(e.day(9, 0), "reserva-ana-0001"))
	if err != nil {
		t.Fatal(err)
	}
	if a.Status != domain.StatusPendingConfirmation || a.Customer.Name != "Ana Cliente" || a.Customer.Phone != "+573001112233" ||
		a.Service.Name != "Corte" || a.Service.PriceMinor != 2_500_000 || a.Service.Currency != "COP" || a.Buffer != 10*time.Minute ||
		!a.End.Equal(e.day(9, 30)) || a.Version != 1 || a.CreatedBy != e.ana.UserID {
		t.Errorf("appointment = %+v", a)
	}
	if e.ag.locks != 1 || len(e.store.Events) != 1 || e.store.Events[0].Type != domain.EventCreated || e.store.Events[0].Actor != e.ana.UserID {
		t.Errorf("locks %d, events %+v", e.ag.locks, e.store.Events)
	}

	// Same key and body: the same appointment, nothing new. Same key, other body: refused.
	again, err := e.svc.Book(ctx, e.ana, e.in(e.day(9, 0), "reserva-ana-0001"))
	if err != nil || again.ID != a.ID || len(e.store.Appointments) != 1 {
		t.Errorf("replay: %v %v (%d appointments)", again.ID, err, len(e.store.Appointments))
	}
	if _, err := e.svc.Book(ctx, e.ana, e.in(e.day(10, 0), "reserva-ana-0001")); !errors.Is(err, application.ErrIdempotencyKeyReused) {
		t.Errorf("key reused: %v", err)
	}
	// Bob's identical key is his own.
	if _, err := e.svc.Book(ctx, e.bob, e.in(e.day(9, 0), "reserva-ana-0001")); !errors.Is(err, application.ErrSlotUnavailable) {
		t.Errorf("Bob on Ana's slot: %v", err)
	}

	// 09:00–09:30 + 10 min buffer occupies until 09:40: 09:10…09:30 are gone (their own buffer
	// or start overlaps), 09:40 is free.
	av, err := e.svc.PublicAvailability(ctx, "barberia", e.svcID, e.pro, domain.LocalDateOf(e.day(9, 0), e.day(9, 0).Location()))
	if err != nil || len(av.Slots) == 0 || !av.Slots[0].Start.Equal(e.day(9, 40)) {
		t.Fatalf("availability after booking: %+v %v", av.Slots, err)
	}
	for name, c := range map[string]struct {
		start time.Time
		want  error
	}{
		"overlaps the booking":  {e.day(9, 20), application.ErrSlotUnavailable},
		"off the 10-min grid":   {e.day(9, 45), application.ErrSlotUnavailable},
		"outside working hours": {e.day(12, 0), application.ErrSlotUnavailable},
		"past the horizon":      {e.day(9, 0).AddDate(0, 3, 0), domain.ErrOutsideBookingWindow},
		"in the past":           {time.Date(2030, 2, 1, 15, 0, 0, 0, time.UTC), domain.ErrOutsideBookingWindow},
	} {
		if _, err := e.svc.Book(ctx, e.bob, e.in(c.start, "bob-"+uuid.NewString()[:8])); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", name, err, c.want)
		}
	}
	if b, err := e.svc.Book(ctx, e.bob, e.in(e.day(9, 40), "bob-0940-ok")); err != nil || b.Customer.Name != "Bob Cliente" {
		t.Errorf("Bob at 09:40: %+v %v", b, err)
	}
	if len(e.store.Appointments) != 2 {
		t.Errorf("appointments = %d", len(e.store.Appointments))
	}
}

func TestBookRejections(t *testing.T) {
	ctx := context.Background()
	e := newBookingEnv(t)
	nobody := identity.Principal{UserID: uuid.Must(uuid.NewV7())} // no phone
	if _, err := e.svc.Book(ctx, nobody, e.in(e.day(9, 0), "sin-telefono-1")); !errors.Is(err, application.ErrPhoneRequired) {
		t.Errorf("no phone: %v", err)
	}
	for _, k := range []string{"", "corta", "con espacios no", "ñññññññññ"} {
		if _, err := e.svc.Book(ctx, e.ana, e.in(e.day(9, 0), k)); !errors.Is(err, application.ErrInvalidIdempotencyKey) {
			t.Errorf("key %q: %v", k, err)
		}
	}
	other := e.in(e.day(9, 0), "otro-negocio-1")
	other.BusinessID = uuid.Must(uuid.NewV7())
	if _, err := e.svc.Book(ctx, e.ana, other); !errors.Is(err, application.ErrNotFound) {
		t.Errorf("unknown business: %v", err)
	}
	e.cat.err = catalog.ErrNotBookable
	if _, err := e.svc.Book(ctx, e.ana, e.in(e.day(9, 0), "no-reservable-1")); !errors.Is(err, application.ErrNotFound) {
		t.Errorf("service not bookable: %v", err)
	}
	e.cat.err, e.ag.lockErr = nil, professionals.ErrProfessionalNotFound
	if _, err := e.svc.Book(ctx, e.ana, e.in(e.day(9, 0), "sin-barbero-1")); !errors.Is(err, application.ErrNotFound) {
		t.Errorf("professional gone: %v", err)
	}
	boom := errors.New("boom")
	e.ag.lockErr = boom
	if _, err := e.svc.Book(ctx, e.ana, e.in(e.day(9, 0), "falla-lock-1")); !errors.Is(err, boom) {
		t.Errorf("lock failure: %v", err)
	}
	if len(e.store.Appointments) != 0 {
		t.Errorf("a rejected booking left %d appointments", len(e.store.Appointments))
	}
	// A failed attempt does not burn the key: the same key works once the problem is gone.
	e.ag.lockErr = nil
	if _, err := e.svc.Book(ctx, e.ana, e.in(e.day(9, 0), "falla-lock-1")); err != nil {
		t.Errorf("retry with the same key: %v", err)
	}
}
