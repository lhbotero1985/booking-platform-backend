// Package application holds the geography use cases: public reads of active reference data
// and PLATFORM_ADMIN writes, audited in the same transaction.
//
// Rules (enforced by depguard): may import domain, the identity contract and platform
// packages (audit, clock); no HTTP, database drivers or River.
package application

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/domain"
	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/audit"
)

// Repository errors returned by Tx implementations.
var (
	ErrNotFound = errors.New("geography: not found")
	ErrConflict = errors.New("geography: already exists")
	// ErrInUse: a country, division or city with active businesses cannot be deactivated
	// (domain-model.md §3); the businesses must be moved or suspended first.
	ErrInUse = errors.New("geography: in use by active businesses")
)

// BusinessUsage counts active businesses located in an area. Implemented by the businesses
// module and wired in the composition root; nil disables the check (tests).
type BusinessUsage interface {
	ActiveBusinessesInCountry(ctx context.Context, countryCode string) (int, error)
	ActiveBusinessesInDivision(ctx context.Context, divisionID uuid.UUID) (int, error)
	ActiveBusinessesInCity(ctx context.Context, cityID uuid.UUID) (int, error)
}

// Authorizer is the part of the identity contract these use cases need.
type Authorizer interface {
	RequirePlatformAdmin(ctx context.Context, p identity.Principal) error
}

// Store runs a unit of work in a database transaction.
type Store interface {
	WithinTx(ctx context.Context, fn func(tx Tx) error) error
}

// Tx is the persistence available inside a transaction. Lookups return ErrNotFound when
// absent; writes return ErrConflict on a duplicate code or name.
type Tx interface {
	ListActiveCountries(ctx context.Context) ([]domain.Country, error)
	CountryByCode(ctx context.Context, code domain.CountryCode) (domain.Country, error)
	InsertCountry(ctx context.Context, c domain.Country, now time.Time) error
	UpdateCountry(ctx context.Context, c domain.Country, now time.Time) error

	ListActiveDivisions(ctx context.Context, country domain.CountryCode) ([]domain.Division, error)
	DivisionByID(ctx context.Context, id uuid.UUID) (domain.Division, error)
	InsertDivision(ctx context.Context, d domain.Division, now time.Time) error
	UpdateDivision(ctx context.Context, d domain.Division, now time.Time) error

	ListActiveCities(ctx context.Context, divisionID uuid.UUID) ([]domain.City, error)
	CityByID(ctx context.Context, id uuid.UUID) (domain.City, error)
	InsertCity(ctx context.Context, c domain.City, now time.Time) error
	UpdateCity(ctx context.Context, c domain.City, now time.Time) error

	AppendAudit(ctx context.Context, e *audit.Entry) error
}

// Field error codes exposed in Problem.errors[].message (stable, translatable).
const (
	CodeRequired            = "REQUIRED"
	CodeInvalidCountryCode  = "INVALID_COUNTRY_CODE"
	CodeInvalidName         = "INVALID_NAME"
	CodeInvalidPhoneCode    = "INVALID_PHONE_CODE"
	CodeUnsupportedCurrency = "UNSUPPORTED_CURRENCY"
	CodeInvalidDivisionCode = "INVALID_DIVISION_CODE"
	CodeInvalidDivisionType = "INVALID_DIVISION_TYPE"
	CodeInvalidTimeZone     = "INVALID_TIME_ZONE"
	CodeUnknownCountry      = "UNKNOWN_COUNTRY"
	CodeUnknownDivision     = "UNKNOWN_DIVISION"
)

// FieldError is a validation failure on one input field.
type FieldError struct {
	Field string
	Code  string
}

// ValidationError groups every invalid field of a request.
type ValidationError struct {
	Fields []FieldError
}

func (e *ValidationError) Error() string {
	parts := make([]string, 0, len(e.Fields))
	for _, f := range e.Fields {
		parts = append(parts, f.Field+":"+f.Code)
	}
	return "geography: validation failed (" + strings.Join(parts, ", ") + ")"
}

func (e *ValidationError) add(field, code string) {
	e.Fields = append(e.Fields, FieldError{field, code})
}

func (e *ValidationError) orNil() error {
	if len(e.Fields) == 0 {
		return nil
	}
	return e
}

func invalid(field, code string) error {
	return &ValidationError{Fields: []FieldError{{field, code}}}
}
