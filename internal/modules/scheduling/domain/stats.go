package domain

// Statistics of a business (frontend proposal P-12, 2026-11-27): the rules of every figure,
// grouped by local day, month or year of the business.

import (
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
)

// Grouping is the period of a statistics series.
type Grouping string

// Groupings.
const (
	GroupByDay   Grouping = "day"
	GroupByMonth Grouping = "month"
	GroupByYear  Grouping = "year"
)

// Range limits (inclusive days from–to): one year by day, five years by month or year.
const (
	MaxDaysByDay   = 366
	MaxDaysByMonth = 5*365 + 2
)

// ParseGrouping reads day, month or year; empty is day.
func ParseGrouping(s string) (Grouping, bool) {
	switch Grouping(s) {
	case "", GroupByDay:
		return GroupByDay, true
	case GroupByMonth, GroupByYear:
		return Grouping(s), true
	}
	return "", false
}

// MaxDays is the longest range (inclusive days) allowed for the grouping.
func (g Grouping) MaxDays() int {
	if g == GroupByDay {
		return MaxDaysByDay
	}
	return MaxDaysByMonth
}

// Period labels the period of d: "2026-11-27", "2026-11" or "2026".
func (g Grouping) Period(d LocalDate) string {
	switch g {
	case GroupByMonth:
		return fmt.Sprintf("%04d-%02d", d.Year, d.Month)
	case GroupByYear:
		return fmt.Sprintf("%04d", d.Year)
	case GroupByDay:
	}
	return d.String()
}

// Periods lists the labels of every period touched by [from, to], in order.
func (g Grouping) Periods(from, to LocalDate) []string {
	var out []string
	for d := from; !to.Before(d); d = d.AddDays(1) {
		if p := g.Period(d); len(out) == 0 || out[len(out)-1] != p {
			out = append(out, p)
		}
	}
	return out
}

// DaysBetween counts the days of [from, to] (inclusive); 0 or less if to is before from.
func DaysBetween(from, to LocalDate) int {
	return int(to.civil().Sub(from.civil()).Hours()/24) + 1
}

// StatsAppointment is what the statistics read of an appointment.
type StatsAppointment struct {
	Start                     time.Time
	ProfessionalID, ServiceID uuid.UUID
	CustomerID                uuid.UUID // uuid.Nil if none
	ServiceName               string
	Status                    Status
	PriceMinor                int64
	DurationMinutes           int
}

// FirstVisit is the first COMPLETED appointment of a customer in the business.
type FirstVisit struct {
	CustomerID, ProfessionalID, ServiceID uuid.UUID
	Start                                 time.Time
}

// Figures are the statistics of a period, a professional, a service or a business.
type Figures struct {
	Appointments, Completed, Canceled, NoShow int
	RevenueMinor, AverageTicketMinor          int64
	CustomersServed, NewCustomers             int
	BookedMinutes, AvailableMinutes           int
	OccupancyPercent                          *int // nil without working hours
}

// Tally accumulates Figures; customers served are counted once each.
type Tally struct {
	f         Figures
	customers map[uuid.UUID]struct{}
}

func (t *Tally) add(a StatsAppointment) {
	switch a.Status {
	case StatusCancelled:
		t.f.Canceled++
		return
	case StatusNoShow:
		t.f.NoShow++
	case StatusPendingConfirmation, StatusConfirmed, StatusInProgress:
	case StatusCompleted:
		t.f.Completed++
		t.f.RevenueMinor += a.PriceMinor
		if a.CustomerID != uuid.Nil {
			if t.customers == nil {
				t.customers = map[uuid.UUID]struct{}{}
			}
			t.customers[a.CustomerID] = struct{}{}
		}
	}
	t.f.Appointments++ // every status but the canceled one
	if a.Status != StatusNoShow {
		t.f.BookedMinutes += a.DurationMinutes // PENDING_CONFIRMATION, CONFIRMED, IN_PROGRESS, COMPLETED
	}
}

