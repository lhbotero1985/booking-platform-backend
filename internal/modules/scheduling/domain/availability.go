// Package domain holds the scheduling rules: the availability engine (T6.1) and, later, the
// appointment aggregate. It is pure: no database, no clock, no other module
// (availability-engine.md, ADR-0004, ADR-0005).
package domain

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// Errors of the availability engine.
var (
	// ErrOutsideBookingWindow: the date is before today or beyond the maximum advance, in the
	// business's local calendar.
	ErrOutsideBookingWindow = errors.New("scheduling: date outside the booking window")
	// ErrInvalidInput: a programming error (missing location, non-positive duration or interval).
	ErrInvalidInput = errors.New("scheduling: invalid availability input")
)

// LocalDate is a calendar day in the business's time zone.
type LocalDate struct {
	Year  int
	Month time.Month
	Day   int
}

// ParseLocalDate parses "YYYY-MM-DD".
func ParseLocalDate(s string) (LocalDate, bool) {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil || t.Format(time.DateOnly) != s {
		return LocalDate{}, false
	}
	return LocalDate{t.Year(), t.Month(), t.Day()}, true
}

// LocalDateOf is the calendar day of the instant t in loc.
func LocalDateOf(t time.Time, loc *time.Location) LocalDate {
	y, m, d := t.In(loc).Date()
	return LocalDate{y, m, d}
}

// String formats "YYYY-MM-DD".
func (d LocalDate) String() string { return fmt.Sprintf("%04d-%02d-%02d", d.Year, d.Month, d.Day) }

// Weekday of the date.
func (d LocalDate) Weekday() time.Weekday { return d.civil().Weekday() }

// AddDays returns the date n days later.
func (d LocalDate) AddDays(n int) LocalDate {
	t := d.civil().AddDate(0, 0, n)
	return LocalDate{t.Year(), t.Month(), t.Day()}
}

// Before reports whether d is earlier than o.
func (d LocalDate) Before(o LocalDate) bool { return d.civil().Before(o.civil()) }

func (d LocalDate) civil() time.Time { return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC) }

// LocalRange is [Start, End) in minutes since local midnight (0–1439), as stored for working
// hours and breaks.
type LocalRange struct{ Start, End int }

// Interval is an absolute [Start, End).
type Interval struct{ Start, End time.Time }

func (i Interval) overlaps(o Interval) bool { return i.Start.Before(o.End) && o.Start.Before(i.End) }

// Booked is an active appointment of the professional: it occupies [Start, End + Buffer).
type Booked struct {
	Start, End time.Time
	Buffer     time.Duration
}

// Policy is the booking policy of the business.
type Policy struct {
	SlotInterval   time.Duration // spacing of the offered start times (5/10/15/20 min)
	Buffer         time.Duration // preparation time after each appointment (0–60 min)
	MinLead        time.Duration // minimum notice before the start
	MaxAdvanceDays int           // furthest bookable local day, counted from today
}

// AvailabilityInput is everything the engine needs for one professional, one service and one
// local day.
type AvailabilityInput struct {
	Location     *time.Location
	Policy       Policy
	Duration     time.Duration // service duration (snapshot), never rounded
	Date         LocalDate
	WorkingHours []LocalRange // of Date's weekday
	Breaks       []LocalRange // of Date's weekday
	Absences     []Interval
	Booked       []Booked
	Now          time.Time
}

// Slot is a bookable start: the service runs [Start, End). Times are UTC; present them with
// Start.In(location).
type Slot struct{ Start, End time.Time }

