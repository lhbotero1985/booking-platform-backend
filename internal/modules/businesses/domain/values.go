// Package domain holds the business aggregate (the tenant root, domain-model.md §4) and its
// value objects. Pure Go: no infrastructure.
package domain

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/nyaruka/phonenumbers"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Domain errors; the application maps them to field codes.
var (
	ErrInvalidName        = errors.New("businesses: invalid name")
	ErrInvalidSlug        = errors.New("businesses: invalid slug")
	ErrReservedSlug       = errors.New("businesses: reserved slug")
	ErrInvalidPhone       = errors.New("businesses: invalid phone number")
	ErrInvalidDescription = errors.New("businesses: invalid description")
	ErrInvalidAddress     = errors.New("businesses: invalid address")
	ErrInvalidCoordinates = errors.New("businesses: invalid coordinates")
	ErrInvalidTimeZone    = errors.New("businesses: invalid time zone")
	ErrInvalidCurrency    = errors.New("businesses: invalid currency")
	ErrInvalidStatus      = errors.New("businesses: invalid status")
	ErrInvalidPolicy      = errors.New("businesses: invalid booking policy")
	ErrInvalidReminder    = errors.New("businesses: invalid reminder settings")
	ErrInvalidLocation    = errors.New("businesses: invalid location")
	ErrInvalidID          = errors.New("businesses: invalid id")
)

// Length limits (runes).
const (
	MinNameLength        = 2
	MaxNameLength        = 120
	MaxDescriptionLength = 1000
	MaxAddressLength     = 200
	MinSlugLength        = 3
	MaxSlugLength        = 60
)

