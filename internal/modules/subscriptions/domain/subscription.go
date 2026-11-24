// Package domain holds the subscription of a business (ADR-0018): the right to use the
// service, separate from the business's moderation status. Pure Go.
package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// TrialDuration is the length of the free demo (decision of 2026-11-24).
const TrialDuration = 7 * 24 * time.Hour

// MaxTrialExtension bounds how far an administrator may push a demo's end.
const MaxTrialExtension = 90 * 24 * time.Hour

// Errors.
var (
	ErrInvalidStatus    = errors.New("subscriptions: invalid status")
	ErrInvalidExtension = errors.New("subscriptions: invalid trial extension")
	ErrNotExtendable    = errors.New("subscriptions: only demos (running or expired) can be extended")
)

// Status of a subscription (matches subscriptions_status_valid).
type Status string

// Statuses. ACTIVE, PAST_DUE and CANCELED are driven by payments (phase 11B); until then an
// administrator may set ACTIVE as a courtesy.
const (
	StatusTrialing Status = "TRIALING"
	StatusActive   Status = "ACTIVE"
	StatusPastDue  Status = "PAST_DUE"
	StatusExpired  Status = "EXPIRED"
	StatusCanceled Status = "CANCELED"
)

// ParseStatus validates a stored status.
func ParseStatus(s string) (Status, error) {
	switch st := Status(s); st {
	case StatusTrialing, StatusActive, StatusPastDue, StatusExpired, StatusCanceled:
		return st, nil
	default:
		return "", ErrInvalidStatus
	}
}

// Subscription is the billing state of one business.
type Subscription struct {
	BusinessID  uuid.UUID
	Status      Status
	TrialEndsAt *time.Time
	ExpiredAt   *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// NewTrial starts the 7-day demo at now.
func NewTrial(businessID uuid.UUID, now time.Time) Subscription {
	end := now.Add(TrialDuration)
	return Subscription{BusinessID: businessID, Status: StatusTrialing, TrialEndsAt: &end, CreatedAt: now, UpdatedAt: now}
}

// Entitled reports whether the business may be visible and take bookings at now. A demo whose
// end has passed is not entitled even before the hourly job marks it EXPIRED.
func (s Subscription) Entitled(now time.Time) bool {
	switch s.Status {
	case StatusActive, StatusPastDue:
		return true
	case StatusTrialing:
		return s.TrialEndsAt != nil && now.Before(*s.TrialEndsAt)
	default:
		return false
	}
}

// ExtendTrial moves the end of a running or expired demo to until (> now, ≤ now + 90 days); an
// expired demo is reopened.
func (s *Subscription) ExtendTrial(until, now time.Time) error {
	if s.Status != StatusTrialing && s.Status != StatusExpired {
		return ErrNotExtendable
	}
	if !until.After(now) || until.After(now.Add(MaxTrialExtension)) {
		return ErrInvalidExtension
	}
	s.Status, s.TrialEndsAt, s.ExpiredAt, s.UpdatedAt = StatusTrialing, &until, nil, now
	return nil
}

// ActivateCourtesy marks the subscription ACTIVE without payment (administrator, until payments
// exist). Idempotent; reports whether it changed.
func (s *Subscription) ActivateCourtesy(now time.Time) bool {
	if s.Status == StatusActive {
		return false
	}
	s.Status, s.ExpiredAt, s.UpdatedAt = StatusActive, nil, now
	return true
}
