package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	businesses "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/domain"
	scheduling "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
)

type recipients struct {
	accounts map[uuid.UUID]string
	pros     map[uuid.UUID]string
	err      error
}

func (r *recipients) AccountEmail(_ context.Context, id uuid.UUID) (string, error) {
	return r.accounts[id], r.err
}

func (r *recipients) ProfessionalEmail(_ context.Context, businessID, id uuid.UUID) (string, error) {
	if businessID != biz {
		return "", errors.New("wrong business")
	}
	return r.pros[id], r.err
}

type emailWorld struct {
	world
	mails *sender
	rcpts *recipients
	rs    *reminderSettings
}

func newEmailWorld(p application.Policy) emailWorld {
	w := emailWorld{world: newWorld(p), mails: &sender{}, rcpts: &recipients{accounts: map[uuid.UUID]string{}, pros: map[uuid.UUID]string{}},
		rs: &reminderSettings{pol: businesses.BookingPolicy{CancellationCutoffHours: 2, RescheduleCutoffHours: 4}}}
	w.svc = application.NewService(application.Deps{
		Store: w.st, Appointments: w.appts, Recipients: w.rcpts, Reminders: w.rs,
		Senders: map[domain.Channel]application.Sender{domain.ChannelWhatsApp: w.sender, domain.ChannelEmail: w.mails},
		Policy:  p, Clock: clock.NewFixed(now),
	})
	return w
}

// sendEmail records e and sends its email to role.
func (w emailWorld) sendEmail(t *testing.T, e scheduling.AppointmentEvent, role domain.Role) domain.Message {
	t.Helper()
	if err := w.svc.AppointmentEventRecorded(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	for _, m := range w.st.Messages() {
		if m.EventID == e.ID && m.Channel == domain.ChannelEmail && m.RecipientRole == role && m.Kind != domain.KindReminder {
			if err := w.svc.Send(context.Background(), m.ID, false); err != nil {
				t.Fatal(err)
			}
			return w.msg(m.ID)
		}
	}
	t.Fatalf("no %s email for %s by %s", role, e.Type, e.ActorRole)
	return domain.Message{}
}

// TestEmailRecipients (T7.6): the customer's email registered by the business, else its account's;
// the professional's account; none → skipped. The address used is stored with the message.
func TestEmailRecipients(t *testing.T) {
	account := uuid.Must(uuid.NewV7())
	on := application.Policy{EmailEnabled: true}

	w := newEmailWorld(on)
	w.appts.state.CustomerEmail, w.appts.state.CustomerUserID = "laura@negocio.co", account
	w.rcpts.accounts[account] = "laura@cuenta.co"
	m := w.sendEmail(t, event("CREATED", "CUSTOMER"), domain.RecipientCustomer)
	if m.Status != domain.StatusSent || m.Recipient != "laura@negocio.co" || w.mails.sent[0].To != "laura@negocio.co" ||
		w.mails.sent[0].Role != domain.RecipientCustomer {
		t.Fatalf("registered email: %+v", m)
	}
	// The content carries the policy and contact the email shows.
	if c := w.mails.sent[0].Content; c.CancelCutoffHours != 2 || c.RescheduleCutoffHours != 4 {
		t.Errorf("policy %+v", c)
	}

	w = newEmailWorld(on)
	w.appts.state.CustomerUserID = account
	w.rcpts.accounts[account] = "laura@cuenta.co"
	if m := w.sendEmail(t, event("RESCHEDULED", "BUSINESS"), domain.RecipientCustomer); m.Recipient != "laura@cuenta.co" || m.Status != domain.StatusSent {
		t.Errorf("account email: %+v", m)
	}

	w = newEmailWorld(on)
	w.rcpts.pros[pro] = "andres@barberia.co"
	if m := w.sendEmail(t, event("CREATED", "CUSTOMER"), domain.RecipientProfessional); m.Recipient != "andres@barberia.co" || m.Status != domain.StatusSent ||
		w.mails.sent[0].Role != domain.RecipientProfessional {
		t.Errorf("professional: %+v", m)
	}

	// No address anywhere: skipped, nothing sent.
	w = newEmailWorld(on)
	for _, role := range []domain.Role{domain.RecipientCustomer, domain.RecipientProfessional} {
		if m := w.sendEmail(t, event("CREATED", "CUSTOMER"), role); m.Status != domain.StatusSkipped || m.SkipReason != domain.SkipNoRecipient {
			t.Errorf("%s without email: %+v", role, m)
		}
	}
	if len(w.mails.sent) != 0 {
		t.Errorf("sent %d", len(w.mails.sent))
	}

	// Without a directory only the registered email counts.
	w = newEmailWorld(on)
	w.svc = application.NewService(application.Deps{Store: w.st, Appointments: w.appts,
		Senders: map[domain.Channel]application.Sender{domain.ChannelEmail: w.mails}, Policy: on, Clock: clock.NewFixed(now)})
	w.appts.state.CustomerUserID = account
	if m := w.sendEmail(t, event("CREATED", "CUSTOMER"), domain.RecipientCustomer); m.SkipReason != domain.SkipNoRecipient {
		t.Errorf("no directory: %+v", m)
	}
}

func TestEmailSwitchAndFailures(t *testing.T) {
	ctx := context.Background()
	// Emails off: skipped as CHANNEL_DISABLED, WhatsApp untouched.
	w := newEmailWorld(application.Policy{WhatsAppEnabled: true})
	w.appts.state.CustomerEmail = "laura@negocio.co"
	if m := w.sendEmail(t, event("CREATED", "CUSTOMER"), domain.RecipientCustomer); m.SkipReason != domain.SkipChannelDisabled {
		t.Errorf("off: %+v", m)
	}

	// The directory fails: retried, still pending.
	w = newEmailWorld(application.Policy{EmailEnabled: true})
	w.rcpts.err = errors.New("db down")
	e := event("CREATED", "CUSTOMER")
	if err := w.svc.AppointmentEventRecorded(ctx, e); err != nil {
		t.Fatal(err)
	}
	for _, m := range w.st.Messages() {
		if m.Channel == domain.ChannelEmail && m.RecipientRole == domain.RecipientProfessional {
			if err := w.svc.Send(ctx, m.ID, false); !errors.Is(err, w.rcpts.err) || w.msg(m.ID).Status != domain.StatusPending {
				t.Errorf("directory down: %v %+v", err, w.msg(m.ID))
			}
		}
	}

	// The policy cannot be read: retried.
	w = newEmailWorld(application.Policy{EmailEnabled: true})
	w.appts.state.CustomerEmail = "laura@negocio.co"
	w.rs.err = errors.New("db down")
	e = event("CREATED", "BUSINESS")
	if err := w.svc.AppointmentEventRecorded(ctx, e); err != nil {
		t.Fatal(err)
	}
	for _, m := range w.st.Messages() {
		if m.Channel == domain.ChannelEmail {
			if err := w.svc.Send(ctx, m.ID, false); !errors.Is(err, w.rs.err) {
				t.Errorf("policy down: %v", err)
			}
		}
	}
}
