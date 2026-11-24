// Package catalogtest provides in-memory doubles for the catalog use cases.
package catalogtest

import (
	"context"
	"maps"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"

	businesses "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/domain"
	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/audit"
)

// Store is an in-memory application.Store: changes commit only when fn succeeds.
type Store struct {
	mu         sync.Mutex
	categories map[uuid.UUID]domain.Category
	services   map[uuid.UUID]domain.Service
	Audit      []audit.Entry
}

// NewStore returns an empty store.
func NewStore() *Store {
	return &Store{categories: map[uuid.UUID]domain.Category{}, services: map[uuid.UUID]domain.Service{}}
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
	t := &tx{categories: maps.Clone(s.categories), services: maps.Clone(s.services)}
	if err := fn(ctx, t); err != nil {
		return err
	}
	s.categories, s.services = t.categories, t.services
	s.Audit = append(s.Audit, t.audit...)
	return nil
}

type tx struct {
	categories map[uuid.UUID]domain.Category
	services   map[uuid.UUID]domain.Service
	audit      []audit.Entry
}

func (t *tx) InsertCategory(_ context.Context, c domain.Category) error {
	for _, o := range t.categories {
		if o.ID != c.ID && o.BusinessID == c.BusinessID && strings.EqualFold(o.Name, c.Name) {
			return application.ErrNameTaken
		}
	}
	t.categories[c.ID] = c
	return nil
}

func (t *tx) UpdateCategory(ctx context.Context, c domain.Category) error {
	return t.InsertCategory(ctx, c)
}

func (t *tx) DeleteCategory(_ context.Context, businessID, id uuid.UUID) error {
	delete(t.categories, id)
	for k, s := range t.services { // ON DELETE SET NULL (category_id)
		if s.BusinessID == businessID && s.CategoryID == id {
			s.CategoryID = uuid.Nil
			t.services[k] = s
		}
	}
	return nil
}

func (t *tx) CategoryByID(_ context.Context, businessID, id uuid.UUID) (domain.Category, error) {
	c, ok := t.categories[id]
	if !ok || c.BusinessID != businessID {
		return domain.Category{}, application.ErrNotFound
	}
	return c, nil
}

func (t *tx) ListCategories(_ context.Context, businessID uuid.UUID) ([]domain.Category, error) {
	var out []domain.Category
	for _, c := range t.categories {
		if c.BusinessID == businessID {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SortOrder != out[j].SortOrder {
			return out[i].SortOrder < out[j].SortOrder
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func (t *tx) InsertService(_ context.Context, s domain.Service) error {
	for _, o := range t.services {
		if o.ID != s.ID && o.BusinessID == s.BusinessID && strings.EqualFold(o.Name, s.Name) {
			return application.ErrNameTaken
		}
	}
	t.services[s.ID] = s
	return nil
}

func (t *tx) UpdateService(ctx context.Context, s domain.Service) error {
	return t.InsertService(ctx, s)
}

func (t *tx) ServiceByID(_ context.Context, businessID, id uuid.UUID) (domain.Service, error) {
	s, ok := t.services[id]
	if !ok || s.BusinessID != businessID {
		return domain.Service{}, application.ErrNotFound
	}
	return s, nil
}

func (t *tx) ListServices(_ context.Context, businessID uuid.UUID) ([]domain.Service, error) {
	var out []domain.Service
	for _, s := range t.services {
		if s.BusinessID == businessID {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SortOrder != out[j].SortOrder {
			return out[i].SortOrder < out[j].SortOrder
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func (t *tx) AppendAudit(_ context.Context, e *audit.Entry) error {
	t.audit = append(t.audit, *e)
	return nil
}

// Identity is an in-memory authorizer with the identity contract's rules (PLATFORM_ADMIN
// passes as OWNER; no membership → not found).
type Identity struct {
	mu      sync.Mutex
	members map[uuid.UUID]map[uuid.UUID]identity.Role
}

// NewIdentity returns an authorizer without memberships.
func NewIdentity() *Identity { return &Identity{members: map[uuid.UUID]map[uuid.UUID]identity.Role{}} }

// Grant gives userID a role in businessID.
func (i *Identity) Grant(businessID, userID uuid.UUID, role identity.Role) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.members[businessID] == nil {
		i.members[businessID] = map[uuid.UUID]identity.Role{}
	}
	i.members[businessID][userID] = role
}

// RequireBusinessRole implements application.Identity.
func (i *Identity) RequireBusinessRole(_ context.Context, p identity.Principal, businessID uuid.UUID, allowed ...identity.Role) (identity.Role, error) {
	if identity.CheckPlatformAdmin(p) == nil && businessID != uuid.Nil {
		return identity.RoleOwner, nil
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	var ms []identity.Membership
	if r, ok := i.members[businessID][p.UserID]; ok {
		ms = append(ms, identity.Membership{BusinessID: businessID, Role: r})
	}
	return identity.CheckBusinessRole(ms, businessID, allowed...)
}

// Currencies maps businesses to currencies; unknown businesses default to COP.
type Currencies map[uuid.UUID]string

// CurrencyOf implements businesses.Currencies.
func (c Currencies) CurrencyOf(_ context.Context, businessID uuid.UUID) (string, error) {
	if v, ok := c[businessID]; ok {
		if v == "" {
			return "", businesses.ErrBusinessNotFound
		}
		return v, nil
	}
	return "COP", nil
}
