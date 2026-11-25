package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/domain"
	scheduling "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
)

var svcID = uuid.MustParse("01900000-0000-7000-8000-0000000000d1")

type linksWorld struct {
	emailWorld
	replies *replies
	clk     *clock.Fixed
	links   map[domain.Action]string
	msg     domain.Message
}

// newLinksWorld sends the email of a new pending booking and keeps its links.
func newLinksWorld(t *testing.T) linksWorld {
	t.Helper()
	w := linksWorld{emailWorld: newEmailWorld(application.Policy{EmailEnabled: true}), replies: &replies{}, clk: clock.NewFixed(now)}
	w.appts.state.Status, w.appts.state.CustomerEmail = scheduling.StatusPendingConfirmation, "laura@negocio.co"
	w.appts.state.ServiceID, w.appts.state.BusinessSlug, w.appts.state.ServiceName = svcID, "barberia-el-paisa", "Corte"
	w.svc = application.NewService(application.Deps{
		Store: w.st, Appointments: w.appts, Replies: w.replies, Recipients: w.rcpts, Reminders: w.rs,
		Senders: map[domain.Channel]application.Sender{domain.ChannelEmail: w.mails}, Policy: application.Policy{EmailEnabled: true}, Clock: w.clk,
	})
	w.msg = w.sendEmail(t, event("CREATED", "CUSTOMER"), domain.RecipientCustomer)
	w.links = w.mails.sent[len(w.mails.sent)-1].Links
	return w
}

