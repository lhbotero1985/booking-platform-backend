// Package geotest provides in-memory test doubles for the geography use cases.
package geotest

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/audit"
)

// Store is an in-memory application.Store. A transaction works on a copy that is committed
// only when fn succeeds, like the real store.
type Store struct {
	mu        sync.Mutex
	countries map[domain.CountryCode]domain.Country
	divisions map[uuid.UUID]domain.Division
	cities    map[uuid.UUID]domain.City
	Audit     []audit.Entry
}

// NewStore returns an empty store.
func NewStore() *Store {
	return &Store{
		countries: map[domain.CountryCode]domain.Country{},
		divisions: map[uuid.UUID]domain.Division{},
		cities:    map[uuid.UUID]domain.City{},
	}
}

// Seed IDs (same as migration 00005).
var (
	ValleID     = uuid.MustParse("01a20aae-7e56-796c-9de9-5409ce51e7d4")
	AntioquiaID = uuid.MustParse("01a20aae-7e5a-7f3e-953c-9ec690399d8d")
	CaliID      = uuid.MustParse("01a20aae-7e5d-73d7-b7d3-a11c5afb9655")
)

// NewSeededStore returns a store with Colombia, Valle del Cauca (Cali) and Antioquia.
func NewSeededStore() *Store {
	s := NewStore()
	s.countries["CO"] = domain.Country{Code: "CO", Name: "Colombia", PhoneCode: "+57", DefaultCurrency: "COP", Active: true}
	s.divisions[ValleID] = domain.Division{ID: ValleID, CountryCode: "CO", Code: "CO-VAC", Name: "Valle del Cauca", Type: domain.DivisionDepartment, Active: true}
	s.divisions[AntioquiaID] = domain.Division{ID: AntioquiaID, CountryCode: "CO", Code: "CO-ANT", Name: "Antioquia", Type: domain.DivisionDepartment, Active: true}
	s.cities[CaliID] = domain.City{ID: CaliID, DivisionID: ValleID, Name: "Cali", TimeZone: "America/Bogota", Active: true}
	return s
}

// AuditActions lists the audited actions in order.
func (s *Store) AuditActions() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.Audit))
	for i, e := range s.Audit {
		out[i] = e.Action
	}
	return out
}

// WithinTx implements application.Store.
func (s *Store) WithinTx(_ context.Context, fn func(application.Tx) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx := &tx{
		countries: clone(s.countries), divisions: clone(s.divisions), cities: clone(s.cities),
	}
	if err := fn(tx); err != nil {
		return err
	}
	s.countries, s.divisions, s.cities = tx.countries, tx.divisions, tx.cities
	s.Audit = append(s.Audit, tx.audit...)
	return nil
}

func clone[K comparable, V any](m map[K]V) map[K]V {
	out := make(map[K]V, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

type tx struct {
	countries map[domain.CountryCode]domain.Country
	divisions map[uuid.UUID]domain.Division
	cities    map[uuid.UUID]domain.City
	audit     []audit.Entry
}

func (t *tx) ListActiveCountries(context.Context) ([]domain.Country, error) {
	var out []domain.Country
	for _, c := range t.countries {
		if c.Active {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (t *tx) CountryByCode(_ context.Context, code domain.CountryCode) (domain.Country, error) {
	c, ok := t.countries[code]
	if !ok {
		return domain.Country{}, application.ErrNotFound
	}
	return c, nil
}

func (t *tx) InsertCountry(_ context.Context, c domain.Country, _ time.Time) error {
	if _, dup := t.countries[c.Code]; dup {
		return application.ErrConflict
	}
	t.countries[c.Code] = c
	return nil
}

func (t *tx) UpdateCountry(_ context.Context, c domain.Country, _ time.Time) error {
	t.countries[c.Code] = c
	return nil
}

func (t *tx) ListActiveDivisions(_ context.Context, country domain.CountryCode) ([]domain.Division, error) {
	var out []domain.Division
	for _, d := range t.divisions {
		if d.CountryCode == country && d.Active {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (t *tx) DivisionByID(_ context.Context, id uuid.UUID) (domain.Division, error) {
	d, ok := t.divisions[id]
	if !ok {
		return domain.Division{}, application.ErrNotFound
	}
	return d, nil
}

func (t *tx) divisionTaken(d domain.Division) bool {
	for _, o := range t.divisions {
		if o.ID == d.ID {
			continue
		}
		if (d.Code != "" && o.Code == d.Code) || (o.CountryCode == d.CountryCode && strings.EqualFold(o.Name, d.Name)) {
			return true
		}
	}
	return false
}

func (t *tx) InsertDivision(_ context.Context, d domain.Division, _ time.Time) error {
	if t.divisionTaken(d) {
		return application.ErrConflict
	}
	t.divisions[d.ID] = d
	return nil
}

func (t *tx) UpdateDivision(_ context.Context, d domain.Division, _ time.Time) error {
	if t.divisionTaken(d) {
		return application.ErrConflict
	}
	t.divisions[d.ID] = d
	return nil
}

func (t *tx) ListActiveCities(_ context.Context, divisionID uuid.UUID) ([]domain.City, error) {
	var out []domain.City
	for _, c := range t.cities {
		if c.DivisionID == divisionID && c.Active {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (t *tx) CityByID(_ context.Context, id uuid.UUID) (domain.City, error) {
	c, ok := t.cities[id]
	if !ok {
		return domain.City{}, application.ErrNotFound
	}
	return c, nil
}

func (t *tx) cityTaken(c domain.City) bool {
	for _, o := range t.cities {
		if o.ID != c.ID && o.DivisionID == c.DivisionID && strings.EqualFold(o.Name, c.Name) {
			return true
		}
	}
	return false
}

func (t *tx) InsertCity(_ context.Context, c domain.City, _ time.Time) error {
	if t.cityTaken(c) {
		return application.ErrConflict
	}
	t.cities[c.ID] = c
	return nil
}

func (t *tx) UpdateCity(_ context.Context, c domain.City, _ time.Time) error {
	if t.cityTaken(c) {
		return application.ErrConflict
	}
	t.cities[c.ID] = c
	return nil
}

func (t *tx) AppendAudit(_ context.Context, e *audit.Entry) error {
	t.audit = append(t.audit, *e)
	return nil
}

// Authorizer allows principals flagged IsPlatformAdmin, mirroring contract.CheckPlatformAdmin.
type Authorizer struct{}

// RequirePlatformAdmin implements application.Authorizer.
func (Authorizer) RequirePlatformAdmin(_ context.Context, p contract.Principal) error {
	return contract.CheckPlatformAdmin(p)
}
