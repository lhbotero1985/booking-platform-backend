package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/authz"
)

// TestAgendaGuard (T6.7): which appointments an absence or a schedule would break.
func TestAgendaGuard(t *testing.T) {
	ctx := context.Background()
	e := newChangeEnv(t)
	a, err := e.svc.Book(ctx, e.bookingEnv.ana, e.in(e.day(9, 0), "guarda-0900")) // Tuesday 09:00–09:30 Bogota
	if err != nil {
		t.Fatal(err)
	}
	if got, err := e.svc.AppointmentsDuring(ctx, e.biz, e.pro, e.day(9, 15), e.day(10, 0)); err != nil || len(got) != 1 || got[0].ID != a.ID || got[0].CustomerName != "Ana Cliente" {
		t.Errorf("absence over it: %+v %v", got, err)
	}
	if got, _ := e.svc.AppointmentsDuring(ctx, e.biz, e.pro, e.day(9, 30), e.day(10, 0)); len(got) != 0 {
		t.Errorf("absence right after: %+v", got)
	}
	tuesday := func(work []contract.Range, breaks ...contract.Range) [7]contract.Day {
		var w [7]contract.Day
		w[2] = contract.Day{WorkingHours: work, Breaks: breaks}
		return w
	}
	for name, c := range map[string]struct {
		week [7]contract.Day
		want int
	}{
		"same hours":            {tuesday([]contract.Range{{Start: 540, End: 720}}), 0},
		"starts at 09:30":       {tuesday([]contract.Range{{Start: 570, End: 720}}), 1},
		"ends exactly at 09:30": {tuesday([]contract.Range{{Start: 480, End: 570}}), 0},
		"break 09:15–09:45":     {tuesday([]contract.Range{{Start: 480, End: 720}}, contract.Range{Start: 555, End: 585}), 1},
		"break right after":     {tuesday([]contract.Range{{Start: 480, End: 720}}, contract.Range{Start: 570, End: 600}), 0},
		"Tuesday off":           {[7]contract.Day{}, 1},
		"split range at 09:15":  {tuesday([]contract.Range{{Start: 480, End: 555}, {Start: 555, End: 720}}), 1},
	} {
		got, err := e.svc.AppointmentsOutside(ctx, e.biz, e.pro, c.week)
		if err != nil || len(got) != c.want {
			t.Errorf("%s: %d %v", name, len(got), err)
		}
	}
	// A canceled appointment no longer counts.
	if _, err := e.svc.CustomerChange(ctx, e.bookingEnv.ana, a.ID, application.ActionCancel, ""); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.svc.AppointmentsOutside(ctx, e.biz, e.pro, [7]contract.Day{}); len(got) != 0 {
		t.Errorf("after canceling: %+v", got)
	}
}

// TestToReassign (T6.7): appointments of a professional who is no longer bookable.
func TestToReassign(t *testing.T) {
	ctx := context.Background()
	e := newChangeEnv(t)
	a, err := e.svc.Book(ctx, e.bookingEnv.ana, e.in(e.day(9, 0), "reasignar-0900"))
	if err != nil {
		t.Fatal(err)
	}
	if page, err := e.svc.ToReassign(ctx, e.owner, e.biz); err != nil || len(page.Items) != 0 {
		t.Fatalf("nothing to reassign yet: %+v %v", page.Items, err)
	}
	e.ag.gone = map[uuid.UUID]bool{e.pro: true} // Juan left the business
	page, err := e.svc.ToReassign(ctx, e.owner, e.biz)
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != a.ID || !page.Items[0].NeedsReassignment || page.Items[0].ProfessionalName != "Juan" {
		t.Fatalf("to reassign: %+v %v", page.Items, err)
	}
	tuesday, _ := domain.ParseLocalDate("2030-03-05")
	if ag, err := e.svc.Agenda(ctx, e.owner, e.biz, application.AgendaQuery{Date: tuesday}); err != nil || len(ag.Items) != 1 || !ag.Items[0].NeedsReassignment {
		t.Errorf("agenda flag: %+v %v", ag.Items, err)
	}
	if _, err := e.svc.ToReassign(ctx, e.ana, e.biz); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("a professional asks: %v", err)
	}
	if _, err := e.svc.ToReassign(ctx, identity.Principal{UserID: uuid.Must(uuid.NewV7())}, e.biz); !errors.Is(err, authz.ErrNotFound) {
		t.Errorf("an outsider asks: %v", err)
	}
	// The owner moves it to Ana: nothing left.
	if _, err := e.svc.StaffReschedule(ctx, e.owner, e.biz, a.ID, e.day(9, 0), e.anaPro); err != nil {
		t.Fatal(err)
	}
	if page, _ := e.svc.ToReassign(ctx, e.owner, e.biz); len(page.Items) != 0 {
		t.Errorf("after reassigning: %+v", page.Items)
	}
}
