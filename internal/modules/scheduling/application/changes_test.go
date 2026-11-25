package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/authz"
)

type changeEnv struct {
	bookingEnv
	owner, juan, ana identity.Principal // staff: OWNER, Juan (e.pro), Ana (anaPro)
	anaPro           uuid.UUID
}

func newChangeEnv(t *testing.T) changeEnv {
	t.Helper()
	newP := func() identity.Principal { return identity.Principal{UserID: uuid.Must(uuid.NewV7())} }
	e := changeEnv{bookingEnv: newBookingEnv(t), owner: newP(), juan: newP(), ana: newP(), anaPro: uuid.Must(uuid.NewV7())}
	e.roles[[2]uuid.UUID{e.biz, e.owner.UserID}] = identity.RoleOwner
	e.roles[[2]uuid.UUID{e.biz, e.juan.UserID}] = identity.RoleProfessional
	e.roles[[2]uuid.UUID{e.biz, e.ana.UserID}] = identity.RoleProfessional
	e.ag.owners = map[uuid.UUID]uuid.UUID{e.pro: e.juan.UserID, e.anaPro: e.ana.UserID}
	e.ag.names = map[uuid.UUID]string{e.pro: "Juan", e.anaPro: "Ana"}
	return e
}

func (e changeEnv) at(t *testing.T, local time.Time) {
	t.Helper()
	e.clk.Advance(local.Sub(e.clk.Now()))
}

func TestCustomerChanges(t *testing.T) {
	ctx := context.Background()
	e := newChangeEnv(t)
	a, err := e.svc.Book(ctx, e.bookingEnv.ana, e.in(e.day(9, 0), "cliente-cambia-1"))
	if err != nil {
		t.Fatal(err)
	}
	customer := e.bookingEnv.ana
	c, err := e.svc.CustomerChange(ctx, customer, a.ID, application.ActionConfirm, "")
	if err != nil || c.Status != domain.StatusConfirmed || c.Version != 2 {
		t.Fatalf("confirm: %+v %v", c, err)
	}
	if c, err := e.svc.CustomerChange(ctx, customer, a.ID, application.ActionConfirm, ""); err != nil || c.Version != 2 {
		t.Errorf("confirm again (no change): %+v %v", c, err)
	}
	if _, err := e.svc.CustomerChange(ctx, e.bob, a.ID, application.ActionConfirm, ""); !errors.Is(err, application.ErrAppointmentNotFound) {
		t.Errorf("another customer: %v", err)
	}
	if _, err := e.svc.CustomerChange(ctx, customer, a.ID, application.ActionStart, ""); !errors.Is(err, application.ErrAppointmentNotFound) {
		t.Errorf("a customer cannot start: %v", err)
	}
	// Move 09:00 → 09:10: overlaps itself, allowed; the status does not change.
	m, err := e.svc.CustomerReschedule(ctx, customer, a.ID, e.day(9, 10))
	if err != nil || !m.Start.Equal(e.day(9, 10)) || m.Status != domain.StatusConfirmed {
		t.Fatalf("customer reschedule: %+v %v", m, err)
	}
	other, _ := e.svc.Book(ctx, e.bob, e.in(e.day(11, 0), "bob-cambia-1"))
	if _, err := e.svc.CustomerReschedule(ctx, customer, a.ID, e.day(10, 50)); !errors.Is(err, application.ErrSlotUnavailable) {
		t.Errorf("onto Bob's appointment: %v", err)
	}
	// Cut-off: 2 hours.
	e.at(t, e.day(7, 30))
	if _, err := e.svc.CustomerChange(ctx, customer, a.ID, application.ActionCancel, ""); !errors.Is(err, domain.ErrCutoffPassed) {
		t.Errorf("cancel inside the cut-off: %v", err)
	}
	if _, err := e.svc.CustomerReschedule(ctx, customer, a.ID, e.day(10, 0)); !errors.Is(err, domain.ErrCutoffPassed) {
		t.Errorf("reschedule inside the cut-off: %v", err)
	}
	if x, err := e.svc.CustomerChange(ctx, e.bob, other.ID, application.ActionCancel, "  viaje  "); err != nil ||
		x.Status != domain.StatusCancelled || x.CancelReason != "viaje" || x.CancelledBy != domain.ActorCustomer {
		t.Errorf("Bob cancels in time: %+v %v", x, err)
	}
	if x, err := e.svc.CustomerChange(ctx, e.bob, other.ID, application.ActionCancel, ""); err != nil || x.Status != domain.StatusCancelled {
		t.Errorf("cancel again (no change): %v", err)
	}
}

