// Package domain holds the professional (barber/stylist) profile of a business
// (domain-model.md §5). Pure Go.
package domain

import (
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Errors.
var (
	ErrInvalidDisplayName = errors.New("professionals: invalid display name")
	ErrInvalidBio         = errors.New("professionals: invalid bio")
	ErrInvalidStatus      = errors.New("professionals: invalid status")
	ErrInvalidSortOrder   = errors.New("professionals: invalid sort order")
)

// Limits.
const (
	MinDisplayName = 2
	MaxDisplayName = 60
	MaxBio         = 500
	MaxSortOrder   = 1000
)

// Status of a profile (matches professionals_status_valid).
type Status string

// Statuses.
const (
	StatusActive   Status = "ACTIVE"
	StatusInactive Status = "INACTIVE"
)

// ParseStatus validates a status.
func ParseStatus(s string) (Status, error) {
	switch st := Status(strings.ToUpper(strings.TrimSpace(s))); st {
	case StatusActive, StatusInactive:
		return st, nil
	default:
		return "", ErrInvalidStatus
	}
}

func text(s string, minLen, maxLen int, newlines bool) (string, bool) {
	s = strings.TrimSpace(s)
	n := utf8.RuneCountInString(s)
	if !utf8.ValidString(s) || n < minLen || n > maxLen {
		return "", false
	}
	for _, r := range s {
		lineBreak := r == '\n' || r == '\r' || r == '\t'
		if unicode.IsControl(r) && (!newlines || !lineBreak) {
			return "", false
		}
	}
	return s, true
}

// ParseDisplayName validates the name customers see (2–60 printable runes).
func ParseDisplayName(s string) (string, error) {
	v, ok := text(s, MinDisplayName, MaxDisplayName, false)
	if !ok {
		return "", ErrInvalidDisplayName
	}
	return v, nil
}

// ParseBio validates an optional short description (≤ 500 runes, line breaks allowed).
func ParseBio(s string) (string, error) {
	if strings.TrimSpace(s) == "" {
		return "", nil
	}
	v, ok := text(s, 1, MaxBio, true)
	if !ok {
		return "", ErrInvalidBio
	}
	return v, nil
}

// ValidateSortOrder checks 0–1000 (lower appears first).
func ValidateSortOrder(n int) error {
	if n < 0 || n > MaxSortOrder {
		return ErrInvalidSortOrder
	}
	return nil
}

// Professional is the bookable profile of a member of the business (a PROFESSIONAL, or an
// OWNER who also serves customers). Every professional has an account (UserID).
type Professional struct {
	ID          uuid.UUID
	BusinessID  uuid.UUID
	UserID      uuid.UUID
	DisplayName string
	Bio         string
	Status      Status
	SortOrder   int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Bookable reports whether customers may book the professional: the profile is ACTIVE and the
// account is still an active member of the business (memberActive comes from identity).
func (p Professional) Bookable(memberActive bool) bool {
	return p.Status == StatusActive && memberActive
}
