//go:build integration

package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	bizapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/application"
	catapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/application"
	idapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application/apptest"
	identitycontract "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	idpg "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/infrastructure/postgres"
	profapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/application"
	profdomain "github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/domain"
	schedapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/application"
	schedcontract "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/contract"
	scheddomain "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/domain"
	schedpg "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/infrastructure/postgres"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/pgtest"
)

type bookingWorld struct {
	pool     *pgxpool.Pool
	identity *idapp.Service
	catalog  *catapp.Service
	profs    *profapp.Service
	clk      *clock.Fixed
	sched    *schedapp.Service
	owner    identitycontract.Principal
	barber   identitycontract.Principal
	biz      uuid.UUID
	slug     string
	pro      uuid.UUID
	svc      uuid.UUID
	bogota   *time.Location
}

// newBookingWorld: a public business in Cali whose barber works Tuesdays 09:00–12:00 and
// performs "Corte" (40 min). Booking policy: every 10 min, no buffer, 60 min notice. The engine's
// "now" is Friday 1 March 2030, so Tuesday 5 March is bookable.
func newBookingWorld(t *testing.T) bookingWorld {
	t.Helper()
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
	w := bookingWorld{pool: pool, identity: identity, catalog: catalog, profs: profs, clk: clock.NewFixed(time.Date(2030, 3, 1, 17, 0, 0, 0, time.UTC))}
	w.bogota, _ = time.LoadLocation("America/Bogota")
	w.sched = schedapp.NewService(schedapp.Deps{
		Store: schedpg.NewStore(pool), Authz: identity, Businesses: biz, Catalog: catalog, Professionals: profs, Bookable: profs, Customers: identity,
		Clock: w.clk,
	})
	LinkAgendaGuard(profs, w.sched)

	ownerID, _ := activeUser(t, pool, "ACTIVE")
	w.owner = identitycontract.Principal{UserID: ownerID}
	b, err := biz.CreateOwnBusiness(ctx, w.owner, bizapp.BusinessInput{Name: "Barbería Reservas", ContactPhone: "3001234567", CityID: pgtest.SeedCaliID, AddressLine: "Calle 1"})
	if err != nil {
		t.Fatal(err)
	}
	w.biz, w.slug = b.ID(), string(b.Slug())
	barberID, barberEmail := activeUser(t, pool, "ACTIVE")
	w.barber = identitycontract.Principal{UserID: barberID}
	juan, err := profs.Create(ctx, w.owner, w.biz, profapp.CreateInput{Email: barberEmail, DisplayName: "Juan"})
	if err != nil {
		t.Fatal(err)
	}
	w.pro = juan.ID
	if _, err := profs.ReplaceSchedule(ctx, w.owner, w.biz, w.pro, []profdomain.DayInput{
		{Weekday: "TUESDAY", WorkingHours: []profdomain.RangeInput{{Start: "09:00", End: "12:00"}}},
	}); err != nil {
		t.Fatal(err)
	}
	s, err := catalog.CreateService(ctx, w.owner, w.biz, catapp.ServiceInput{Name: "Corte", DurationMinutes: ptrTo(40), PriceMinor: ptrTo(int64(2_500_000))})
	if err != nil {
		t.Fatal(err)
	}
	w.svc = s.ID
	if _, err := catalog.ReplaceProfessionalServices(ctx, w.owner, w.biz, w.pro, []uuid.UUID{w.svc}); err != nil {
		t.Fatal(err)
	}
	return w
}

// customer is a verified account with a phone.
func (w bookingWorld) customer(t *testing.T) identitycontract.Principal {
	t.Helper()
	id, _ := activeUser(t, w.pool, "ACTIVE")
	p := identitycontract.Principal{UserID: id}
	if _, err := w.identity.SetPhone(t.Context(), p, "300 123 4567"); err != nil {
		t.Fatal(err)
	}
	return p
}

func (w bookingWorld) at(h, m int) time.Time { return time.Date(2030, 3, 5, h, m, 0, 0, w.bogota) }

func (w bookingWorld) in(start time.Time, key string) schedapp.BookingInput {
	return schedapp.BookingInput{BusinessID: w.biz, ServiceID: w.svc, ProfessionalID: w.pro, Start: start, IdempotencyKey: key}
}

func (w bookingWorld) activeAppointments(t *testing.T) int {
	t.Helper()
	return count(t, w.pool, `SELECT count(*) FROM appointments WHERE professional_id = $1 AND status IN ('PENDING_CONFIRMATION','CONFIRMED','IN_PROGRESS')`, w.pro)
}

