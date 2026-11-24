package application

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/domain"
	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/audit"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
)

// Audited actions (audit_log.action). Stable identifiers.
const (
	AuditCountryCreated  = "geography.country_created"
	AuditCountryUpdated  = "geography.country_updated"
	AuditDivisionCreated = "geography.division_created"
	AuditDivisionUpdated = "geography.division_updated"
	AuditCityCreated     = "geography.city_created"
	AuditCityUpdated     = "geography.city_updated"
)

// Service implements the geography use cases.
type Service struct {
	store Store
	authz Authorizer
	usage BusinessUsage
	clock clock.Clock
}

// NewService builds the service; a nil clock defaults to the system clock and a nil usage
// disables the "in use by active businesses" check.
func NewService(store Store, authz Authorizer, usage BusinessUsage, clk clock.Clock) *Service {
	if clk == nil {
		clk = clock.System{}
	}
	return &Service{store: store, authz: authz, usage: usage, clock: clk}
}

// PlaceOfCity implements the geography contract: the city with its division, country, time
// zone and currency. Active is true only if the city and its parents are all active.
func (s *Service) PlaceOfCity(ctx context.Context, cityID uuid.UUID) (contract.Place, error) {
	var out contract.Place
	err := s.store.WithinTx(ctx, func(tx Tx) error {
		c, err := tx.CityByID(ctx, cityID)
		if err != nil {
			return err
		}
		d, err := tx.DivisionByID(ctx, c.DivisionID)
		if err != nil {
			return err
		}
		co, err := tx.CountryByCode(ctx, d.CountryCode)
		if err != nil {
			return err
		}
		out = contract.Place{
			CityID: c.ID, DivisionID: d.ID, CountryCode: string(co.Code), TimeZone: c.TimeZone,
			Currency: co.DefaultCurrency, Active: c.Active && d.Active && co.Active,
		}
		return nil
	})
	if errors.Is(err, ErrNotFound) {
		return contract.Place{}, contract.ErrCityNotFound
	}
	return out, err
}

// checkNotInUse refuses to deactivate an area that still has active businesses.
func (s *Service) checkNotInUse(deactivating bool, count func(BusinessUsage) (int, error)) error {
	if !deactivating || s.usage == nil {
		return nil
	}
	n, err := count(s.usage)
	if err != nil {
		return err
	}
	if n > 0 {
		return ErrInUse
	}
	return nil
}

// --- public reads (active data only) ----------------------------------------------------

// ListCountries returns the active countries sorted by name.
func (s *Service) ListCountries(ctx context.Context) ([]domain.Country, error) {
	var out []domain.Country
	err := s.store.WithinTx(ctx, func(tx Tx) (err error) {
		out, err = tx.ListActiveCountries(ctx)
		return err
	})
	return out, err
}

// ListDivisions returns the active divisions of an active country; ErrNotFound otherwise
// (also for a malformed code: it comes from the URL path).
func (s *Service) ListDivisions(ctx context.Context, rawCode string) ([]domain.Division, error) {
	code, err := domain.ParseCountryCode(rawCode)
	if err != nil || rawCode != string(code) {
		return nil, ErrNotFound
	}
	var out []domain.Division
	err = s.store.WithinTx(ctx, func(tx Tx) error {
		c, err := tx.CountryByCode(ctx, code)
		if err != nil {
			return err
		}
		if !c.Active {
			return ErrNotFound
		}
		out, err = tx.ListActiveDivisions(ctx, code)
		return err
	})
	return out, err
}

// ListCities returns the active cities of an active division of an active country.
func (s *Service) ListCities(ctx context.Context, divisionID uuid.UUID) ([]domain.City, error) {
	var out []domain.City
	err := s.store.WithinTx(ctx, func(tx Tx) error {
		d, err := tx.DivisionByID(ctx, divisionID)
		if err != nil {
			return err
		}
		c, err := tx.CountryByCode(ctx, d.CountryCode)
		if err != nil {
			return err
		}
		if !d.Active || !c.Active {
			return ErrNotFound
		}
		out, err = tx.ListActiveCities(ctx, divisionID)
		return err
	})
	return out, err
}

// --- PLATFORM_ADMIN writes ---------------------------------------------------------------

// CountryInput creates a country. Active defaults to true.
type CountryInput struct {
	Code, Name, PhoneCode, DefaultCurrency string
	Active                                 *bool
}

