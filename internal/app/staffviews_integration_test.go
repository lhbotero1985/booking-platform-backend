//go:build integration

package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	schedapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/application"
	scheddomain "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/domain"
	schedpg "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/infrastructure/postgres"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/authz"
)

// TestStaffViewsOnPostgres (P-9, P-10): the staff's free times and a customer's appointments with
// the real modules and SQL.
func TestStaffViewsOnPostgres(t *testing.T) {
	ctx := t.Context()
	w := newBookingWorld(t)
	pedro := w.secondBarber(t)
	carlos, err := w.sched.CreateCustomer(ctx, w.owner, w.biz, schedapp.CustomerInput{FullName: "Carlos", Phone: "3005550101"})
	if err != nil {
		t.Fatal(err)
	}
	diana, _ := w.sched.CreateCustomer(ctx, w.owner, w.biz, schedapp.CustomerInput{FullName: "Diana", Phone: "3015550202"})
	book := func(pro, customer uuid.UUID, start time.Time, key string) scheddomain.Appointment {
		t.Helper()
		a, err := w.sched.BookManual(ctx, w.owner, w.biz, schedapp.ManualInput{ProfessionalID: pro, ServiceID: w.svc, CustomerID: customer, Start: start, Confirmed: true, IdempotencyKey: key})
		if err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		return a
	}
	withJuan := book(w.pro, carlos.ID, w.at(10, 0), "pg-carlos-juan-1000")
	book(pedro, carlos.ID, w.at(9, 0), "pg-carlos-pedro-0900")
	book(w.pro, diana.ID, w.at(11, 0), "pg-diana-juan-1100")
	for _, days := range []int{7, 14} { // past appointments of Carlos with Juan
		a, _ := scheddomain.NewAppointment(uuid.Must(uuid.NewV7()), w.biz, w.pro,
			scheddomain.ServiceSnapshot{ServiceID: w.svc, Name: "Corte", DurationMinutes: 40, PriceMinor: 2_500_000, Currency: "COP"},
			scheddomain.Customer{CustomerID: carlos.ID, Name: "Carlos", Phone: carlos.Phone}, w.clk.Now().AddDate(0, 0, -days), 0, w.owner.UserID, w.clk.Now())
		a.Status = scheddomain.StatusCompleted
		if err := schedpg.NewStore(w.pool).WithinTx(ctx, func(ctx context.Context, tx schedapp.Tx) error { return tx.InsertAppointment(ctx, a) }); err != nil {
			t.Fatal(err)
		}
	}

	// P-10.
	up := schedapp.CustomerAppointmentsQuery{Upcoming: true}
	for name, c := range map[string]struct {
		q    schedapp.CustomerAppointmentsQuery
		p    bool // true: the owner, false: Juan
		want int
	}{
		"owner, upcoming": {up, true, 2},
		"owner, past":     {schedapp.CustomerAppointmentsQuery{}, true, 2},
		"Juan, upcoming":  {up, false, 1},
		"Juan, past":      {schedapp.CustomerAppointmentsQuery{}, false, 2},
	} {
		who := w.barber
		if c.p {
			who = w.owner
		}
		page, err := w.sched.CustomerAppointments(ctx, who, w.biz, carlos.ID, c.q)
		if err != nil || len(page.Items) != c.want {
			t.Errorf("%s: %d %v", name, len(page.Items), err)
		}
	}
	p1, err := w.sched.CustomerAppointments(ctx, w.owner, w.biz, carlos.ID, schedapp.CustomerAppointmentsQuery{Upcoming: true, Limit: 1})
	if err != nil || len(p1.Items) != 1 || !p1.Items[0].Start.Equal(w.at(9, 0)) || p1.Items[0].ProfessionalName != "Pedro" || p1.NextCursor == "" {
		t.Fatalf("page 1: %+v %v", p1, err)
	}
	p2, err := w.sched.CustomerAppointments(ctx, w.owner, w.biz, carlos.ID, schedapp.CustomerAppointmentsQuery{Upcoming: true, Limit: 1, Cursor: p1.NextCursor})
	if err != nil || len(p2.Items) != 1 || p2.Items[0].ID != withJuan.ID || p2.NextCursor != "" {
		t.Errorf("page 2: %+v %v", p2, err)
	}
	past, _ := w.sched.CustomerAppointments(ctx, w.owner, w.biz, carlos.ID, schedapp.CustomerAppointmentsQuery{Limit: 1})
	pastNext, _ := w.sched.CustomerAppointments(ctx, w.owner, w.biz, carlos.ID, schedapp.CustomerAppointmentsQuery{Limit: 1, Cursor: past.NextCursor})
	if len(past.Items) != 1 || len(pastNext.Items) != 1 || !past.Items[0].Start.After(pastNext.Items[0].Start) {
		t.Errorf("past is not newest first: %+v / %+v", past.Items, pastNext.Items)
	}
	if _, err := w.sched.CustomerAppointments(ctx, w.owner, w.biz, uuid.Must(uuid.NewV7()), up); !errors.Is(err, schedapp.ErrCustomerNotFound) {
		t.Errorf("unknown customer: %v", err)
	}

	// P-9: Tuesday 09:00–12:00, Corte 40 min every 10 min. Now: Tuesday 10:00 in Bogotá.
	w.clk.Advance(w.at(10, 0).Sub(w.clk.Now()))
	q := func(pro uuid.UUID) schedapp.StaffAvailabilityQuery {
		return schedapp.StaffAvailabilityQuery{ServiceID: w.svc.String(), ProfessionalID: pro.String(), Date: "2030-03-05"}
	}
	tuesday, _ := scheddomain.ParseLocalDate("2030-03-05")
	public, err := w.sched.PublicAvailability(ctx, w.slug, w.svc, pedro, tuesday)
	if err != nil {
		t.Fatal(err)
	}
	staff, err := w.sched.StaffAvailability(ctx, w.owner, w.biz, q(pedro))
	if err != nil || len(staff.Slots) == 0 || !staff.Slots[0].Start.Equal(w.at(10, 0)) || len(staff.Slots) <= len(public.Slots) {
		t.Fatalf("staff %v vs public %v: %v", staff.Slots, public.Slots, err)
	}
	if _, err := w.sched.BookManual(ctx, w.owner, w.biz, schedapp.ManualInput{ProfessionalID: pedro, ServiceID: w.svc, Start: staff.Slots[0].Start, Confirmed: true,
		IdempotencyKey: "pg-ahora-mismo-1000", Customer: &schedapp.CustomerInput{FullName: "Sin cita", Phone: "3117778899"}}); err != nil {
		t.Errorf("manual booking of the first staff time: %v", err)
	}
	// Juan's 10:00 appointment blocks it, unless it is the one being moved.
	juan, _ := w.sched.StaffAvailability(ctx, w.barber, w.biz, q(w.pro))
	moving := q(w.pro)
	moving.ExcludeAppointmentID = withJuan.ID.String()
	juanMoving, err := w.sched.StaffAvailability(ctx, w.barber, w.biz, moving)
	if err != nil || len(juanMoving.Slots) == 0 || !juanMoving.Slots[0].Start.Equal(w.at(10, 0)) || (len(juan.Slots) > 0 && juan.Slots[0].Start.Equal(w.at(10, 0))) {
		t.Errorf("Juan %v, moving his 10:00 %v: %v", juan.Slots, juanMoving.Slots, err)
	}
	if _, err := w.sched.StaffAvailability(ctx, w.barber, w.biz, q(pedro)); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("Juan asks for Pedro: %v", err)
	}
}
