//go:build integration

package app

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	notifapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/application"
	notifdomain "github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/infrastructure/email"
	schedapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/application"
)

// mailbox is a fake SMTP server: it keeps every email.
type mailbox struct {
	mu   sync.Mutex
	sent []sentMail
}

type sentMail struct{ to, subject, text, html string }

func (m *mailbox) SendMessage(_ context.Context, to, subject, text, html string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, sentMail{to, subject, text, html})
	return nil
}

func emailOf(t *testing.T, w notifWorld, userID uuid.UUID) string {
	t.Helper()
	var e string
	if err := w.pool.QueryRow(t.Context(), `SELECT email FROM users WHERE id = $1`, userID).Scan(&e); err != nil {
		t.Fatal(err)
	}
	return e
}

// emails sends the pending emails of an appointment and returns them by recipient role.
func (w notifWorld) emails(t *testing.T, id uuid.UUID) map[notifdomain.Role][]notifdomain.Message {
	t.Helper()
	out := map[notifdomain.Role][]notifdomain.Message{}
	for _, m := range w.messages(t, id) {
		if m.Channel != notifdomain.ChannelEmail {
			continue
		}
		if m.Status == notifdomain.StatusPending && m.Kind != notifdomain.KindReminder {
			if err := w.notif.Send(t.Context(), m.ID, false); err != nil {
				t.Fatal(err)
			}
		}
		out[m.RecipientRole] = append(out[m.RecipientRole], m)
	}
	for role, ms := range out {
		for i, m := range ms {
			for _, now := range w.messages(t, id) {
				if now.ID == m.ID {
					out[role][i] = now
				}
			}
		}
	}
	return out
}

// TestAppointmentEmailsOnPostgres (T7.6): a customer's booking emails the customer (its account's
// address) and the professional (its account's), with the business's data; the addresses are
// stored; the customer's cancellation emails both and sends no WhatsApp.
func TestAppointmentEmailsOnPostgres(t *testing.T) {
	ctx := t.Context()
	box := &mailbox{}
	w := newNotifWorld(t, func(w *notifWorld, d *notifapp.Deps) {
		d.Recipients = NewEmailRecipients(w.identity, w.profs)
		d.Senders[notifdomain.ChannelEmail] = email.New(box, email.Config{AppDownloadURL: "https://booka.example/app"})
		d.Policy.EmailEnabled = true
	})

	ana := w.customer(t)
	a, err := w.sched.Book(ctx, ana, w.in(w.at(9, 0), "correo-ana-0001"))
	if err != nil {
		t.Fatal(err)
	}
	got := w.emails(t, a.ID)
	anaEmail, barberEmail := emailOf(t, w, ana.UserID), emailOf(t, w, w.barber.UserID)
	if c := got[notifdomain.RecipientCustomer]; len(c) != 2 || c[0].Kind != notifdomain.KindCreated || c[0].Status != notifdomain.StatusSent ||
		c[0].Recipient != anaEmail || c[1].Kind != notifdomain.KindReminder || c[1].Status != notifdomain.StatusPending {
		t.Fatalf("customer emails %+v", c)
	}
	if p := got[notifdomain.RecipientProfessional]; len(p) != 1 || p[0].Kind != notifdomain.KindCreated || p[0].Status != notifdomain.StatusSent ||
		p[0].Recipient != barberEmail {
		t.Fatalf("professional emails %+v", p)
	}
	if len(box.sent) != 2 || box.sent[0].to != anaEmail || box.sent[1].to != barberEmail {
		t.Fatalf("mailbox %+v", box.sent)
	}
	// What the customer reads comes from the business and the appointment.
	for _, want := range []string{"está agendada", "Corte", "Pendiente de confirmar", "Martes 5 de marzo de 2030", "Políticas del negocio", "Booka"} {
		if !strings.Contains(box.sent[0].subject+box.sent[0].html, want) {
			t.Errorf("customer email without %q", want)
		}
	}
	if !strings.HasPrefix(box.sent[1].subject, "Nueva cita: ") {
		t.Errorf("professional subject %q", box.sent[1].subject)
	}

	// The customer cancels: an email receipt to the customer and a notice to the professional,
	// no WhatsApp.
	if _, err := w.sched.CustomerChange(ctx, ana, a.ID, schedapp.ActionCancel, ""); err != nil {
		t.Fatal(err)
	}
	if n := len(byKind(onWhatsApp(w.messages(t, a.ID)), notifdomain.KindCancelledByCustomer)); n != 0 {
		t.Errorf("%d WhatsApp messages for the customer's cancellation", n)
	}
	got = w.emails(t, a.ID)
	for role, to := range map[notifdomain.Role]string{notifdomain.RecipientCustomer: anaEmail, notifdomain.RecipientProfessional: barberEmail} {
		cancel := byKind(got[role], notifdomain.KindCancelledByCustomer)
		if len(cancel) != 1 || cancel[0].Status != notifdomain.StatusSent || cancel[0].Recipient != to {
			t.Errorf("%s cancellation email %+v", role, cancel)
		}
	}
	// The pending email reminder of the canceled appointment is dropped when due.
	for _, m := range byKind(got[notifdomain.RecipientCustomer], notifdomain.KindReminder) {
		if err := w.notif.Send(ctx, m.ID, false); err != nil {
			t.Fatal(err)
		}
	}
	if r := byKind(w.emails(t, a.ID)[notifdomain.RecipientCustomer], notifdomain.KindReminder); r[0].SkipReason != notifdomain.SkipCancelled {
		t.Errorf("reminder of a canceled appointment: %+v", r[0])
	}
}