// CountryPatch changes the given fields of a country (nil = unchanged).
type CountryPatch struct {
	Name, PhoneCode, DefaultCurrency *string
	Active                           *bool
}

// CreateCountry adds a country (PLATFORM_ADMIN).
func (s *Service) CreateCountry(ctx context.Context, p identity.Principal, in CountryInput) (domain.Country, error) {
	if err := s.authz.RequirePlatformAdmin(ctx, p); err != nil {
		return domain.Country{}, err
	}
	var verr ValidationError
	c := domain.Country{Active: valueOr(in.Active, true)}
	var err error
	if c.Code, err = domain.ParseCountryCode(in.Code); err != nil {
		verr.add("code", required(in.Code, CodeInvalidCountryCode))
	}
	c.Name = parseName(&verr, "name", in.Name)
	c.PhoneCode = parsePhoneCode(&verr, in.PhoneCode)
	c.DefaultCurrency = parseCurrency(&verr, in.DefaultCurrency)
	if err := verr.orNil(); err != nil {
		return domain.Country{}, err
	}
	err = s.store.WithinTx(ctx, func(tx Tx) error {
		now := s.clock.Now()
		if err := tx.InsertCountry(ctx, c, now); err != nil {
			return err
		}
		return tx.AppendAudit(ctx, s.audit(ctx, p, AuditCountryCreated, "country", string(c.Code)))
	})
	return c, err
}

// UpdateCountry changes a country (PLATFORM_ADMIN). Unknown code: ErrNotFound.
func (s *Service) UpdateCountry(ctx context.Context, p identity.Principal, rawCode string, in CountryPatch) (domain.Country, error) {
	if err := s.authz.RequirePlatformAdmin(ctx, p); err != nil {
		return domain.Country{}, err
	}
	code, err := domain.ParseCountryCode(rawCode)
	if err != nil || rawCode != string(code) {
		return domain.Country{}, ErrNotFound
	}
	var verr ValidationError
	var changed []string
	var name, phone, currency string
	if in.Name != nil {
		name = parseName(&verr, "name", *in.Name)
	}
	if in.PhoneCode != nil {
		phone = parsePhoneCode(&verr, *in.PhoneCode)
	}
	if in.DefaultCurrency != nil {
		currency = parseCurrency(&verr, *in.DefaultCurrency)
	}
	if err := verr.orNil(); err != nil {
		return domain.Country{}, err
	}
	var c domain.Country
	err = s.store.WithinTx(ctx, func(tx Tx) error {
		var err error
		if c, err = tx.CountryByCode(ctx, code); err != nil {
			return err
		}
		setIfChanged(&c.Name, in.Name != nil, name, "name", &changed)
		setIfChanged(&c.PhoneCode, in.PhoneCode != nil, phone, "phoneCode", &changed)
		setIfChanged(&c.DefaultCurrency, in.DefaultCurrency != nil, currency, "defaultCurrency", &changed)
		if err := s.checkNotInUse(c.Active && in.Active != nil && !*in.Active, func(u BusinessUsage) (int, error) {
			return u.ActiveBusinessesInCountry(ctx, string(c.Code))
		}); err != nil {
			return err
		}
		setIfChanged(&c.Active, in.Active != nil, valueOr(in.Active, false), "active", &changed)
		if len(changed) == 0 {
			return nil
		}
		if err := tx.UpdateCountry(ctx, c, s.clock.Now()); err != nil {
			return err
		}
		return tx.AppendAudit(ctx, s.audit(ctx, p, AuditCountryUpdated, "country", string(c.Code)).
			Meta("fields", joinSorted(changed)))
	})
	return c, err
}

// DivisionInput creates a division. Code (ISO 3166-2) is optional; Active defaults to true.
type DivisionInput struct {
	CountryCode, Code, Name, Type string
	Active                        *bool
}

// DivisionPatch changes the given fields of a division (nil = unchanged; Code "" removes it).
type DivisionPatch struct {
	Code, Name, Type *string
	Active           *bool
}

