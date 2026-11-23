package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestParseCountryCode(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want CountryCode
	}{{"CO", "CO"}, {" co ", "CO"}, {"mx", "MX"}} {
		got, err := ParseCountryCode(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("ParseCountryCode(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}
	for _, in := range []string{"", "C", "COL", "C0", "1A", "ÑA"} {
		if _, err := ParseCountryCode(in); !errors.Is(err, ErrInvalidCountryCode) {
			t.Errorf("ParseCountryCode(%q) err = %v, want ErrInvalidCountryCode", in, err)
		}
	}
}

func TestParseName(t *testing.T) {
	if got, err := ParseName("  Medellín  "); err != nil || got != "Medellín" {
		t.Errorf("ParseName trims: %q, %v", got, err)
	}
	if _, err := ParseName(strings.Repeat("á", MaxNameLength)); err != nil {
		t.Errorf("100 runes must be accepted: %v", err)
	}
	for _, in := range []string{"", "   ", strings.Repeat("a", MaxNameLength+1), "Cali\x00", "Ca\nli", "\xff"} {
		if _, err := ParseName(in); !errors.Is(err, ErrInvalidName) {
			t.Errorf("ParseName(%q) err = %v, want ErrInvalidName", in, err)
		}
	}
}

func TestParsePhoneCode(t *testing.T) {
	for _, in := range []string{"+57", "+1", "+1809"} {
		if _, err := ParsePhoneCode(in); err != nil {
			t.Errorf("ParsePhoneCode(%q): %v", in, err)
		}
	}
	for _, in := range []string{"", "57", "+0", "+12345", "+5a", "++57"} {
		if _, err := ParsePhoneCode(in); !errors.Is(err, ErrInvalidPhoneCode) {
			t.Errorf("ParsePhoneCode(%q) err = %v", in, err)
		}
	}
}

func TestParseCurrency(t *testing.T) {
	if got, err := ParseCurrency("cop"); err != nil || got != "COP" {
		t.Errorf("ParseCurrency(cop) = %q, %v", got, err)
	}
	for _, in := range []string{"", "EUR", "XXX", "CO", "COPP"} {
		if _, err := ParseCurrency(in); !errors.Is(err, ErrInvalidCurrency) {
			t.Errorf("ParseCurrency(%q) err = %v, want unsupported", in, err)
		}
	}
}

func TestParseDivisionCode(t *testing.T) {
	if got, err := ParseDivisionCode(" co-vac ", "CO"); err != nil || got != "CO-VAC" {
		t.Errorf("got %q, %v", got, err)
	}
	if got, err := ParseDivisionCode("", "CO"); err != nil || got != "" {
		t.Errorf("empty code is optional: %q, %v", got, err)
	}
	for _, in := range []string{"MX-JAL", "CO", "CO-", "CO-ABCD", "COVAC", "CO_VAC"} {
		if _, err := ParseDivisionCode(in, "CO"); !errors.Is(err, ErrInvalidDivisionCode) {
			t.Errorf("ParseDivisionCode(%q) err = %v", in, err)
		}
	}
}

func TestParseTimeZone(t *testing.T) {
	for _, in := range []string{"America/Bogota", "UTC", "America/Mexico_City"} {
		if _, err := ParseTimeZone(in); err != nil {
			t.Errorf("ParseTimeZone(%q): %v", in, err)
		}
	}
	for _, in := range []string{"", "Local", "Mars/Olympus", "../../etc/passwd", strings.Repeat("A", 65)} {
		if _, err := ParseTimeZone(in); !errors.Is(err, ErrInvalidTimeZone) {
			t.Errorf("ParseTimeZone(%q) err = %v", in, err)
		}
	}
}

func TestParseDivisionType(t *testing.T) {
	for _, in := range []string{"DEPARTMENT", "state", " Province ", "REGION", "DISTRICT"} {
		if _, err := ParseDivisionType(in); err != nil {
			t.Errorf("ParseDivisionType(%q): %v", in, err)
		}
	}
	if _, err := ParseDivisionType("COUNTY"); !errors.Is(err, ErrInvalidDivisionType) {
		t.Errorf("COUNTY err = %v", err)
	}
}
