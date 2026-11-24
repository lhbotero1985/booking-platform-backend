// Package domain holds the catalog of a business (domain-model.md §6, T5.5): optional
// categories and services. Pure Go.
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
	ErrInvalidName        = errors.New("catalog: invalid name")
	ErrInvalidDescription = errors.New("catalog: invalid description")
	ErrInvalidDuration    = errors.New("catalog: invalid duration")
	ErrInvalidPrice       = errors.New("catalog: invalid price")
	ErrInvalidStatus      = errors.New("catalog: invalid status")
	ErrInvalidSortOrder   = errors.New("catalog: invalid sort order")
)

// Limits (match the CHECK constraints of migration 00014).
const (
	MinName         = 2
	MaxCategoryName = 60
	MaxServiceName  = 80
	MaxDescription  = 500
	MinDuration     = 5
	MaxDuration     = 480
	DurationStep    = 5
	MaxPriceMinor   = 1_000_000_000
	PriceStepMinor  = 100 // whole units: no cents (decision of 2026-11-24)
	MaxSortOrder    = 1000
)

// Status of a category or service.
type Status string

// Statuses.
const (
	StatusActive   Status = "ACTIVE"
	StatusInactive Status = "INACTIVE"
)

// ParseStatus validates a status (case-insensitive).
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

// ParseCategoryName validates a category name (2–60 printable runes, one line).
func ParseCategoryName(s string) (string, error) {
	v, ok := text(s, MinName, MaxCategoryName, false)
	if !ok {
		return "", ErrInvalidName
	}
	return v, nil
}

// ParseServiceName validates a service name (2–80 printable runes, one line).
func ParseServiceName(s string) (string, error) {
	v, ok := text(s, MinName, MaxServiceName, false)
	if !ok {
		return "", ErrInvalidName
	}
	return v, nil
}

// ParseDescription validates an optional description (≤ 500 runes, line breaks allowed).
func ParseDescription(s string) (string, error) {
	if strings.TrimSpace(s) == "" {
		return "", nil
	}
	v, ok := text(s, 1, MaxDescription, true)
	if !ok {
		return "", ErrInvalidDescription
	}
	return v, nil
}

// ValidateDuration checks 5–480 minutes in steps of 5.
func ValidateDuration(minutes int) error {
	if minutes < MinDuration || minutes > MaxDuration || minutes%DurationStep != 0 {
		return ErrInvalidDuration
	}
	return nil
}

// ValidatePrice checks 0–10,000,000 whole units, expressed in minor units without cents.
func ValidatePrice(minor int64) error {
	if minor < 0 || minor > MaxPriceMinor || minor%PriceStepMinor != 0 {
		return ErrInvalidPrice
	}
	return nil
}

// ValidateSortOrder checks 0–1000 (lower appears first).
func ValidateSortOrder(n int) error {
	if n < 0 || n > MaxSortOrder {
		return ErrInvalidSortOrder
	}
	return nil
}

// Category groups services (optional, e.g. "Cortes", "Barba").
type Category struct {
	ID         uuid.UUID
	BusinessID uuid.UUID
	Name       string
	Status     Status
	SortOrder  int
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Service is something a business offers: its duration is the same for every professional and
// its price is in the business's currency (minor units).
type Service struct {
	ID              uuid.UUID
	BusinessID      uuid.UUID
	CategoryID      uuid.UUID // Nil = uncategorized
	Name            string
	Description     string
	DurationMinutes int
	PriceMinor      int64
	Currency        string
	Status          Status
	SortOrder       int
	CreatedAt       time.Time
	UpdatedAt       time.Time
}