// TestBookingOnPostgres (T6.3): the booking through the real modules and SQL: snapshot, event,
// idempotency key, phone requirement and the availability without the booked slot.
func TestBookingOnPostgres(t *testing.T) {
	ctx := t.Context()
	w := newBookingWorld(t)
	noPhone, _ := activeUser(t, w.pool, "ACTIVE")
	if _, err := w.sched.Book(ctx, identitycontract.Principal{UserID: noPhone}, w.in(w.at(9, 0), "sin-telefono")); !errors.Is(err, schedapp.ErrPhoneRequired) {
		t.Fatalf("no phone: %v", err)
	}
	ana := w.customer(t)
	a, err := w.sched.Book(ctx, ana, w.in(w.at(9, 0), "ana-reserva-1"))
	if err != nil {
		t.Fatal(err)
	}
	if a.Customer.Phone != "+573001234567" || a.Service.Name != "Corte" || a.Service.Currency != "COP" || !a.End.Equal(w.at(9, 40)) {
		t.Errorf("appointment = %+v", a)
	}
	if n := count(t, w.pool, `SELECT count(*) FROM appointment_events WHERE appointment_id = $1 AND type = 'CREATED' AND actor_user_id = $2`, a.ID, ana.UserID); n != 1 {
		t.Errorf("CREATED events = %d", n)
	}
	// Replay returns the stored appointment; the same key with another body is refused.
	again, err := w.sched.Book(ctx, ana, w.in(w.at(9, 0), "ana-reserva-1"))
	if err != nil || again.ID != a.ID || !again.Start.Equal(a.Start) || again.Customer.Name != a.Customer.Name {
		t.Errorf("replay: %+v %v", again, err)
	}
	if _, err := w.sched.Book(ctx, ana, w.in(w.at(10, 0), "ana-reserva-1")); !errors.Is(err, schedapp.ErrIdempotencyKeyReused) {
		t.Errorf("key reused: %v", err)
	}
	// A later price change does not alter the booked appointment (snapshot).
	if _, err := w.catalog.UpdateService(ctx, w.owner, w.biz, w.svc, catapp.ServicePatch{PriceMinor: ptrTo(int64(3_000_000))}); err != nil {
		t.Fatal(err)
	}
	if n := count(t, w.pool, `SELECT count(*) FROM appointments WHERE id = $1 AND price_minor = 2500000`, a.ID); n != 1 {
		t.Error("the snapshot price changed")
	}
	// The public availability no longer offers the booked range.
	day, _ := scheddomain.ParseLocalDate("2030-03-05")
	av, err := w.sched.PublicAvailability(ctx, w.slug, w.svc, w.pro, day)
	if err != nil || len(av.Slots) == 0 || !av.Slots[0].Start.Equal(w.at(9, 40)) {
		t.Errorf("availability after booking: %+v %v", av.Slots, err)
	}
	if _, err := w.sched.Book(ctx, w.customer(t), w.in(w.at(9, 20), "bob-reserva-1")); !errors.Is(err, schedapp.ErrSlotUnavailable) {
		t.Errorf("overlapping booking: %v", err)
	}
	if w.activeAppointments(t) != 1 {
		t.Errorf("active appointments = %d", w.activeAppointments(t))
	}
}

