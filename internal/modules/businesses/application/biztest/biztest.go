// Package biztest provides in-memory test doubles for the businesses use cases.
package biztest

import (
	"bytes"
	"context"
	"sort"
	"sync"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/domain"
	geography "github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/contract"
	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	subscriptions "github.com/lhbotero1985/booking-platform-backend/internal/modules/subscriptions/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/audit"
)

// Store is an in-memory application.Store: a transaction works on a copy committed only when
// fn succeeds. Businesses are stored by pointer copies so callers cannot mutate them.
type Store struct {
	mu         sync.Mutex
	businesses map[uuid.UUID]domain.Business
	Audit      []audit.Entry
}

// NewStore returns an empty store.
func NewStore() *Store { return &Store{businesses: map[uuid.UUID]domain.Business{}} }

// Count returns the number of stored businesses.
func (s *Store) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.businesses)
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
func (s *Store) WithinTx(ctx context.Context, fn func(context.Context, application.Tx) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := &tx{businesses: make(map[uuid.UUID]domain.Business, len(s.businesses))}
	for k, v := range s.businesses {
		t.businesses[k] = v
	}
	if err := fn(ctx, t); err != nil {
		return err
	}
	s.businesses = t.businesses
	s.Audit = append(s.Audit, t.audit...)
	return nil
}

type tx struct {
	businesses map[uuid.UUID]domain.Business
	audit      []audit.Entry
}

func (t *tx) InsertBusiness(_ context.Context, b *domain.Business) error {
	for _, o := range t.businesses {
		if o.Slug() == b.Slug() {
			return application.ErrSlugTaken
		}
	}
	t.businesses[b.ID()] = *b
	return nil
}

func (t *tx) UpdateBusiness(_ context.Context, b *domain.Business) error {
	t.businesses[b.ID()] = *b
	return nil
}

func (t *tx) BusinessByID(_ context.Context, id uuid.UUID) (*domain.Business, error) {
	b, ok := t.businesses[id]
	if !ok {
		return nil, application.ErrNotFound
	}
	return &b, nil
}

func (t *tx) BusinessBySlug(_ context.Context, slug domain.Slug) (*domain.Business, error) {
	for _, b := range t.businesses {
		if b.Slug() == slug {
			return &b, nil
		}
	}
	return nil, application.ErrNotFound
}

func (t *tx) SlugTaken(_ context.Context, slug domain.Slug) (bool, error) {
	for _, b := range t.businesses {
		if b.Slug() == slug {
			return true, nil
		}
	}
	return false, nil
}

func (t *tx) ListBusinesses(_ context.Context, f application.ListFilter) ([]*domain.Business, error) {
	var out []*domain.Business
	for _, b := range t.businesses {
		if (f.Status == "" || b.Status() == f.Status) && (f.CityID == uuid.Nil || b.Location().CityID == f.CityID) &&
			(f.After == uuid.Nil || less(f.After, b.ID())) {
			out = append(out, &b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return less(out[i].ID(), out[j].ID()) })
	if len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out, nil
}

func less(a, b uuid.UUID) bool { return bytes.Compare(a[:], b[:]) < 0 }

func (t *tx) AppendAudit(_ context.Context, e *audit.Entry) error {
	t.audit = append(t.audit, *e)
	return nil
}

// Places resolves cities from a map (geography contract).
type Places map[uuid.UUID]geography.Place

// PlaceOfCity implements geography.Places.
func (p Places) PlaceOfCity(_ context.Context, id uuid.UUID) (geography.Place, error) {
	pl, ok := p[id]
	if !ok {
		return geography.Place{}, geography.ErrCityNotFound
	}
	return pl, nil
}

// Members records AddMember calls; Err makes them fail.
type Members struct {
	Err   error
	Added []identity.Membership
}

// AddMember implements identity.MemberAdder.
func (m *Members) AddMember(_ context.Context, _ identity.Principal, businessID uuid.UUID, _ string, role identity.Role) (uuid.UUID, error) {
	if m.Err != nil {
		return uuid.Nil, m.Err
	}
	m.Added = append(m.Added, identity.Membership{BusinessID: businessID, Role: role})
	return uuid.Must(uuid.NewV7()), nil
}

// Authorizer allows principals flagged IsPlatformAdmin, and business roles from Memberships
// (user → memberships), mirroring the identity authorizer.
type Authorizer struct {
	Memberships map[uuid.UUID][]identity.Membership
}

// RequirePlatformAdmin implements application.Authorizer.
func (Authorizer) RequirePlatformAdmin(_ context.Context, p identity.Principal) error {
	return identity.CheckPlatformAdmin(p)
}

// RequireBusinessRole implements application.Authorizer.
func (a Authorizer) RequireBusinessRole(_ context.Context, p identity.Principal, businessID uuid.UUID, allowed ...identity.Role) (identity.Role, error) {
	if identity.CheckPlatformAdmin(p) == nil && businessID != uuid.Nil {
		return identity.RoleOwner, nil
	}
	return identity.CheckBusinessRole(a.Memberships[p.UserID], businessID, allowed...)
}

// Summaries returns a fixed subscription summary for every business.
type Summaries struct {
	Summary subscriptions.Summary
	Err     error
}

// SummaryOf implements subscriptions contract Summaries.
func (s Summaries) SummaryOf(context.Context, uuid.UUID) (subscriptions.Summary, error) {
	return s.Summary, s.Err
}

// Owners records self-service ownership claims; a second claim by the same user fails like the
// identity module (one owned business per account). Err makes every claim fail.
type Owners struct {
	Err   error
	owned map[uuid.UUID]bool
}

// ClaimOwnership implements identity.OwnershipClaimer.
func (o *Owners) ClaimOwnership(_ context.Context, p identity.Principal, _ uuid.UUID) error {
	if o.Err != nil {
		return o.Err
	}
	if o.owned == nil {
		o.owned = map[uuid.UUID]bool{}
	}
	if o.owned[p.UserID] {
		return identity.ErrAlreadyOwner
	}
	o.owned[p.UserID] = true
	return nil
}

// Trials records started demos; Err makes StartTrial fail.
type Trials struct {
	Err     error
	Started []uuid.UUID
}

// StartTrial implements subscriptions contract Trials.
func (t *Trials) StartTrial(_ context.Context, businessID uuid.UUID) error {
	if t.Err != nil {
		return t.Err
	}
	t.Started = append(t.Started, businessID)
	return nil
}
