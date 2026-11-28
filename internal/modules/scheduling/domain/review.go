package domain

// Reviews of appointments (2026-11-28, decisions of the user): the customer of a COMPLETED
// appointment rates the business and the professional (1 to 5 each) with an optional comment,
// once per appointment and up to ReviewWindow after its end; the owner replies in public and
// cannot hide it; only a platform administrator hides one.

import (
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Limits of a review.
const (
	MinRating          = 1
	MaxRating          = 5
	MaxReviewComment   = 1000
	MaxReviewReply     = 1000
	MaxHideReason      = 200
	ReviewWindow       = 30 * 24 * time.Hour
	maxAuthorNameRunes = 80
)

// Errors of reviews.
var (
	// ErrNotReviewable: the appointment is not the caller's or is not COMPLETED.
	ErrNotReviewable = errors.New("scheduling: the appointment cannot be reviewed")
	// ErrReviewWindowClosed: more than ReviewWindow passed since the appointment ended.
	ErrReviewWindowClosed = errors.New("scheduling: the time to review the appointment is over")
)

// Review is the customer's opinion of a completed appointment.
type Review struct {
	ID, BusinessID, AppointmentID, ProfessionalID uuid.UUID
	CustomerUserID                                uuid.UUID
	// AuthorName is what the public sees of the customer ("Ana G."); ServiceName the service as
	// booked.
	AuthorName, ServiceName            string
	BusinessRating, ProfessionalRating int
	Comment                            string
	// Reply of the business; RepliedAt is zero without one.
	Reply     string
	RepliedAt time.Time
	RepliedBy uuid.UUID
	// HiddenAt is zero unless a platform administrator hid it.
	HiddenAt     time.Time
	HiddenBy     uuid.UUID
	HiddenReason string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Hidden tells whether a platform administrator hid the review.
func (r Review) Hidden() bool { return !r.HiddenAt.IsZero() }

// ReviewDeadline is the last moment the customer may write or change the review of a.
func ReviewDeadline(a Appointment) time.Time { return a.End.Add(ReviewWindow) }

// CanReview is nil when the account userID may write or change the review of a at now.
func CanReview(a Appointment, userID uuid.UUID, now time.Time) error {
	if userID == uuid.Nil || a.Customer.UserID != userID || a.Status != StatusCompleted {
		return ErrNotReviewable
	}
	if now.After(ReviewDeadline(a)) {
		return ErrReviewWindowClosed
	}
	return nil
}

// ValidRating tells whether n is a number of stars.
func ValidRating(n int) bool { return n >= MinRating && n <= MaxRating }

// CleanText trims raw and tells whether it fits in limit characters (and has no control
// characters other than line breaks and tabs).
func CleanText(raw string, limit int) (string, bool) {
	s := strings.TrimSpace(raw)
	if !utf8.ValidString(s) || utf8.RuneCountInString(s) > limit {
		return "", false
	}
	for _, r := range s {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return "", false
		}
	}
	return s, true
}

// AuthorName is what the public sees of a customer: the first name and the initial of the
// second word ("Ana María Gómez" → "Ana M."), or "Cliente" without a name.
func AuthorName(full string) string {
	words := strings.Fields(full)
	if len(words) == 0 {
		return "Cliente"
	}
	out := words[0]
	if len(words) > 1 {
		r, _ := utf8.DecodeRuneInString(words[1])
		out += " " + string(unicode.ToUpper(r)) + "."
	}
	if utf8.RuneCountInString(out) > maxAuthorNameRunes {
		out = string([]rune(out)[:maxAuthorNameRunes])
	}
	return out
}

// RatingSummary is the average of the visible reviews and how many there are.
type RatingSummary struct {
	Count int
	Sum   int
}

// Average is the average with one decimal (4.67 → 4.7); nil without reviews.
func (s RatingSummary) Average() *float64 {
	if s.Count == 0 {
		return nil
	}
	tenths := (s.Sum*10*2 + s.Count) / (2 * s.Count) // rounded half up, in tenths
	v := float64(tenths) / 10
	return &v
}