// TestNoDoubleBookingUnderConcurrency (availability-engine.md §7): with PostgreSQL, many
// customers racing for the same or overlapping slots get exactly one appointment.
func TestNoDoubleBookingUnderConcurrency(t *testing.T) {
	ctx := t.Context()
	w := newBookingWorld(t)
	race := func(n int, input func(i int) (identitycontract.Principal, schedapp.BookingInput)) (ok, taken int, others []error) {
		var (
			mu sync.Mutex
			wg sync.WaitGroup
		)
		ps := make([]identitycontract.Principal, n)
		ins := make([]schedapp.BookingInput, n)
		for i := range n {
			ps[i], ins[i] = input(i)
		}
		start := make(chan struct{})
		for i := range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_, err := w.sched.Book(ctx, ps[i], ins[i])
				mu.Lock()
				defer mu.Unlock()
				switch {
				case err == nil:
					ok++
				case errors.Is(err, schedapp.ErrSlotUnavailable):
					taken++
				default:
					others = append(others, err)
				}
			}()
		}
		close(start)
		wg.Wait()
		return ok, taken, others
	}

	// 50 customers, the same slot: exactly one wins.
	customers := make([]identitycontract.Principal, 50)
	for i := range customers {
		customers[i] = w.customer(t)
	}
	ok, taken, others := race(50, func(i int) (identitycontract.Principal, schedapp.BookingInput) {
		return customers[i], w.in(w.at(9, 0), fmt.Sprintf("mismo-slot-%02d", i))
	})
	if ok != 1 || taken != 49 || len(others) > 0 || w.activeAppointments(t) != 1 {
		t.Fatalf("same slot: %d ok, %d taken, others %v, %d active", ok, taken, others, w.activeAppointments(t))
	}

	// Partially overlapping (10:00 and 10:20, 40 min each), 20 in parallel: one wins.
	ok, taken, others = race(20, func(i int) (identitycontract.Principal, schedapp.BookingInput) {
		return customers[i], w.in(w.at(10, 20*(i%2)), fmt.Sprintf("solapada-%02d", i))
	})
	if ok != 1 || taken != 19 || len(others) > 0 || w.activeAppointments(t) != 2 {
		t.Fatalf("overlapping: %d ok, %d taken, others %v, %d active", ok, taken, others, w.activeAppointments(t))
	}

	// The same Idempotency-Key 10 times in parallel: one appointment, every reply is it.
	cust := w.customer(t)
	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		ids = map[uuid.UUID]int{}
	)
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a, err := w.sched.Book(ctx, cust, w.in(w.at(11, 0), "misma-clave-paralelo"))
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				t.Errorf("same key: %v", err)
				return
			}
			ids[a.ID]++
		}()
	}
	wg.Wait()
	if len(ids) != 1 || w.activeAppointments(t) != 3 {
		t.Fatalf("same key: replies %v, %d active", ids, w.activeAppointments(t))
	}
}

