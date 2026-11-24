// Package contract is the only part of the geography module other modules may import
// (ADR-0002): resolving a city with its division, country, time zone and currency.
package contract

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// ErrCityNotFound is returned when the city does not exist.
var ErrCityNotFound = errors.New("geography: city not found")

// Place is a city with its parents, as other modules need it to place a resource.
type Place struct {
	CityID     uuid.UUID
	DivisionID uuid.UUID
	// CountryCode is ISO 3166-1 alpha-2 ("CO"); also the default region for phone numbers.
	CountryCode string
	// TimeZone is the city's IANA zone, the default for resources located there.
	TimeZone string
	// Currency is the country's ISO 4217 default currency.
	Currency string
	// Active is true when the city, its division and its country are all active.
	Active bool
}

// Places resolves cities.
type Places interface {
	PlaceOfCity(ctx context.Context, cityID uuid.UUID) (Place, error)
}