func hasControl(s string, allowNewlines bool) bool {
	for _, r := range s {
		if allowNewlines && (r == '\n' || r == '\r' || r == '\t') {
			continue
		}
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

func boundedText(s string, minLen, maxLen int, allowNewlines bool) (string, bool) {
	s = strings.TrimSpace(s)
	n := utf8.RuneCountInString(s)
	if !utf8.ValidString(s) || n < minLen || n > maxLen || hasControl(s, allowNewlines) {
		return "", false
	}
	return s, true
}

// ParseName validates a business name (2–120 printable runes, trimmed).
func ParseName(s string) (string, error) {
	v, ok := boundedText(s, MinNameLength, MaxNameLength, false)
	if !ok {
		return "", ErrInvalidName
	}
	return v, nil
}

// ParseDescription validates an optional description (≤ 1000 runes; line breaks allowed).
func ParseDescription(s string) (string, error) {
	if strings.TrimSpace(s) == "" {
		return "", nil
	}
	v, ok := boundedText(s, 1, MaxDescriptionLength, true)
	if !ok {
		return "", ErrInvalidDescription
	}
	return v, nil
}

// ParseAddress validates the street address (1–200 printable runes).
func ParseAddress(s string) (string, error) {
	v, ok := boundedText(s, 1, MaxAddressLength, false)
	if !ok {
		return "", ErrInvalidAddress
	}
	return v, nil
}

// Slug is the public, URL-safe identifier of a business ("barberia-el-paisa").
type Slug string

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,58}[a-z0-9]$`)

// reservedSlugs are words a business may not take: they look like platform pages or could be
// used to impersonate the platform.
var reservedSlugs = map[string]bool{
	"admin": true, "api": true, "app": true, "login": true, "logout": true, "register": true,
	"signup": true, "settings": true, "support": true, "help": true, "soporte": true, "ayuda": true,
	"booking": true, "reservas": true, "www": true, "static": true, "assets": true, "me": true,
	"business": true, "businesses": true, "negocio": true, "negocios": true, "root": true,
	"null": true, "undefined": true, "test": true, "status": true, "security": true,
	// First-level routes of the frontend's business-web, where each business has its public page
	// at the site root (frontend ADR-F006). Keep in sync with PLATFORM_ROUTE_SEGMENTS in
	// booking-platform-frontend/packages/core/src/routes.ts.
	"verify-email": true, "reset-password": true, "forgot-password": true, "healthz": true,
	"readyz": true, "ingresar": true, "entrar": true, "salir": true, "registro": true,
	"recuperar": true, "crear-negocio": true, "planes": true, "precios": true, "pricing": true,
	"funcionalidades": true, "features": true, "preguntas": true, "faq": true, "contacto": true,
	"contact": true, "legal": true, "terminos": true, "terms": true, "privacidad": true,
	"privacy": true, "panel": true, "dashboard": true, "cuenta": true, "account": true,
	"perfil": true, "profile": true, "citas": true, "mis-citas": true, "agenda": true,
	"descargar": true, "download": true, "auth": true, "callback": true, "blog": true,
	"nosotros": true, "about": true, "booka": true,
}

// ParseSlug validates a slug: 3–60 of [a-z0-9-], no leading, trailing or double hyphen, not
// reserved.
func ParseSlug(s string) (Slug, error) {
	s = strings.TrimSpace(s)
	if !slugRe.MatchString(s) || strings.Contains(s, "--") {
		return "", ErrInvalidSlug
	}
	if reservedSlugs[s] {
		return "", ErrReservedSlug
	}
	return Slug(s), nil
}

var stripMarks = transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)

// SlugFromName derives a slug candidate from a name: accents removed ("Peluquería Ñandú" →
// "peluqueria-nandu"), lower case, runs of other characters become one hyphen, cut to the
// maximum length. Short or reserved results get a "negocio-" prefix so the candidate is always
// valid; uniqueness is the caller's job (WithSuffix).
func SlugFromName(name string) Slug {
	plain, _, err := transform.String(stripMarks, name)
	if err != nil {
		plain = name
	}
	var b strings.Builder
	hyphen := false
	for _, r := range strings.ToLower(plain) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			hyphen = false
		case !hyphen && b.Len() > 0:
			b.WriteByte('-')
			hyphen = true
		}
	}
	s := strings.Trim(b.String(), "-")
	if len(s) > MaxSlugLength {
		s = strings.TrimRight(s[:MaxSlugLength], "-")
	}
	if _, err := ParseSlug(s); err != nil {
		s = strings.TrimRight(("negocio-" + s)[:min(len("negocio-"+s), MaxSlugLength)], "-")
		if _, err := ParseSlug(s); err != nil {
			s = "mi-negocio"
		}
	}
	return Slug(s)
}

// WithSuffix returns the slug with "-n" appended (n ≥ 2), shortening the base if needed.
func (s Slug) WithSuffix(n int) Slug {
	suffix := "-" + strconv.Itoa(n)
	base := string(s)
	if len(base)+len(suffix) > MaxSlugLength {
		base = strings.TrimRight(base[:MaxSlugLength-len(suffix)], "-")
	}
	return Slug(base + suffix)
}

// ParsePhone parses a phone number for the country region ("CO") and returns it in E.164
// ("+573001234567"). Numbers in international format may belong to another country.
func ParsePhone(raw, region string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 32 {
		return "", ErrInvalidPhone
	}
	num, err := phonenumbers.Parse(raw, strings.ToUpper(region))
	if err != nil || !phonenumbers.IsValidNumber(num) {
		return "", ErrInvalidPhone
	}
	return phonenumbers.Format(num, phonenumbers.E164), nil
}

// Coordinates is a WGS84 point.
type Coordinates struct {
	Lat, Lng float64
}

// NewCoordinates validates latitude [-90, 90] and longitude [-180, 180]. (0, 0) is rejected:
// it is almost always a missing value, not a business in the Gulf of Guinea.
func NewCoordinates(lat, lng float64) (Coordinates, error) {
	if lat < -90 || lat > 90 || lng < -180 || lng > 180 || (lat == 0 && lng == 0) || lat != lat || lng != lng {
		return Coordinates{}, ErrInvalidCoordinates
	}
	return Coordinates{Lat: lat, Lng: lng}, nil
}

// ParseTimeZone accepts IANA names; "Local" and empty are rejected.
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

var currencyRe = regexp.MustCompile(`^[A-Z]{3}$`)

// ParseCurrency validates the ISO 4217 format. Which currencies are supported is decided by
// the geography module (country default currency).
func ParseCurrency(s string) (string, error) {
	if !currencyRe.MatchString(s) {
		return "", ErrInvalidCurrency
	}
	return s, nil
}
