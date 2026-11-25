//go:build integration

package app

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/stdlib"

	schedapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/application"
	scheddomain "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/migrate"
)

// TestRescheduleLimitOnPostgres (T7.8): the business's limit with the real SQL; the default is 2;
// the owner's moves do not count; two moves of the customer at once with one left: exactly one
// applies; migration 00025 rebuilds the counter from the history.
func TestRescheduleLimitOnPostgres(t *testing.T) {
	ctx := t.Context()
	w := newBookingWorld(t)
	if n := count(t, w.pool, `SELECT max_customer_reschedules FROM businesses WHERE id = $1`, w.biz); n != 2 {
		t.Fatalf("default limit %d", n)
	}
	ana := w.customer(t)
	a, err := w.sched.Book(ctx, ana, w.in(w.at(9, 0), "limite-pg-1"))
	if err != nil {
		t.Fatal(err)
	}
	for i, h := range []int{10, 11} {
		if m, err := w.sched.CustomerReschedule(ctx, ana, a.ID, w.at(h, 0)); err != nil || m.CustomerReschedules != i+1 {
			t.Fatalf("move %d: %+v %v", i+1, m, err)
		}
	}
	if _, err := w.sched.CustomerReschedule(ctx, ana, a.ID, w.at(9, 0)); !errors.Is(err, scheddomain.ErrRescheduleLimit) {
		t.Fatalf("third move: %v", err)
	}
	if _, err := w.sched.StaffReschedule(ctx, w.owner, w.biz, a.ID, w.at(9, 30), uuid.Nil); err != nil {
		t.Fatalf("the owner moves it: %v", err)
	}
	if n := count(t, w.pool, `SELECT customer_reschedules FROM appointments WHERE id = $1`, a.ID); n != 2 {
		t.Errorf("stored counter %d", n)
	}
	got, err := w.sched.MyAppointmentDetail(ctx, ana, a.ID)
	if err != nil || got.CustomerReschedules != 2 {
		t.Errorf("read back: %+v %v", got, err)
	}

	// One move left, two at once: exactly one applies.
	if _, err := w.pool.Exec(ctx, `UPDATE businesses SET max_customer_reschedules = 1 WHERE id = $1`, w.biz); err != nil {
		t.Fatal(err)
	}
	for round := range 4 {
		b, err := w.sched.Book(ctx, ana, w.in(w.at(10, 30), fmt.Sprintf("limite-carrera-%d", round)))
		if err != nil {
			t.Fatal(err)
		}
		errs := make([]error, 2)
		var wg sync.WaitGroup
		begin := make(chan struct{})
		for i, m := range []int{0, 20} { // 11:00 or 11:20

			wg.Add(1)
			go func() {
				defer wg.Done()
				<-begin
				_, errs[i] = w.sched.CustomerReschedule(ctx, ana, b.ID, w.at(11, m))
			}()
		}
		close(begin)
		wg.Wait()
		applied := 0
		for _, err := range errs {
			switch {
			case err == nil:
				applied++
			case !errors.Is(err, scheddomain.ErrRescheduleLimit) && !errors.Is(err, schedapp.ErrAppointmentChanged):
				t.Fatalf("round %d: %v", round, err)
			}
		}
		if applied != 1 || count(t, w.pool, `SELECT customer_reschedules FROM appointments WHERE id = $1`, b.ID) != 1 {
			t.Fatalf("round %d: %v", round, errs)
		}
		if _, err := w.sched.CustomerChange(ctx, ana, b.ID, schedapp.ActionCancel, ""); err != nil {
			t.Fatal(err)
		}
	}

	// Down and up again: the counter comes back from the history (two moves by the customer,
	// not the owner's), the businesses get the default.
	sqlDB := stdlib.OpenDBFromPool(w.pool)
	t.Cleanup(func() { _ = sqlDB.Close() })
	m, err := migrate.New(sqlDB)
	if err != nil {
		t.Fatal(err)
	}
	if r, err := m.Down(ctx); err != nil || r.Version != 25 {
		t.Fatalf("down: %+v %v", r, err)
	}
	if _, err := m.Up(ctx); err != nil {
		t.Fatal(err)
	}
	if n := count(t, w.pool, `SELECT customer_reschedules FROM appointments WHERE id = $1`, a.ID); n != 2 {
		t.Errorf("rebuilt counter %d", n)
	}
	if n := count(t, w.pool, `SELECT max_customer_reschedules FROM businesses WHERE id = $1`, w.biz); n != 2 {
		t.Errorf("limit after the migration %d", n)
	}
}
