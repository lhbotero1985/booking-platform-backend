package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/application/notiftest"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/domain"
	scheduling "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
)

type appointments struct {
	state   scheduling.AppointmentState
	err     error
	authErr error
}

func (a *appointments) AppointmentState(context.Context, uuid.UUID, uuid.UUID) (scheduling.AppointmentState, error) {
	return a.state, a.err
}

func (a *appointments) AuthorizeAppointmentView(context.Context, identity.Principal, uuid.UUID, uuid.UUID) error {
	return a.authErr
}

type sender struct {
	sent []application.Outgoing
	err  error
}

func (s *sender) Send(_ context.Context, m application.Outgoing) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	s.sent = append(s.sent, m)
	return "wamid-1", nil
}

var (
	now   = time.Date(2030, 3, 1, 17, 0, 0, 0, time.UTC)
	start = time.Date(2030, 3, 5, 15, 0, 0, 0, time.UTC)
	biz   = uuid.MustParse("01900000-0000-7000-8000-0000000000b1")
	appt  = uuid.MustParse("01900000-0000-7000-8000-0000000000a1")
	pro   = uuid.MustParse("01900000-0000-7000-8000-0000000000c1")
	phone = "+573001234567"
)

type world struct {
	st     *notiftest.Store
	appts  *appointments
	sender *sender
	svc    *application.Service
}

func newWorld(p application.Policy) world {
	w := world{
		st:     notiftest.NewStore(),
		appts:  &appointments{state: scheduling.AppointmentState{ID: appt, BusinessID: biz, ProfessionalID: pro, Status: "CONFIRMED", Start: start, Version: 1}},
		sender: &sender{},
	}
	w.svc = application.NewService(application.Deps{
		Store: w.st, Appointments: w.appts, Senders: map[domain.Channel]application.Sender{domain.ChannelWhatsApp: w.sender},
		Policy: p, Clock: clock.NewFixed(now),
	})
	return w
}

func event(typ, actor string) scheduling.AppointmentEvent {
	return scheduling.AppointmentEvent{
		ID: uuid.Must(uuid.NewV7()), BusinessID: biz, AppointmentID: appt, Type: typ, ActorRole: actor,
		ProfessionalID: pro, Start: start, CustomerPhone: phone, At: now,
	}
}

func (w world) msg(id uuid.UUID) domain.Message {
	m, _ := w.st.Message(context.Background(), id)
	return m
}

