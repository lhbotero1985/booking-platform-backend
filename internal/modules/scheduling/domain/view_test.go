package domain_test

import (
	"testing"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/domain"
)

func TestViewRange(t *testing.T) {
	d := date("2026-12-31") // a Thursday
	for v, want := range map[string][2]string{
		"day":   {"2026-12-31", "2027-01-01"},
		"week":  {"2026-12-28", "2027-01-04"}, // Monday to Monday
		"month": {"2026-12-01", "2027-01-01"},
		"year":  {"2026-01-01", "2027-01-01"},
	} {
		view, ok := domain.ParseView(v)
		from, to := view.Range(d)
		if !ok || from.String() != want[0] || to.String() != want[1] {
			t.Errorf("%s: %s – %s", v, from, to)
		}
	}
	if from, _ := domain.ViewWeek.Range(date("2026-11-23")); from.String() != "2026-11-23" { // a Monday
		t.Errorf("week of a Monday starts %s", from)
	}
	if from, _ := domain.ViewWeek.Range(date("2026-11-29")); from.String() != "2026-11-23" { // a Sunday
		t.Errorf("week of a Sunday starts %s", from)
	}
	if _, ok := domain.ParseView("decade"); ok {
		t.Error("unknown view accepted")
	}
}
