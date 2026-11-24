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
	scheddomain "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/domain"
	schedpg "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/infrastructure/postgres"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/pgtest"
)

type bookingWorld struct {
	pool     *pgxpool.Pool
	identity *idapp.Service
	catalog  *catapp.Service
	sched    *schedapp.Service
	owner    identitycontract.Principal
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
	w := bookingWorld{pool: pool, identity: identity, catalog: catalog}
	w.bogota, _ = time.LoadLocation("America/Bogota")
	w.sched = schedapp.NewService(schedapp.Deps{
		Store: schedpg.NewStore(pool), Businesses: biz, Catalog: catalog, Professionals: profs, Customers: identity,
		Clock: clock.NewFixed(time.Date(2030, 3, 1, 17, 0, 0, 0, time.UTC)),
	})

	ownerID, _ := activeUser(t, pool, "ACTIVE")
	w.owner = identitycontract.Principal{UserID: ownerID}
	b, err := biz.CreateOwnBusiness(ctx, w.owner, bizapp.BusinessInput{Name: "Barbería Reservas", ContactPhone: "3001234567", CityID: pgtest.SeedCaliID, AddressLine: "Calle 1"})
	if err != nil {
		t.Fatal(err)
	}
	w.biz, w.slug = b.ID(), string(b.Slug())
	_, barberEmail := activeUser(t, pool, "ACTIVE")
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
