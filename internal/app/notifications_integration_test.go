//go:build integration

package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"

	notifapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/application/notiftest"
	notifdomain "github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/domain"
	notifpg "github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/infrastructure/postgres"
	notifworker "github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/infrastructure/worker"
	schedcontract "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/jobs"
)

type notifWorld struct {
	bookingWorld
	notif  *notifapp.Service
	sender *notiftest.Sender
}

func newNotifWorld(t *testing.T) notifWorld {
	t.Helper()
	w := notifWorld{bookingWorld: newBookingWorld(t), sender: &notiftest.Sender{}}
	inserter, err := jobs.NewInsertClient(w.pool, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	w.notif = notifapp.NewService(notifapp.Deps{
		Store: notifpg.NewStore(w.pool, inserter), Appointments: w.sched,
		Senders: map[notifdomain.Channel]notifapp.Sender{notifdomain.ChannelWhatsApp: w.sender},
		Policy:  notifapp.Policy{WhatsAppEnabled: true}, Clock: w.clk,
	})
	LinkNotifications(w.sched, w.notif)
	return w
}

// messages returns the messages of an appointment and checks each has exactly one queued job.
func (w notifWorld) messages(t *testing.T, appointmentID uuid.UUID) []notifdomain.Message {
	t.Helper()
	ms, err := w.notif.AppointmentMessages(t.Context(), w.owner, w.biz, appointmentID)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range ms {
		var jobs int
		if err := w.pool.QueryRow(t.Context(), `SELECT count(*) FROM river_job WHERE kind = 'notifications.send_message' AND args->>'message_id' = $1`, m.ID.String()).Scan(&jobs); err != nil {
			t.Fatal(err)
		}
		if jobs != 1 {
			t.Errorf("message %s has %d delivery jobs", m.Kind, jobs)
		}
	}
	return ms
}

// failingListener stores the messages and then fails, as if a later step of the change failed.
type failingListener struct{ inner schedcontract.EventListener }

func (f failingListener) AppointmentEventRecorded(ctx context.Context, e schedcontract.AppointmentEvent) error {
	if err := f.inner.AppointmentEventRecorded(ctx, e); err != nil {
		return err
	}
	return errors.New("later step failed")
}

// TestNotificationsOnPostgres (T7.1): the message and its delivery job are stored in the
// transaction of the change (and rolled back with it); sending re-checks the appointment.
func TestNotificationsOnPostgres(t *testing.T) {
	ctx := t.Context()
	w := newNotifWorld(t)
	ana := w.customer(t)

	a, err := w.sched.Book(ctx, ana, w.in(w.at(9, 0), "aviso-ana-0001"))
	if err != nil {
		t.Fatal(err)
	}
	ms := w.messages(t, a.ID)
	if len(ms) != 1 || ms[0].Kind != notifdomain.KindCreated || ms[0].Status != notifdomain.StatusPending || ms[0].Recipient != "+573001234567" {
		t.Fatalf("after booking: %+v", ms)
	}
	if err := w.notif.Send(ctx, ms[0].ID, false); err != nil {
		t.Fatal(err)
	}
	ms = w.messages(t, a.ID)
	if ms[0].Status != notifdomain.StatusSent || ms[0].ProviderMessageID != "test-"+ms[0].ID.String() || ms[0].Attempts != 1 || ms[0].SentAt.IsZero() {
		t.Errorf("after sending: %+v", ms[0])
	}
	if err := w.notif.Send(ctx, ms[0].ID, false); err != nil || len(w.sender.Sent) != 1 {
		t.Errorf("a repeated job: %v, %d sent", err, len(w.sender.Sent))
	}

	// Moved before its confirmation went out: the old message is dropped, the new one is sent.
	b, err := w.sched.Book(ctx, ana, w.in(w.at(10, 0), "aviso-ana-0002"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.sched.StaffReschedule(ctx, w.owner, w.biz, b.ID, w.at(11, 0), w.pro); err != nil {
		t.Fatal(err)
	}
	ms = w.messages(t, b.ID)
	if len(ms) != 2 || ms[1].Kind != notifdomain.KindRescheduled {
		t.Fatalf("after moving: %+v", ms)
	}
	for _, m := range ms {
		if err := w.notif.Send(ctx, m.ID, false); err != nil {
			t.Fatal(err)
		}
	}
	ms = w.messages(t, b.ID)
	if ms[0].Status != notifdomain.StatusSkipped || ms[0].SkipReason != notifdomain.SkipChanged || ms[1].Status != notifdomain.StatusSent {
		t.Errorf("stale then fresh: %+v / %+v", ms[0], ms[1])
	}

	// A change that fails after the messages were stored leaves no message and no job.
	before := countRows(t, w, `SELECT count(*) FROM notification_messages`) + countRows(t, w, `SELECT count(*) FROM river_job WHERE kind = 'notifications.send_message'`)
	w.sched.SetEventListener(failingListener{inner: w.notif})
	if _, err := w.sched.Book(ctx, ana, w.in(w.at(9, 50), "aviso-ana-0003")); err == nil {
		t.Fatal("the booking must fail")
	}
	after := countRows(t, w, `SELECT count(*) FROM notification_messages`) + countRows(t, w, `SELECT count(*) FROM river_job WHERE kind = 'notifications.send_message'`)
	if after != before {
		t.Errorf("rolled-back change left %d rows", after-before)
	}
	w.sched.SetEventListener(w.notif)
}

// TestNotificationsWorkerOnPostgres: the real worker picks the job up and delivers.
func TestNotificationsWorkerOnPostgres(t *testing.T) {
	ctx := t.Context()
	w := newNotifWorld(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	workers := river.NewWorkers()
	if err := notifworker.Register(workers, w.notif); err != nil {
		t.Fatal(err)
	}
	client, err := jobs.NewWorkerClient(w.pool, logger, workers, river.Config{FetchPollInterval: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Stop(context.WithoutCancel(ctx)) })

	a, err := w.sched.Book(ctx, w.customer(t), w.in(w.at(9, 0), "aviso-worker-1"))
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		ms := w.messages(t, a.ID)
		if len(ms) == 1 && ms[0].Status == notifdomain.StatusSent {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("not delivered: %+v", ms)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func countRows(t *testing.T, w notifWorld, q string) int {
	t.Helper()
	var n int
	if err := w.pool.QueryRow(t.Context(), q).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
