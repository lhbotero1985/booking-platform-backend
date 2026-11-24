// Package application holds the subscriptions use cases (ADR-0018): starting demos, the
// administrator's adjustments and the hourly expiry.
//
// Rules (enforced by depguard): may import domain, the identity contract and platform packages;
// no HTTP, database drivers or River.
package application

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	subscriptions "github.com/lhbotero1985/booking-platform-backend/internal/modules/subscriptions/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/subscriptions/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/audit"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
)

// Audited actions.
const (
	AuditTrialStarted      = "subscription.trial_started"
	AuditTrialExtended     = "subscription.trial_extended"
	AuditTrialExpired      = "subscription.expired"
	AuditCourtesyActivated = "subscription.courtesy_activated"
)

// ErrNotFound is returned when a business has no subscription (or does not exist).
var ErrNotFound = errors.New("subscriptions: not found")

// expireBatch bounds one expiry transaction; the run loops until no demo is due.
const expireBatch = 500

// MaxExtensionDays bounds an administrator's extension request.
const MaxExtensionDays = 90

// Store runs a unit of work in a transaction (joining the caller's one when ctx carries it).
type Store interface {
	WithinTx(ctx context.Context, fn func(tx Tx) error) error
}

// Tx is the persistence available inside a transaction.
type Tx interface {
	InsertSubscription(ctx context.Context, s domain.Subscription) error
	// SubscriptionByBusiness returns ErrNotFound when absent; forUpdate locks the row.
	SubscriptionByBusiness(ctx context.Context, businessID uuid.UUID, forUpdate bool) (domain.Subscription, error)
	UpdateSubscription(ctx context.Context, s domain.Subscription) error
	// ExpireDueTrials marks up to limit demos ended at now as EXPIRED and returns their businesses.
	ExpireDueTrials(ctx context.Context, now time.Time, limit int) ([]uuid.UUID, error)
	AppendAudit(ctx context.Context, e *audit.Entry) error
}

// Authorizer is the part of the identity contract these use cases need.
type Authorizer interface {
	RequirePlatformAdmin(ctx context.Context, p contract.Principal) error
}

// Service implements the subscriptions use cases.
type Service struct {
	store Store
	authz Authorizer
	clock clock.Clock
}

// NewService builds the service; a nil clock defaults to the system clock.
func NewService(store Store, authz Authorizer, clk clock.Clock) *Service {
	if clk == nil {
		clk = clock.System{}
	}
	return &Service{store: store, authz: authz, clock: clk}
}

// StartTrial implements contract.Trials. It is not authorized here: only the businesses use
// cases call it, right after creating the business in the same transaction.
func (s *Service) StartTrial(ctx context.Context, businessID uuid.UUID) error {
	now := s.clock.Now()
	sub := domain.NewTrial(businessID, now)
	return s.store.WithinTx(ctx, func(tx Tx) error {
		if err := tx.InsertSubscription(ctx, sub); err != nil {
			return err
		}
		return tx.AppendAudit(ctx, audit.New(ctx, now, AuditTrialStarted, audit.Success).
			Business(businessID).Resource("subscription", businessID.String()).
			Meta("trial_ends_at", sub.TrialEndsAt.Format(time.RFC3339)))
	})
}

// Entitled implements contract.Entitlements.
func (s *Service) Entitled(ctx context.Context, businessID uuid.UUID) (bool, error) {
	var ok bool
	err := s.store.WithinTx(ctx, func(tx Tx) error {
		sub, err := tx.SubscriptionByBusiness(ctx, businessID, false)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		ok = sub.Entitled(s.clock.Now())
		return nil
	})
	return ok, err
}

// SummaryOf implements contract.Summaries (the caller has authorized the request).
func (s *Service) SummaryOf(ctx context.Context, businessID uuid.UUID) (subscriptions.Summary, error) {
	var out subscriptions.Summary
	err := s.store.WithinTx(ctx, func(tx Tx) error {
		sub, err := tx.SubscriptionByBusiness(ctx, businessID, false)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		out = subscriptions.Summary{Status: string(sub.Status), TrialEndsAt: sub.TrialEndsAt, Entitled: sub.Entitled(s.clock.Now())}
		return nil
	})
	return out, err
}

