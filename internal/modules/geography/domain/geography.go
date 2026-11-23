// Package domain holds the geography model: countries, administrative divisions and cities
// (reference data managed by PLATFORM_ADMIN, domain-model.md §3). Pure Go, no infrastructure.
package domain

import (
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Domain errors; the application maps them to field codes.
var (
	ErrInvalidCountryCode  = errors.New("geography: invalid country code")
	ErrInvalidName         = errors.New("geography: invalid name")
	ErrInvalidPhoneCode    = errors.New("geography: invalid phone code")
	ErrInvalidCurrency     = errors.New("geography: unsupported currency")
	ErrInvalidDivisionCode = errors.New("geography: invalid division code")
	ErrInvalidDivisionType = errors.New("geography: invalid division type")
	ErrInvalidTimeZone     = errors.New("geography: invalid time zone")
	ErrInvalidID           = errors.New("geography: invalid id")
)

// MaxNameLength bounds every geography name (runes).
const MaxNameLength = 100

var (
	countryCodeRe  = regexp.MustCompile(`^[A-Z]{2}$`)
	phoneCodeRe    = regexp.MustCompile(`^\+[1-9][0-9]{0,3}$`)
	divisionCodeRe = regexp.MustCompile(`^[A-Z]{2}-[A-Z0-9]{1,3}$`)
)

// supportedCurrencies is the allowlist of ISO 4217 codes a country may use (ADR-0015). Adding
// a currency is a deliberate code change: amounts, formatting and payments depend on it.
var supportedCurrencies = map[string]bool{"COP": true, "USD": true}

// CountryCode is an ISO 3166-1 alpha-2 code ("CO").
type CountryCode string

// ParseCountryCode normalizes to upper case and validates the format.
func ParseCountryCode(s string) (CountryCode, error) {
	s = strings.ToUpper(strings.TrimSpace(s))
	if !countryCodeRe.MatchString(s) {
		return "", ErrInvalidCountryCode
	}
	return CountryCode(s), nil
}

// ParseName trims s and requires 1–MaxNameLength printable runes.
func ParseName(s string) (string, error) {
	s = strings.TrimSpace(s)
	n := utf8.RuneCountInString(s)
	if n == 0 || n > MaxNameLength || !utf8.ValidString(s) {
		return "", ErrInvalidName
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return "", ErrInvalidName
		}
	}
	return s, nil
}

// ParsePhoneCode validates an international calling code ("+57").
func ParsePhoneCode(s string) (string, error) {
	s = strings.TrimSpace(s)
	if !phoneCodeRe.MatchString(s) {
		return "", ErrInvalidPhoneCode
	}
	return s, nil
}

// ParseCurrency validates an ISO 4217 code against the supported list.
func ParseCurrency(s string) (string, error) {
	s = strings.ToUpper(strings.TrimSpace(s))
	if !supportedCurrencies[s] {
		return "", ErrInvalidCurrency
	}
	return s, nil
}

// ParseDivisionCode validates an optional ISO 3166-2 code ("CO-VAC") that must belong to
// country. Empty means "no code".
func ParseDivisionCode(s string, country CountryCode) (string, error) {
	s = strings.ToUpper(strings.TrimSpace(s))
	if s == "" {
		return "", nil
	}
	if !divisionCodeRe.MatchString(s) || !strings.HasPrefix(s, string(country)+"-") {
		return "", ErrInvalidDivisionCode
	}
	return s, nil
}

// ParseTimeZone accepts IANA zone names ("America/Bogota") and "UTC". "Local" and empty are
// rejected: they depend on the server.
func ParseTimeZone(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "Local" || len(s) > 64 {
		return "", ErrInvalidTimeZone
	}
	if _, err := time.LoadLocation(s); err != nil {
		return "", ErrInvalidTimeZone
	}
	return s, nil
}

// DivisionType is the kind of first-level subdivision (matches the CHECK constraint).
type DivisionType string

// Division types.
const (
	DivisionDepartment DivisionType = "DEPARTMENT"
	DivisionState      DivisionType = "STATE"
	DivisionProvince   DivisionType = "PROVINCE"
	DivisionRegion     DivisionType = "REGION"
	DivisionDistrict   DivisionType = "DISTRICT"
)

// ParseDivisionType validates a division type.
func ParseDivisionType(s string) (DivisionType, error) {
	switch t := DivisionType(strings.ToUpper(strings.TrimSpace(s))); t {
	case DivisionDepartment, DivisionState, DivisionProvince, DivisionRegion, DivisionDistrict:
		return t, nil
	default:
		return "", ErrInvalidDivisionType
	}
}

// Country is a country where businesses can operate.
type Country struct {
	Code            CountryCode
	Name            string
	PhoneCode       string
	DefaultCurrency string
	Active          bool
}

// Division is a first-level administrative subdivision of a country.
type Division struct {
	ID          uuid.UUID
	CountryCode CountryCode
	Code        string // ISO 3166-2, optional
	Name        string
	Type        DivisionType
	Active      bool
}

// City is a city of a division; its time zone is the default for businesses located in it.
type City struct {
	ID         uuid.UUID
	DivisionID uuid.UUID
	Name       string
	TimeZone   string
	Active     bool
}
