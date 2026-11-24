package domain

import (
	"regexp"
	"strings"
	"time"
)

var (
	e164           = regexp.MustCompile(`^\+[1-9][0-9]{6,14}$`)
	colombianLocal = regexp.MustCompile(`^3[0-9]{9}$`)
)

// ParsePhone normalizes a customer's mobile number to E.164 (T6.3; it receives WhatsApp
// messages in Phase 7). Spaces, dashes, dots and parentheses are ignored. A 10-digit Colombian
// mobile (3XX XXX XXXX) gets +57, the pilot country; any other number must already be E.164.
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

// Phone is the customer's mobile in E.164, or "" if not set.
func (u *User) Phone() string { return u.phone }

// WithPhone sets the stored phone while rehydrating (no validation; storage CHECK applies).
func (u *User) WithPhone(phone string) *User {
	u.phone = phone
	return u
}

// ChangePhone sets a phone already normalized by ParsePhone.
func (u *User) ChangePhone(phone string, now time.Time) {
	u.phone = phone
	u.updatedAt = now
}
