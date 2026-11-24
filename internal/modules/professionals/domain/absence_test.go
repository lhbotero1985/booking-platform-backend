package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestValidatePeriod(t *testing.T) {
	bogota := time.FixedZone("COT", -5*3600)
	start := time.Date(2026, 12, 1, 8, 0, 0, 0, bogota)
	s, e, err := ValidatePeriod(start, start.Add(3*time.Hour+500*time.Millisecond))
	if err != nil || s.Location() != time.UTC || s.Hour() != 13 || e.Sub(s) != 3*time.Hour {
		t.Errorf("valid: %v %v %v", s, e, err)
	}
	if _, _, err := ValidatePeriod(start, start.Add(MaxAbsenceDuration)); err != nil {
		t.Errorf("366 days: %v", err)
	}
	for name, tc := range map[string]struct {
		start, end time.Time
		want       error
	}{
		"equal":        {start, start, ErrInvalidPeriod},
		"inverted":     {start, start.Add(-time.Minute), ErrInvalidPeriod},
		"sub-second":   {start, start.Add(time.Millisecond), ErrInvalidPeriod},
		"zero start":   {time.Time{}, start, ErrInvalidPeriod},
		"over a year":  {start, start.Add(MaxAbsenceDuration + time.Second), ErrPeriodTooLong},
		"zero end now": {start, time.Time{}, ErrInvalidPeriod},
	} {
		if _, _, err := ValidatePeriod(tc.start, tc.end); !errors.Is(err, tc.want) {
			t.Errorf("%s: %v, want %v", name, err, tc.want)
		}
	}
}

func TestParseReason(t *testing.T) {
	if r, err := ParseReason("  Incapacidad médica "); err != nil || r != "Incapacidad médica" {
		t.Errorf("reason = %q %v", r, err)
	}
	if r, err := ParseReason("   "); err != nil || r != "" {
		t.Errorf("blank = %q %v", r, err)
	}
	for _, bad := range []string{strings.Repeat("a", MaxAbsenceReason+1), "línea\notra", "x\x00"} {
		if _, err := ParseReason(bad); !errors.Is(err, ErrInvalidReason) {
			t.Errorf("%q accepted", bad)
		}
	}
}
