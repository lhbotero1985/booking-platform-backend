package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/domain"
	scheduling "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/contract"
)

// fakeGuard reports an appointment in [busyFrom, busyTo) and, for schedules, conflicts unless
// Tuesday keeps 09:00–12:00 working.
type fakeGuard struct {
	busyFrom, busyTo time.Time
	calls            int
}

func (g *fakeGuard) AppointmentsDuring(_ context.Context, _, _ uuid.UUID, from, to time.Time) ([]scheduling.ConflictingAppointment, error) {
	g.calls++
	if from.Before(g.busyTo) && g.busyFrom.Before(to) {
		return []scheduling.ConflictingAppointment{{ID: uuid.Must(uuid.NewV7()), Start: g.busyFrom, End: g.busyTo, CustomerName: "Laura", Status: "CONFIRMED"}}, nil
	}
	return nil, nil
}

func (g *fakeGuard) AppointmentsOutside(_ context.Context, _, _ uuid.UUID, week [7]scheduling.Day) ([]scheduling.ConflictingAppointment, error) {
	g.calls++
	for _, r := range week[time.Tuesday].WorkingHours {
		if r.Start <= 540 && 720 <= r.End {
			return nil, nil
		}
	}
	return []scheduling.ConflictingAppointment{{ID: uuid.Must(uuid.NewV7()), CustomerName: "Laura", Status: "CONFIRMED"}}, nil
}

// TestAgendaChangesAgainstAppointments (T6.7, P11): absences and schedules that would break
// active appointments are refused with the list; the others go through.
func TestAgendaChangesAgainstAppointments(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	juan, _ := e.svc.Create(ctx, e.owner, e.biz, application.CreateInput{Email: "barbero@example.com", DisplayName: "Juan"})
	guard := &fakeGuard{busyFrom: at(10, 14), busyTo: at(10, 15)} // an appointment on 10 December, 14:00–15:00 UTC
	e.svc.SetAgendaGuard(guard)

	var conflict *application.ConflictError
	audited := len(e.store.Audit)
	if _, err := e.svc.CreateAbsence(ctx, e.owner, e.biz, juan.ID, application.AbsenceInput{Start: at(10, 13), End: at(10, 18), Reason: "Médico"}); !errors.As(err, &conflict) ||
		len(conflict.Appointments) != 1 || conflict.Appointments[0].CustomerName != "Laura" {
		t.Fatalf("absence over an appointment: %v", err)
	}
	if n := len(e.store.Audit); n != audited {
		t.Errorf("a refused absence was audited: %d → %d", audited, n)
	}
	free, err := e.svc.CreateAbsence(ctx, e.owner, e.biz, juan.ID, application.AbsenceInput{Start: at(10, 15), End: at(10, 18)})
	if err != nil {
		t.Fatalf("absence right after the appointment: %v", err)
	}
	start := at(10, 12)
	if _, err := e.svc.UpdateAbsence(ctx, e.owner, e.biz, juan.ID, free.ID, application.AbsencePatch{Start: &start}); !errors.As(err, &conflict) {
		t.Errorf("stretching the absence over the appointment: %v", err)
	}
	reason := "Trámite"
	calls := guard.calls
	if _, err := e.svc.UpdateAbsence(ctx, e.owner, e.biz, juan.ID, free.ID, application.AbsencePatch{Reason: &reason}); err != nil || guard.calls != calls+1 {
		t.Errorf("changing only the reason: %v (guard calls %d → %d)", err, calls, guard.calls)
	}

	// Weekly schedule: dropping Tuesday 09:00–12:00 breaks the appointment; keeping it works.
	if _, err := e.svc.ReplaceSchedule(ctx, e.owner, e.biz, juan.ID, week(day("MONDAY", []domain.RangeInput{r("09:00", "18:00")}))); !errors.As(err, &conflict) {
		t.Errorf("schedule without Tuesday: %v", err)
	}
	if _, err := e.svc.ReplaceSchedule(ctx, e.owner, e.biz, juan.ID, week(day("TUESDAY", []domain.RangeInput{r("08:00", "13:00")}))); err != nil {
		t.Errorf("schedule keeping Tuesday morning: %v", err)
	}
}