// Get returns a business's subscription (PLATFORM_ADMIN).
func (s *Service) Get(ctx context.Context, p contract.Principal, businessID uuid.UUID) (domain.Subscription, error) {
	if err := s.authz.RequirePlatformAdmin(ctx, p); err != nil {
		return domain.Subscription{}, err
	}
	var sub domain.Subscription
	err := s.store.WithinTx(ctx, func(tx Tx) (err error) {
		sub, err = tx.SubscriptionByBusiness(ctx, businessID, false)
		return err
	})
	return sub, err
}

// ExtendTrial gives a running or expired demo `days` more days counted from now
// (PLATFORM_ADMIN).
func (s *Service) ExtendTrial(ctx context.Context, p contract.Principal, businessID uuid.UUID, days int) (domain.Subscription, error) {
	if err := s.authz.RequirePlatformAdmin(ctx, p); err != nil {
		return domain.Subscription{}, err
	}
	if days < 1 || days > MaxExtensionDays {
		return domain.Subscription{}, domain.ErrInvalidExtension
	}
	return s.change(ctx, p, businessID, AuditTrialExtended, func(sub *domain.Subscription, now time.Time) (bool, error) {
		return true, sub.ExtendTrial(now.Add(time.Duration(days)*24*time.Hour), now)
	})
}

// ActivateCourtesy marks the subscription ACTIVE without payment (PLATFORM_ADMIN), until the
// payments phase exists.
func (s *Service) ActivateCourtesy(ctx context.Context, p contract.Principal, businessID uuid.UUID) (domain.Subscription, error) {
	if err := s.authz.RequirePlatformAdmin(ctx, p); err != nil {
		return domain.Subscription{}, err
	}
	return s.change(ctx, p, businessID, AuditCourtesyActivated, func(sub *domain.Subscription, now time.Time) (bool, error) {
		return sub.ActivateCourtesy(now), nil
	})
}

func (s *Service) change(ctx context.Context, p contract.Principal, businessID uuid.UUID, action string,
	apply func(*domain.Subscription, time.Time) (bool, error)) (domain.Subscription, error) {
	var sub domain.Subscription
	err := s.store.WithinTx(ctx, func(tx Tx) error {
		var err error
		if sub, err = tx.SubscriptionByBusiness(ctx, businessID, true); err != nil {
			return err
		}
		now := s.clock.Now()
		changed, err := apply(&sub, now)
		if err != nil || !changed {
			return err
		}
		if err := tx.UpdateSubscription(ctx, sub); err != nil {
			return err
		}
		e := audit.New(ctx, now, action, audit.Success).Actor(p.UserID).Business(businessID).
			Resource("subscription", businessID.String()).Meta("status", string(sub.Status))
		if sub.TrialEndsAt != nil {
			e.Meta("trial_ends_at", sub.TrialEndsAt.Format(time.RFC3339))
		}
		return tx.AppendAudit(ctx, e)
	})
	return sub, err
}

// ExpireDueTrials marks ended demos as EXPIRED (hourly River job). Each expiry is audited
// without actor (system). Returns how many expired.
func (s *Service) ExpireDueTrials(ctx context.Context) (int, error) {
	total := 0
	for {
		var n int
		err := s.store.WithinTx(ctx, func(tx Tx) error {
			now := s.clock.Now()
			ids, err := tx.ExpireDueTrials(ctx, now, expireBatch)
			if err != nil {
				return err
			}
			for _, id := range ids {
				if err := tx.AppendAudit(ctx, audit.New(ctx, now, AuditTrialExpired, audit.Success).
					Business(id).Resource("subscription", id.String()).Meta("source", "job")); err != nil {
					return err
				}
			}
			n = len(ids)
			return nil
		})
		if err != nil {
			return total, err
		}
		total += n
		if n < expireBatch {
			return total, nil
		}
	}
}
