//go:build integration

package app

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	notifapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/application/notiftest"
	notifdomain "github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/domain"
)

type linksPGWorld struct {
	notifWorld
	mails *notiftest.Sender
}

func newLinksPGWorld(t *testing.T) linksPGWorld {
	t.Helper()
	mails := &notiftest.Sender{}
	w := newNotifWorld(t, func(w *notifWorld, d *notifapp.Deps) {
		d.Recipients = NewEmailRecipients(w.identity, w.profs)
		d.Replies = w.sched
		d.Senders[notifdomain.ChannelEmail] = mails
		d.Policy.EmailEnabled = true
	})
	return linksPGWorld{notifWorld: w, mails: mails}
}

// book books a pending appointment, sends the customer's email and returns its links.
func (w linksPGWorld) book(t *testing.T, hour int, key string) (uuid.UUID, map[notifdomain.Action]string) {
	t.Helper()
	a, err := w.sched.Book(t.Context(), w.customer(t), w.in(w.at(hour, 0), key))
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range w.messages(t, a.ID) {
		if m.Channel == notifdomain.ChannelEmail && m.RecipientRole == notifdomain.RecipientCustomer && m.Kind == notifdomain.KindCreated {
			if err := w.notif.Send(t.Context(), m.ID, false); err != nil {
				t.Fatal(err)
			}
		}
	}
	sent := w.mails.Delivered()
	links := sent[len(sent)-1].Links
	if len(links) != 3 {
		t.Fatalf("links %v", links)
	}
	return a.ID, links
}

func (w linksPGWorld) status(t *testing.T, id uuid.UUID) (string, time.Time) {
	t.Helper()
	st, err := w.sched.AppointmentState(t.Context(), w.biz, id)
	if err != nil {
		t.Fatal(err)
	}
	return st.Status, st.Start
}

// TestEmailLinksOnPostgres (T7.7): first tap changes nothing; confirm spends only its link; the
// new time comes from the link as the customer and spends the rest; raw codes are not stored.
func TestEmailLinksOnPostgres(t *testing.T) {
	ctx := t.Context()
	w := newLinksPGWorld(t)
	id, links := w.book(t, 9, "enlace-ana-0001")

	if v, err := w.notif.Link(ctx, links[notifdomain.ActionConfirm], notifdomain.ActionConfirm); err != nil || v.Outcome != "" || v.Content.ServiceName != "Corte" {
		t.Fatalf("first tap: %+v %v", v, err)
	}
	if st, _ := w.status(t, id); st != "PENDING_CONFIRMATION" {
		t.Fatalf("the first tap changed it: %s", st)
	}
	if v, err := w.notif.UseLink(ctx, links[notifdomain.ActionConfirm], notifdomain.ActionConfirm, time.Time{}); err != nil || v.Outcome != notifdomain.OutcomeConfirmed {
		t.Fatalf("confirm: %+v %v", v, err)
	}
	if st, _ := w.status(t, id); st != "CONFIRMED" {
		t.Errorf("after confirming: %s", st)
	}
	if v, _ := w.notif.UseLink(ctx, links[notifdomain.ActionConfirm], notifdomain.ActionConfirm, time.Time{}); v.Outcome != notifdomain.OutcomeInvalid {
		t.Errorf("confirm twice: %s", v.Outcome)
	}
	// A taken time: the link stays valid.
	other, _ := w.book(t, 10, "enlace-ana-0002")
	_ = other
	if v, _ := w.notif.UseLink(ctx, links[notifdomain.ActionReschedule], notifdomain.ActionReschedule, w.at(10, 0)); v.Outcome != notifdomain.OutcomeSlotUnavailable {
		t.Errorf("taken time: %s", v.Outcome)
	}
	v, err := w.notif.UseLink(ctx, links[notifdomain.ActionReschedule], notifdomain.ActionReschedule, w.at(11, 0))
	if err != nil || v.Outcome != notifdomain.OutcomeRescheduled || !v.Content.Start.Equal(w.at(11, 0)) {
		t.Fatalf("reschedule: %+v %v", v, err)
	}
	if _, start := w.status(t, id); !start.Equal(w.at(11, 0)) {
		t.Errorf("not moved: %s", start)
	}
	// Recorded as the customer, and it emails the professional (only what the customer did).
	var byCustomer bool
	if err := w.pool.QueryRow(ctx, `SELECT e.actor_user_id = a.customer_user_id FROM appointment_events e JOIN appointments a ON a.id = e.appointment_id
		WHERE e.appointment_id = $1 ORDER BY e.occurred_at DESC, e.id DESC LIMIT 1`, id).Scan(&byCustomer); err != nil {
		t.Fatal(err)
	}
	if !byCustomer || len(byKind(onEmailTo(w.messages(t, id), notifdomain.RecipientProfessional), notifdomain.KindRescheduled)) != 1 {
		t.Errorf("the move is not the customer's (%v) or the professional was not told", byCustomer)
	}
	if v, _ := w.notif.UseLink(ctx, links[notifdomain.ActionCancel], notifdomain.ActionCancel, time.Time{}); v.Outcome != notifdomain.OutcomeInvalid {
		t.Errorf("cancel after the move: %s", v.Outcome)
	}
	if n := countRows(t, w.notifWorld, `SELECT count(*) FROM appointment_action_tokens WHERE token_hash = convert_to('`+links[notifdomain.ActionCancel]+`', 'UTF8')`); n != 0 {
		t.Error("a raw code is stored")
	}
}

// TestEmailLinksConcurrently: "Cancelar" and "Reagendar" of one email at the same time: exactly
// one applies.
func TestEmailLinksConcurrently(t *testing.T) {
	ctx := t.Context()
	w := newLinksPGWorld(t)
	for round := range 4 {
		id, links := w.book(t, 9, "enlace-carrera-"+strconv.Itoa(round))
		outcomes := make([]notifdomain.Outcome, 2)
		var wg sync.WaitGroup
		for i, a := range []notifdomain.Action{notifdomain.ActionCancel, notifdomain.ActionReschedule} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				v, err := w.notif.UseLink(context.WithoutCancel(ctx), links[a], a, w.at(11, 0))
				if err != nil {
					t.Error(err)
				}
				outcomes[i] = v.Outcome
			}()
		}
		wg.Wait()
		applied := 0
		for _, o := range outcomes {
			if o == notifdomain.OutcomeCancelled || o == notifdomain.OutcomeRescheduled {
				applied++
			}
		}
		st, start := w.status(t, id)
		if applied != 1 || (outcomes[0] == notifdomain.OutcomeCancelled) != (st == "CANCELLED") { //nolint:misspell // stored value
			t.Fatalf("round %d: %v, status %s %s", round, outcomes, st, start)
		}
		if st != "CANCELLED" { //nolint:misspell // stored value
			if _, err := w.sched.StaffChange(ctx, w.owner, w.biz, id, "cancel", "Prueba"); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// onEmailTo keeps the emails to one recipient role.
func onEmailTo(ms []notifdomain.Message, role notifdomain.Role) []notifdomain.Message {
	var out []notifdomain.Message
	for _, m := range ms {
		if m.Channel == notifdomain.ChannelEmail && m.RecipientRole == role {
			out = append(out, m)
		}
	}
	return out
}
