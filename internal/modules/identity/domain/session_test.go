package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestPolicyFor(t *testing.T) {
	cases := []struct {
		c     ClientType
		admin bool
		want  SessionPolicy
	}{
		{ClientMobile, false, SessionPolicy{30 * 24 * time.Hour, 90 * 24 * time.Hour}},
		{ClientWeb, false, SessionPolicy{12 * time.Hour, 7 * 24 * time.Hour}},
		{ClientMobile, true, SessionPolicy{time.Hour, 12 * time.Hour}},
		{ClientWeb, true, SessionPolicy{time.Hour, 12 * time.Hour}},
	}
	for _, tc := range cases {
		if got := PolicyFor(tc.c, tc.admin); got != tc.want {
			t.Errorf("PolicyFor(%s, %v) = %+v, want %+v", tc.c, tc.admin, got, tc.want)
		}
	}
}

func TestParseClientType(t *testing.T) {
	for _, ok := range []string{"MOBILE", "WEB"} {
		if _, err := ParseClientType(ok); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "web", "DESKTOP"} {
		if _, err := ParseClientType(bad); !errors.Is(err, ErrInvalidClientType) {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestSessionLifecycle(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	p := SessionPolicy{Idle: time.Hour, Absolute: 3 * time.Hour}
	s := NewSession(uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), ClientWeb, p, now)

	if !s.IsActive(now) || !s.IdleExpiresAt().Equal(now.Add(time.Hour)) || !s.AbsoluteExpiresAt().Equal(now.Add(3*time.Hour)) {
		t.Fatalf("new session: %+v", s)
	}
	if s.Touch(now.Add(time.Minute), p) {
		t.Fatal("touch within TouchInterval must be throttled")
	}
	t1 := now.Add(50 * time.Minute)
	if !s.Touch(t1, p) || !s.IdleExpiresAt().Equal(t1.Add(time.Hour)) || !s.LastSeenAt().Equal(t1) {
		t.Fatalf("touch did not slide idle expiry: %v", s.IdleExpiresAt())
	}
	// Sliding never passes the absolute expiry.
	t2 := now.Add(150 * time.Minute)
	s.Touch(t2, p)
	if !s.IdleExpiresAt().Equal(s.AbsoluteExpiresAt()) {
		t.Fatalf("idle %v beyond absolute %v", s.IdleExpiresAt(), s.AbsoluteExpiresAt())
	}
	if s.IsActive(now.Add(3 * time.Hour)) {
		t.Fatal("active at absolute expiry")
	}
	// Idle expiry.
	s2 := NewSession(uuid.New(), uuid.New(), ClientMobile, p, now)
	if s2.IsActive(now.Add(61 * time.Minute)) {
		t.Fatal("active after idle expiry")
	}
	// Idle longer than absolute is capped.
	s3 := NewSession(uuid.New(), uuid.New(), ClientMobile, SessionPolicy{Idle: 10 * time.Hour, Absolute: time.Hour}, now)
	if !s3.IdleExpiresAt().Equal(s3.AbsoluteExpiresAt()) {
		t.Fatal("idle not capped at creation")
	}
	r := RehydrateSession(s.ID(), s.UserID(), s.ClientType(), s.CreatedAt(), s.LastSeenAt(), s.IdleExpiresAt(), s.AbsoluteExpiresAt())
	if *r != *s {
		t.Fatal("rehydrate mismatch")
	}
}