func TestStaffChanges(t *testing.T) {
	ctx := context.Background()
	e := newChangeEnv(t)
	walkIn := func(start time.Time, key string, pro uuid.UUID, who identity.Principal) domain.Appointment {
		t.Helper()
		a, err := e.svc.BookManual(ctx, who, e.biz, application.ManualInput{ProfessionalID: pro, ServiceID: e.svcID, Start: start, IdempotencyKey: key,
			Customer: &application.CustomerInput{FullName: "Carlos", Phone: "3005550101"}})
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	a := walkIn(e.day(9, 0), "personal-cambia-1", e.pro, e.juan)
	app, err := e.svc.Book(ctx, e.bookingEnv.ana, e.in(e.day(10, 0), "personal-cambia-2"))
	if err != nil {
		t.Fatal(err)
	}

	var verr *application.ValidationError
	if _, err := e.svc.StaffChange(ctx, e.juan, e.biz, a.ID, application.ActionCancel, " "); !errors.As(err, &verr) {
		t.Errorf("staff cancel without reason: %v", err)
	}
	if _, err := e.svc.StaffChange(ctx, e.ana, e.biz, a.ID, application.ActionConfirm, ""); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("Ana on Juan's agenda: %v", err)
	}
	if _, err := e.svc.StaffChange(ctx, e.juan, uuid.Must(uuid.NewV7()), a.ID, application.ActionConfirm, ""); !errors.Is(err, authz.ErrNotFound) {
		t.Errorf("another business: %v", err)
	}
	// Juan moves his walk-in (no account): stays CONFIRMED. The app customer's: PENDING again.
	if m, err := e.svc.StaffReschedule(ctx, e.juan, e.biz, a.ID, e.day(11, 0), uuid.Nil); err != nil || m.Status != domain.StatusConfirmed {
		t.Errorf("move a walk-in: %+v %v", m, err)
	}
	if _, err := e.svc.CustomerChange(ctx, e.bookingEnv.ana, app.ID, application.ActionConfirm, ""); err != nil {
		t.Fatal(err)
	}
	if m, err := e.svc.StaffReschedule(ctx, e.juan, e.biz, app.ID, e.day(10, 20), uuid.Nil); err != nil || m.Status != domain.StatusPendingConfirmation {
		t.Errorf("move an app customer's: %+v %v", m, err)
	}
	// Only the OWNER reassigns to another professional.
	if _, err := e.svc.StaffReschedule(ctx, e.juan, e.biz, app.ID, e.day(10, 20), e.anaPro); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("barber reassigns: %v", err)
	}
	r, err := e.svc.StaffReschedule(ctx, e.owner, e.biz, app.ID, e.day(10, 20), e.anaPro)
	if err != nil || r.ProfessionalID != e.anaPro {
		t.Fatalf("owner reassigns: %+v %v", r, err)
	}
	h, err := e.svc.History(ctx, e.owner, e.biz, app.ID)
	if err != nil || len(h) != 4 || h[0].Type != domain.EventCreated || h[1].Type != domain.EventConfirmed ||
		h[2].Type != domain.EventRescheduled || !h[2].PreviousStart.Equal(e.day(10, 0)) ||
		h[3].Type != domain.EventReassigned || h[3].PreviousProfessionalID != e.pro || h[3].ActorRole != domain.ActorBusiness {
		t.Errorf("history: %+v %v", h, err)
	}
	if _, err := e.svc.History(ctx, e.juan, e.biz, app.ID); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("Juan reads an appointment now in Ana's agenda: %v", err)
	}

	// The day: start too early, start, complete; a no-show frees the slot at once.
	b := walkIn(e.day(9, 0), "personal-cambia-3", e.pro, e.juan)
	e.at(t, e.day(8, 40))
	if _, err := e.svc.StaffChange(ctx, e.juan, e.biz, b.ID, application.ActionStart, ""); !errors.Is(err, domain.ErrTooEarly) {
		t.Errorf("start 20 min early: %v", err)
	}
	e.at(t, e.day(9, 5))
	if _, err := e.svc.StaffChange(ctx, e.juan, e.biz, b.ID, application.ActionNoShow, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.StaffReschedule(ctx, e.juan, e.biz, b.ID, e.day(12, 0), uuid.Nil); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("move a no-show: %v", err)
	}
	if c, err := e.svc.BookManual(ctx, e.juan, e.biz, application.ManualInput{ProfessionalID: e.pro, ServiceID: e.svcID, Start: e.day(9, 10), Confirmed: true,
		IdempotencyKey: "personal-cambia-4", Customer: &application.CustomerInput{FullName: "Espera", Phone: "3005550202"}}); err != nil || c.Status != domain.StatusConfirmed {
		t.Fatalf("booking the freed slot: %v", err)
	} else {
		if _, err := e.svc.StaffChange(ctx, e.juan, e.biz, c.ID, application.ActionStart, ""); err != nil {
			t.Errorf("start: %v", err)
		}
		if x, err := e.svc.StaffChange(ctx, e.juan, e.biz, c.ID, application.ActionComplete, ""); err != nil || x.Status != domain.StatusCompleted {
			t.Errorf("complete: %v", err)
		}
		if _, err := e.svc.StaffChange(ctx, e.juan, e.biz, c.ID, application.ActionCancel, "tarde"); !errors.Is(err, domain.ErrInvalidTransition) {
			t.Errorf("cancel a completed one: %v", err)
		}
	}
	// The 11:00 walk-in has not started yet at 09:05: the owner can still cancel it.
	if _, err := e.svc.StaffChange(ctx, e.owner, e.biz, a.ID, application.ActionCancel, "cierre"); err != nil {
		t.Errorf("cancel the 11:00 at 09:05: %v", err)
	}
}
