// Package proftest provides an in-memory store for the professionals use cases.
package proftest

import (
	"context"
	"maps"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/audit"
)

// Store is an in-memory application.Store: changes commit only when fn succeeds.
type Store struct {
	mu        sync.Mutex
	profs     map[uuid.UUID]domain.Professional
	schedules map[uuid.UUID]domain.WeeklySchedule
	absences  map[uuid.UUID]domain.Absence
	Audit     []audit.Entry
}

// NewStore returns an empty store.
func NewStore() *Store {
	return &Store{profs: map[uuid.UUID]domain.Professional{}, schedules: map[uuid.UUID]domain.WeeklySchedule{}, absences: map[uuid.UUID]domain.Absence{}}
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

// Count returns the number of stored professionals.
func (s *Store) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.profs)
}

// WithinTx implements application.Store.
func (s *Store) WithinTx(ctx context.Context, fn func(context.Context, application.Tx) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := &tx{profs: maps.Clone(s.profs), schedules: maps.Clone(s.schedules), absences: maps.Clone(s.absences)}
	if err := fn(ctx, t); err != nil {
		return err
	}
	s.profs, s.schedules, s.absences = t.profs, t.schedules, t.absences
	s.Audit = append(s.Audit, t.audit...)
	return nil
}

type tx struct {
	profs     map[uuid.UUID]domain.Professional
	schedules map[uuid.UUID]domain.WeeklySchedule
	absences  map[uuid.UUID]domain.Absence
	audit     []audit.Entry
}

func (t *tx) overlaps(a domain.Absence) bool {
	for _, o := range t.absences {
		if o.ID != a.ID && o.ProfessionalID == a.ProfessionalID && a.Start.Before(o.End) && o.Start.Before(a.End) {
			return true
		}
	}
	return false
}

func (t *tx) InsertAbsence(_ context.Context, a domain.Absence) error {
	if t.overlaps(a) {
		return application.ErrAbsenceOverlap
	}
	t.absences[a.ID] = a
	return nil
}

func (t *tx) UpdateAbsence(ctx context.Context, a domain.Absence) error {
	return t.InsertAbsence(ctx, a)
}

func (t *tx) DeleteAbsence(_ context.Context, a domain.Absence) error {
	delete(t.absences, a.ID)
	return nil
}

func (t *tx) AbsenceByID(_ context.Context, businessID, professionalID, id uuid.UUID) (domain.Absence, error) {
	a, ok := t.absences[id]
	if !ok || a.BusinessID != businessID || a.ProfessionalID != professionalID {
		return domain.Absence{}, application.ErrNotFound
	}
	return a, nil
}