// TestExclusionConstraintIsTheSafetyNet (ADR-0004): even bypassing the lock and the engine, the
// database refuses an overlapping active appointment (buffer included), and ignores canceled
// ones.
func TestExclusionConstraintIsTheSafetyNet(t *testing.T) {
	ctx := t.Context()
	w := newBookingWorld(t)
	store := schedpg.NewStore(w.pool)
	mk := func(start time.Time, buffer time.Duration, status scheddomain.Status) scheddomain.Appointment {
		a, err := scheddomain.NewAppointment(uuid.Must(uuid.NewV7()), w.biz, w.pro,
			scheddomain.ServiceSnapshot{ServiceID: w.svc, Name: "Corte", DurationMinutes: 40, PriceMinor: 2_500_000, Currency: "COP"},
			scheddomain.Customer{Name: "Directo", Phone: "+573001234567"}, start, buffer, w.owner.UserID, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		a.Status = status
		if status == scheddomain.StatusCancelled {
			a.CancelledAt, a.CancelledBy = time.Now(), scheddomain.ActorBusiness
		}
		return a
	}
	insert := func(a scheddomain.Appointment) error {
		return store.WithinTx(ctx, func(ctx context.Context, tx schedapp.Tx) error { return tx.InsertAppointment(ctx, a) })
	}
	if err := insert(mk(w.at(9, 0), 10*time.Minute, scheddomain.StatusConfirmed)); err != nil { // occupies 09:00–09:50
		t.Fatal(err)
	}
	if err := insert(mk(w.at(9, 45), 0, scheddomain.StatusPendingConfirmation)); !errors.Is(err, schedapp.ErrSlotUnavailable) {
		t.Errorf("inside the buffer: %v", err)
	}
	if err := insert(mk(w.at(9, 50), 0, scheddomain.StatusPendingConfirmation)); err != nil {
		t.Errorf("right after the buffer: %v", err)
	}
	if err := insert(mk(w.at(9, 0), 0, scheddomain.StatusCancelled)); err != nil {
		t.Errorf("a canceled appointment does not occupy: %v", err)
	}
}

// TestManualBookingOnPostgres (T6.4): customers of the business with the real SQL (search with
// literal wildcards, unique mobile), the app booking joining that list, and a manual booking and
// app bookings racing for the same slot: one appointment.
func TestManualBookingOnPostgres(t *testing.T) {
	ctx := t.Context()
	w := newBookingWorld(t)
	carlos, err := w.sched.CreateCustomer(ctx, w.owner, w.biz, schedapp.CustomerInput{FullName: "Carlos 100%", Phone: "300 555 0101"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.sched.CreateCustomer(ctx, w.barber, w.biz, schedapp.CustomerInput{FullName: "Otro", Phone: "+573005550101"}); !errors.Is(err, schedapp.ErrCustomerPhoneTaken) {
		t.Errorf("duplicate mobile: %v", err)
	}
	if _, err := w.sched.CreateCustomer(ctx, w.barber, w.biz, schedapp.CustomerInput{FullName: "Diana", Phone: "3015550202"}); err != nil {
		t.Fatal(err)
	}
	for q, want := range map[string]int{"": 2, "%": 1, "carlos": 1, "555 0202": 1, "nadie": 0} {
		if got, err := w.sched.ListCustomers(ctx, w.owner, w.biz, q); err != nil || len(got) != want {
			t.Errorf("q=%q: %d %v", q, len(got), err)
		}
	}

	// The barber books Carlos in his own agenda, confirmed.
	a, err := w.sched.BookManual(ctx, w.barber, w.biz, schedapp.ManualInput{ProfessionalID: w.pro, ServiceID: w.svc, CustomerID: carlos.ID, Start: w.at(9, 0), Confirmed: true, IdempotencyKey: "manual-juan-0900"})
	if err != nil || a.Status != scheddomain.StatusConfirmed {
		t.Fatalf("manual booking: %+v %v", a, err)
	}
	if n := count(t, w.pool, `SELECT count(*) FROM appointments WHERE id = $1 AND customer_id = $2 AND customer_user_id IS NULL AND status = 'CONFIRMED'`, a.ID, carlos.ID); n != 1 {
		t.Error("manual appointment not stored with its customer")
	}
	// The quick way: the customer typed in the booking (with email) is created in the same
	// transaction; a failed booking leaves no customer behind.
	q, err := w.sched.BookManual(ctx, w.barber, w.biz, schedapp.ManualInput{ProfessionalID: w.pro, ServiceID: w.svc, Start: w.at(9, 40), Confirmed: true, IdempotencyKey: "manual-rapida-0940",
		Customer: &schedapp.CustomerInput{FullName: "Laura", Phone: "311 555 0303", Email: "Laura@Example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	if n := count(t, w.pool, `SELECT count(*) FROM customers WHERE id = $1 AND email = 'laura@example.com' AND phone = '+573115550303'`, q.Customer.CustomerID); n != 1 {
		t.Error("quick customer not stored")
	}
	if _, err := w.sched.BookManual(ctx, w.barber, w.biz, schedapp.ManualInput{ProfessionalID: w.pro, ServiceID: w.svc, Start: w.at(9, 50), Confirmed: true, IdempotencyKey: "manual-rapida-0950",
		Customer: &schedapp.CustomerInput{FullName: "Tarde", Phone: "314 555 0606"}}); !errors.Is(err, schedapp.ErrSlotUnavailable) {
		t.Fatalf("overlapping quick booking: %v", err)
	}
	if n := count(t, w.pool, `SELECT count(*) FROM customers WHERE phone = '+573145550606'`); n != 0 {
		t.Error("a failed booking left its customer")
	}

	// An app booking joins the customer list.
	ana := w.customer(t)
	if _, err := w.sched.Book(ctx, ana, w.in(w.at(10, 20), "ana-app-1020")); err != nil {
		t.Fatal(err)
	}
	if n := count(t, w.pool, `SELECT count(*) FROM customers WHERE business_id = $1 AND user_id = $2`, w.biz, ana.UserID); n != 1 {
		t.Errorf("app customer records = %d", n)
	}

	// The owner (manual) and 10 customers (app) race for 11:00: one wins.
	customers := make([]identitycontract.Principal, 10)
	for i := range customers {
		customers[i] = w.customer(t)
	}
	var (
		mu       sync.Mutex
		wg       sync.WaitGroup
		ok, lost int
		others   []error
	)
	record := func(err error) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case err == nil:
			ok++
		case errors.Is(err, schedapp.ErrSlotUnavailable):
			lost++
		default:
			others = append(others, err)
		}
	}
	start := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		_, err := w.sched.BookManual(ctx, w.owner, w.biz, schedapp.ManualInput{ProfessionalID: w.pro, ServiceID: w.svc, CustomerID: carlos.ID, Start: w.at(11, 0), Confirmed: true, IdempotencyKey: "manual-owner-1100"})
		record(err)
	}()
	for i, c := range customers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := w.sched.Book(ctx, c, w.in(w.at(11, 0), fmt.Sprintf("app-1100-%02d", i)))
			record(err)
		}()
	}
	close(start)
	wg.Wait()
	if ok != 1 || lost != 10 || len(others) > 0 {
		t.Fatalf("manual vs app on 11:00: %d ok, %d lost, %v", ok, lost, others)
	}
	if w.activeAppointments(t) != 4 {
		t.Errorf("active appointments = %d", w.activeAppointments(t))
	}
}

