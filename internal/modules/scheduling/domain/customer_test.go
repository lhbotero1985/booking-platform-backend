package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/domain"
)

func TestParsePhoneAndName(t *testing.T) {
	for raw, want := range map[string]string{
		"3001234567": "+573001234567", " (300) 123-4567 ": "+573001234567", "300.123.4567": "+573001234567",
		"+57 300 123 4567": "+573001234567", "+1 415 555 0100": "+14155550100",
	} {
		if got, err := domain.ParsePhone(raw); err != nil || got != want {
			t.Errorf("%q → %q %v", raw, got, err)
		}
	}
	for _, raw := range []string{"", "12345", "2001234567", "+0573001234567", "abc", "+57300123456789012"} {
		if _, err := domain.ParsePhone(raw); !errors.Is(err, domain.ErrInvalidPhone) {
			t.Errorf("%q: %v", raw, err)
		}
	}
	for raw, want := range map[string]string{"": "", "  ": "", " Ana@Example.com ": "ana@example.com"} {
		if got, err := domain.ParseEmail(raw); err != nil || got != want {
			t.Errorf("email %q → %q %v", raw, got, err)
		}
	}
	for _, raw := range []string{"ana", "ana@", "@x.co", "a b@x.co", "ana@x", strings.Repeat("a", 250) + "@x.co"} {
		if _, err := domain.ParseEmail(raw); !errors.Is(err, domain.ErrInvalidEmail) {
			t.Errorf("email %q: %v", raw, err)
		}
	}
	if got, err := domain.ParseCustomerName("  Ana   María  "); err != nil || got != "Ana María" {
		t.Errorf("name = %q %v", got, err)
	}
	for _, raw := range []string{"", "   ", strings.Repeat("a", 121), "Ana\x00"} {
		if _, err := domain.ParseCustomerName(raw); !errors.Is(err, domain.ErrInvalidCustomerName) {
			t.Errorf("%q: %v", raw, err)
		}
	}
}