// Available returns the bookable slots of the day, sorted and without duplicates
// (availability-engine.md §3):
//   - working ranges are converted to instants (ResolveLocal), then breaks and absences are
//     removed, leaving free segments;
//   - candidates step by SlotInterval in absolute time from the start of their working range,
//     so the grid is stable around breaks and DST transitions;
//   - a candidate is valid when the whole service fits in a free segment (the buffer does not
//     need to fit before closing), [t, t+Duration+Buffer) does not overlap any booked
//     [start, end+buffer), and t >= now + MinLead.
func Available(in AvailabilityInput) ([]Slot, error) {
	if in.Location == nil || in.Duration <= 0 || in.Policy.SlotInterval <= 0 || in.Policy.Buffer < 0 || in.Policy.MinLead < 0 {
		return nil, ErrInvalidInput
	}
	today := LocalDateOf(in.Now, in.Location)
	if in.Date.Before(today) || today.AddDays(in.Policy.MaxAdvanceDays).Before(in.Date) {
		return nil, ErrOutsideBookingWindow
	}
	earliest := in.Now.Add(in.Policy.MinLead)
	var holes []Interval
	for _, b := range in.Breaks {
		holes = append(holes, localInterval(in.Date, b, in.Location))
	}
	holes = append(holes, in.Absences...)

	seen := map[int64]bool{}
	var out []Slot
	for _, wr := range in.WorkingHours {
		window := localInterval(in.Date, wr, in.Location)
		for _, seg := range subtract(window, holes) {
			// First grid point at or after the segment start.
			k := int64(0)
			if gap := seg.Start.Sub(window.Start); gap > 0 {
				k = int64((gap + in.Policy.SlotInterval - 1) / in.Policy.SlotInterval)
			}
			for t := window.Start.Add(time.Duration(k) * in.Policy.SlotInterval); !t.Add(in.Duration).After(seg.End); t = t.Add(in.Policy.SlotInterval) {
				if t.Before(earliest) || seen[t.UnixNano()] {
					continue
				}
				if collides(Interval{t, t.Add(in.Duration + in.Policy.Buffer)}, in.Booked) {
					continue
				}
				seen[t.UnixNano()] = true
				out = append(out, Slot{Start: t.UTC(), End: t.Add(in.Duration).UTC()})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	return out, nil
}

func collides(occ Interval, booked []Booked) bool {
	for _, b := range booked {
		if occ.overlaps(Interval{b.Start, b.End.Add(b.Buffer)}) {
			return true
		}
	}
	return false
}

// subtract removes the holes from w and returns the remaining non-empty segments in order.
func subtract(w Interval, holes []Interval) []Interval {
	segs := []Interval{w}
	for _, h := range holes {
		var next []Interval
		for _, s := range segs {
			if !s.overlaps(h) {
				next = append(next, s)
				continue
			}
			if s.Start.Before(h.Start) {
				next = append(next, Interval{s.Start, h.Start})
			}
			if h.End.Before(s.End) {
				next = append(next, Interval{h.End, s.End})
			}
		}
		segs = next
	}
	return segs
}

func localInterval(d LocalDate, r LocalRange, loc *time.Location) Interval {
	return Interval{ResolveLocal(d, r.Start, loc), ResolveLocal(d, r.End, loc)}
}

// ResolveLocal converts a local wall clock (minutes since midnight of d) to an instant,
// deterministically (ADR-0005):
//   - a normal time maps to its only instant;
//   - an ambiguous time (clocks go back) maps to its first occurrence;
//   - a nonexistent time (clocks go forward) maps to the instant of the jump (02:30 → 03:00),
//     so a working range that starts inside the gap keeps only its existing part and the
//     conversion never goes backwards.
//
// Go's time.Date leaves both edge cases unspecified, hence this function. It assumes at most
// one offset change within a day either side of d, true for every real zone.
func ResolveLocal(d LocalDate, minute int, loc *time.Location) time.Time {
	wall := time.Date(d.Year, d.Month, d.Day, minute/60, minute%60, 0, 0, time.UTC)
	before := offsetAt(wall.Add(-24*time.Hour), loc)
	after := offsetAt(wall.Add(24*time.Hour), loc)
	var best time.Time
	for _, off := range []int{before, after} {
		t := wall.Add(-time.Duration(off) * time.Second)
		if sameWall(t.In(loc), wall) && (best.IsZero() || t.Before(best)) {
			best = t
		}
	}
	if best.IsZero() { // in the gap: the instant the clock jumps over it
		best = jumpBetween(wall.Add(-time.Duration(after)*time.Second), wall.Add(-time.Duration(before)*time.Second), loc)
	}
	return best.UTC()
}

// jumpBetween finds, to the second, the first instant in (lo, hi] whose offset differs from
// lo's: the moment of the transition.
func jumpBetween(lo, hi time.Time, loc *time.Location) time.Time {
	from := offsetAt(lo, loc)
	for hi.Sub(lo) > time.Second {
		mid := lo.Add(hi.Sub(lo) / 2).Truncate(time.Second)
		if offsetAt(mid, loc) == from {
			lo = mid
		} else {
			hi = mid
		}
	}
	return hi
}

func offsetAt(t time.Time, loc *time.Location) int {
	_, off := t.In(loc).Zone()
	return off
}

func sameWall(local, wall time.Time) bool {
	y, m, d := local.Date()
	wy, wm, wd := wall.Date()
	return y == wy && m == wm && d == wd && local.Hour() == wall.Hour() && local.Minute() == wall.Minute()
}