// CreateDivision adds a division to an existing country (PLATFORM_ADMIN).
func (s *Service) CreateDivision(ctx context.Context, p identity.Principal, in DivisionInput) (domain.Division, error) {
	if err := s.authz.RequirePlatformAdmin(ctx, p); err != nil {
		return domain.Division{}, err
	}
	var verr ValidationError
	d := domain.Division{ID: uuid.Must(uuid.NewV7()), Active: valueOr(in.Active, true)}
	var err error
	if d.CountryCode, err = domain.ParseCountryCode(in.CountryCode); err != nil {
		verr.add("countryCode", required(in.CountryCode, CodeInvalidCountryCode))
	} else if d.Code, err = domain.ParseDivisionCode(in.Code, d.CountryCode); err != nil {
		verr.add("code", CodeInvalidDivisionCode)
	}
	d.Name = parseName(&verr, "name", in.Name)
	d.Type = parseDivisionType(&verr, in.Type)
	if err := verr.orNil(); err != nil {
		return domain.Division{}, err
	}
	err = s.store.WithinTx(ctx, func(tx Tx) error {
		if _, err := tx.CountryByCode(ctx, d.CountryCode); err != nil {
			if errors.Is(err, ErrNotFound) {
				return invalid("countryCode", CodeUnknownCountry)
			}
			return err
		}
		if err := tx.InsertDivision(ctx, d, s.clock.Now()); err != nil {
			return err
		}
		return tx.AppendAudit(ctx, s.audit(ctx, p, AuditDivisionCreated, "division", d.ID.String()).
			Meta("country", string(d.CountryCode)))
	})
	return d, err
}

// UpdateDivision changes a division (PLATFORM_ADMIN). It cannot move to another country.
func (s *Service) UpdateDivision(ctx context.Context, p identity.Principal, id uuid.UUID, in DivisionPatch) (domain.Division, error) {
	if err := s.authz.RequirePlatformAdmin(ctx, p); err != nil {
		return domain.Division{}, err
	}
	var verr ValidationError
	var name string
	var typ domain.DivisionType
	if in.Name != nil {
		name = parseName(&verr, "name", *in.Name)
	}
	if in.Type != nil {
		typ = parseDivisionType(&verr, *in.Type)
	}
	if err := verr.orNil(); err != nil {
		return domain.Division{}, err
	}
	var d domain.Division
	err := s.store.WithinTx(ctx, func(tx Tx) error {
		var err error
		if d, err = tx.DivisionByID(ctx, id); err != nil {
			return err
		}
		var changed []string
		if in.Code != nil {
			code, err := domain.ParseDivisionCode(*in.Code, d.CountryCode)
			if err != nil {
				return invalid("code", CodeInvalidDivisionCode)
			}
			setIfChanged(&d.Code, true, code, "code", &changed)
		}
		setIfChanged(&d.Name, in.Name != nil, name, "name", &changed)
		setIfChanged(&d.Type, in.Type != nil, typ, "type", &changed)
		if err := s.checkNotInUse(d.Active && in.Active != nil && !*in.Active, func(u BusinessUsage) (int, error) {
			return u.ActiveBusinessesInDivision(ctx, d.ID)
		}); err != nil {
			return err
		}
		setIfChanged(&d.Active, in.Active != nil, valueOr(in.Active, false), "active", &changed)
		if len(changed) == 0 {
			return nil
		}
		if err := tx.UpdateDivision(ctx, d, s.clock.Now()); err != nil {
			return err
		}
		return tx.AppendAudit(ctx, s.audit(ctx, p, AuditDivisionUpdated, "division", d.ID.String()).
			Meta("fields", joinSorted(changed)))
	})
	return d, err
}

// CityInput creates a city. Active defaults to true.
type CityInput struct {
	DivisionID     uuid.UUID
	Name, TimeZone string
	Active         *bool
}

// CityPatch changes the given fields of a city (nil = unchanged).
type CityPatch struct {
	Name, TimeZone *string
	Active         *bool
}

// CreateCity adds a city to an existing division (PLATFORM_ADMIN).
func (s *Service) CreateCity(ctx context.Context, p identity.Principal, in CityInput) (domain.City, error) {
	if err := s.authz.RequirePlatformAdmin(ctx, p); err != nil {
		return domain.City{}, err
	}
	var verr ValidationError
	c := domain.City{ID: uuid.Must(uuid.NewV7()), DivisionID: in.DivisionID, Active: valueOr(in.Active, true)}
	if in.DivisionID == uuid.Nil {
		verr.add("divisionId", CodeRequired)
	}
	c.Name = parseName(&verr, "name", in.Name)
	c.TimeZone = parseTimeZone(&verr, in.TimeZone)
	if err := verr.orNil(); err != nil {
		return domain.City{}, err
	}
	err := s.store.WithinTx(ctx, func(tx Tx) error {
		d, err := tx.DivisionByID(ctx, c.DivisionID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return invalid("divisionId", CodeUnknownDivision)
			}
			return err
		}
		if err := tx.InsertCity(ctx, c, s.clock.Now()); err != nil {
			return err
		}
		return tx.AppendAudit(ctx, s.audit(ctx, p, AuditCityCreated, "city", c.ID.String()).
			Meta("division", d.ID.String()))
	})
	return c, err
}

