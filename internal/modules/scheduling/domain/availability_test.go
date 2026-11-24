package domain_test

import (
	"errors"
	"math/rand/v2"
	"reflect"
	"testing"
	"time"
	_ "time/tzdata" // same IANA database as the binaries (ADR-0005)

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/domain"
)

const mins = time.Minute

func zone(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func date(s string) domain.LocalDate {
	d, ok := domain.ParseLocalDate(s)
	if !ok {
		panic(s)
	}
	return d
}

func hm(h, m int) int { return h*60 + m }

// at is the instant of a local wall clock in loc (only for unambiguous times).
func at(loc *time.Location, d string, h, m int) time.Time {
	x := date(d)
	return time.Date(x.Year, x.Month, x.Day, h, m, 0, 0, loc)
}

func locals(slots []domain.Slot, loc *time.Location) []string {
	out := []string{}
	for _, s := range slots {
		out = append(out, s.Start.In(loc).Format("15:04"))
	}
	return out
}

func TestAvailable(t *testing.T) {
	bog := zone(t, "America/Bogota")
	const day = "2030-03-04"
	base := func() domain.AvailabilityInput {
		return domain.AvailabilityInput{
			Location:     bog,
			Policy:       domain.Policy{SlotInterval: 10 * mins, MinLead: 60 * mins, MaxAdvanceDays: 60},
			Duration:     40 * mins,
			Date:         date(day),
			WorkingHours: []domain.LocalRange{{hm(9, 0), hm(12, 0)}},
			Now:          at(bog, "2030-03-01", 12, 0),
		}
	}
	cases := []struct {
		name string
		edit func(*domain.AvailabilityInput)
		want []string
	}{
		{"design example: booked 09:40-10:20, D=40", func(in *domain.AvailabilityInput) {
			in.Booked = []domain.Booked{{Start: at(bog, day, 9, 40), End: at(bog, day, 10, 20)}}
		}, []string{"09:00", "10:20", "10:30", "10:40", "10:50", "11:00", "11:10", "11:20"}},
		{"buffers: new [t, t+D+B) vs booked [s, e+Bi)", func(in *domain.AvailabilityInput) {
			in.Policy.Buffer = 10 * mins
			in.Booked = []domain.Booked{{Start: at(bog, day, 9, 40), End: at(bog, day, 10, 20), Buffer: 10 * mins}}
		}, []string{"10:30", "10:40", "10:50", "11:00", "11:10", "11:20"}},
		{"buffer does not need to fit before closing", func(in *domain.AvailabilityInput) {
			in.Policy.Buffer = 30 * mins
			in.WorkingHours = []domain.LocalRange{{hm(11, 0), hm(12, 0)}}
		}, []string{"11:00", "11:10", "11:20"}},
		{"break splits the day; grid anchored to the range start", func(in *domain.AvailabilityInput) {
			in.Duration = 30 * mins
			in.WorkingHours = []domain.LocalRange{{hm(9, 5), hm(12, 0)}}
			in.Breaks = []domain.LocalRange{{hm(10, 0), hm(10, 30)}}
		}, []string{"09:05", "09:15", "09:25", "10:35", "10:45", "10:55", "11:05", "11:15", "11:25"}},
		{"absence removes part of the day", func(in *domain.AvailabilityInput) {
			in.Duration, in.Policy.SlotInterval = 30*mins, 15*mins
			in.Absences = []domain.Interval{{Start: at(bog, day, 10, 15), End: at(bog, day, 11, 0)}}
		}, []string{"09:00", "09:15", "09:30", "09:45", "11:00", "11:15", "11:30"}},
		{"absence covering the whole day", func(in *domain.AvailabilityInput) {
			in.Absences = []domain.Interval{{Start: at(bog, "2030-03-03", 0, 0), End: at(bog, "2030-03-06", 0, 0)}}
		}, []string{}},
		{"minimum notice on the same day", func(in *domain.AvailabilityInput) {
			in.Duration = 30 * mins
			in.Now = at(bog, day, 9, 25) // earliest 10:25 → next grid point 10:30
		}, []string{"10:30", "10:40", "10:50", "11:00", "11:10", "11:20", "11:30"}},
		{"two working ranges; the service never spans the gap", func(in *domain.AvailabilityInput) {
			in.Duration, in.Policy.SlotInterval = 60*mins, 20*mins
			in.WorkingHours = []domain.LocalRange{{hm(8, 0), hm(9, 30)}, {hm(9, 30), hm(10, 40)}}
		}, []string{"08:00", "08:20", "09:30"}},
		{"service longer than the day", func(in *domain.AvailabilityInput) {
			in.Duration = 4 * time.Hour
		}, []string{}},
		{"does not work that day", func(in *domain.AvailabilityInput) { in.WorkingHours = nil }, []string{}},
		{"canceled appointments are simply not passed", func(in *domain.AvailabilityInput) {
			in.Duration, in.Policy.SlotInterval = 60*mins, 60*mins
		}, []string{"09:00", "10:00", "11:00"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := base()
			c.edit(&in)
			got, err := domain.Available(in)
			if err != nil {
				t.Fatal(err)
			}
			if l := locals(got, bog); !reflect.DeepEqual(l, c.want) {
				t.Errorf("slots = %v, want %v", l, c.want)
			}
			for _, s := range got {
				if s.End.Sub(s.Start) != in.Duration || s.Start.Location() != time.UTC {
					t.Errorf("slot %v–%v: wrong duration or not UTC", s.Start, s.End)
				}
			}
		})
	}
}

func TestAvailableBookingWindow(t *testing.T) {
	bog := zone(t, "America/Bogota")
	in := domain.AvailabilityInput{
		Location: bog, Policy: domain.Policy{SlotInterval: 10 * mins, MaxAdvanceDays: 60}, Duration: 30 * mins,
		WorkingHours: []domain.LocalRange{{hm(9, 0), hm(10, 0)}},
		Now:          at(bog, "2030-03-01", 23, 30), // already 2 March in UTC
	}
	for d, want := range map[string]error{
		"2030-02-28": domain.ErrOutsideBookingWindow,
		"2030-03-01": nil, // today in the business's zone, even though UTC is a day ahead
		"2030-04-30": nil, // today + 60
		"2030-05-01": domain.ErrOutsideBookingWindow,
	} {
		in.Date = date(d)
		if _, err := domain.Available(in); !errors.Is(err, want) {
			t.Errorf("%s: err = %v, want %v", d, err, want)
		}
	}
	for name, edit := range map[string]func(*domain.AvailabilityInput){
		"no location":   func(in *domain.AvailabilityInput) { in.Location = nil },
		"zero duration": func(in *domain.AvailabilityInput) { in.Duration = 0 },
		"zero interval": func(in *domain.AvailabilityInput) { in.Policy.SlotInterval = 0 },
	} {
		bad := in
		bad.Date = date("2030-03-02")
		edit(&bad)
		if _, err := domain.Available(bad); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestAvailableAcrossDST(t *testing.T) {
	ny, mad := zone(t, "America/New_York"), zone(t, "Europe/Madrid")
	run := func(loc *time.Location, d string, interval, dur time.Duration, wh ...domain.LocalRange) []domain.Slot {
		t.Helper()
		got, err := domain.Available(domain.AvailabilityInput{
			Location: loc, Policy: domain.Policy{SlotInterval: interval, MaxAdvanceDays: 365}, Duration: dur,
			Date: date(d), WorkingHours: wh, Now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		})
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	utc := func(slots []domain.Slot) []string {
		out := []string{}
		for _, s := range slots {
			out = append(out, s.Start.Format("15:04Z"))
		}
		return out
	}

	// Spring forward (02:00 → 03:00): 01:00–04:00 local lasts two real hours.
	got := run(ny, "2026-03-08", 30*mins, 30*mins, domain.LocalRange{Start: hm(1, 0), End: hm(4, 0)})
	if l, u := locals(got, ny), utc(got); !reflect.DeepEqual(l, []string{"01:00", "01:30", "03:00", "03:30"}) ||
		!reflect.DeepEqual(u, []string{"06:00Z", "06:30Z", "07:00Z", "07:30Z"}) {
		t.Errorf("NY spring forward: %v %v", l, u)
	}
	// A range starting inside the gap keeps its existing part: it starts at the jump (02:30 → 03:00).
	got = run(ny, "2026-03-08", 30*mins, 30*mins, domain.LocalRange{Start: hm(2, 30), End: hm(5, 0)})
	if l := locals(got, ny); !reflect.DeepEqual(l, []string{"03:00", "03:30", "04:00", "04:30"}) {
		t.Errorf("NY range starting in the gap: %v", l)
	}
	// Fall back (02:00 → 01:00): 00:00–03:00 local lasts four real hours; 01:00 is offered twice,
	// as two distinct instants.
	got = run(ny, "2026-11-01", 60*mins, 60*mins, domain.LocalRange{Start: 0, End: hm(3, 0)})
	if l, u := locals(got, ny), utc(got); !reflect.DeepEqual(l, []string{"00:00", "01:00", "01:00", "02:00"}) ||
		!reflect.DeepEqual(u, []string{"04:00Z", "05:00Z", "06:00Z", "07:00Z"}) {
		t.Errorf("NY fall back: %v %v", l, u)
	}
	// Madrid: 02:00 → 03:00 on 29 March 2026, 03:00 → 02:00 on 25 October 2026.
	if l := locals(run(mad, "2026-03-29", 60*mins, 60*mins, domain.LocalRange{Start: hm(1, 0), End: hm(5, 0)}), mad); !reflect.DeepEqual(l, []string{"01:00", "03:00", "04:00"}) {
		t.Errorf("Madrid spring forward: %v", l)
	}
	if u := utc(run(mad, "2026-10-25", 60*mins, 60*mins, domain.LocalRange{Start: hm(1, 0), End: hm(4, 0)})); !reflect.DeepEqual(u, []string{"23:00Z", "00:00Z", "01:00Z", "02:00Z"}) {
		t.Errorf("Madrid fall back: %v", u)
	}
}

// TestResolveLocal checks the wall-clock conversion on every half hour of 2026 in zones with
// and without DST (including Santiago, whose transitions happen at midnight).
func TestResolveLocal(t *testing.T) {
	ny := zone(t, "America/New_York")
	if got := domain.ResolveLocal(date("2026-11-01"), hm(1, 30), ny); !got.Equal(time.Date(2026, 11, 1, 5, 30, 0, 0, time.UTC)) {
		t.Errorf("ambiguous 01:30 → %v, want the first occurrence (EDT)", got)
	}
	if got := domain.ResolveLocal(date("2026-03-08"), hm(2, 30), ny); !got.Equal(time.Date(2026, 3, 8, 7, 0, 0, 0, time.UTC)) {
		t.Errorf("nonexistent 02:30 → %v, want the jump (03:00 EDT)", got)
	}
	for _, name := range []string{"America/Bogota", "America/Mexico_City", "America/Santiago", "America/New_York", "Europe/Madrid"} {
		loc := zone(t, name)
		gaps, repeats := 0, 0
		for d := date("2026-01-01"); d.Before(date("2027-01-01")); d = d.AddDays(1) {
			prev := time.Time{}
			for m := 0; m < 24*60; m += 30 {
				r := domain.ResolveLocal(d, m, loc)
				local := r.In(loc)
				shift := time.Date(local.Year(), local.Month(), local.Day(), local.Hour(), local.Minute(), 0, 0, time.UTC).
					Sub(time.Date(d.Year, d.Month, d.Day, m/60, m%60, 0, 0, time.UTC))
				switch {
				case shift == 0:
				case shift > 0 && shift <= time.Hour:
					gaps++ // nonexistent time, moved forward
				default:
					t.Fatalf("%s %s %d: resolved to %v (shift %v)", name, d, m, local, shift)
				}
				if earlier := r.Add(-time.Hour).In(loc); shift == 0 && earlier.Hour() == local.Hour() && earlier.Minute() == local.Minute() && earlier.Day() == local.Day() {
					t.Fatalf("%s %s %d: %v is the second occurrence", name, d, m, local)
				}
				if r.Add(-time.Hour).In(loc).Format("2006-01-02 15:04") == local.Format("2006-01-02 15:04") {
					repeats++
				}
				if r.Before(prev) {
					t.Fatalf("%s %s %d: goes backwards (%v after %v)", name, d, m, r, prev)
				}
				prev = r
			}
		}
		hasDST := name != "America/Bogota" && name != "America/Mexico_City" // Mexico dropped DST in 2022
		if hasDST != (gaps > 0) || repeats != 0 {
			t.Errorf("%s: %d nonexistent half hours, %d second occurrences", name, gaps, repeats)
		}
	}
}

// TestAvailableMatchesNaive compares the engine with a minute-by-minute reference on random
// days (Bogota, no DST, so local minutes map 1:1 to instants).
func TestAvailableMatchesNaive(t *testing.T) {
	bog := zone(t, "America/Bogota")
	rng := rand.New(rand.NewPCG(5, 8)) //nolint:gosec // deterministic test data, not security
	d := date("2030-03-04")
	instant := func(m int) time.Time { return at(bog, d.String(), 0, 0).Add(time.Duration(m) * mins) }
	for iter := 0; iter < 3000; iter++ {
		in := domain.AvailabilityInput{
			Location: bog, Date: d, Now: instant(rng.IntN(16 * 60)).Add(-24 * time.Hour * time.Duration(rng.IntN(2))),
			Policy: domain.Policy{
				SlotInterval:   time.Duration([]int{5, 10, 15, 20}[rng.IntN(4)]) * mins,
				Buffer:         time.Duration(5*rng.IntN(4)) * mins,
				MinLead:        time.Duration(15*rng.IntN(5)) * mins,
				MaxAdvanceDays: 2,
			},
			Duration: time.Duration(5*(1+rng.IntN(24))) * mins,
		}
		for m := hm(6, 0) + 5*rng.IntN(12); m < hm(22, 0) && len(in.WorkingHours) < 3; {
			end := m + 5*(6+rng.IntN(60))
			in.WorkingHours = append(in.WorkingHours, domain.LocalRange{Start: m, End: end})
			if rng.IntN(2) == 0 {
				b := m + 5*rng.IntN((end-m)/5)
				in.Breaks = append(in.Breaks, domain.LocalRange{Start: b, End: b + 5*(1+rng.IntN(8))})
			}
			m = end + 5*rng.IntN(20)
		}
		for range rng.IntN(3) {
			s := hm(6, 0) + 5*rng.IntN(16*12)
			in.Absences = append(in.Absences, domain.Interval{Start: instant(s), End: instant(s + 5*(1+rng.IntN(30)))})
		}
		for range rng.IntN(5) {
			s := hm(6, 0) + 5*rng.IntN(16*12)
			in.Booked = append(in.Booked, domain.Booked{Start: instant(s), End: instant(s + 5*(1+rng.IntN(18))), Buffer: time.Duration(5*rng.IntN(3)) * mins})
		}
		got, err := domain.Available(in)
		if err != nil {
			t.Fatal(err)
		}
		want := naive(in, instant)
		if g := locals(got, bog); !reflect.DeepEqual(g, want) {
			t.Fatalf("iteration %d: %+v\n got  %v\n want %v", iter, in, g, want)
		}
	}
}

func naive(in domain.AvailabilityInput, instant func(int) time.Time) []string {
	free := func(wr domain.LocalRange, m int) bool {
		if m < wr.Start || m >= wr.End {
			return false
		}
		for _, b := range in.Breaks {
			if m >= b.Start && m < b.End {
				return false
			}
		}
		for _, a := range in.Absences {
			if !instant(m).Before(a.Start) && instant(m).Before(a.End) {
				return false
			}
		}
		return true
	}
	step, dur := int(in.Policy.SlotInterval/mins), int(in.Duration/mins)
	seen := map[int]bool{}
	var starts []int
	for _, wr := range in.WorkingHours {
	candidate:
		for t := wr.Start; t < wr.End; t += step {
			if seen[t] || instant(t).Before(in.Now.Add(in.Policy.MinLead)) {
				continue
			}
			for m := t; m < t+dur; m++ {
				if !free(wr, m) {
					continue candidate
				}
			}
			occEnd := instant(t + dur).Add(in.Policy.Buffer)
			for _, b := range in.Booked {
				if instant(t).Before(b.End.Add(b.Buffer)) && b.Start.Before(occEnd) {
					continue candidate
				}
			}
			seen[t] = true
			starts = append(starts, t)
		}
	}
	out := []string{}
	for _, s := range sortInts(starts) {
		out = append(out, instant(s).In(in.Location).Format("15:04"))
	}
	return out
}

func sortInts(a []int) []int {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j] < a[j-1]; j-- {
			a[j], a[j-1] = a[j-1], a[j]
		}
	}
	return a
}

func TestParseLocalDate(t *testing.T) {
	for s, ok := range map[string]bool{"2030-03-04": true, "2030-02-30": false, "2030-3-4": false, "": false, "2030-03-04T00:00:00Z": false} {
		if _, got := domain.ParseLocalDate(s); got != ok {
			t.Errorf("%q: %v", s, got)
		}
	}
	d := date("2030-12-31")
	if d.AddDays(1).String() != "2031-01-01" || d.Weekday() != time.Tuesday || !d.Before(d.AddDays(1)) || d.Before(d) {
		t.Error("LocalDate arithmetic")
	}
}