func (t *tx) ListAbsences(_ context.Context, businessID, professionalID uuid.UUID, from, to time.Time, limit int) ([]domain.Absence, error) {
	var out []domain.Absence
	for _, a := range t.absences {
		if a.BusinessID == businessID && a.ProfessionalID == professionalID && a.End.After(from) && a.Start.Before(to) {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (t *tx) LockProfessional(ctx context.Context, businessID, id uuid.UUID) (domain.Professional, error) {
	return t.ProfessionalByID(ctx, businessID, id) // the store mutex already serializes transactions
}

func (t *tx) WeeklySchedule(_ context.Context, businessID, id uuid.UUID) (domain.WeeklySchedule, error) {
	if p, ok := t.profs[id]; !ok || p.BusinessID != businessID {
		return domain.EmptyWeek(), nil
	}
	if w, ok := t.schedules[id]; ok {
		return w, nil
	}
	return domain.EmptyWeek(), nil
}

func (t *tx) ReplaceWeeklySchedule(_ context.Context, businessID, id uuid.UUID, w domain.WeeklySchedule) error {
	if p, ok := t.profs[id]; !ok || p.BusinessID != businessID {
		return application.ErrNotFound
	}
	t.schedules[id] = w
	return nil
}

func (t *tx) InsertProfessional(_ context.Context, p domain.Professional) error {
	for _, o := range t.profs {
		if o.BusinessID == p.BusinessID && o.UserID == p.UserID {
			return application.ErrAlreadyExists
		}
	}
	t.profs[p.ID] = p
	return nil
}

func (t *tx) UpdateProfessional(_ context.Context, p domain.Professional) error {
	t.profs[p.ID] = p
	return nil
}

func (t *tx) ProfessionalByID(_ context.Context, businessID, id uuid.UUID) (domain.Professional, error) {
	p, ok := t.profs[id]
	if !ok || p.BusinessID != businessID {
		return domain.Professional{}, application.ErrNotFound
	}
	return p, nil
}

func (t *tx) ListProfessionals(_ context.Context, businessID uuid.UUID) ([]domain.Professional, error) {
	var out []domain.Professional
	for _, p := range t.profs {
		if p.BusinessID == businessID {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SortOrder != out[j].SortOrder {
			return out[i].SortOrder < out[j].SortOrder
		}
		return out[i].DisplayName < out[j].DisplayName
	})
	return out, nil
}

func (t *tx) AppendAudit(_ context.Context, e *audit.Entry) error {
	t.audit = append(t.audit, *e)
	return nil
}

// Identity is an in-memory identity contract: accounts by email, memberships per business and the
// same authorization rules (PLATFORM_ADMIN passes; no membership → not found).
type Identity struct {
	mu      sync.Mutex
	users   map[string]user
	members map[uuid.UUID]map[uuid.UUID]identity.Role // business → user → role
}

type user struct {
	id     uuid.UUID
	active bool
}

// NewIdentity returns an empty identity double.
func NewIdentity() *Identity {
	return &Identity{users: map[string]user{}, members: map[uuid.UUID]map[uuid.UUID]identity.Role{}}
}

// AddUser registers an account; active false means unverified or suspended.
func (i *Identity) AddUser(email string, active bool) uuid.UUID {
	i.mu.Lock()
	defer i.mu.Unlock()
	id := uuid.Must(uuid.NewV7())
	i.users[email] = user{id: id, active: active}
	return id
}

// Grant gives userID a role in businessID.
func (i *Identity) Grant(businessID, userID uuid.UUID, role identity.Role) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.members[businessID] == nil {
		i.members[businessID] = map[uuid.UUID]identity.Role{}
	}
	i.members[businessID][userID] = role
}

// Remove revokes the membership of userID in businessID.
func (i *Identity) Remove(businessID, userID uuid.UUID) {
	i.mu.Lock()
	defer i.mu.Unlock()
	delete(i.members[businessID], userID)
}

// Role returns the current role of userID in businessID ("" if none).
func (i *Identity) Role(businessID, userID uuid.UUID) identity.Role {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.members[businessID][userID]
}

// RequireBusinessRole implements the identity Authorizer.
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

// EnsureMember implements identity.MemberEnsurer.
func (i *Identity) EnsureMember(ctx context.Context, actor identity.Principal, businessID uuid.UUID, email string, role identity.Role) (uuid.UUID, error) {
	if _, err := i.RequireBusinessRole(ctx, actor, businessID, identity.RolesOwner...); err != nil {
		return uuid.Nil, err
	}
	i.mu.Lock()
	u, ok := i.users[email]
	i.mu.Unlock()
	if !ok || !u.active {
		return uuid.Nil, identity.ErrAccountNotEligible
	}
	if i.Role(businessID, u.id) == "" {
		i.Grant(businessID, u.id, role)
	}
	return u.id, nil
}

// ActiveMembersAmong implements identity.MembershipChecker.
func (i *Identity) ActiveMembersAmong(_ context.Context, businessID uuid.UUID, userIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	out := map[uuid.UUID]bool{}
	for _, id := range userIDs {
		if _, ok := i.members[businessID][id]; ok {
			out[id] = true
		}
	}
	return out, nil
}
