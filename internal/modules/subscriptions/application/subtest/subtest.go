// Package subtest provides an in-memory store for the subscriptions use cases.
package subtest

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/subscriptions/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/subscriptions/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/audit"
)

// Store is an in-memory application.Store: changes commit only when fn succeeds.
type Store struct {
	mu    sync.Mutex
	subs  map[uuid.UUID]domain.Subscription
	Audit []audit.Entry
}

// NewStore returns an empty store.
func NewStore() *Store { return &Store{subs: map[uuid.UUID]domain.Subscription{}} }

// Get returns the stored subscription of a business (test helper).
func (s *Store) Get(businessID uuid.UUID) (domain.Subscription, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sub, ok := s.subs[businessID]
	return sub, ok
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
	t := &tx{subs: make(map[uuid.UUID]domain.Subscription, len(s.subs))}
	for k, v := range s.subs {
		t.subs[k] = v
	}
	if err := fn(t); err != nil {
		return err
	}
	s.subs = t.subs
	s.Audit = append(s.Audit, t.audit...)
	return nil
}

type tx struct {
	subs  map[uuid.UUID]domain.Subscription
	audit []audit.Entry
}

func (t *tx) InsertSubscription(_ context.Context, s domain.Subscription) error {
	t.subs[s.BusinessID] = s
	return nil
}

func (t *tx) SubscriptionByBusiness(_ context.Context, id uuid.UUID, _ bool) (domain.Subscription, error) {
	s, ok := t.subs[id]
	if !ok {
		return domain.Subscription{}, application.ErrNotFound
	}
	return s, nil
}

func (t *tx) SubscriptionsOf(_ context.Context, ids []uuid.UUID) ([]domain.Subscription, error) {
	var out []domain.Subscription
	for _, id := range ids {
		if s, ok := t.subs[id]; ok {
			out = append(out, s)
		}
	}
	return out, nil
}

// Set stores a subscription directly (test helper).
func (s *Store) Set(sub domain.Subscription) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subs[sub.BusinessID] = sub
}

func (t *tx) UpdateSubscription(_ context.Context, s domain.Subscription) error {
	t.subs[s.BusinessID] = s
	return nil
}

func (t *tx) ExpireDueTrials(_ context.Context, now time.Time, limit int) ([]uuid.UUID, error) {
	var due []domain.Subscription
	for _, s := range t.subs {
		if s.Status == domain.StatusTrialing && !s.TrialEndsAt.After(now) {
			due = append(due, s)
		}
	}
	sort.Slice(due, func(i, j int) bool { return due[i].TrialEndsAt.Before(*due[j].TrialEndsAt) })
	if len(due) > limit {
		due = due[:limit]
	}
	ids := make([]uuid.UUID, 0, len(due))
	for _, s := range due {
		at := now
		s.Status, s.ExpiredAt, s.UpdatedAt = domain.StatusExpired, &at, now
		t.subs[s.BusinessID] = s
		ids = append(ids, s.BusinessID)
	}
	return ids, nil
}

func (t *tx) AppendAudit(_ context.Context, e *audit.Entry) error {
	t.audit = append(t.audit, *e)
	return nil
}
