package domain

// Absence of a professional (T5.4, domain-model.md §5): vacation, sick leave or any time off,
// as an absolute [Start, End) interval in UTC plus an optional reason. Absences of the same
// professional never overlap (exclusion constraint in the database).

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Errors.
var (
	ErrInvalidPeriod = errors.New("professionals: invalid absence period")
	ErrPeriodTooLong = errors.New("professionals: absence longer than allowed")
	ErrInvalidReason = errors.New("professionals: invalid absence reason")
)

// Limits.
const (
	MaxAbsenceReason = 200
	// MaxAbsenceDuration bounds one absence; longer leaves are registered in several parts.
	MaxAbsenceDuration = 366 * 24 * time.Hour
)

// Absence is a period in which the professional cannot be booked.
type Absence struct {
	ID             uuid.UUID
	BusinessID     uuid.UUID
	ProfessionalID uuid.UUID
	Start, End     time.Time // UTC, [Start, End)
	Reason         string
	CreatedBy      uuid.UUID
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// ValidatePeriod checks start < end and the maximum duration; it returns both in UTC,
// truncated to the second.
func ValidatePeriod(start, end time.Time) (time.Time, time.Time, error) {
	start, end = start.UTC().Truncate(time.Second), end.UTC().Truncate(time.Second)
	if start.IsZero() || end.IsZero() || !start.Before(end) {
		return start, end, ErrInvalidPeriod
	}
	if end.Sub(start) > MaxAbsenceDuration {
		return start, end, ErrPeriodTooLong
	}
	return start, end, nil
}

// ParseReason validates an optional reason (≤ 200 printable runes, single line).
func ParseReason(s string) (string, error) {
	if strings.TrimSpace(s) == "" {
		return "", nil
	}
	v, ok := text(s, 1, MaxAbsenceReason, false)
	if !ok {
		return "", ErrInvalidReason
	}
	return v, nil
}
