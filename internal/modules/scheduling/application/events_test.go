package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/authz"
)

type listener struct {
	events []contract.AppointmentEvent
	err    error
}

func (l *listener) AppointmentEventRecorded(_ context.Context, e contract.AppointmentEvent) error {
	if l.err != nil {
		return l.err
	}
	l.events = append(l.events, e)
	return nil
}

// TestEveryChangeTellsTheListener (T7.1): each recorded change reaches the listener inside its
// transaction, with what the notifications need; a failing listener aborts the change.
func TestEveryChangeTellsTheListener(t *testing.T) {
	ctx := context.Background()
	e := newChangeEnv(t)
	l := &listener{}
	e.svc.SetEventListener(l)

	a, err := e.svc.Book(ctx, e.bookingEnv.ana, e.in(e.day(9, 0), "eventos-ana-1"))
	if err != nil {
		t.Fatal(err)
	}
	if len(l.events) != 1 {
		t.Fatalf("after booking: %+v", l.events)
	}
	got := l.events[0]
	if got.Type != contract.EventCreated || got.ActorRole != contract.ActorCustomer || got.AppointmentID != a.ID || got.BusinessID != e.biz ||
		got.ProfessionalID != e.pro || !got.Start.Equal(a.Start) || got.CustomerPhone != "+573001112233" || !got.CustomerHasAccount ||
		got.ID == uuid.Nil || !got.At.Equal(e.clk.Now()) {
		t.Errorf("created: %+v", got)
	}

	m, err := e.svc.BookManual(ctx, e.owner, e.biz, application.ManualInput{
		ProfessionalID: e.pro, ServiceID: e.svcID, Start: e.day(10, 0), Confirmed: true, IdempotencyKey: "eventos-manual-1",
		Customer: &application.CustomerInput{FullName: "Laura", Phone: "3005550199"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := l.events[1]; got.Type != contract.EventCreated || got.ActorRole != contract.ActorBusiness || got.CustomerHasAccount ||
		got.CustomerPhone != "+573005550199" || got.AppointmentID != m.ID {
		t.Errorf("manual: %+v", got)
	}

	if _, err := e.svc.StaffReschedule(ctx, e.owner, e.biz, a.ID, e.day(11, 0), e.anaPro); err != nil {
		t.Fatal(err)
	}
	if got := l.events[2]; got.Type != contract.EventReassigned || got.ProfessionalID != e.anaPro || !got.Start.Equal(e.day(11, 0)) {
		t.Errorf("reassigned: %+v", got)
	}
	if _, err := e.svc.StaffChange(ctx, e.owner, e.biz, m.ID, application.ActionCancel, "Cierre"); err != nil {
		t.Fatal(err)
	}
	if got := l.events[3]; got.Type != contract.EventCancelled || got.ActorRole != contract.ActorBusiness {
		t.Errorf("canceled: %+v", got)
	}

	// A failing listener rolls the change back.
	l.err = errors.New("queue down")
	if _, err := e.svc.Book(ctx, e.bob, e.in(e.day(9, 40), "eventos-bob-1")); !errors.Is(err, l.err) {
		t.Fatalf("booking with a failing listener: %v", err)
	}
	for _, x := range e.store.Appointments {
		if x.Customer.UserID == e.bob.UserID {
			t.Error("the booking must be rolled back")
		}
	}
}

func TestAppointmentsContract(t *testing.T) {
	ctx := context.Background()
	e := newChangeEnv(t)
	a, err := e.svc.Book(ctx, e.bookingEnv.ana, e.in(e.day(9, 0), "contrato-ana-1"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := e.svc.AppointmentState(ctx, e.biz, a.ID)
	if err != nil || st.ID != a.ID || st.BusinessID != e.biz || st.ProfessionalID != e.pro || st.Status != "PENDING_CONFIRMATION" ||
		!st.Start.Equal(a.Start) || st.Version != 1 || st.CustomerName != "Ana Cliente" || st.ServiceName != "Corte" ||
		st.BusinessName != "Barbería Centro" || st.TimeZone != "America/Bogota" || st.ProfessionalName != "Juan" || st.CancelReason != "" {
		t.Fatalf("state: %+v %v", st, err)
	}
	if _, err := e.svc.AppointmentState(ctx, uuid.Must(uuid.NewV7()), a.ID); !errors.Is(err, contract.ErrAppointmentNotFound) {
		t.Errorf("another business: %v", err)
	}
	if _, err := e.svc.AppointmentState(ctx, e.biz, uuid.Must(uuid.NewV7())); !errors.Is(err, contract.ErrAppointmentNotFound) {
		t.Errorf("unknown: %v", err)
	}

	if err := e.svc.AuthorizeAppointmentView(ctx, e.owner, e.biz, a.ID); err != nil {
		t.Errorf("owner: %v", err)
	}
	if err := e.svc.AuthorizeAppointmentView(ctx, e.juan, e.biz, a.ID); err != nil {
		t.Errorf("its professional: %v", err)
	}
	if err := e.svc.AuthorizeAppointmentView(ctx, e.ana, e.biz, a.ID); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("another professional: %v", err)
	}
	if err := e.svc.AuthorizeAppointmentView(ctx, e.bob, e.biz, a.ID); err == nil {
		t.Error("not a member")
	}
}

// TestRepliesAsCustomer (T7.5): the customer's answer from WhatsApp, with the customer's rules.
func TestRepliesAsCustomer(t *testing.T) {
	ctx := context.Background()
	e := newChangeEnv(t)
	a, err := e.svc.Book(ctx, e.bookingEnv.ana, e.in(e.day(9, 0), "respuesta-ana-1"))
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.ConfirmAsCustomer(ctx, uuid.Must(uuid.NewV7()), a.ID); !errors.Is(err, contract.ErrAppointmentNotFound) {
		t.Errorf("another business: %v", err)
	}
	if err := e.svc.ConfirmAsCustomer(ctx, e.biz, a.ID); err != nil {
		t.Fatal(err)
	}
	st, _ := e.svc.AppointmentState(ctx, e.biz, a.ID)
	if st.Status != "CONFIRMED" || st.Version != 2 {
		t.Errorf("confirmed: %+v", st)
	}
	last := e.store.Events[len(e.store.Events)-1]
	if last.Type != "CONFIRMED" || last.Event.ActorRole != "CUSTOMER" || last.Actor != e.bookingEnv.ana.UserID {
		t.Errorf("event %+v", last)
	}
	if err := e.svc.ConfirmAsCustomer(ctx, e.biz, a.ID); err != nil {
		t.Errorf("confirm again: %v", err)
	}
	// Inside the 2-hour cut-off the customer cannot cancel.
	e.at(t, e.day(7, 30))
	if err := e.svc.CancelAsCustomer(ctx, e.biz, a.ID); !errors.Is(err, contract.ErrCutoffPassed) {
		t.Errorf("inside the cut-off: %v", err)
	}
	e.at(t, e.day(6, 0))
	if err := e.svc.CancelAsCustomer(ctx, e.biz, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.ConfirmAsCustomer(ctx, e.biz, a.ID); !errors.Is(err, contract.ErrNotActive) {
		t.Errorf("confirm a canceled one: %v", err)
	}
}
