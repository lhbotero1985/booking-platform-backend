//go:build integration

package app

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	profapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/application"
	schedapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/application"
	scheddomain "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/domain"
	schedpg "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/infrastructure/postgres"
)

// TestStatsOnPostgres (P-12): the statistics with the real SQL, schedules and absences. March
// 2030 in Bogotá: Juan and Pedro work Tuesdays 09:00–12:00 (5, 12, 19, 26); Juan is absent on
// the 12th.
func TestStatsOnPostgres(t *testing.T) {
	ctx := t.Context()
	w := newBookingWorld(t)
	pedro := w.secondBarber(t)
	carlos, err := w.sched.CreateCustomer(ctx, w.owner, w.biz, schedapp.CustomerInput{FullName: "Carlos", Phone: "3005550101"})
	if err != nil {
		t.Fatal(err)
	}
	diana, _ := w.sched.CreateCustomer(ctx, w.owner, w.biz, schedapp.CustomerInput{FullName: "Diana", Phone: "3015550202"})
	march := func(d, h int) time.Time { return time.Date(2030, 3, d, h, 0, 0, 0, w.bogota) }
	insert := func(pro uuid.UUID, c scheddomain.BusinessCustomer, start time.Time, st scheddomain.Status) {
		t.Helper()
		a, err := scheddomain.NewAppointment(uuid.Must(uuid.NewV7()), w.biz, pro,
			scheddomain.ServiceSnapshot{ServiceID: w.svc, Name: "Corte", DurationMinutes: 40, PriceMinor: 2_500_000, Currency: "COP"},
			scheddomain.Customer{CustomerID: c.ID, Name: c.FullName, Phone: c.Phone}, start, 0, w.owner.UserID, w.clk.Now())
		if err != nil {
			t.Fatal(err)
		}
		a.Status = st
		if st == scheddomain.StatusCancelled {
			a.CancelledAt, a.CancelReason, a.CancelledBy = w.clk.Now(), "Viaje", scheddomain.ActorCustomer
		}
		if err := schedpg.NewStore(w.pool).WithinTx(ctx, func(ctx context.Context, tx schedapp.Tx) error { return tx.InsertAppointment(ctx, a) }); err != nil {
			t.Fatal(err)
		}
	}
	insert(w.pro, carlos, time.Date(2030, 2, 26, 10, 0, 0, 0, w.bogota), scheddomain.StatusCompleted) // Carlos's first visit
	insert(w.pro, carlos, march(5, 10), scheddomain.StatusCompleted)
	insert(pedro, diana, march(5, 9), scheddomain.StatusCompleted) // Diana's first visit
	insert(w.pro, diana, march(12, 9), scheddomain.StatusCancelled)
	insert(pedro, diana, march(19, 9), scheddomain.StatusNoShow)
	if _, err := w.profs.CreateAbsence(ctx, w.owner, w.biz, w.pro, profapp.AbsenceInput{Start: march(12, 0), End: march(13, 0), Reason: "Cita médica"}); err != nil {
		t.Fatal(err)
	}

	q := schedapp.StatsQuery{From: "2030-03-01", To: "2030-03-31", GroupBy: "month"}
	s, err := w.sched.BusinessStats(ctx, w.owner, w.biz, q)
	if err != nil {
		t.Fatal(err)
	}
	tot := s.Totals
	if tot.Appointments != 3 || tot.Completed != 2 || tot.Canceled != 1 || tot.NoShow != 1 || tot.RevenueMinor != 5_000_000 ||
		tot.AverageTicketMinor != 2_500_000 || tot.CustomersServed != 2 || tot.NewCustomers != 1 || tot.BookedMinutes != 80 ||
		tot.AvailableMinutes != 1260 || tot.OccupancyPercent == nil || *tot.OccupancyPercent != 6 || s.Currency != "COP" {
		t.Errorf("owner totals %+v %s", *tot, s.Currency)
	}
	sum := 0
	for _, p := range s.ByProfessional {
		sum += int(p.Figures.RevenueMinor)
	}
	if len(s.ByProfessional) != 2 || sum != 5_000_000 {
		t.Errorf("by professional %+v", s.ByProfessional)
	}
	juan, err := w.sched.BusinessStats(ctx, w.barber, w.biz, q)
	if err != nil || juan.Totals.RevenueMinor != 2_500_000 || juan.Totals.AvailableMinutes != 540 || juan.Totals.NewCustomers != 0 ||
		juan.Totals.Canceled != 1 || juan.ByProfessional != nil {
		t.Errorf("Juan %+v %v", juan.Totals, err)
	}
	byDay, _ := w.sched.BusinessStats(ctx, w.owner, w.biz, schedapp.StatsQuery{From: "2030-03-01", To: "2030-03-31"})
	if len(byDay.Series) != 31 || byDay.Series[4].Figures.RevenueMinor != 5_000_000 || byDay.Series[11].Figures.AvailableMinutes != 180 {
		t.Errorf("by day: %d periods, 5th %+v, 12th %+v", len(byDay.Series), byDay.Series[4].Figures, byDay.Series[11].Figures)
	}
	mine, err := w.sched.MyStats(ctx, w.owner, q)
	if err != nil || len(mine.ByBusiness) != 1 || mine.Totals.RevenueMinor != 5_000_000 || mine.Totals.AvailableMinutes != 1260 {
		t.Errorf("my stats %+v %v", mine, err)
	}
	none, err := w.sched.MyStats(ctx, w.barber, q)
	if err != nil || len(none.ByBusiness) != 0 || none.Totals.Appointments != 0 {
		t.Errorf("the barber owns nothing: %+v %v", none, err)
	}
}
