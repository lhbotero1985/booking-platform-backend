package domain

import (
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestGrouping(t *testing.T) {
	for in, want := range map[string]Grouping{"": GroupByDay, "day": GroupByDay, "month": GroupByMonth, "year": GroupByYear} {
		if g, ok := ParseGrouping(in); !ok || g != want {
			t.Errorf("ParseGrouping(%q) = %q %v", in, g, ok)
		}
	}
	for _, bad := range []string{"week", "DAY", "dia"} {
		if _, ok := ParseGrouping(bad); ok {
			t.Errorf("ParseGrouping(%q) accepted", bad)
		}
	}
	from, _ := ParseLocalDate("2026-11-29")
	to, _ := ParseLocalDate("2027-01-02")
	if got := GroupByMonth.Periods(from, to); !reflect.DeepEqual(got, []string{"2026-11", "2026-12", "2027-01"}) {
		t.Errorf("months = %v", got)
	}
	if got := GroupByYear.Periods(from, to); !reflect.DeepEqual(got, []string{"2026", "2027"}) {
		t.Errorf("years = %v", got)
	}
	if got := GroupByDay.Periods(from, to); len(got) != 35 || got[0] != "2026-11-29" || got[34] != "2027-01-02" {
		t.Errorf("days = %d %v", len(got), got)
	}
	if n := DaysBetween(from, to); n != 35 {
		t.Errorf("DaysBetween = %d", n)
	}
	if n := DaysBetween(to, from); n > 0 {
		t.Errorf("reversed DaysBetween = %d", n)
	}
}

// TestStatsFigures applies every rule of P-12 to known appointments.
func TestStatsFigures(t *testing.T) {
	bog, _ := time.LoadLocation("America/Bogota")
	from, _ := ParseLocalDate("2026-11-01")
	to, _ := ParseLocalDate("2026-12-31")
	at := func(m time.Month, d, h int) time.Time { return time.Date(2026, m, d, h, 0, 0, 0, bog) }
	id := func() uuid.UUID { return uuid.Must(uuid.NewV7()) }
	juan, ana, corte, barba, laura, pedro := id(), id(), id(), id(), id(), id()
	s := NewStats(bog, GroupByMonth, from, to)
	for _, a := range []StatsAppointment{
		{Start: at(11, 3, 9), ProfessionalID: juan, ServiceID: corte, ServiceName: "Corte", CustomerID: laura, Status: StatusCompleted, PriceMinor: 30_000, DurationMinutes: 30},
		{Start: at(11, 10, 9), ProfessionalID: juan, ServiceID: corte, ServiceName: "Corte", CustomerID: laura, Status: StatusCompleted, PriceMinor: 30_000, DurationMinutes: 30},
		{Start: at(11, 10, 10), ProfessionalID: ana, ServiceID: barba, ServiceName: "Barba", CustomerID: pedro, Status: StatusCompleted, PriceMinor: 15_000, DurationMinutes: 20},
		{Start: at(11, 11, 9), ProfessionalID: juan, ServiceID: corte, CustomerID: pedro, Status: StatusCancelled, PriceMinor: 30_000, DurationMinutes: 30},
		{Start: at(11, 12, 9), ProfessionalID: ana, ServiceID: corte, CustomerID: laura, Status: StatusNoShow, PriceMinor: 30_000, DurationMinutes: 30},
		{Start: at(12, 1, 9), ProfessionalID: ana, ServiceID: corte, CustomerID: pedro, Status: StatusConfirmed, PriceMinor: 30_000, DurationMinutes: 30},
		{Start: at(12, 2, 9), ProfessionalID: juan, ServiceID: barba, CustomerID: laura, Status: StatusPendingConfirmation, PriceMinor: 15_000, DurationMinutes: 20},
	} {
		s.AddAppointment(a)
	}
	s.AddFirstVisit(FirstVisit{CustomerID: pedro, ProfessionalID: ana, ServiceID: barba, Start: at(11, 10, 10)})
	s.AddAvailable(juan, from, 300)
	s.AddAvailable(ana, from.AddDays(31), 100) // 2026-12-02
	s.AddAvailable(ana, from, 0)

	total := s.Total.Figures()
	pct := func(n int) *int { return &n }
	want := Figures{Appointments: 6, Completed: 3, Canceled: 1, NoShow: 1, RevenueMinor: 75_000, AverageTicketMinor: 25_000,
		CustomersServed: 2, NewCustomers: 1, BookedMinutes: 130, AvailableMinutes: 400, OccupancyPercent: pct(33)}
	if !reflect.DeepEqual(total, want) {
		t.Errorf("totals\n got %+v\nwant %+v", total, want)
	}
	nov, dec := s.Series["2026-11"].Figures(), s.Series["2026-12"].Figures()
	if nov.Appointments != 4 || nov.RevenueMinor != 75_000 || nov.BookedMinutes != 80 || *nov.OccupancyPercent != 27 || nov.NewCustomers != 1 {
		t.Errorf("november %+v", nov)
	}
	if dec.Appointments != 2 || dec.Completed != 0 || dec.AverageTicketMinor != 0 || dec.CustomersServed != 0 || dec.BookedMinutes != 50 || *dec.OccupancyPercent != 50 {
		t.Errorf("december %+v", dec)
	}
	// By professional, the additive figures add up to the totals.
	j, a := s.ByProfessional[juan].Figures(), s.ByProfessional[ana].Figures()
	if j.RevenueMinor+a.RevenueMinor != total.RevenueMinor || j.Appointments+a.Appointments != total.Appointments ||
		j.AvailableMinutes+a.AvailableMinutes != total.AvailableMinutes || j.NewCustomers+a.NewCustomers != 1 {
		t.Errorf("juan %+v ana %+v", j, a)
	}
	if b := s.ByService[barba].Figures(); b.Appointments != 2 || b.OccupancyPercent != nil || s.ServiceNames[barba] != "Barba" {
		t.Errorf("barba %+v %q", b, s.ServiceNames[barba])
	}
	// Occupancy is capped at 100 and absent without working hours.
	var over Tally
	over.add(StatsAppointment{Status: StatusConfirmed, DurationMinutes: 90})
	over.f.AvailableMinutes = 60
	if f := over.Figures(); *f.OccupancyPercent != 100 {
		t.Errorf("over-booked occupancy = %d", *f.OccupancyPercent)
	}
	var none Tally
	if f := none.Figures(); f.OccupancyPercent != nil {
		t.Error("occupancy without working hours")
	}
	// Merging two businesses keeps their customers apart.
	other := NewStats(bog, GroupByMonth, from, to)
	other.AddAppointment(StatsAppointment{Start: at(11, 5, 9), ProfessionalID: id(), ServiceID: id(), CustomerID: id(), Status: StatusCompleted, PriceMinor: 10_000, DurationMinutes: 30})
	s.Total.Merge(&other.Total)
	if f := s.Total.Figures(); f.CustomersServed != 3 || f.RevenueMinor != 85_000 || f.Completed != 4 {
		t.Errorf("merged %+v", f)
	}
}

func TestWorkingMinutes(t *testing.T) {
	bog, _ := time.LoadLocation("America/Bogota")
	d, _ := ParseLocalDate("2026-11-27")
	working := []LocalRange{{Start: 9 * 60, End: 13 * 60}, {Start: 14 * 60, End: 18 * 60}}
	breaks := []LocalRange{{Start: 11 * 60, End: 11*60 + 30}}
	if n := WorkingMinutes(d, working, breaks, nil, bog); n != 450 {
		t.Errorf("working - break = %d, want 450", n)
	}
	// An absence from 12:00 to 15:00 removes 1 h of the morning and 1 h of the afternoon.
	abs := []Interval{{Start: time.Date(2026, 11, 27, 12, 0, 0, 0, bog), End: time.Date(2026, 11, 27, 15, 0, 0, 0, bog)}}
	if n := WorkingMinutes(d, working, breaks, abs, bog); n != 330 {
		t.Errorf("with absence = %d, want 330", n)
	}
	whole := []Interval{{Start: time.Date(2026, 11, 26, 0, 0, 0, 0, bog), End: time.Date(2026, 11, 28, 0, 0, 0, 0, bog)}}
	if n := WorkingMinutes(d, working, breaks, whole, bog); n != 0 {
		t.Errorf("absent all day = %d", n)
	}
	// DST: Madrid's 2026-03-29 loses 02:00–03:00, a 01:00–05:00 shift lasts 3 h.
	mad, _ := time.LoadLocation("Europe/Madrid")
	spring, _ := ParseLocalDate("2026-03-29")
	if n := WorkingMinutes(spring, []LocalRange{{Start: 60, End: 300}}, nil, nil, mad); n != 180 {
		t.Errorf("DST day = %d, want 180", n)
	}
}
