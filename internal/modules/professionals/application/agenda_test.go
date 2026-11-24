package application_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/application"
	profcontract "github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/contract"
)

// TestDayAgenda (T6.2): the agenda the availability engine reads, without authorization.
func TestDayAgenda(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	juan, _ := e.svc.Create(ctx, e.owner, e.biz, application.CreateInput{Email: "barbero@example.com", DisplayName: "Juan"})
	if _, err := e.svc.ReplaceSchedule(ctx, e.owner, e.biz, juan.ID, mondayToFriday); err != nil {
		t.Fatal(err)
	}
	for _, p := range [][2]time.Time{{at(7, 10), at(7, 12)}, {at(8, 0), at(9, 0)}, {at(20, 0), at(21, 0)}} {
		if _, err := e.svc.CreateAbsence(ctx, e.owner, e.biz, juan.ID, application.AbsenceInput{Start: p[0], End: p[1]}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := e.svc.DayAgenda(ctx, e.biz, juan.ID, time.Monday, at(7, 0), at(8, 6))
	if err != nil {
		t.Fatal(err)
	}
	want := profcontract.DayAgenda{
		WorkingHours: []profcontract.MinuteRange{{Start: 540, End: 780}, {Start: 840, End: 1140}},
		Breaks:       []profcontract.MinuteRange{{Start: 660, End: 675}},
		Absences:     []profcontract.Interval{{Start: at(7, 10), End: at(7, 12)}, {Start: at(8, 0), End: at(9, 0)}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("agenda = %+v", got)
	}
	if got, err := e.svc.DayAgenda(ctx, e.biz, juan.ID, time.Sunday, at(1, 0), at(2, 0)); err != nil || len(got.WorkingHours)+len(got.Breaks)+len(got.Absences) != 0 {
		t.Errorf("day off: %+v %v", got, err)
	}
	if _, err := e.svc.DayAgenda(ctx, uuid.Must(uuid.NewV7()), juan.ID, time.Monday, at(7, 0), at(8, 0)); !errors.Is(err, profcontract.ErrProfessionalNotFound) {
		t.Errorf("another business: %v", err)
	}
}