// TestAgendaOnPostgres (T6.5): the agenda and the customer's appointments with the real SQL
// (keyset pagination, professional filter, upcoming/past) and the real names.
func TestAgendaOnPostgres(t *testing.T) {
	ctx := t.Context()
	w := newBookingWorld(t)
	ana := w.customer(t)
	if _, err := w.sched.Book(ctx, ana, w.in(w.at(9, 0), "agenda-app-0900")); err != nil {
		t.Fatal(err)
	}
	if _, err := w.sched.BookManual(ctx, w.barber, w.biz, schedapp.ManualInput{ProfessionalID: w.pro, ServiceID: w.svc, Start: w.at(10, 0), Confirmed: true, IdempotencyKey: "agenda-manual-1000",
		Customer: &schedapp.CustomerInput{FullName: "Laura", Phone: "3115550303"}}); err != nil {
		t.Fatal(err)
	}
	// A past appointment of Ana (seeded below the engine).
	past, _ := scheddomain.NewAppointment(uuid.Must(uuid.NewV7()), w.biz, w.pro,
		scheddomain.ServiceSnapshot{ServiceID: w.svc, Name: "Corte", DurationMinutes: 40, PriceMinor: 2_500_000, Currency: "COP"},
		scheddomain.Customer{UserID: ana.UserID, Name: "Ana", Phone: "+573001234567"}, time.Date(2030, 2, 5, 14, 0, 0, 0, time.UTC), 0, ana.UserID, time.Now())
	past.Status = scheddomain.StatusCompleted
	if err := schedpg.NewStore(w.pool).WithinTx(ctx, func(ctx context.Context, tx schedapp.Tx) error { return tx.InsertAppointment(ctx, past) }); err != nil {
		t.Fatal(err)
	}

	tuesday, _ := scheddomain.ParseLocalDate("2030-03-05")
	for name, c := range map[string]struct {
		p    identitycontract.Principal
		q    schedapp.AgendaQuery
		want int
	}{
		"owner, day":              {w.owner, schedapp.AgendaQuery{View: scheddomain.ViewDay, Date: tuesday}, 2},
		"owner, Juan only":        {w.owner, schedapp.AgendaQuery{View: scheddomain.ViewDay, Date: tuesday, ProfessionalID: w.pro}, 2},
		"barber, his week":        {w.barber, schedapp.AgendaQuery{View: scheddomain.ViewWeek, Date: tuesday}, 2},
		"owner, month of March":   {w.owner, schedapp.AgendaQuery{View: scheddomain.ViewMonth, Date: tuesday}, 2},
		"owner, year (with past)": {w.owner, schedapp.AgendaQuery{View: scheddomain.ViewYear, Date: tuesday}, 3},
		"owner, the next day":     {w.owner, schedapp.AgendaQuery{View: scheddomain.ViewDay, Date: tuesday.AddDays(1)}, 0},
	} {
		page, err := w.sched.Agenda(ctx, c.p, w.biz, c.q)
		if err != nil || len(page.Items) != c.want {
			t.Errorf("%s: %d %v", name, len(page.Items), err)
		}
	}
	p1, err := w.sched.Agenda(ctx, w.owner, w.biz, schedapp.AgendaQuery{View: scheddomain.ViewDay, Date: tuesday, Limit: 1})
	if err != nil || len(p1.Items) != 1 || p1.NextCursor == "" || p1.Items[0].ProfessionalName != "Juan" || !p1.Items[0].Start.Equal(w.at(9, 0)) {
		t.Fatalf("page 1: %+v %v", p1, err)
	}
	p2, err := w.sched.Agenda(ctx, w.owner, w.biz, schedapp.AgendaQuery{View: scheddomain.ViewDay, Date: tuesday, Limit: 1, Cursor: p1.NextCursor})
	if err != nil || len(p2.Items) != 1 || p2.NextCursor != "" || p2.Items[0].Customer.Name != "Laura" {
		t.Errorf("page 2: %+v %v", p2, err)
	}

	up, _, err := w.sched.MyAppointments(ctx, ana, schedapp.MyAppointmentsQuery{Upcoming: true})
	if err != nil || len(up) != 1 || up[0].Business.Name != "Barbería Reservas" || up[0].ProfessionalName != "Juan" {
		t.Errorf("upcoming: %+v %v", up, err)
	}
	if old, _, err := w.sched.MyAppointments(ctx, ana, schedapp.MyAppointmentsQuery{}); err != nil || len(old) != 1 || old[0].ID != past.ID {
		t.Errorf("past: %+v %v", old, err)
	}
	if _, err := w.sched.MyAppointmentDetail(ctx, w.customer(t), up[0].ID); !errors.Is(err, schedapp.ErrAppointmentNotFound) {
		t.Errorf("another customer's appointment: %v", err)
	}
}

