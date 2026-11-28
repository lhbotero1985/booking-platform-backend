package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/domain"
)

func TestCanReview(t *testing.T) {
	user := uuid.Must(uuid.NewV7())
	end := time.Date(2030, 3, 5, 15, 0, 0, 0, time.UTC)
	a := domain.Appointment{Status: domain.StatusCompleted, End: end, Customer: domain.Customer{UserID: user}}
	for name, c := range map[string]struct {
		a    domain.Appointment
		user uuid.UUID
		now  time.Time
		want error
	}{
		"right after":         {a, user, end, nil},
		"the last moment":     {a, user, end.Add(domain.ReviewWindow), nil},
		"a second too late":   {a, user, end.Add(domain.ReviewWindow + time.Second), domain.ErrReviewWindowClosed},
		"another account":     {a, uuid.Must(uuid.NewV7()), end, domain.ErrNotReviewable},
		"no account":          {domain.Appointment{Status: domain.StatusCompleted, End: end}, uuid.Nil, end, domain.ErrNotReviewable},
		"not completed":       {domain.Appointment{Status: domain.StatusConfirmed, End: end, Customer: a.Customer}, user, end, domain.ErrNotReviewable},
		"a no-show":           {domain.Appointment{Status: domain.StatusNoShow, End: end, Customer: a.Customer}, user, end, domain.ErrNotReviewable},
		"canceled":            {domain.Appointment{Status: domain.StatusCancelled, End: end, Customer: a.Customer}, user, end, domain.ErrNotReviewable},
		"before the end, too": {a, user, end.Add(-time.Hour), nil},
	} {
		if err := domain.CanReview(c.a, c.user, c.now); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", name, err, c.want)
		}
	}
}

func TestAuthorName(t *testing.T) {
	for in, want := range map[string]string{
		"Ana María Gómez": "Ana M.",
		"  luis  botero ": "luis B.",
		"Ñandú":           "Ñandú",
		"José ángel":      "José Á.",
		"":                "Cliente",
		"   ":             "Cliente",
	} {
		if got := domain.AuthorName(in); got != want {
			t.Errorf("AuthorName(%q) = %q, want %q", in, got, want)
		}
	}
	if got := domain.AuthorName(strings.Repeat("x", 200)); len([]rune(got)) != 80 {
		t.Errorf("long name: %d runes", len([]rune(got)))
	}
}

func TestCleanText(t *testing.T) {
	if s, ok := domain.CleanText("  hola\nqué tal  ", 20); !ok || s != "hola\nqué tal" {
		t.Errorf("trim: %q %v", s, ok)
	}
	if _, ok := domain.CleanText(strings.Repeat("é", 11), 10); ok {
		t.Error("11 characters fit in 10")
	}
	if _, ok := domain.CleanText(strings.Repeat("é", 10), 10); !ok {
		t.Error("10 characters (20 bytes) do not fit in 10")
	}
	if _, ok := domain.CleanText("bell\x07", 10); ok {
		t.Error("control character accepted")
	}
	if _, ok := domain.CleanText("\xff", 10); ok {
		t.Error("invalid UTF-8 accepted")
	}
}

func TestRatingAverage(t *testing.T) {
	if (domain.RatingSummary{}).Average() != nil {
		t.Error("no reviews has an average")
	}
	for _, c := range []struct {
		count, sum int
		want       float64
	}{{1, 5, 5}, {3, 14, 4.7}, {2, 9, 4.5}, {3, 10, 3.3}, {6, 25, 4.2}, {7, 7, 1}} {
		if got := *(domain.RatingSummary{Count: c.count, Sum: c.sum}).Average(); got != c.want {
			t.Errorf("%d/%d = %v, want %v", c.sum, c.count, got, c.want)
		}
	}
	if !domain.ValidRating(1) || !domain.ValidRating(5) || domain.ValidRating(0) || domain.ValidRating(6) {
		t.Error("ValidRating")
	}
}
