package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

var t0 = time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)

func TestTrialLifecycle(t *testing.T) {
	s := NewTrial(uuid.Must(uuid.NewV7()), t0)
	if s.Status != StatusTrialing || !s.TrialEndsAt.Equal(t0.Add(7*24*time.Hour)) {
		t.Fatalf("new trial = %+v", s)
	}
	if !s.Entitled(t0) || !s.Entitled(t0.Add(7*24*time.Hour-time.Second)) {
		t.Error("a running demo must be entitled")
	}
	if s.Entitled(t0.Add(7 * 24 * time.Hour)) {
		t.Error("a demo at its end must not be entitled, even before the job marks it EXPIRED")
	}

	// Extension of a running demo; then of an expired one (reopened).
	now := t0.Add(24 * time.Hour)
	if err := s.ExtendTrial(now.Add(10*24*time.Hour), now); err != nil || !s.TrialEndsAt.Equal(now.Add(10*24*time.Hour)) {
		t.Fatalf("extend: %v %+v", err, s)
	}
	at := now
	s.Status, s.ExpiredAt = StatusExpired, &at
	if s.Entitled(now) {
		t.Error("EXPIRED must not be entitled")
	}
	if err := s.ExtendTrial(now.Add(time.Hour), now); err != nil || s.Status != StatusTrialing || s.ExpiredAt != nil {
		t.Errorf("reopen expired demo: %v %+v", err, s)
	}
	for _, until := range []time.Time{now, now.Add(-time.Hour), now.Add(91 * 24 * time.Hour)} {
		if err := s.ExtendTrial(until, now); !errors.Is(err, ErrInvalidExtension) {
			t.Errorf("extend until %v: %v", until, err)
		}
	}

	// Courtesy activation; idempotent; ACTIVE demos are not extendable.
	if !s.ActivateCourtesy(now) || s.Status != StatusActive || !s.Entitled(now.Add(365*24*time.Hour)) {
		t.Errorf("courtesy: %+v", s)
	}
	if s.ActivateCourtesy(now) {
		t.Error("second courtesy activation reported a change")
	}
	if err := s.ExtendTrial(now.Add(time.Hour), now); !errors.Is(err, ErrNotExtendable) {
		t.Errorf("extend ACTIVE: %v", err)
	}
	for st, want := range map[Status]bool{StatusPastDue: true, StatusCanceled: false} {
		if got := (Subscription{Status: st}).Entitled(now); got != want {
			t.Errorf("%s entitled = %v, want %v", st, got, want)
		}
	}
	if (Subscription{Status: StatusTrialing}).Entitled(now) {
		t.Error("a demo without end date must not be entitled")
	}
}

func TestParseStatus(t *testing.T) {
	for _, s := range []string{"TRIALING", "ACTIVE", "PAST_DUE", "EXPIRED", "CANCELED"} {
		if _, err := ParseStatus(s); err != nil {
			t.Error(err)
		}
	}
	if _, err := ParseStatus("FREE"); !errors.Is(err, ErrInvalidStatus) {
		t.Errorf("FREE: %v", err)
	}
}
