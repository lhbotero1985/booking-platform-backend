package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/authz"
)

// The env clock is 2026-11-24 12:00 UTC.
func at(day, hour int) time.Time { return time.Date(2026, 12, day, hour, 0, 0, 0, time.UTC) }

func TestAbsencesOwnerAndSelf(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	juan, _ := e.svc.Create(ctx, e.owner, e.biz, application.CreateInput{Email: "barbero@example.com", DisplayName: "Juan"})
	ana, _ := e.svc.Create(ctx, e.owner, e.biz, application.CreateInput{Email: "ana@example.com", DisplayName: "Ana"})
	barber := e.principal("barbero@example.com")

	// The OWNER registers vacation for the barber.
	vac, err := e.svc.CreateAbsence(ctx, e.owner, e.biz, juan.ID, application.AbsenceInput{Start: at(20, 0), End: at(27, 0), Reason: " Vacaciones "})
	if err != nil || vac.Reason != "Vacaciones" || vac.CreatedBy != e.owner.UserID {
		t.Fatalf("owner creates: %+v %v", vac, err)
	}
	// The barber registers his own sick leave, with a local offset; stored in UTC.
	cot := time.FixedZone("COT", -5*3600)
	sick, err := e.svc.CreateAbsence(ctx, barber, e.biz, juan.ID, application.AbsenceInput{
		Start: time.Date(2026, 12, 2, 8, 0, 0, 0, cot), End: time.Date(2026, 12, 3, 19, 0, 0, 0, cot), Reason: "Incapacidad",
	})
	if err != nil || sick.Start != at(2, 13) || sick.Start.Location() != time.UTC {
		t.Fatalf("barber creates: %+v %v", sick, err)
	}
	// Overlaps are rejected; touching periods are fine.
	if _, err := e.svc.CreateAbsence(ctx, barber, e.biz, juan.ID, application.AbsenceInput{Start: at(26, 0), End: at(28, 0)}); !errors.Is(err, application.ErrAbsenceOverlap) {
		t.Errorf("overlap: %v", err)
	}
	if _, err := e.svc.CreateAbsence(ctx, barber, e.biz, juan.ID, application.AbsenceInput{Start: at(27, 0), End: at(28, 0)}); err != nil {
		t.Errorf("touching: %v", err)
	}
	// Another professional may be absent at the same time.
	if _, err := e.svc.CreateAbsence(ctx, e.owner, e.biz, ana.ID, application.AbsenceInput{Start: at(20, 0), End: at(27, 0)}); err != nil {
		t.Errorf("owner's own absence: %v", err)
	}
	// The barber cannot touch a colleague's absences, but sees them.
	if _, err := e.svc.CreateAbsence(ctx, barber, e.biz, ana.ID, application.AbsenceInput{Start: at(5, 0), End: at(6, 0)}); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("barber creates for colleague: %v", err)
	}
	list, err := e.svc.ListAbsences(ctx, barber, e.biz, ana.ID, nil, nil)
	if err != nil || len(list) != 1 {
		t.Errorf("barber lists colleague: %v %v", list, err)
	}
	// Listing: from now by default, ordered by start, window filter.
	list, _ = e.svc.ListAbsences(ctx, barber, e.biz, juan.ID, nil, nil)
	if len(list) != 3 || list[0].ID != sick.ID {
		t.Errorf("list = %+v", list)
	}
	from, to := at(21, 0), at(22, 0)
	if list, _ := e.svc.ListAbsences(ctx, e.owner, e.biz, juan.ID, &from, &to); len(list) != 1 || list[0].ID != vac.ID {
		t.Errorf("window = %+v", list)
	}
	if _, err := e.svc.ListAbsences(ctx, e.owner, e.biz, juan.ID, &to, &from); fieldCodes(t, err)["to"] != application.CodeInvalidWindow {
		t.Errorf("inverted window: %v", err)
	}

	// The barber shortens the vacation registered by the OWNER (it is his agenda).
	end := at(25, 0)
	upd, err := e.svc.UpdateAbsence(ctx, barber, e.biz, juan.ID, vac.ID, application.AbsencePatch{End: &end})
	if err != nil || upd.End != end {
		t.Fatalf("barber updates: %+v %v", upd, err)
	}
	if last := e.store.Audit[len(e.store.Audit)-1]; last.Action != application.AuditAbsenceUpdated || last.Metadata["fields"] != "end" || last.Metadata["self"] != "true" {
		t.Errorf("update audit = %+v", last)
	}
	before := len(e.store.Audit)
	if _, err := e.svc.UpdateAbsence(ctx, barber, e.biz, juan.ID, vac.ID, application.AbsencePatch{End: &end}); err != nil || len(e.store.Audit) != before {
		t.Errorf("no-op update audited: %v", err)
	}
	// Extending it onto the sick leave overlaps.
	early := at(1, 0)
	if _, err := e.svc.UpdateAbsence(ctx, e.owner, e.biz, juan.ID, vac.ID, application.AbsencePatch{Start: &early}); !errors.Is(err, application.ErrAbsenceOverlap) {
		t.Errorf("update overlap: %v", err)
	}
	if err := e.svc.DeleteAbsence(ctx, barber, e.biz, juan.ID, sick.ID); err != nil {
		t.Errorf("barber deletes own: %v", err)
	}
	if err := e.svc.DeleteAbsence(ctx, barber, e.biz, juan.ID, sick.ID); !errors.Is(err, application.ErrNotFound) {
		t.Errorf("delete twice: %v", err)
	}
	// PLATFORM_ADMIN manages any.
	admin := identity.Principal{UserID: uuid.Must(uuid.NewV7()), IsPlatformAdmin: true}
	if err := e.svc.DeleteAbsence(ctx, admin, e.biz, juan.ID, vac.ID); err != nil {
		t.Errorf("admin deletes: %v", err)
	}
	created, deleted := 0, 0
	for _, a := range e.store.AuditActions() {
		switch a {
		case application.AuditAbsenceCreated:
			created++
		case application.AuditAbsenceDeleted:
			deleted++
		}
	}
	if created != 4 || deleted != 2 {
		t.Errorf("audited created=%d deleted=%d", created, deleted)
	}
}

