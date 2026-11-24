package application_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/subscriptions/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/subscriptions/application/subtest"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/subscriptions/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/authz"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
)

type adminOnly struct{}

func (adminOnly) RequirePlatformAdmin(_ context.Context, p contract.Principal) error {
	return contract.CheckPlatformAdmin(p)
}

var (
	admin   = contract.Principal{UserID: uuid.Must(uuid.NewV7()), IsPlatformAdmin: true}
	someone = contract.Principal{UserID: uuid.Must(uuid.NewV7())}
)

func newService() (*application.Service, *subtest.Store, *clock.Fixed) {
	st := subtest.NewStore()
	clk := clock.NewFixed(time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC))
	return application.NewService(st, adminOnly{}, clk), st, clk
}

func TestTrialStartAndExpiry(t *testing.T) {
	ctx := context.Background()
	svc, st, clk := newService()
	a, b := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	for _, id := range []uuid.UUID{a, b} {
		if err := svc.StartTrial(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	if ok, err := svc.Entitled(ctx, a); !ok || err != nil {
		t.Fatalf("entitled during the demo: %v %v", ok, err)
	}
	if ok, _ := svc.Entitled(ctx, uuid.Must(uuid.NewV7())); ok {
		t.Error("a business without subscription must not be entitled")
	}

	// Day 6: nothing expires. Extend b. Day 8: only a expires.
	clk.Advance(6 * 24 * time.Hour)
	if n, err := svc.ExpireDueTrials(ctx); n != 0 || err != nil {
		t.Fatalf("day 6 expired %d, %v", n, err)
	}
	if _, err := svc.ExtendTrial(ctx, admin, b, 5); err != nil {
		t.Fatal(err)
	}
	clk.Advance(2 * 24 * time.Hour)
	if ok, _ := svc.Entitled(ctx, a); ok {
		t.Error("a demo past its end must not be entitled before the job runs")
	}
	if n, err := svc.ExpireDueTrials(ctx); n != 1 || err != nil {
		t.Fatalf("day 8 expired %d, %v", n, err)
	}
	if s, _ := st.Get(a); s.Status != domain.StatusExpired || s.ExpiredAt == nil {
		t.Errorf("a = %+v", s)
	}
	if s, _ := st.Get(b); s.Status != domain.StatusTrialing {
		t.Errorf("extended b = %+v", s)
	}
	want := []string{
		application.AuditTrialStarted, application.AuditTrialStarted, application.AuditTrialExtended, application.AuditTrialExpired,
	}
	if got := st.AuditActions(); !reflect.DeepEqual(got, want) {
		t.Errorf("audit = %v, want %v", got, want)
	}
	if e := st.Audit[3]; e.ActorUserID != nil || *e.BusinessID != a || e.Metadata["source"] != "job" {
		t.Errorf("expiry audit = %+v", e)
	}

	// Reopen the expired demo, then courtesy activation (idempotent: audited once).
	if s, err := svc.ExtendTrial(ctx, admin, a, 3); err != nil || s.Status != domain.StatusTrialing {
		t.Errorf("reopen: %+v %v", s, err)
	}
	for i := 0; i < 2; i++ {
		if s, err := svc.ActivateCourtesy(ctx, admin, a); err != nil || s.Status != domain.StatusActive {
			t.Fatalf("courtesy: %+v %v", s, err)
		}
	}
	if n := len(st.AuditActions()); n != 6 {
		t.Errorf("audit entries = %d, want 6 (idempotent courtesy audited once)", n)
	}
	if _, err := svc.ExtendTrial(ctx, admin, a, 3); !errors.Is(err, domain.ErrNotExtendable) {
		t.Errorf("extend an ACTIVE subscription: %v", err)
	}
}

func TestAdminOperations(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newService()
	id := uuid.Must(uuid.NewV7())
	if err := svc.StartTrial(ctx, id); err != nil {
		t.Fatal(err)
	}
	if s, err := svc.Get(ctx, admin, id); err != nil || s.Status != domain.StatusTrialing {
		t.Errorf("get: %+v %v", s, err)
	}
	for name, call := range map[string]func(contract.Principal) error{
		"get":      func(p contract.Principal) error { _, err := svc.Get(ctx, p, id); return err },
		"extend":   func(p contract.Principal) error { _, err := svc.ExtendTrial(ctx, p, id, 3); return err },
		"courtesy": func(p contract.Principal) error { _, err := svc.ActivateCourtesy(ctx, p, id); return err },
	} {
		if err := call(someone); !errors.Is(err, authz.ErrForbidden) {
			t.Errorf("%s as non-admin: %v", name, err)
		}
	}
	for _, days := range []int{0, 91, -1} {
		if _, err := svc.ExtendTrial(ctx, admin, id, days); !errors.Is(err, domain.ErrInvalidExtension) {
			t.Errorf("extend %d days: %v", days, err)
		}
	}
	if _, err := svc.Get(ctx, admin, uuid.Must(uuid.NewV7())); !errors.Is(err, application.ErrNotFound) {
		t.Errorf("unknown: %v", err)
	}
}

func TestSummaryAndEntitledAmong(t *testing.T) {
	ctx := context.Background()
	svc, _, clk := newService()
	running, expired, none := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	for _, id := range []uuid.UUID{running, expired} {
		if err := svc.StartTrial(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	clk.Advance(3 * 24 * time.Hour)
	if _, err := svc.ExtendTrial(ctx, admin, running, 30); err != nil {
		t.Fatal(err)
	}
	clk.Advance(5 * 24 * time.Hour) // "expired" passed its 7 days; "running" was extended

	got, err := svc.EntitledAmong(ctx, []uuid.UUID{running, expired, none})
	if err != nil || len(got) != 1 || !got[running] {
		t.Errorf("EntitledAmong = %v, %v; want only the running demo", got, err)
	}
	if got, err := svc.EntitledAmong(ctx, nil); err != nil || len(got) != 0 {
		t.Errorf("empty batch: %v %v", got, err)
	}
	s, err := svc.SummaryOf(ctx, running)
	if err != nil || s.Status != "TRIALING" || !s.Entitled || s.TrialEndsAt == nil {
		t.Errorf("summary running = %+v %v", s, err)
	}
	if s, _ := svc.SummaryOf(ctx, expired); s.Entitled {
		t.Errorf("summary expired = %+v", s)
	}
	if s, err := svc.SummaryOf(ctx, none); err != nil || s.Status != "" || s.Entitled {
		t.Errorf("summary without subscription = %+v %v", s, err)
	}
}
