package domain

import (
	"reflect"
	"testing"
	"time"
)

func rng(start, end string) RangeInput { return RangeInput{Start: start, End: end} }

func TestParseLocalTime(t *testing.T) {
	for in, want := range map[string]LocalTime{"00:00": 0, "09:30": 570, "23:59": 1439} {
		if got, ok := ParseLocalTime(in); !ok || got != want {
			t.Errorf("%s = %d %v", in, got, ok)
		}
		if want.String() != in {
			t.Errorf("String(%d) = %s", want, want.String())
		}
	}
	for _, in := range []string{"", "9:30", "24:00", "12:60", "12-30", "12:3a", "093:0"} {
		if _, ok := ParseLocalTime(in); ok {
			t.Errorf("%q accepted", in)
		}
	}
}

func TestNewWeeklyScheduleValid(t *testing.T) {
	w, vs := NewWeeklySchedule([]DayInput{
		{Weekday: "monday", WorkingHours: []RangeInput{rng("14:00", "19:00"), rng("09:00", "13:00")}, Breaks: []RangeInput{rng("16:00", "16:15"), rng("11:00", "11:30")}},
		{Weekday: " SATURDAY ", WorkingHours: []RangeInput{rng("08:00", "12:00"), rng("12:00", "14:00")}}, // touching ranges
		{Weekday: "SUNDAY"}, // explicit day off
	})
	if vs != nil {
		t.Fatalf("violations: %v", vs)
	}
	mon := w.Days[time.Monday]
	if mon.Weekday != time.Monday || len(mon.WorkingHours) != 2 || mon.WorkingHours[0] != (TimeRange{540, 780}) || mon.Breaks[0] != (TimeRange{660, 690}) {
		t.Errorf("monday sorted = %+v", mon)
	}
	if len(w.Days[time.Saturday].WorkingHours) != 2 || len(w.Days[time.Tuesday].WorkingHours) != 0 || w.Days[time.Sunday].WorkingHours != nil {
		t.Errorf("week = %+v", w.Days)
	}
	if same := w; !w.Equal(same) || w.Equal(EmptyWeek()) {
		t.Error("Equal")
	}
	other, _ := NewWeeklySchedule([]DayInput{{Weekday: "MONDAY", WorkingHours: []RangeInput{rng("09:00", "13:00"), rng("14:00", "19:00")}, Breaks: []RangeInput{rng("11:00", "11:30")}}})
	if w.Equal(other) {
		t.Error("different breaks are equal")
	}
	if empty, vs := NewWeeklySchedule(nil); vs != nil || !empty.Equal(EmptyWeek()) {
		t.Errorf("empty week: %v", vs)
	}
}

func TestNewWeeklyScheduleViolations(t *testing.T) {
	seven := make([]RangeInput, MaxRangesPerDay+1)
	for i := range seven {
		seven[i] = rng("09:00", "10:00")
	}
	_, vs := NewWeeklySchedule([]DayInput{
		{Weekday: "FUNDAY"},
		{Weekday: "MONDAY", WorkingHours: []RangeInput{rng("09:00", "13:00"), rng("12:00", "15:00")}, Breaks: []RangeInput{rng("12:30", "13:30"), rng("10:00", "10:30"), rng("10:15", "10:45")}},
		{Weekday: "monday"},
		{Weekday: "TUESDAY", WorkingHours: []RangeInput{rng("9:00", "13:00"), rng("13:00", "13:00")}, Breaks: []RangeInput{rng("14:00", "13:00")}},
		{Weekday: "WEDNESDAY", WorkingHours: seven},
		{Weekday: "THURSDAY", WorkingHours: []RangeInput{rng("09:00", "12:00")}, Breaks: []RangeInput{rng("08:30", "09:30"), rng("12:00", "12:30")}},
		{Weekday: "FRIDAY", Breaks: []RangeInput{rng("12:00", "13:00")}},
	})
	want := []Violation{
		{"days[0].weekday", CodeInvalidWeekday},
		{"days[1].workingHours[1]", CodeOverlapping},
		{"days[1].breaks[2]", CodeOverlapping},
		{"days[2].weekday", CodeDuplicateWeekday},
		{"days[3].workingHours[0]", CodeInvalidTime},
		{"days[3].workingHours[1]", CodeInvalidRange},
		{"days[3].breaks[0]", CodeInvalidRange},
		{"days[4].workingHours", CodeTooManyRanges},
		{"days[5].breaks[0]", CodeBreakOutside},
		{"days[5].breaks[1]", CodeBreakOutside},
		{"days[6].breaks[0]", CodeBreakOutside},
	}
	// Monday's 12:30–13:30 break lies inside the (overlapping) 12:00–15:00 range: only overlaps
	// are reported there.
	if !reflect.DeepEqual(vs, want) {
		t.Errorf("violations:\n got %v\nwant %v", vs, want)
	}
}

func TestWeekdayName(t *testing.T) {
	if WeekdayName(time.Wednesday) != "WEDNESDAY" {
		t.Error(WeekdayName(time.Wednesday))
	}
}
