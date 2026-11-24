package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/authz"
)

func week(days ...domain.DayInput) []domain.DayInput { return days }

func day(weekday string, work []domain.RangeInput, breaks ...domain.RangeInput) domain.DayInput {
	return domain.DayInput{Weekday: weekday, WorkingHours: work, Breaks: breaks}
}

func r(start, end string) domain.RangeInput { return domain.RangeInput{Start: start, End: end} }

var mondayToFriday = week(
	day("MONDAY", []domain.RangeInput{r("09:00", "13:00"), r("14:00", "19:00")}, r("11:00", "11:15")),
	day("FRIDAY", []domain.RangeInput{r("09:00", "18:00")}, r("13:00", "14:00")),
)

func TestScheduleOwnerAndSelf(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	juan, _ := e.svc.Create(ctx, e.owner, e.biz, application.CreateInput{Email: "barbero@example.com", DisplayName: "Juan"})
	ana, _ := e.svc.Create(ctx, e.owner, e.biz, application.CreateInput{Email: "ana@example.com", DisplayName: "Ana"})
	barber := e.principal("barbero@example.com")

	// A new professional has no working hours.
	if w, err := e.svc.GetSchedule(ctx, barber, e.biz, juan.ID); err != nil || !w.Equal(domain.EmptyWeek()) {
		t.Fatalf("initial: %+v %v", w, err)
	}
	// The OWNER defines the barber's week.
	w, err := e.svc.ReplaceSchedule(ctx, e.owner, e.biz, juan.ID, mondayToFriday)
	if err != nil || len(w.Days[time.Monday].WorkingHours) != 2 || len(w.Days[time.Friday].Breaks) != 1 {
		t.Fatalf("owner replace: %+v %v", w, err)
	}
	last := e.store.Audit[len(e.store.Audit)-1]
	if last.Action != application.AuditScheduleReplaced || last.Metadata["workingDays"] != "MONDAY,FRIDAY" || last.Metadata["self"] != "false" {
		t.Errorf("audit = %+v", last)
	}
	// Identical schedule: no-op, not audited.
	before := len(e.store.Audit)
	if _, err := e.svc.ReplaceSchedule(ctx, e.owner, e.biz, juan.ID, mondayToFriday); err != nil || len(e.store.Audit) != before {
		t.Errorf("no-op: %v (%d audit entries added)", err, len(e.store.Audit)-before)
	}
	// The barber adjusts his own week (breaks included) and reads it back.
	own := week(day("TUESDAY", []domain.RangeInput{r("10:00", "20:00")}, r("13:00", "14:00"), r("17:00", "17:15")))
	if _, err := e.svc.ReplaceSchedule(ctx, barber, e.biz, juan.ID, own); err != nil {
		t.Fatalf("barber own schedule: %v", err)
	}
	got, err := e.svc.GetSchedule(ctx, barber, e.biz, juan.ID)
	if err != nil || len(got.Days[time.Monday].WorkingHours) != 0 || len(got.Days[time.Tuesday].Breaks) != 2 {
		t.Errorf("after barber replace: %+v %v", got, err)
	}
	if last := e.store.Audit[len(e.store.Audit)-1]; last.Metadata["self"] != "true" || last.Metadata["workingDays"] != "TUESDAY" {
		t.Errorf("self audit = %+v", last.Metadata)
	}
	// ... but not a colleague's, though he can read it.
	if _, err := e.svc.ReplaceSchedule(ctx, barber, e.biz, ana.ID, own); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("barber edits a colleague: %v", err)
	}
	if _, err := e.svc.GetSchedule(ctx, barber, e.biz, ana.ID); err != nil {
		t.Errorf("barber reads a colleague: %v", err)
	}
	// The OWNER who also serves edits its own schedule; PLATFORM_ADMIN edits any.
	if _, err := e.svc.ReplaceSchedule(ctx, e.owner, e.biz, ana.ID, mondayToFriday); err != nil {
		t.Errorf("owner own schedule: %v", err)
	}
	admin := identity.Principal{UserID: uuid.Must(uuid.NewV7()), IsPlatformAdmin: true}
	if _, err := e.svc.ReplaceSchedule(ctx, admin, e.biz, juan.ID, nil); err != nil {
		t.Errorf("admin clears: %v", err)
	}
	if got, _ := e.svc.GetSchedule(ctx, e.owner, e.biz, juan.ID); !got.Equal(domain.EmptyWeek()) {
		t.Errorf("cleared week = %+v", got)
	}
}

func TestScheduleRejected(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	juan, _ := e.svc.Create(ctx, e.owner, e.biz, application.CreateInput{Email: "barbero@example.com", DisplayName: "Juan"})
	audits := len(e.store.Audit)

	// Field paths and codes come from the domain.
	_, err := e.svc.ReplaceSchedule(ctx, e.owner, e.biz, juan.ID, week(day("MONDAY", []domain.RangeInput{r("09:00", "13:00")}, r("12:30", "13:30"))))
	if got := fieldCodes(t, err); got["days[0].breaks[0]"] != domain.CodeBreakOutside {
		t.Errorf("fields = %v", got)
	}
	// Unknown professional, another business, a stranger.
	if _, err := e.svc.ReplaceSchedule(ctx, e.owner, e.biz, uuid.Must(uuid.NewV7()), mondayToFriday); !errors.Is(err, application.ErrNotFound) {
		t.Errorf("unknown professional: %v", err)
	}
	if _, err := e.svc.GetSchedule(ctx, e.owner, e.biz, uuid.Must(uuid.NewV7())); !errors.Is(err, application.ErrNotFound) {
		t.Errorf("get unknown: %v", err)
	}
	stranger := e.principal("otro@example.com")
	if _, err := e.svc.GetSchedule(ctx, stranger, e.biz, juan.ID); !errors.Is(err, authz.ErrNotFound) {
		t.Errorf("stranger get: %v", err)
	}
	if _, err := e.svc.ReplaceSchedule(ctx, stranger, e.biz, juan.ID, mondayToFriday); !errors.Is(err, authz.ErrNotFound) {
		t.Errorf("stranger replace: %v", err)
	}
	other := uuid.Must(uuid.NewV7())
	e.identity.Grant(other, stranger.UserID, identity.RoleOwner)
	if _, err := e.svc.ReplaceSchedule(ctx, stranger, other, juan.ID, mondayToFriday); !errors.Is(err, application.ErrNotFound) {
		t.Errorf("professional through another business: %v", err)
	}
	// A barber removed from the business cannot touch his schedule any more.
	e.identity.Remove(e.biz, juan.UserID)
	if _, err := e.svc.ReplaceSchedule(ctx, e.principal("barbero@example.com"), e.biz, juan.ID, mondayToFriday); !errors.Is(err, authz.ErrNotFound) {
		t.Errorf("removed barber: %v", err)
	}
	if len(e.store.Audit) != audits {
		t.Errorf("rejected requests were audited as changes: %v", e.store.AuditActions()[audits:])
	}
}