// Merge adds o into t (the customers of different businesses are different records).
func (t *Tally) Merge(o *Tally) {
	t.f.Appointments += o.f.Appointments
	t.f.Completed += o.f.Completed
	t.f.Canceled += o.f.Canceled
	t.f.NoShow += o.f.NoShow
	t.f.RevenueMinor += o.f.RevenueMinor
	t.f.NewCustomers += o.f.NewCustomers
	t.f.BookedMinutes += o.f.BookedMinutes
	t.f.AvailableMinutes += o.f.AvailableMinutes
	for c := range o.customers {
		if t.customers == nil {
			t.customers = map[uuid.UUID]struct{}{}
		}
		t.customers[c] = struct{}{}
	}
}

// Figures computes the derived figures: average ticket, customers served and occupancy.
func (t *Tally) Figures() Figures {
	f := t.f
	f.CustomersServed = len(t.customers)
	if f.Completed > 0 {
		f.AverageTicketMinor = f.RevenueMinor / int64(f.Completed)
	}
	if f.AvailableMinutes > 0 {
		pct := min(int(math.Round(float64(f.BookedMinutes)*100/float64(f.AvailableMinutes))), 100)
		f.OccupancyPercent = &pct
	}
	return f
}

// Stats aggregates one business's statistics in its calendar.
type Stats struct {
	loc            *time.Location
	grouping       Grouping
	Total          Tally
	Series         map[string]*Tally
	ByProfessional map[uuid.UUID]*Tally
	ByService      map[uuid.UUID]*Tally
	ServiceNames   map[uuid.UUID]string
}

// NewStats prepares the statistics of [from, to] with an empty Tally for every period.
func NewStats(loc *time.Location, g Grouping, from, to LocalDate) *Stats {
	s := &Stats{loc: loc, grouping: g, Series: map[string]*Tally{}, ByProfessional: map[uuid.UUID]*Tally{},
		ByService: map[uuid.UUID]*Tally{}, ServiceNames: map[uuid.UUID]string{}}
	for _, p := range g.Periods(from, to) {
		s.Series[p] = &Tally{}
	}
	return s
}

func tallyOf(m map[uuid.UUID]*Tally, id uuid.UUID) *Tally {
	t, ok := m[id]
	if !ok {
		t = &Tally{}
		m[id] = t
	}
	return t
}

// period returns the Tally of the period of t (nil outside the range).
func (s *Stats) period(t time.Time) *Tally {
	return s.Series[s.grouping.Period(LocalDateOf(t, s.loc))]
}

// AddAppointment counts an appointment that starts in the range.
func (s *Stats) AddAppointment(a StatsAppointment) {
	s.Total.add(a)
	if p := s.period(a.Start); p != nil {
		p.add(a)
	}
	tallyOf(s.ByProfessional, a.ProfessionalID).add(a)
	tallyOf(s.ByService, a.ServiceID).add(a)
	if _, ok := s.ServiceNames[a.ServiceID]; !ok {
		s.ServiceNames[a.ServiceID] = a.ServiceName
	}
}

// AddFirstVisit counts a new customer: its first COMPLETED appointment in the business started
// in the range.
func (s *Stats) AddFirstVisit(v FirstVisit) {
	s.Total.f.NewCustomers++
	if p := s.period(v.Start); p != nil {
		p.f.NewCustomers++
	}
	tallyOf(s.ByProfessional, v.ProfessionalID).f.NewCustomers++
	tallyOf(s.ByService, v.ServiceID).f.NewCustomers++
}

// AddAvailable counts the working minutes of a professional on a day of the range.
func (s *Stats) AddAvailable(professionalID uuid.UUID, d LocalDate, minutes int) {
	if minutes <= 0 {
		return
	}
	s.Total.f.AvailableMinutes += minutes
	if p := s.Series[s.grouping.Period(d)]; p != nil {
		p.f.AvailableMinutes += minutes
	}
	tallyOf(s.ByProfessional, professionalID).f.AvailableMinutes += minutes
}

// WorkingMinutes returns the minutes a professional can serve on d: its working ranges minus its
// breaks and absences (the same subtraction as Available), in the business's time zone.
func WorkingMinutes(d LocalDate, working, breaks []LocalRange, absences []Interval, loc *time.Location) int {
	var holes []Interval
	for _, b := range breaks {
		holes = append(holes, localInterval(d, b, loc))
	}
	holes = append(holes, absences...)
	var total time.Duration
	for _, w := range working {
		for _, seg := range subtract(localInterval(d, w, loc), holes) {
			total += seg.End.Sub(seg.Start)
		}
	}
	return int(total / time.Minute)
}
