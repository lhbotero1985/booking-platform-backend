package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/application/notiftest"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/domain"
	scheduling "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
)

// replies is the scheduling side of the answers.
type replies struct {
	confirms, cancels int
	err               error
}

func (r *replies) ConfirmAsCustomer(context.Context, uuid.UUID, uuid.UUID) error {
	r.confirms++
	return r.err
}

func (r *replies) CancelAsCustomer(context.Context, uuid.UUID, uuid.UUID) error {
	r.cancels++
	return r.err
}

type textSender struct {
	sent []string
	err  error
}

func (s *textSender) SendText(_ context.Context, to, text string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	s.sent = append(s.sent, to+": "+text)
	return "wamid.reply", nil
}

type buttonsWorld struct {
	world
	clk     *clock.Fixed
	replies *replies
	texts   *textSender
	payload map[domain.Action]string // the reminder's buttons as delivered
	msg     domain.Message
}

// newButtonsWorld sends a reminder of a pending appointment (Laura, Barbería El Paisa) and keeps
// the payloads of its buttons.
func newButtonsWorld(t *testing.T) buttonsWorld {
	t.Helper()
	w := buttonsWorld{world: newWorld(application.Policy{WhatsAppEnabled: true}), clk: clock.NewFixed(now), replies: &replies{}, texts: &textSender{}}
	w.appts.state.Status = scheduling.StatusPendingConfirmation
	w.appts.state.CustomerName, w.appts.state.BusinessName, w.appts.state.ServiceName = "Laura", "Barbería El Paisa", "Corte"
	w.appts.state.TimeZone = "America/Bogota"
	w.svc = application.NewService(application.Deps{
		Store: w.st, Appointments: w.appts, Replies: w.replies, TextSender: w.texts,
		Reminders: &reminderSettings{pol: reminderPolicy()},
		Senders:   map[domain.Channel]application.Sender{domain.ChannelWhatsApp: w.sender},
		Policy:    application.Policy{WhatsAppEnabled: true}, Clock: w.clk,
	})
	e := event("CREATED", "CUSTOMER")
	e.Status = scheduling.StatusPendingConfirmation
	if err := w.svc.AppointmentEventRecorded(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	for _, m := range w.st.Messages() {
		if m.Kind == domain.KindReminder && m.Channel == domain.ChannelWhatsApp {
			w.msg = m
		}
	}
	if err := w.svc.Send(context.Background(), w.msg.ID, false); err != nil {
		t.Fatal(err)
	}
	out := w.sender.sent[len(w.sender.sent)-1]
	if out.Kind != domain.KindReminder || len(out.ButtonPayloads) != 2 {
		t.Fatalf("reminder sent %+v", out)
	}
	w.payload = map[domain.Action]string{domain.ActionConfirm: out.ButtonPayloads[0], domain.ActionCancel: out.ButtonPayloads[1]}
	return w
}

// press processes a button answer from a number and returns the answer queued for it.
func (w buttonsWorld) press(t *testing.T, payload, from string) string {
	t.Helper()
	before := len(w.st.Replies())
	if err := w.svc.ProcessWebhookItem(context.Background(), domain.WebhookItem{
		Kind: domain.WebhookButton, EventID: "message:" + uuid.NewString(), From: from, Payload: payload,
	}); err != nil {
		t.Fatal(err)
	}
	rs := w.st.Replies()
	if len(rs) != before+1 || rs[len(rs)-1].To != from {
		t.Fatalf("replies %+v", rs)
	}
	return rs[len(rs)-1].Text
}

func TestReminderTokens(t *testing.T) {
	w := newButtonsWorld(t)
	toks := w.st.Tokens(w.msg.ID)
	if len(toks) != 2 {
		t.Fatalf("tokens %+v", toks)
	}
	for _, k := range toks {
		raw := w.payload[k.Action]
		if len(raw) != 43 || string(k.Hash) == raw || string(k.Hash) != string(domain.HashActionToken(raw)) ||
			k.Recipient != phone || !k.ExpiresAt.Equal(start) || k.AppointmentID != appt || !k.UsedAt.IsZero() {
			t.Errorf("token %+v (raw %q)", k, raw)
		}
	}
	if w.payload[domain.ActionConfirm] == w.payload[domain.ActionCancel] {
		t.Error("the two buttons share a token")
	}
}

func TestConfirmButton(t *testing.T) {
	w := newButtonsWorld(t)
	got := w.press(t, w.payload[domain.ActionConfirm], phone)
	if w.replies.confirms != 1 || !strings.HasPrefix(got, "¡Listo, Laura! Tu cita de Corte en Barbería El Paisa quedó confirmada para el martes 5 de marzo, 10:00 a. m.") {
		t.Fatalf("confirm: %d %q", w.replies.confirms, got)
	}
	// Both buttons are spent: the other one, or the same again, does nothing.
	for _, p := range []string{w.payload[domain.ActionCancel], w.payload[domain.ActionConfirm]} {
		if got := w.press(t, p, phone); !strings.Contains(got, "ya no es válido") {
			t.Errorf("second answer: %q", got)
		}
	}
	if w.replies.confirms != 1 || w.replies.cancels != 0 {
		t.Errorf("applied %d confirms, %d cancels", w.replies.confirms, w.replies.cancels)
	}
}

func TestCancelButton(t *testing.T) {
	w := newButtonsWorld(t)
	if got := w.press(t, w.payload[domain.ActionCancel], phone); w.replies.cancels != 1 || !strings.Contains(got, "quedó cancelada") {
		t.Fatalf("cancel: %q", got)
	}
}

func TestButtonOutcomes(t *testing.T) {
	cases := []struct {
		name   string
		setup  func(w buttonsWorld)
		from   string
		action domain.Action
		want   string
		calls  int
	}{
		{"cut-off passed", func(w buttonsWorld) { w.replies.err = scheduling.ErrCutoffPassed }, phone, domain.ActionCancel, "falta muy poco", 1},
		{"not active", func(w buttonsWorld) { w.replies.err = scheduling.ErrNotActive }, phone, domain.ActionConfirm, "ya no está activa", 1},
		{"gone", func(w buttonsWorld) { w.replies.err = scheduling.ErrAppointmentNotFound }, phone, domain.ActionConfirm, "ya no es válido", 1},
		{"another number", func(buttonsWorld) {}, "+573009999999", domain.ActionConfirm, "ya no es válido", 0},
		{"after the start", func(w buttonsWorld) { w.clk.Advance(start.Sub(now)) }, phone, domain.ActionConfirm, "ya no es válido", 0},
		{"moved since", func(w buttonsWorld) { w.appts.state.Start = start.Add(time.Hour) }, phone, domain.ActionConfirm, "ya no es válido", 0},
		{"appointment unreadable", func(w buttonsWorld) { w.appts.err = scheduling.ErrAppointmentNotFound }, phone, domain.ActionConfirm, "ya no es válido", 0},
	}
	for _, c := range cases {
		w := newButtonsWorld(t)
		c.setup(w)
		got := w.press(t, w.payload[c.action], c.from)
		if !strings.Contains(got, c.want) || w.replies.confirms+w.replies.cancels != c.calls {
			t.Errorf("%s: %q, %d calls", c.name, got, w.replies.confirms+w.replies.cancels)
		}
	}
	w := newButtonsWorld(t)
	if got := w.press(t, "not-a-token", phone); !strings.Contains(got, "ya no es válido") || w.replies.confirms != 0 {
		t.Errorf("unknown token: %q", got)
	}
}

func TestButtonFailureIsRetried(t *testing.T) {
	w := newButtonsWorld(t)
	w.replies.err = errors.New("db down")
	err := w.svc.ProcessWebhookItem(context.Background(), domain.WebhookItem{Kind: domain.WebhookButton, EventID: "message:x", From: phone, Payload: w.payload[domain.ActionConfirm]})
	if !errors.Is(err, w.replies.err) || len(w.st.Replies()) != 0 {
		t.Fatalf("temporary failure: %v, %d replies", err, len(w.st.Replies()))
	}
	// The token was not spent: the retry works.
	w.replies.err = nil
	if got := w.press(t, w.payload[domain.ActionConfirm], phone); !strings.Contains(got, "quedó confirmada") {
		t.Errorf("retry: %q", got)
	}
}

func TestReceiveWebhookOnce(t *testing.T) {
	w := newWorld(application.Policy{WhatsAppEnabled: true})
	items := []domain.WebhookItem{
		{Kind: domain.WebhookStatus, EventID: "status:wamid.1:DELIVERED", ProviderMessageID: "wamid.1", Status: domain.StatusDelivered},
		{Kind: domain.WebhookButton, EventID: "message:wamid.2", From: phone, Payload: "p"},
	}
	ctx := context.Background()
	for range 2 { // Meta retries the same batch
		if err := w.svc.ReceiveWebhook(ctx, items); err != nil {
			t.Fatal(err)
		}
	}
	if got := w.st.WebhookItems(); len(got) != 2 || got[0] != items[0] || got[1] != items[1] {
		t.Errorf("queued %+v", got)
	}
	if err := w.svc.ReceiveWebhook(ctx, nil); err != nil {
		t.Error(err)
	}
}

func TestDeliveryStatuses(t *testing.T) {
	w := newWorld(application.Policy{WhatsAppEnabled: true})
	ctx := context.Background()
	m := w.queued(t, event("CREATED", "CUSTOMER"))
	if err := w.svc.Send(ctx, m.ID, false); err != nil {
		t.Fatal(err)
	}
	status := func(s domain.Status, code string) domain.Status {
		t.Helper()
		if err := w.svc.ProcessWebhookItem(ctx, domain.WebhookItem{Kind: domain.WebhookStatus, ProviderMessageID: "wamid-1", Status: s, ErrorCode: code}); err != nil {
			t.Fatal(err)
		}
		return w.msg(m.ID).Status
	}
	if got := status(domain.StatusRead, ""); got != domain.StatusRead {
		t.Errorf("read: %s", got)
	}
	if got := status(domain.StatusDelivered, ""); got != domain.StatusRead {
		t.Errorf("a late delivered must not go back: %s", got)
	}
	if got := status(domain.StatusFailed, "META_131026"); got != domain.StatusRead {
		t.Errorf("failed after read: %s", got)
	}
	if err := w.svc.ProcessWebhookItem(ctx, domain.WebhookItem{Kind: "OTHER"}); err != nil {
		t.Error(err)
	}
}

func TestSendReply(t *testing.T) {
	ctx := context.Background()
	texts := &textSender{}
	svc := func(p application.Policy, ts application.TextSender) *application.Service {
		return application.NewService(application.Deps{Store: notiftest.NewStore(), TextSender: ts, Policy: p, Clock: clock.NewFixed(now)})
	}
	if err := svc(application.Policy{WhatsAppEnabled: true}, texts).SendReply(ctx, phone, "Listo"); err != nil || len(texts.sent) != 1 {
		t.Fatalf("send: %v %v", err, texts.sent)
	}
	for name, p := range map[string]application.Policy{"off": {}, "not allowlisted": {WhatsAppEnabled: true, WhatsAppAllowlist: []string{"+573009999999"}}} {
		if err := svc(p, texts).SendReply(ctx, phone, "x"); err != nil || len(texts.sent) != 1 {
			t.Errorf("%s: %v %d", name, err, len(texts.sent))
		}
	}
	if err := svc(application.Policy{WhatsAppEnabled: true}, nil).SendReply(ctx, phone, "x"); err != nil {
		t.Errorf("without sender: %v", err)
	}
	texts.err = &application.DeliveryError{Code: "RECIPIENT_UNAVAILABLE", Permanent: true, Err: errors.New("x")}
	if err := svc(application.Policy{WhatsAppEnabled: true}, texts).SendReply(ctx, phone, "x"); err != nil {
		t.Errorf("permanent failure is dropped: %v", err)
	}
	texts.err = &application.DeliveryError{Code: "RATE_LIMITED", Err: errors.New("x")}
	if err := svc(application.Policy{WhatsAppEnabled: true}, texts).SendReply(ctx, phone, "x"); err == nil {
		t.Error("temporary failure is retried")
	}
}
