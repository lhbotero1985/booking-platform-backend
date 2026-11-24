package domain

// Weekly schedule of a professional (T5.3, domain-model.md §5): working ranges per weekday and
// recurring breaks, in the business's local time (interpreted with its IANA time zone by the
// availability engine). Ranges never cross midnight in the MVP.

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Limits.
const (
	// MaxRangesPerDay bounds working ranges and breaks per weekday.
	MaxRangesPerDay = 6
)

// Violation codes of a weekly schedule.
const (
	CodeInvalidWeekday   = "INVALID_WEEKDAY"
	CodeDuplicateWeekday = "DUPLICATE_WEEKDAY"
	CodeInvalidTime      = "INVALID_TIME"
	CodeInvalidRange     = "INVALID_RANGE"
	CodeTooManyRanges    = "TOO_MANY_RANGES"
	CodeOverlapping      = "OVERLAPPING_RANGES"
	CodeBreakOutside     = "BREAK_OUTSIDE_WORKING_HOURS"
)

// Violation is a rule broken by a schedule, located by a JSON-like path
// (e.g. "days[0].breaks[1]").
type Violation struct{ Path, Code string }

// LocalTime is a time of day in minutes since midnight (0–1439).
type LocalTime int

// ParseLocalTime parses "HH:MM" (00:00–23:59).
func ParseLocalTime(s string) (LocalTime, bool) {
	if len(s) != 5 || s[2] != ':' {
		return 0, false
	}
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, false
	}
	return LocalTime(t.Hour()*60 + t.Minute()), true
}

// String formats "HH:MM".
func (t LocalTime) String() string { return fmt.Sprintf("%02d:%02d", int(t)/60, int(t)%60) }

// TimeRange is [Start, End) within one day.
type TimeRange struct{ Start, End LocalTime }

func (r TimeRange) overlaps(o TimeRange) bool { return r.Start < o.End && o.Start < r.End }

func (r TimeRange) contains(o TimeRange) bool { return r.Start <= o.Start && o.End <= r.End }

// Day is the schedule of one weekday; no working ranges means the professional does not work.
type Day struct {
	Weekday      time.Weekday
	WorkingHours []TimeRange
	Breaks       []TimeRange
}

// WeeklySchedule holds the seven days (index = time.Weekday), ranges sorted by start.
type WeeklySchedule struct{ Days [7]Day }

// EmptyWeek returns a schedule with no working hours.
func EmptyWeek() WeeklySchedule {
	var w WeeklySchedule
	for d := range w.Days {
		w.Days[d].Weekday = time.Weekday(d)
	}
	return w
}

// Equal reports whether both schedules are identical.
func (w WeeklySchedule) Equal(o WeeklySchedule) bool {
	for d := range w.Days {
		if !sameRanges(w.Days[d].WorkingHours, o.Days[d].WorkingHours) || !sameRanges(w.Days[d].Breaks, o.Days[d].Breaks) {
			return false
		}
	}
	return true
}

func sameRanges(a, b []TimeRange) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// RangeInput is an unvalidated "HH:MM"–"HH:MM" range.
type RangeInput struct{ Start, End string }

// DayInput is an unvalidated day of the week (Weekday: MONDAY … SUNDAY).
type DayInput struct {
	Weekday      string
	WorkingHours []RangeInput
	Breaks       []RangeInput
}

var weekdays = map[string]time.Weekday{
	"SUNDAY": time.Sunday, "MONDAY": time.Monday, "TUESDAY": time.Tuesday, "WEDNESDAY": time.Wednesday,
	"THURSDAY": time.Thursday, "FRIDAY": time.Friday, "SATURDAY": time.Saturday,
}

// WeekdayName returns MONDAY … SUNDAY.
func WeekdayName(d time.Weekday) string { return strings.ToUpper(d.String()) }

// NewWeeklySchedule validates a full week; omitted days are days off. It returns every
// violation found (a day with invalid ranges is not checked for overlaps or containment).
func NewWeeklySchedule(days []DayInput) (WeeklySchedule, []Violation) {
	w := EmptyWeek()
	var vs []Violation
	seen := map[time.Weekday]bool{}
	for i, in := range days {
		path := fmt.Sprintf("days[%d]", i)
		wd, ok := weekdays[strings.ToUpper(strings.TrimSpace(in.Weekday))]
		if !ok {
			vs = append(vs, Violation{path + ".weekday", CodeInvalidWeekday})
			continue
		}
		if seen[wd] {
			vs = append(vs, Violation{path + ".weekday", CodeDuplicateWeekday})
			continue
		}
		seen[wd] = true
		work, v1 := parseRanges(path+".workingHours", in.WorkingHours)
		breaks, v2 := parseRanges(path+".breaks", in.Breaks)
		vs = append(vs, v1...)
		vs = append(vs, v2...)
		if len(v1)+len(v2) > 0 {
			continue
		}
		vs = append(vs, overlaps(path+".workingHours", work)...)
		vs = append(vs, overlaps(path+".breaks", breaks)...)
		for _, b := range breaks {
			if !containedInAny(b.r, work) {
				vs = append(vs, Violation{fmt.Sprintf("%s.breaks[%d]", path, b.i), CodeBreakOutside})
			}
		}
		w.Days[wd].WorkingHours = sortedRanges(work)
		w.Days[wd].Breaks = sortedRanges(breaks)
	}
	if len(vs) > 0 {
		return WeeklySchedule{}, vs
	}
	return w, nil
}

type indexed struct {
	i int
	r TimeRange
}

func parseRanges(path string, in []RangeInput) ([]indexed, []Violation) {
	if len(in) > MaxRangesPerDay {
		return nil, []Violation{{path, CodeTooManyRanges}}
	}
	var out []indexed
	var vs []Violation
	for i, r := range in {
		p := fmt.Sprintf("%s[%d]", path, i)
		start, ok1 := ParseLocalTime(strings.TrimSpace(r.Start))
		end, ok2 := ParseLocalTime(strings.TrimSpace(r.End))
		switch {
		case !ok1 || !ok2:
			vs = append(vs, Violation{p, CodeInvalidTime})
		case start >= end:
			vs = append(vs, Violation{p, CodeInvalidRange})
		default:
			out = append(out, indexed{i, TimeRange{start, end}})
		}
	}
	return out, vs
}

// overlaps reports each range that overlaps an earlier one (touching ranges are allowed).
func overlaps(path string, rs []indexed) []Violation {
	var vs []Violation
	for j := range rs {
		for k := 0; k < j; k++ {
			if rs[j].r.overlaps(rs[k].r) {
				vs = append(vs, Violation{fmt.Sprintf("%s[%d]", path, rs[j].i), CodeOverlapping})
				break
			}
		}
	}
	return vs
}

func containedInAny(r TimeRange, work []indexed) bool {
	for _, w := range work {
		if w.r.contains(r) {
			return true
		}
	}
	return false
}

func sortedRanges(rs []indexed) []TimeRange {
	if len(rs) == 0 {
		return nil
	}
	out := make([]TimeRange, len(rs))
	for i, r := range rs {
		out[i] = r.r
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	return out
}
