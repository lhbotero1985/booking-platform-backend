package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestNames(t *testing.T) {
	if v, err := ParseServiceName("  Corte + barba "); err != nil || v != "Corte + barba" {
		t.Errorf("service name = %q %v", v, err)
	}
	if v, err := ParseCategoryName("Barba"); err != nil || v != "Barba" {
		t.Errorf("category name = %q %v", v, err)
	}
	for _, bad := range []string{"", " x ", "a\nb", "a\x00b", strings.Repeat("a", MaxServiceName+1)} {
		if _, err := ParseServiceName(bad); !errors.Is(err, ErrInvalidName) {
			t.Errorf("service name %q accepted", bad)
		}
	}
	if _, err := ParseCategoryName(strings.Repeat("a", MaxCategoryName+1)); !errors.Is(err, ErrInvalidName) {
		t.Error("long category name accepted")
	}
	if v, err := ParseDescription(" Línea 1\nLínea 2 "); err != nil || v != "Línea 1\nLínea 2" {
		t.Errorf("description = %q %v", v, err)
	}
	if v, err := ParseDescription("  "); err != nil || v != "" {
		t.Errorf("blank description = %q %v", v, err)
	}
	for _, bad := range []string{"a\x07", strings.Repeat("a", MaxDescription+1)} {
		if _, err := ParseDescription(bad); !errors.Is(err, ErrInvalidDescription) {
			t.Errorf("description %q accepted", bad)
		}
	}
}

func TestNumbers(t *testing.T) {
	for _, ok := range []int{5, 30, 45, 480} {
		if ValidateDuration(ok) != nil {
			t.Errorf("duration %d rejected", ok)
		}
	}
	for _, bad := range []int{0, 4, 7, 485, -5} {
		if !errors.Is(ValidateDuration(bad), ErrInvalidDuration) {
			t.Errorf("duration %d accepted", bad)
		}
	}
	for _, ok := range []int64{0, 2_500_000, MaxPriceMinor} {
		if ValidatePrice(ok) != nil {
			t.Errorf("price %d rejected", ok)
		}
	}
	for _, bad := range []int64{-100, 2_500_050, MaxPriceMinor + 100} {
		if !errors.Is(ValidatePrice(bad), ErrInvalidPrice) {
			t.Errorf("price %d accepted", bad)
		}
	}
	if ValidateSortOrder(0) != nil || ValidateSortOrder(MaxSortOrder+1) == nil || ValidateSortOrder(-1) == nil {
		t.Error("sort order")
	}
	if s, err := ParseStatus(" inactive "); err != nil || s != StatusInactive {
		t.Errorf("status = %q %v", s, err)
	}
	if _, err := ParseStatus("PAUSED"); !errors.Is(err, ErrInvalidStatus) {
		t.Error("PAUSED accepted")
	}
}
