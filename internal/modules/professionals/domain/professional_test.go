package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestProfessionalValues(t *testing.T) {
	if v, err := ParseDisplayName("  Juan el Barbero "); err != nil || v != "Juan el Barbero" {
		t.Errorf("name: %q %v", v, err)
	}
	for _, in := range []string{"", "J", strings.Repeat("a", 61), "Ju\nan"} {
		if _, err := ParseDisplayName(in); !errors.Is(err, ErrInvalidDisplayName) {
			t.Errorf("name %q: %v", in, err)
		}
	}
	if v, err := ParseBio(" "); err != nil || v != "" {
		t.Errorf("empty bio: %q %v", v, err)
	}
	if _, err := ParseBio("Fades.\nBarba."); err != nil {
		t.Error(err)
	}
	for _, in := range []string{strings.Repeat("a", 501), "a\x07"} {
		if _, err := ParseBio(in); !errors.Is(err, ErrInvalidBio) {
			t.Errorf("bio: %v", err)
		}
	}
	if s, err := ParseStatus(" inactive "); err != nil || s != StatusInactive {
		t.Errorf("status: %q %v", s, err)
	}
	if _, err := ParseStatus("PAUSED"); !errors.Is(err, ErrInvalidStatus) {
		t.Error("PAUSED accepted")
	}
	for n, ok := range map[int]bool{0: true, 1000: true, -1: false, 1001: false} {
		if (ValidateSortOrder(n) == nil) != ok {
			t.Errorf("sort order %d", n)
		}
	}
	p := Professional{Status: StatusActive}
	if !p.Bookable(true) || p.Bookable(false) || (Professional{Status: StatusInactive}).Bookable(true) {
		t.Error("Bookable")
	}
}
