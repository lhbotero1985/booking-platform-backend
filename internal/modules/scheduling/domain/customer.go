package domain

// Customers of a business (T6.4): created by the staff (name and mobile) or when an account books
// from the app. The mobile identifies the person inside a business.

import (
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Customer validation errors.
var (
	ErrInvalidCustomerName = errors.New("scheduling: invalid customer name")
	ErrInvalidPhone        = errors.New("scheduling: invalid phone")
	ErrInvalidEmail        = errors.New("scheduling: invalid email")
)

// MaxCustomerName matches the customers_full_name_length CHECK.
const MaxCustomerName = 120

var (
	emailShape     = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)
	e164           = regexp.MustCompile(`^\+[1-9][0-9]{6,14}$`)
	colombianLocal = regexp.MustCompile(`^3[0-9]{9}$`)
)

// ParsePhone normalizes a mobile number to E.164 with the same rule as the accounts (identity):
// spaces, dashes, dots and parentheses are ignored; a 10-digit Colombian mobile gets +57.
func ParsePhone(raw string) (string, error) {
	s := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '-', '.', '(', ')':
			return -1
		}
		return r
	}, strings.TrimSpace(raw))
	if colombianLocal.MatchString(s) {
		s = "+57" + s
	}
	if !e164.MatchString(s) {
		return "", ErrInvalidPhone
	}
	return s, nil
}

// ParseEmail normalizes an optional customer email (trimmed, lower case; "" stays ""). It only
// checks the shape: the business is not asked to verify it.
func ParseEmail(raw string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" {
		return "", nil
	}
	if len(s) > 254 || !emailShape.MatchString(s) {
		return "", ErrInvalidEmail
	}
	return s, nil
}

// ParseCustomerName trims the name and checks its length (1–120) and that it has no control
// characters.
func ParseCustomerName(raw string) (string, error) {
	s := strings.Join(strings.Fields(raw), " ")
	if s == "" || utf8.RuneCountInString(s) > MaxCustomerName || strings.IndexFunc(s, unicode.IsControl) >= 0 {
		return "", ErrInvalidCustomerName
	}
	return s, nil
}

// BusinessCustomer is a customer of a business. UserID is uuid.Nil until the person books from
// the app with an account.
type BusinessCustomer struct {
	ID         uuid.UUID
	BusinessID uuid.UUID
	FullName   string
	Phone      string // E.164
	Email      string // optional, "" if unknown
	UserID     uuid.UUID
	CreatedBy  uuid.UUID
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