// secondBarber adds Pedro (Tuesdays 09:00–12:00, performs "Corte") and returns his profile.
func (w bookingWorld) secondBarber(t *testing.T) uuid.UUID {
	t.Helper()
	ctx := t.Context()
	_, email := activeUser(t, w.pool, "ACTIVE")
	pedro, err := w.profs.Create(ctx, w.owner, w.biz, profapp.CreateInput{Email: email, DisplayName: "Pedro"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.profs.ReplaceSchedule(ctx, w.owner, w.biz, pedro.ID, []profdomain.DayInput{
		{Weekday: "TUESDAY", WorkingHours: []profdomain.RangeInput{{Start: "09:00", End: "12:00"}}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.catalog.ReplaceProfessionalServices(ctx, w.owner, w.biz, pedro.ID, []uuid.UUID{w.svc}); err != nil {
		t.Fatal(err)
	}
	return pedro.ID
}

func (w bookingWorld) walkIn(t *testing.T, pro uuid.UUID, start time.Time, key string) scheddomain.Appointment {
	t.Helper()
	a, err := w.sched.BookManual(t.Context(), w.owner, w.biz, schedapp.ManualInput{ProfessionalID: pro, ServiceID: w.svc, Start: start, Confirmed: true,
		IdempotencyKey: key, Customer: &schedapp.CustomerInput{FullName: "Cliente " + key, Phone: fmt.Sprintf("301%07d", len(key)*7919%10000000)}})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// TestAppointmentChangesOnPostgres (T6.6): changes with the real SQL under concurrency.
func TestAppointmentChangesOnPostgres(t *testing.T) {
	ctx := t.Context()
	w := newBookingWorld(t)
	pedro := w.secondBarber(t)

	// 1. Cancel and reschedule of the same appointment at the same time, 8 rounds: the final
	// state is always consistent (version = 1 + successful changes, one event per change).
	ana := w.customer(t)
	for round := range 8 {
		a, err := w.sched.Book(ctx, ana, w.in(w.at(9, 0), fmt.Sprintf("carrera-%02d", round)))
		if err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		var errCancel, errMove error
		start := make(chan struct{})
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_, errCancel = w.sched.CustomerChange(ctx, ana, a.ID, schedapp.ActionCancel, "")
		}()
		go func() {
			defer wg.Done()
			<-start
			_, errMove = w.sched.StaffReschedule(ctx, w.owner, w.biz, a.ID, w.at(11, 0), uuid.Nil)
		}()
		close(start)
		wg.Wait()
		if errCancel != nil {
			t.Fatalf("round %d: cancel %v", round, errCancel)
		}
		ok := 1
		if errMove == nil {
			ok++
		} else if !errors.Is(errMove, scheddomain.ErrInvalidTransition) && !errors.Is(errMove, schedapp.ErrAppointmentChanged) {
			t.Fatalf("round %d: move %v", round, errMove)
		}
		if n := count(t, w.pool, `SELECT count(*) FROM appointments WHERE id = $1 AND status = 'CANCELLED' AND version = $2`, a.ID, 1+ok); n != 1 {
			t.Fatalf("round %d: appointment not canceled at version %d", round, 1+ok)
		}
		if n := count(t, w.pool, `SELECT count(*) FROM appointment_events WHERE appointment_id = $1`, a.ID); n != 1+ok {
			t.Fatalf("round %d: %d events, want %d", round, n, 1+ok)
		}
	}

	// 2. Crossed reassignments Juan 09:00 → Pedro 09:40 and Pedro 09:00 → Juan 09:40 at the same
	// time, 5 rounds: both succeed (fixed lock order: no deadlock, no timeout).
	for round := range 5 {
		x := w.walkIn(t, w.pro, w.at(9, 0), fmt.Sprintf("cruce-juan-%02d", round))
		y := w.walkIn(t, pedro, w.at(9, 0), fmt.Sprintf("cruce-pedro-%02d", round))
		var e1, e2 error
		var wg sync.WaitGroup
		gate := make(chan struct{})
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-gate
			_, e1 = w.sched.StaffReschedule(ctx, w.owner, w.biz, x.ID, w.at(9, 40), pedro)
		}()
		go func() {
			defer wg.Done()
			<-gate
			_, e2 = w.sched.StaffReschedule(ctx, w.owner, w.biz, y.ID, w.at(9, 40), w.pro)
		}()
		close(gate)
		wg.Wait()
		if e1 != nil || e2 != nil {
			t.Fatalf("round %d: crossed reassignments: %v / %v", round, e1, e2)
		}
		if n := count(t, w.pool, `SELECT count(*) FROM appointments WHERE id = ANY($1) AND start_at = $2`, []uuid.UUID{x.ID, y.ID}, w.at(9, 40)); n != 2 {
			t.Fatalf("round %d: reassigned %d of 2", round, n)
		}
		for _, id := range []uuid.UUID{x.ID, y.ID} { // keep the agendas free for the next round
			if _, err := w.sched.StaffChange(ctx, w.owner, w.biz, id, schedapp.ActionCancel, "limpieza"); err != nil {
				t.Fatal(err)
			}
		}
	}

	// 3. A move and new bookings race for 10:00: one wins.
	moving := w.walkIn(t, w.pro, w.at(9, 0), "se-mueve-0900")
	customers := make([]identitycontract.Principal, 6)
	for i := range customers {
		customers[i] = w.customer(t)
	}
	var (
		mu      sync.Mutex
		wg      sync.WaitGroup
		winners int
	)
	start := make(chan struct{})
	try := func(f func() error) {
		defer wg.Done()
		<-start
		err := f()
		mu.Lock()
		defer mu.Unlock()
		switch {
		case err == nil:
			winners++
		case !errors.Is(err, schedapp.ErrSlotUnavailable):
			t.Errorf("unexpected: %v", err)
		}
	}
	wg.Add(1 + len(customers))
	go try(func() error {
		_, err := w.sched.StaffReschedule(ctx, w.owner, w.biz, moving.ID, w.at(10, 0), uuid.Nil)
		return err
	})
	for i, c := range customers {
		go try(func() error {
			_, err := w.sched.Book(ctx, c, w.in(w.at(10, 0), fmt.Sprintf("contra-mov-%02d", i)))
			return err
		})
	}
	close(start)
	wg.Wait()
	if winners != 1 || count(t, w.pool, `SELECT count(*) FROM appointments WHERE professional_id = $1 AND start_at = $2 AND status IN ('PENDING_CONFIRMATION','CONFIRMED')`, w.pro, w.at(10, 0)) != 1 {
		t.Fatalf("move vs bookings on 10:00: %d winners", winners)
	}

	// 4. A no-show frees the rest of the slot at once (decision of the user): Pedro's 10:40
	// customer does not come; at 10:45 a waiting customer gets 10:50, which overlaps 10:40–11:20.
	late := w.walkIn(t, pedro, w.at(10, 40), "no-vino-1040")
	w.clk.Advance(w.at(10, 45).Sub(w.clk.Now()))
	if _, err := w.sched.BookManual(ctx, w.owner, w.biz, schedapp.ManualInput{ProfessionalID: pedro, ServiceID: w.svc, Start: w.at(10, 50), Confirmed: true,
		IdempotencyKey: "antes-no-vino", Customer: &schedapp.CustomerInput{FullName: "Esperaba", Phone: "3019998877"}}); !errors.Is(err, schedapp.ErrSlotUnavailable) {
		t.Fatalf("10:50 while 10:40 is still active: %v", err)
	}
	if _, err := w.sched.StaffChange(ctx, w.owner, w.biz, late.ID, schedapp.ActionNoShow, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := w.sched.BookManual(ctx, w.owner, w.biz, schedapp.ManualInput{ProfessionalID: pedro, ServiceID: w.svc, Start: w.at(10, 50), Confirmed: true,
		IdempotencyKey: "tras-no-vino", Customer: &schedapp.CustomerInput{FullName: "Esperaba", Phone: "3019998877"}}); err != nil {
		t.Errorf("10:50 after the no-show: %v", err)
	}
}

// TestAgendaCoherenceOnPostgres (T6.7): absences and schedules against appointments with the
// real modules and SQL, under concurrency; a professional who leaves leaves appointments to
// reassign.
func TestAgendaCoherenceOnPostgres(t *testing.T) {
	ctx := t.Context()
	w := newBookingWorld(t)
	pedro := w.secondBarber(t)
	ana := w.customer(t)

	// 1. A booking and an absence on the same hour at the same time, 10 rounds: exactly one wins.
	// The guard pauses 100 ms between the check and the insert of the absence, so a booking that
	// did not wait for the professional's lock would slip in (shown by mutation).
	w.profs.SetAgendaGuard(slowGuard{AgendaGuard: w.sched, pause: 100 * time.Millisecond})
	for round := range 10 {
		var (
			wg         sync.WaitGroup
			booked     scheddomain.Appointment
			errBook    error
			absence    profdomain.Absence
			errAbsence error
			gate       = make(chan struct{})
		)
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-gate
			booked, errBook = w.sched.Book(ctx, ana, w.in(w.at(9, 0), fmt.Sprintf("vs-ausencia-%02d", round)))
		}()
		go func() {
			defer wg.Done()
			<-gate
			absence, errAbsence = w.profs.CreateAbsence(ctx, w.owner, w.biz, w.pro, profapp.AbsenceInput{Start: w.at(9, 0), End: w.at(10, 0), Reason: "Diligencia"})
		}()
		close(gate)
		wg.Wait()
		var conflict *profapp.ConflictError
		switch {
		case errBook == nil && errors.As(errAbsence, &conflict):
			if len(conflict.Appointments) != 1 || conflict.Appointments[0].ID != booked.ID {
				t.Fatalf("round %d: conflicts %+v", round, conflict.Appointments)
			}
			if _, err := w.sched.CustomerChange(ctx, ana, booked.ID, schedapp.ActionCancel, ""); err != nil {
				t.Fatal(err)
			}
		case errAbsence == nil && errors.Is(errBook, schedapp.ErrSlotUnavailable):
			if err := w.profs.DeleteAbsence(ctx, w.owner, w.biz, w.pro, absence.ID); err != nil {
				t.Fatal(err)
			}
		default:
			t.Fatalf("round %d: booking %v, absence %v (exactly one must win)", round, errBook, errAbsence)
		}
	}

	w.profs.SetAgendaGuard(w.sched)

	// 2. A weekly schedule that would leave an appointment out is refused with the list.
	a, err := w.sched.Book(ctx, ana, w.in(w.at(11, 0), "fuera-de-horario"))
	if err != nil {
		t.Fatal(err)
	}
	var conflict *profapp.ConflictError
	if _, err := w.profs.ReplaceSchedule(ctx, w.owner, w.biz, w.pro, []profdomain.DayInput{
		{Weekday: "TUESDAY", WorkingHours: []profdomain.RangeInput{{Start: "09:00", End: "11:00"}}},
	}); !errors.As(err, &conflict) || len(conflict.Appointments) != 1 || conflict.Appointments[0].ID != a.ID {
		t.Fatalf("schedule leaving 11:00 out: %v", err)
	}
	if _, err := w.profs.ReplaceSchedule(ctx, w.owner, w.biz, w.pro, []profdomain.DayInput{
		{Weekday: "TUESDAY", WorkingHours: []profdomain.RangeInput{{Start: "08:00", End: "12:00"}}, Breaks: []profdomain.RangeInput{{Start: "10:00", End: "10:30"}}},
	}); err != nil {
		t.Errorf("schedule keeping 11:00: %v", err)
	}

	// 3. Juan is deactivated: his appointment is left to reassign; the owner moves it to Pedro.
	if _, err := w.profs.Update(ctx, w.owner, w.biz, w.pro, profapp.UpdateInput{Status: ptrTo("INACTIVE")}); err != nil {
		t.Fatal(err)
	}
	page, err := w.sched.ToReassign(ctx, w.owner, w.biz)
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != a.ID || page.Items[0].ProfessionalName != "Juan" {
		t.Fatalf("to reassign: %+v %v", page.Items, err)
	}
	if _, err := w.sched.StaffReschedule(ctx, w.owner, w.biz, a.ID, w.at(11, 0), pedro); err != nil {
		t.Fatal(err)
	}
	if page, err := w.sched.ToReassign(ctx, w.owner, w.biz); err != nil || len(page.Items) != 0 {
		t.Errorf("after reassigning: %+v %v", page.Items, err)
	}
}

// slowGuard widens the window between an agenda check and the write that follows it.
type slowGuard struct {
	schedcontract.AgendaGuard
	pause time.Duration
}

func (g slowGuard) AppointmentsDuring(ctx context.Context, businessID, professionalID uuid.UUID, from, to time.Time) ([]schedcontract.ConflictingAppointment, error) {
	list, err := g.AgendaGuard.AppointmentsDuring(ctx, businessID, professionalID, from, to)
	time.Sleep(g.pause)
	return list, err
}