// queued records e and returns its single message.
func (w world) queued(t *testing.T, e scheduling.AppointmentEvent) domain.Message {
	t.Helper()
	if err := w.svc.AppointmentEventRecorded(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	for _, m := range w.st.Messages() {
		if m.EventID == e.ID {
			return m
		}
	}
	t.Fatalf("no message for %s", e.Type)
	return domain.Message{}
}

func TestEventsQueueTheirMessagesOnce(t *testing.T) {
	w := newWorld(application.Policy{WhatsAppEnabled: true})
	ctx := context.Background()
	e := event("CREATED", "BUSINESS")
	m := w.queued(t, e)
	if m.Kind != domain.KindCreatedByBusiness || m.Status != domain.StatusPending || len(w.st.Enqueued()) != 1 || w.st.Enqueued()[0] != m.ID {
		t.Fatalf("message %+v, jobs %v", m, w.st.Enqueued())
	}
	// The same event again (a retried transaction): no second message, no second job.
	if err := w.svc.AppointmentEventRecorded(ctx, e); err != nil {
		t.Fatal(err)
	}
	if len(w.st.Messages()) != 1 || len(w.st.Enqueued()) != 1 {
		t.Errorf("repeated event: %d messages, %d jobs", len(w.st.Messages()), len(w.st.Enqueued()))
	}
	// Changes without a message do not touch the store.
	for _, typ := range []string{"CONFIRMED", "STARTED", "COMPLETED", "NO_SHOW"} {
		if err := w.svc.AppointmentEventRecorded(ctx, event(typ, "BUSINESS")); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.svc.AppointmentEventRecorded(ctx, event(scheduling.EventCancelled, "CUSTOMER")); err != nil {
		t.Fatal(err)
	}
	if len(w.st.Messages()) != 1 {
		t.Errorf("changes without a message stored %d", len(w.st.Messages()))
	}
	// A failing transaction surfaces (the change is rolled back with it).
	w.st.FailCommit = errors.New("boom")
	if err := w.svc.AppointmentEventRecorded(ctx, event("RESCHEDULED", "CUSTOMER")); err == nil {
		t.Error("the failure of the transaction must abort the change")
	}
}

func TestSendDelivers(t *testing.T) {
	w := newWorld(application.Policy{WhatsAppEnabled: true, WhatsAppAllowlist: []string{phone}})
	m := w.queued(t, event("CREATED", "CUSTOMER"))
	if err := w.svc.Send(context.Background(), m.ID, false); err != nil {
		t.Fatal(err)
	}
	got := w.msg(m.ID)
	if got.Status != domain.StatusSent || got.ProviderMessageID != "wamid-1" || !got.SentAt.Equal(now) || got.Attempts != 1 || got.ErrorCode != "" {
		t.Fatalf("after sending: %+v", got)
	}
	if len(w.sender.sent) != 1 || w.sender.sent[0].To != phone || w.sender.sent[0].Kind != domain.KindCreated {
		t.Errorf("sent %+v", w.sender.sent)
	}
	// A repeated job sends nothing twice.
	if err := w.svc.Send(context.Background(), m.ID, false); err != nil || len(w.sender.sent) != 1 {
		t.Errorf("second run: %v, sent %d", err, len(w.sender.sent))
	}
	// A message rolled back with its change: nothing to do.
	if err := w.svc.Send(context.Background(), uuid.Must(uuid.NewV7()), false); err != nil {
		t.Errorf("unknown message: %v", err)
	}
}

func TestSendSkips(t *testing.T) {
	cases := []struct {
		name   string
		policy application.Policy
		state  func(*appointments)
		want   domain.SkipReason
	}{
		{"channel off", application.Policy{}, nil, domain.SkipChannelDisabled},
		{"number not allowed", application.Policy{WhatsAppEnabled: true, WhatsAppAllowlist: []string{"+573009999999"}}, nil, domain.SkipNotAllowlisted},
		{"appointment gone", application.Policy{WhatsAppEnabled: true}, func(a *appointments) { a.err = scheduling.ErrAppointmentNotFound }, domain.SkipNotFound},
		{"canceled meanwhile", application.Policy{WhatsAppEnabled: true}, func(a *appointments) { a.state.Status = scheduling.StatusCancelled }, domain.SkipCancelled},
		{"moved meanwhile", application.Policy{WhatsAppEnabled: true}, func(a *appointments) { a.state.Start = start.Add(time.Hour) }, domain.SkipChanged},
	}
	for _, c := range cases {
		w := newWorld(c.policy)
		if c.state != nil {
			c.state(w.appts)
		}
		m := w.queued(t, event("CREATED", "CUSTOMER"))
		if err := w.svc.Send(context.Background(), m.ID, false); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		got := w.msg(m.ID)
		if got.Status != domain.StatusSkipped || got.SkipReason != c.want || got.Attempts != 0 || len(w.sender.sent) != 0 {
			t.Errorf("%s: %+v, sent %d", c.name, got, len(w.sender.sent))
		}
	}
}

func TestSendWithoutSenderForTheChannel(t *testing.T) {
	w := newWorld(application.Policy{WhatsAppEnabled: true})
	w.svc = application.NewService(application.Deps{Store: w.st, Appointments: w.appts, Policy: application.Policy{WhatsAppEnabled: true}, Clock: clock.NewFixed(now)})
	m := w.queued(t, event("CREATED", "CUSTOMER"))
	if err := w.svc.Send(context.Background(), m.ID, false); err != nil || w.msg(m.ID).SkipReason != domain.SkipChannelDisabled {
		t.Errorf("%v %+v", err, w.msg(m.ID))
	}
}

func TestSendFailures(t *testing.T) {
	ctx := context.Background()
	temporary := &application.DeliveryError{Code: "RATE_LIMITED", Err: errors.New("429")}
	permanent := &application.DeliveryError{Code: "INVALID_NUMBER", Permanent: true, Err: errors.New("400")}

	// Temporary: the error goes back to River for a retry; the attempt is recorded.
	w := newWorld(application.Policy{WhatsAppEnabled: true})
	w.sender.err = temporary
	m := w.queued(t, event("CREATED", "CUSTOMER"))
	if err := w.svc.Send(ctx, m.ID, false); !errors.Is(err, temporary) {
		t.Fatalf("temporary failure returned %v", err)
	}
	if got := w.msg(m.ID); got.Status != domain.StatusPending || got.ErrorCode != "RATE_LIMITED" || got.Attempts != 1 {
		t.Errorf("after a temporary failure: %+v", got)
	}
	// The last attempt fails it for good.
	if err := w.svc.Send(ctx, m.ID, true); err != nil {
		t.Fatal(err)
	}
	if got := w.msg(m.ID); got.Status != domain.StatusFailed || got.Attempts != 2 {
		t.Errorf("after the last attempt: %+v", got)
	}

	// Permanent: FAILED at once, no retry.
	w = newWorld(application.Policy{WhatsAppEnabled: true})
	w.sender.err = permanent
	m = w.queued(t, event("CREATED", "CUSTOMER"))
	if err := w.svc.Send(ctx, m.ID, false); err != nil {
		t.Fatalf("a permanent failure is not retried: %v", err)
	}
	if got := w.msg(m.ID); got.Status != domain.StatusFailed || got.ErrorCode != "INVALID_NUMBER" {
		t.Errorf("after a permanent failure: %+v", got)
	}

	// An error that is not a DeliveryError is retried as UNKNOWN.
	w = newWorld(application.Policy{WhatsAppEnabled: true})
	w.sender.err = errors.New("connection reset")
	m = w.queued(t, event("CREATED", "CUSTOMER"))
	if err := w.svc.Send(ctx, m.ID, false); err == nil || w.msg(m.ID).ErrorCode != "UNKNOWN" {
		t.Errorf("unknown failure: %v %+v", err, w.msg(m.ID))
	}
	if temporary.Error() == "" || !errors.Is(temporary, temporary.Err) {
		t.Error("DeliveryError wraps its cause")
	}

	// Reading the appointment fails: retried.
	w = newWorld(application.Policy{WhatsAppEnabled: true})
	w.appts.err = errors.New("db down")
	m = w.queued(t, event("CREATED", "CUSTOMER"))
	if err := w.svc.Send(ctx, m.ID, false); err == nil || w.msg(m.ID).Status != domain.StatusPending {
		t.Errorf("appointment unreadable: %v %+v", err, w.msg(m.ID))
	}
}

func TestAppointmentMessages(t *testing.T) {
	w := newWorld(application.Policy{WhatsAppEnabled: true})
	ctx := context.Background()
	first := w.queued(t, event("CREATED", "BUSINESS"))
	second := w.queued(t, event("RESCHEDULED", "BUSINESS"))
	got, err := w.svc.AppointmentMessages(ctx, identity.Principal{UserID: uuid.Must(uuid.NewV7())}, biz, appt)
	if err != nil || len(got) != 2 || got[0].ID != first.ID || got[1].ID != second.ID {
		t.Fatalf("%v %v", got, err)
	}
	w.appts.authErr = scheduling.ErrAppointmentNotFound
	if _, err := w.svc.AppointmentMessages(ctx, identity.Principal{}, biz, appt); !errors.Is(err, scheduling.ErrAppointmentNotFound) {
		t.Errorf("not allowed: %v", err)
	}
}

// TestSendFillsTheContent (T7.3): the message carries the current names of the appointment and
// its start in the business's zone.
func TestSendFillsTheContent(t *testing.T) {
	w := newWorld(application.Policy{WhatsAppEnabled: true})
	w.appts.state.CustomerName, w.appts.state.BusinessName, w.appts.state.ServiceName = "Laura", "Barbería El Paisa", "Corte"
	w.appts.state.ProfessionalName, w.appts.state.TimeZone, w.appts.state.CancelReason = "Juan", "America/Bogota", "Cierre"
	m := w.queued(t, event("CREATED", "BUSINESS"))
	if m.Kind != domain.KindCreatedByBusiness {
		t.Fatalf("kind %s", m.Kind)
	}
	if err := w.svc.Send(context.Background(), m.ID, false); err != nil {
		t.Fatal(err)
	}
	c := w.sender.sent[0].Content
	if c.CustomerName != "Laura" || c.BusinessName != "Barbería El Paisa" || c.ServiceName != "Corte" || c.ProfessionalName != "Juan" ||
		c.CancelReason != "Cierre" || !c.Start.Equal(start) || c.Location.String() != "America/Bogota" ||
		c.WhenES() != "martes 5 de marzo, 10:00 a. m." {
		t.Errorf("content %+v (%s)", c, c.WhenES())
	}

	// An unknown zone falls back to UTC instead of failing the message.
	w = newWorld(application.Policy{WhatsAppEnabled: true})
	w.appts.state.TimeZone = "Nowhere/Nada"
	m = w.queued(t, event("CREATED", "CUSTOMER"))
	if err := w.svc.Send(context.Background(), m.ID, false); err != nil || w.sender.sent[0].Content.Location != time.UTC {
		t.Errorf("bad zone: %v %v", err, w.sender.sent[0].Content.Location)
	}
}