// UpdateCity changes a city (PLATFORM_ADMIN). It cannot move to another division, and it
// cannot be deactivated while it has active businesses (ErrInUse). The same rule applies to
// divisions and countries.
func (s *Service) UpdateCity(ctx context.Context, p identity.Principal, id uuid.UUID, in CityPatch) (domain.City, error) {
	if err := s.authz.RequirePlatformAdmin(ctx, p); err != nil {
		return domain.City{}, err
	}
	var verr ValidationError
	var name, tz string
	if in.Name != nil {
		name = parseName(&verr, "name", *in.Name)
	}
	if in.TimeZone != nil {
		tz = parseTimeZone(&verr, *in.TimeZone)
	}
	if err := verr.orNil(); err != nil {
		return domain.City{}, err
	}
	var c domain.City
	err := s.store.WithinTx(ctx, func(tx Tx) error {
		var err error
		if c, err = tx.CityByID(ctx, id); err != nil {
			return err
		}
		var changed []string
		setIfChanged(&c.Name, in.Name != nil, name, "name", &changed)
		setIfChanged(&c.TimeZone, in.TimeZone != nil, tz, "timeZone", &changed)
		if err := s.checkNotInUse(c.Active && in.Active != nil && !*in.Active, func(u BusinessUsage) (int, error) {
			return u.ActiveBusinessesInCity(ctx, c.ID)
		}); err != nil {
			return err
		}
		setIfChanged(&c.Active, in.Active != nil, valueOr(in.Active, false), "active", &changed)
		if len(changed) == 0 {
			return nil
		}
		if err := tx.UpdateCity(ctx, c, s.clock.Now()); err != nil {
			return err
		}
		return tx.AppendAudit(ctx, s.audit(ctx, p, AuditCityUpdated, "city", c.ID.String()).
			Meta("fields", joinSorted(changed)))
	})
	return c, err
}

// --- helpers -----------------------------------------------------------------------------

func (s *Service) audit(ctx context.Context, p identity.Principal, action, kind, id string) *audit.Entry {
	return audit.New(ctx, s.clock.Now(), action, audit.Success).Actor(p.UserID).Resource(kind, id)
}

func required(raw, code string) string {
	if strings.TrimSpace(raw) == "" {
		return CodeRequired
	}
	return code
}

func parseName(verr *ValidationError, field, raw string) string {
	n, err := domain.ParseName(raw)
	if err != nil {
		verr.add(field, required(raw, CodeInvalidName))
	}
	return n
}

func parsePhoneCode(verr *ValidationError, raw string) string {
	v, err := domain.ParsePhoneCode(raw)
	if err != nil {
		verr.add("phoneCode", required(raw, CodeInvalidPhoneCode))
	}
	return v
}

func parseCurrency(verr *ValidationError, raw string) string {
	v, err := domain.ParseCurrency(raw)
	if err != nil {
		verr.add("defaultCurrency", required(raw, CodeUnsupportedCurrency))
	}
	return v
}

func parseDivisionType(verr *ValidationError, raw string) domain.DivisionType {
	v, err := domain.ParseDivisionType(raw)
	if err != nil {
		verr.add("type", required(raw, CodeInvalidDivisionType))
	}
	return v
}

func parseTimeZone(verr *ValidationError, raw string) string {
	v, err := domain.ParseTimeZone(raw)
	if err != nil {
		verr.add("timeZone", required(raw, CodeInvalidTimeZone))
	}
	return v
}

func valueOr(b *bool, def bool) bool {
	if b == nil {
		return def
	}
	return *b
}

// setIfChanged assigns v to *dst when present and different, recording the field name.
func setIfChanged[T comparable](dst *T, present bool, v T, field string, changed *[]string) {
	if present && *dst != v {
		*dst = v
		*changed = append(*changed, field)
	}
}

func joinSorted(fields []string) string {
	sort.Strings(fields)
	return strings.Join(fields, ",")
}