func TestAbsencesRejected(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	juan, _ := e.svc.Create(ctx, e.owner, e.biz, application.CreateInput{Email: "barbero@example.com", DisplayName: "Juan"})
	audits := len(e.store.Audit)

	for name, tc := range map[string]struct {
		in   application.AbsenceInput
		want map[string]string
	}{
		"missing":  {application.AbsenceInput{}, map[string]string{"start": application.CodeRequired, "end": application.CodeRequired}},
		"inverted": {application.AbsenceInput{Start: at(5, 0), End: at(4, 0)}, map[string]string{"end": application.CodeInvalidPeriod}},
		"too long": {application.AbsenceInput{Start: at(1, 0), End: at(1, 0).AddDate(1, 1, 0)}, map[string]string{"end": application.CodePeriodTooLong}},
		"reason":   {application.AbsenceInput{Start: at(1, 0), End: at(2, 0), Reason: "a\nb"}, map[string]string{"reason": application.CodeInvalidReason}},
	} {
		_, err := e.svc.CreateAbsence(ctx, e.owner, e.biz, juan.ID, tc.in)
		got := fieldCodes(t, err)
		for f, c := range tc.want {
			if got[f] != c {
				t.Errorf("%s: fields %v, want %v", name, got, tc.want)
			}
		}
	}
	bad := "a\x00"
	if _, err := e.svc.UpdateAbsence(ctx, e.owner, e.biz, juan.ID, uuid.Must(uuid.NewV7()), application.AbsencePatch{Reason: &bad}); fieldCodes(t, err)["reason"] != application.CodeInvalidReason {
		t.Errorf("update reason: %v", err)
	}
	a, err := e.svc.CreateAbsence(ctx, e.owner, e.biz, juan.ID, application.AbsenceInput{Start: at(1, 0), End: at(2, 0)})
	if err != nil {
		t.Fatal(err)
	}
	inverted := at(1, 0).Add(-time.Hour)
	if _, err := e.svc.UpdateAbsence(ctx, e.owner, e.biz, juan.ID, a.ID, application.AbsencePatch{End: &inverted}); fieldCodes(t, err)["end"] != application.CodeInvalidPeriod {
		t.Errorf("update inverted: %v", err)
	}
	// Unknown ids, strangers, other businesses.
	if _, err := e.svc.UpdateAbsence(ctx, e.owner, e.biz, juan.ID, uuid.Must(uuid.NewV7()), application.AbsencePatch{}); !errors.Is(err, application.ErrNotFound) {
		t.Errorf("unknown absence: %v", err)
	}
	if _, err := e.svc.ListAbsences(ctx, e.owner, e.biz, uuid.Must(uuid.NewV7()), nil, nil); !errors.Is(err, application.ErrNotFound) {
		t.Errorf("unknown professional: %v", err)
	}
	stranger := e.principal("otro@example.com")
	if _, err := e.svc.ListAbsences(ctx, stranger, e.biz, juan.ID, nil, nil); !errors.Is(err, authz.ErrNotFound) {
		t.Errorf("stranger list: %v", err)
	}
	if err := e.svc.DeleteAbsence(ctx, stranger, e.biz, juan.ID, a.ID); !errors.Is(err, authz.ErrNotFound) {
		t.Errorf("stranger delete: %v", err)
	}
	other := uuid.Must(uuid.NewV7())
	e.identity.Grant(other, stranger.UserID, identity.RoleOwner)
	if err := e.svc.DeleteAbsence(ctx, stranger, other, juan.ID, a.ID); !errors.Is(err, application.ErrNotFound) {
		t.Errorf("through another business: %v", err)
	}
	if _, err := e.svc.CreateAbsence(ctx, stranger, other, juan.ID, application.AbsenceInput{Start: at(9, 0), End: at(10, 0)}); !errors.Is(err, application.ErrNotFound) {
		t.Errorf("create through another business: %v", err)
	}
	// A barber removed from the business loses access to his absences.
	e.identity.Remove(e.biz, juan.UserID)
	if _, err := e.svc.CreateAbsence(ctx, e.principal("barbero@example.com"), e.biz, juan.ID, application.AbsenceInput{Start: at(9, 0), End: at(10, 0)}); !errors.Is(err, authz.ErrNotFound) {
		t.Errorf("removed barber: %v", err)
	}
	if got := e.store.AuditActions()[audits:]; len(got) != 1 {
		t.Errorf("only the valid absence is audited: %v", got)
	}
}