func (w linksWorld) use(t *testing.T, a domain.Action) domain.Outcome {
	t.Helper()
	v, err := w.svc.UseLink(context.Background(), w.links[a], a, start.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	return v.Outcome
}

func (w linksWorld) look(t *testing.T, a domain.Action) application.LinkView {
	t.Helper()
	v, err := w.svc.Link(context.Background(), w.links[a], a)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// TestEmailLinksIssued (T7.7): an email to the customer about an active appointment carries its
// links (confirm only while pending); none to the professional nor in a cancellation.
func TestEmailLinksIssued(t *testing.T) {
	w := newLinksWorld(t)
	sent := w.mails.sent[0]
	if len(sent.Links) != 3 || len(sent.ButtonPayloads) != 0 {
		t.Fatalf("links %v", sent.Links)
	}
	toks := w.st.Tokens(w.msg.ID)
	if len(toks) != 3 {
		t.Fatalf("tokens %+v", toks)
	}
	for _, k := range toks {
		if k.Recipient != "laura@negocio.co" || !k.ExpiresAt.Equal(start) || string(k.Hash) == w.links[k.Action] {
			t.Errorf("token %+v", k)
		}
	}
	// Confirmed: no confirm link.
	w2 := newEmailWorld(application.Policy{EmailEnabled: true})
	w2.appts.state.CustomerEmail = "laura@negocio.co" // CONFIRMED by default
	w2.sendEmail(t, event("RESCHEDULED", "BUSINESS"), domain.RecipientCustomer)
	if l := w2.mails.sent[0].Links; len(l) != 2 || l[domain.ActionConfirm] != "" {
		t.Errorf("confirmed: %v", l)
	}
	// The professional's email and a cancellation have none.
	w3 := newEmailWorld(application.Policy{EmailEnabled: true})
	w3.rcpts.pros[pro], w3.appts.state.CustomerEmail = "andres@barberia.co", "laura@negocio.co"
	w3.sendEmail(t, event("CREATED", "CUSTOMER"), domain.RecipientProfessional)
	w3.sendEmail(t, event("CANCELLED", "BUSINESS"), domain.RecipientCustomer) //nolint:misspell // stored value
	for _, m := range w3.mails.sent {
		if len(m.Links) != 0 {
			t.Errorf("%s to %s: links %v", m.Kind, m.Role, m.Links)
		}
	}
}

// TestLinkTwoTaps: opening a link changes nothing; the button applies it once. A confirmation
// spends only its own link; the cancellation then spends the rest.
func TestLinkTwoTaps(t *testing.T) {
	w := newLinksWorld(t)
	v := w.look(t, domain.ActionConfirm)
	if v.Outcome != "" || v.Content.CancelCutoffHours != 2 || v.Content.ServiceName != "Corte" || w.replies.confirms != 0 {
		t.Fatalf("first tap: %+v, confirms %d", v, w.replies.confirms)
	}
	if got := w.use(t, domain.ActionConfirm); got != domain.OutcomeConfirmed || w.replies.confirms != 1 {
		t.Fatalf("second tap: %s", got)
	}
	if got := w.use(t, domain.ActionConfirm); got != domain.OutcomeInvalid || w.replies.confirms != 1 {
		t.Errorf("confirm again: %s", got)
	}
	if v := w.look(t, domain.ActionCancel); v.Outcome != "" {
		t.Errorf("cancel link after confirming: %+v", v)
	}
	if got := w.use(t, domain.ActionCancel); got != domain.OutcomeCancelled || w.replies.cancels != 1 {
		t.Errorf("cancel: %s", got)
	}
	if v := w.look(t, domain.ActionReschedule); v.Outcome != domain.OutcomeInvalid || v.Content.ServiceName != "" {
		t.Errorf("reschedule after canceling: %+v", v)
	}
}

func TestLinkRejections(t *testing.T) {
	ctx := context.Background()
	w := newLinksWorld(t)
	if v, err := w.svc.Link(ctx, "nope", domain.ActionConfirm); err != nil || v.Outcome != domain.OutcomeInvalid {
		t.Errorf("unknown code: %+v %v", v, err)
	}
	if v, err := w.svc.UseLink(ctx, "nope", domain.ActionConfirm, time.Time{}); err != nil || v.Outcome != domain.OutcomeInvalid {
		t.Errorf("unknown code used: %+v %v", v, err)
	}
	if v, err := w.svc.UseLink(ctx, w.links[domain.ActionConfirm], domain.ActionCancel, time.Time{}); err != nil || v.Outcome != domain.OutcomeInvalid {
		t.Errorf("another action's code: %+v %v", v, err)
	}
	if w.replies.cancels != 0 || w.replies.confirms != 0 {
		t.Error("a rejected link changed the appointment")
	}
	// Moved since the email: invalid (a newer email has the new links).
	w.appts.state.Start = start.Add(time.Hour)
	if got := w.use(t, domain.ActionCancel); got != domain.OutcomeInvalid || w.replies.cancels != 0 {
		t.Errorf("moved: %s", got)
	}
	// Not active any more.
	w = newLinksWorld(t)
	w.appts.state.Status = scheduling.StatusCancelled
	if v := w.look(t, domain.ActionConfirm); v.Outcome != domain.OutcomeNotActive || v.Content.ServiceName != "Corte" {
		t.Errorf("not active: %+v", v)
	}
	// Expired at the start.
	w = newLinksWorld(t)
	w.clk.Advance(start.Sub(now))
	if got := w.use(t, domain.ActionConfirm); got != domain.OutcomeInvalid {
		t.Errorf("expired: %s", got)
	}
	// Gone.
	w = newLinksWorld(t)
	w.appts.err = scheduling.ErrAppointmentNotFound
	if got := w.use(t, domain.ActionConfirm); got != domain.OutcomeInvalid {
		t.Errorf("gone: %s", got)
	}
}

// TestLinkReschedule: the reschedule page gets what the availability needs; a taken time or the
// cut-off keep the link; the move spends every link of the email.
func TestLinkReschedule(t *testing.T) {
	w := newLinksWorld(t)
	v := w.look(t, domain.ActionReschedule)
	if v.Outcome != "" || v.BusinessSlug != "barberia-el-paisa" || v.ServiceID != svcID || v.ProfessionalID != pro || v.BusinessID != biz ||
		!v.Content.Start.Equal(start) {
		t.Fatalf("view %+v", v)
	}
	w.replies.err = errors.Join(scheduling.ErrSlotUnavailable, errors.New("taken"))
	if got := w.use(t, domain.ActionReschedule); got != domain.OutcomeSlotUnavailable {
		t.Errorf("taken: %s", got)
	}
	w.replies.err = scheduling.ErrCutoffPassed
	if got := w.use(t, domain.ActionReschedule); got != domain.OutcomeCutoff {
		t.Errorf("cut-off: %s", got)
	}
	w.replies.err = nil
	if got := w.use(t, domain.ActionReschedule); got != domain.OutcomeRescheduled || len(w.replies.moves) != 3 || !w.replies.moves[2].Equal(start.Add(time.Hour)) {
		t.Errorf("moved: %s %v", got, w.replies.moves)
	}
	for _, a := range []domain.Action{domain.ActionConfirm, domain.ActionCancel} {
		if got := w.use(t, a); got != domain.OutcomeInvalid {
			t.Errorf("%s after the move: %s", a, got)
		}
	}
	// Not active, or the store fails.
	w = newLinksWorld(t)
	w.replies.err = scheduling.ErrNotActive
	if got := w.use(t, domain.ActionCancel); got != domain.OutcomeNotActive {
		t.Errorf("not active: %s", got)
	}
	w = newLinksWorld(t)
	w.replies.err = errors.New("db down")
	if _, err := w.svc.UseLink(context.Background(), w.links[domain.ActionCancel], domain.ActionCancel, time.Time{}); err == nil {
		t.Error("a failure must surface")
	}
}

// TestLinkRescheduleLimit (T7.8): at the business's limit the email has no "Reagendar" link;
// an older link says so without being spent (it may still cancel), also when scheduling refuses.
func TestLinkRescheduleLimit(t *testing.T) {
	w := newLinksWorld(t) // limit 0: no limit
	if _, ok := w.links[domain.ActionReschedule]; !ok {
		t.Fatal("no limit: no reschedule link")
	}
	w.rs.pol.MaxCustomerReschedules, w.appts.state.CustomerReschedules = 2, 2
	v := w.look(t, domain.ActionReschedule)
	if v.Outcome != domain.OutcomeRescheduleLimit || v.Content.MaxReschedules != 2 || v.Content.Reschedules != 2 {
		t.Errorf("look at the limit: %+v", v)
	}
	if got := w.use(t, domain.ActionReschedule); got != domain.OutcomeRescheduleLimit || len(w.replies.moves) != 0 {
		t.Errorf("use at the limit: %s, moves %v", got, w.replies.moves)
	}
	if got := w.use(t, domain.ActionCancel); got != domain.OutcomeCancelled {
		t.Errorf("cancel after: %s", got)
	}
	// Scheduling refuses (moved by another way meanwhile): not spent.
	w = newLinksWorld(t)
	w.replies.err = errors.Join(scheduling.ErrRescheduleLimit, errors.New("limit"))
	if got := w.use(t, domain.ActionReschedule); got != domain.OutcomeRescheduleLimit {
		t.Errorf("refused: %s", got)
	}
	w.replies.err = nil
	if got := w.use(t, domain.ActionConfirm); got != domain.OutcomeConfirmed {
		t.Errorf("confirm after: %s", got)
	}
	// A new email at the limit: confirm and cancel only.
	w2 := newLinksWorld(t)
	w2.rs.pol.MaxCustomerReschedules, w2.appts.state.CustomerReschedules = 1, 1
	w2.sendEmail(t, event("RESCHEDULED", "CUSTOMER"), domain.RecipientCustomer)
	l := w2.mails.sent[len(w2.mails.sent)-1].Links
	if len(l) != 2 || l[domain.ActionReschedule] != "" {
		t.Errorf("email at the limit: %v", l)
	}
	if c := w2.mails.sent[len(w2.mails.sent)-1].Content; c.MaxReschedules != 1 || c.Reschedules != 1 {
		t.Errorf("content %+v", c)
	}
}
