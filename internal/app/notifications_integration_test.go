//go:build integration

package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"

	notifapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/application/notiftest"
	notifdomain "github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/domain"
	notifpg "github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/infrastructure/postgres"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/infrastructure/whatsapp"
	notifworker "github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/infrastructure/worker"
	schedapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/application"
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
		Store: notifpg.NewStore(w.pool, inserter), Appointments: w.sched, Reminders: w.bizSvc,
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

// byKind returns the messages of one kind, in order.
func byKind(ms []notifdomain.Message, k notifdomain.Kind) []notifdomain.Message {
	var out []notifdomain.Message
	for _, m := range ms {
		if m.Kind == k {
			out = append(out, m)
		}
	}
	return out
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
	if len(ms) != 2 || ms[0].Kind != notifdomain.KindCreated || ms[0].Status != notifdomain.StatusPending || ms[0].Recipient != "+573001234567" ||
		ms[1].Kind != notifdomain.KindReminder {
		t.Fatalf("after booking (notice and reminder): %+v", ms)
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
	created, moved := byKind(ms, notifdomain.KindCreated), byKind(ms, notifdomain.KindRescheduled)
	reminders := byKind(ms, notifdomain.KindReminder)
	if len(created) != 1 || len(moved) != 1 || len(reminders) != 2 { // one reminder per time
		t.Fatalf("after moving: %+v", ms)
	}
	for _, m := range ms {
		if err := w.notif.Send(ctx, m.ID, false); err != nil {
			t.Fatal(err)
		}
	}
	ms = w.messages(t, b.ID)
	created, moved, reminders = byKind(ms, notifdomain.KindCreated), byKind(ms, notifdomain.KindRescheduled), byKind(ms, notifdomain.KindReminder)
	if created[0].Status != notifdomain.StatusSkipped || created[0].SkipReason != notifdomain.SkipChanged || moved[0].Status != notifdomain.StatusSent ||
		reminders[0].SkipReason != notifdomain.SkipChanged || reminders[1].Status != notifdomain.StatusSent {
		t.Errorf("stale then fresh: %+v", ms)
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
		if len(ms) == 2 && ms[0].Status == notifdomain.StatusSent && ms[1].Status == notifdomain.StatusPending { // the reminder waits
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

// TestWhatsAppClientOnPostgres (T7.2): the Meta client behind the service, against a fake Cloud
// API: Meta's id is stored; a recipient Meta refuses is FAILED with its code at once.
func TestWhatsAppClientOnPostgres(t *testing.T) {
	ctx := t.Context()
	w := newNotifWorld(t)
	reply := `{"messages":[{"id":"wamid.REAL1"}]}`
	status := http.StatusOK
	var sent struct {
		Template struct {
			Name       string                `json:"name"`
			Language   struct{ Code string } `json:"language"`
			Components []struct {
				Parameters []struct{ Text string } `json:"parameters"`
			} `json:"components"`
		} `json:"template"`
	}
	meta := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&sent)
		rw.WriteHeader(status)
		_, _ = io.WriteString(rw, reply)
	}))
	params := func() string {
		var ps []string
		for _, c := range sent.Template.Components {
			for _, p := range c.Parameters {
				ps = append(ps, p.Text)
			}
		}
		return sent.Template.Name + "/" + sent.Template.Language.Code + ": " + strings.Join(ps, " | ")
	}
	t.Cleanup(meta.Close)
	inserter, err := jobs.NewInsertClient(w.pool, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	w.notif = notifapp.NewService(notifapp.Deps{
		Store: notifpg.NewStore(w.pool, inserter), Appointments: w.sched,
		Senders: map[notifdomain.Channel]notifapp.Sender{notifdomain.ChannelWhatsApp: whatsapp.New(whatsapp.Config{
			BaseURL: meta.URL, GraphVersion: "v23.0", PhoneNumberID: "106540352242922", AccessToken: fakeMetaToken,
		})},
		Policy: notifapp.Policy{WhatsAppEnabled: true}, Clock: w.clk,
	})
	LinkNotifications(w.sched, w.notif)
	ana := w.customer(t)

	a, err := w.sched.Book(ctx, ana, w.in(w.at(9, 0), "meta-ana-0001"))
	if err != nil {
		t.Fatal(err)
	}
	m := w.messages(t, a.ID)[0]
	if err := w.notif.Send(ctx, m.ID, false); err != nil {
		t.Fatal(err)
	}
	if got := w.messages(t, a.ID)[0]; got.Status != notifdomain.StatusSent || got.ProviderMessageID != "wamid.REAL1" {
		t.Errorf("sent: %+v", got)
	}
	// T7.3: the approved template with the real names, the start in Bogota and in Spanish.
	if got := params(); !strings.HasPrefix(got, "cita_agendada/es: ") ||
		!strings.HasSuffix(got, " | Barbería Reservas | Corte | Juan | martes 5 de marzo, 9:00 a. m.") {
		t.Errorf("customer booking: %s", got)
	}

	// Booked by the staff (P40): the template with the app button; the customer is the one typed.
	manual, err := w.sched.BookManual(ctx, w.owner, w.biz, schedapp.ManualInput{
		ProfessionalID: w.pro, ServiceID: w.svc, Start: w.at(11, 0), Confirmed: true, IdempotencyKey: "meta-manual-0001",
		Customer: &schedapp.CustomerInput{FullName: "Laura Gómez", Phone: "3005550199"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.notif.Send(ctx, w.messages(t, manual.ID)[0].ID, false); err != nil {
		t.Fatal(err)
	}
	if got := params(); got != "cita_agendada_negocio/es: Laura Gómez | Barbería Reservas | Corte | Juan | martes 5 de marzo, 11:00 a. m." {
		t.Errorf("manual booking: %s", got)
	}

	status, reply = http.StatusBadRequest, `{"error":{"code":131030,"fbtrace_id":"X"}}`
	b, err := w.sched.Book(ctx, ana, w.in(w.at(10, 0), "meta-ana-0002"))
	if err != nil {
		t.Fatal(err)
	}
	m = w.messages(t, b.ID)[0]
	if err := w.notif.Send(ctx, m.ID, false); err != nil {
		t.Fatalf("a permanent failure is not retried: %v", err)
	}
	if got := w.messages(t, b.ID)[0]; got.Status != notifdomain.StatusFailed || got.ErrorCode != "RECIPIENT_NOT_ALLOWED" || got.Attempts != 1 {
		t.Errorf("refused: %+v", got)
	}
}

// fakeMetaToken is not a credential: the fake Cloud API above accepts anything.
const fakeMetaToken = "EAAfakeTOKENforTests0000"

// TestRemindersOnPostgres (T7.4): a pending booking queues its reminder in River for 24 h before
// (the default of the business); confirmed meanwhile, the reminder is dropped when due. A manual
// booking (confirmed) gets none.
func TestRemindersOnPostgres(t *testing.T) {
	ctx := t.Context()
	w := newNotifWorld(t)
	ana := w.customer(t)
	a, err := w.sched.Book(ctx, ana, w.in(w.at(9, 0), "recordar-ana-0001"))
	if err != nil {
		t.Fatal(err)
	}
	ms := w.messages(t, a.ID)
	if len(ms) != 2 || ms[1].Kind != notifdomain.KindReminder {
		t.Fatalf("messages %+v", ms)
	}
	due := w.at(9, 0).Add(-24 * time.Hour)
	var scheduled time.Time
	var state string
	if err := w.pool.QueryRow(ctx, `SELECT scheduled_at, state FROM river_job WHERE kind = 'notifications.send_message' AND args->>'message_id' = $1`,
		ms[1].ID.String()).Scan(&scheduled, &state); err != nil {
		t.Fatal(err)
	}
	if !scheduled.Equal(due) || !ms[1].SendAt.Equal(due) || state != "scheduled" {
		t.Errorf("reminder job at %s (%s), message due %s, want %s", scheduled, state, ms[1].SendAt, due)
	}

	if _, err := w.sched.CustomerChange(ctx, ana, a.ID, schedapp.ActionConfirm, ""); err != nil {
		t.Fatal(err)
	}
	if err := w.notif.Send(ctx, ms[1].ID, false); err != nil {
		t.Fatal(err)
	}
	if got := w.messages(t, a.ID)[1]; got.Status != notifdomain.StatusSkipped || got.SkipReason != notifdomain.SkipConfirmed {
		t.Errorf("confirmed meanwhile: %+v", got)
	}

	m, err := w.sched.BookManual(ctx, w.owner, w.biz, schedapp.ManualInput{
		ProfessionalID: w.pro, ServiceID: w.svc, Start: w.at(11, 0), Confirmed: true, IdempotencyKey: "recordar-manual-1",
		Customer: &schedapp.CustomerInput{FullName: "Laura", Phone: "3005550199"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ms := w.messages(t, m.ID); len(ms) != 1 || ms[0].Kind != notifdomain.KindCreatedByBusiness {
		t.Errorf("manual confirmed booking: %+v", ms)
	}
}
