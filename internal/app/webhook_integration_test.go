//go:build integration

package app

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	notifapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/application"
	notifdomain "github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/domain"
	notifpg "github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/infrastructure/postgres"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/infrastructure/whatsapp"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/jobs"
)

// buttonsMeta is a fake Cloud API that keeps the button payloads of the last template.
type buttonsMeta struct {
	mu       sync.Mutex
	sends    atomic.Int64
	payloads []string
}

func (f *buttonsMeta) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Template *struct {
			Components []struct {
				Type       string `json:"type"`
				Parameters []struct {
					Payload string `json:"payload"`
				} `json:"parameters"`
			} `json:"components"`
		} `json:"template"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Template != nil {
		var ps []string
		for _, c := range req.Template.Components {
			if c.Type == "button" {
				ps = append(ps, c.Parameters[0].Payload)
			}
		}
		f.mu.Lock()
		f.payloads = ps
		f.mu.Unlock()
	}
	_, _ = io.WriteString(w, `{"messages":[{"id":"wamid.B`+strconv.FormatInt(f.sends.Add(1), 10)+`"}]}`)
}

type buttonsWorld struct {
	notifWorld
	meta *buttonsMeta
}

func newButtonsWorld(t *testing.T) buttonsWorld {
	t.Helper()
	w := buttonsWorld{notifWorld: newNotifWorld(t), meta: &buttonsMeta{}}
	srv := httptest.NewServer(w.meta)
	t.Cleanup(srv.Close)
	inserter, err := jobs.NewInsertClient(w.pool, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	client := whatsapp.New(whatsapp.Config{BaseURL: srv.URL, GraphVersion: "v23.0", PhoneNumberID: "106540352242922", AccessToken: fakeMetaToken})
	w.notif = notifapp.NewService(notifapp.Deps{
		Store: notifpg.NewStore(w.pool, inserter), Appointments: w.sched, Replies: w.sched, TextSender: client, Reminders: w.bizSvc,
		Senders: map[notifdomain.Channel]notifapp.Sender{notifdomain.ChannelWhatsApp: client},
		Policy:  notifapp.Policy{WhatsAppEnabled: true}, Clock: w.clk,
	})
	LinkNotifications(w.sched, w.notif)
	return w
}

// remind books a pending appointment, sends its reminder and returns the appointment, the
// reminder and its buttons (confirm, cancel).
func (w buttonsWorld) remind(t *testing.T, hour int, key string) (uuid.UUID, notifdomain.Message, []string) {
	t.Helper()
	a, err := w.sched.Book(t.Context(), w.customer(t), w.in(w.at(hour, 0), key))
	if err != nil {
		t.Fatal(err)
	}
	r := byKind(onWhatsApp(w.messages(t, a.ID)), notifdomain.KindReminder)[0]
	if err := w.notif.Send(t.Context(), r.ID, false); err != nil {
		t.Fatal(err)
	}
	w.meta.mu.Lock()
	defer w.meta.mu.Unlock()
	if len(w.meta.payloads) != 2 {
		t.Fatalf("buttons %v", w.meta.payloads)
	}
	return a.ID, onWhatsApp(w.messages(t, a.ID))[1], append([]string(nil), w.meta.payloads...)
}

func (w buttonsWorld) status(t *testing.T, id uuid.UUID) string {
	t.Helper()
	st, err := w.sched.AppointmentState(t.Context(), w.biz, id)
	if err != nil {
		t.Fatal(err)
	}
	return st.Status
}

func (w buttonsWorld) replies(t *testing.T) []string {
	t.Helper()
	rows, err := w.pool.Query(t.Context(), `SELECT args->>'text' FROM river_job WHERE kind = 'notifications.send_reply' ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	return out
}

func button(payload string) notifdomain.WebhookItem {
	return notifdomain.WebhookItem{Kind: notifdomain.WebhookButton, EventID: "message:" + uuid.NewString(), From: "+573001234567", Payload: payload}
}

// TestWhatsAppButtonsOnPostgres (T7.5): the webhook stores each item once; "Confirmar cita"
// confirms the appointment and answers; the other button is then spent; delivery statuses move
// the reminder forward.
func TestWhatsAppButtonsOnPostgres(t *testing.T) {
	ctx := t.Context()
	w := newButtonsWorld(t)
	id, reminder, buttons := w.remind(t, 9, "boton-ana-0001")
	if reminder.Status != notifdomain.StatusSent {
		t.Fatalf("reminder %+v", reminder)
	}
	press := button(buttons[0])
	items := []notifdomain.WebhookItem{
		{Kind: notifdomain.WebhookStatus, EventID: "status:" + reminder.ProviderMessageID + ":READ", ProviderMessageID: reminder.ProviderMessageID, Status: notifdomain.StatusRead},
		press,
	}
	for range 2 { // Meta retries the same delivery
		if err := w.notif.ReceiveWebhook(ctx, items); err != nil {
			t.Fatal(err)
		}
	}
	if n := countRows(t, w.notifWorld, `SELECT count(*) FROM river_job WHERE kind = 'notifications.process_webhook_item'`); n != 2 {
		t.Errorf("%d webhook jobs, want 2 (no duplicates)", n)
	}
	for _, it := range items {
		if err := w.notif.ProcessWebhookItem(ctx, it); err != nil {
			t.Fatal(err)
		}
	}
	if got := w.status(t, id); got != "CONFIRMED" {
		t.Errorf("after Confirmar: %s", got)
	}
	if got := onWhatsApp(w.messages(t, id))[1]; got.Status != notifdomain.StatusRead {
		t.Errorf("reminder status %s", got.Status)
	}
	if rs := w.replies(t); len(rs) != 1 || !strings.Contains(rs[0], "quedó confirmada") {
		t.Errorf("replies %q", rs)
	}
	// "Cancelar cita" of the same reminder is spent: nothing changes.
	if err := w.notif.ProcessWebhookItem(ctx, button(buttons[1])); err != nil {
		t.Fatal(err)
	}
	if got, rs := w.status(t, id), w.replies(t); got != "CONFIRMED" || len(rs) != 2 || !strings.Contains(rs[1], "ya no es válido") {
		t.Errorf("second button: %s %q", got, rs)
	}
	// The raw tokens are not stored.
	if n := countRows(t, w.notifWorld, `SELECT count(*) FROM appointment_action_tokens WHERE token_hash = convert_to('`+buttons[0]+`', 'UTF8')`); n != 0 {
		t.Error("a raw token is stored")
	}
}

// TestWhatsAppButtonsConcurrently (T7.5): "Confirmar" and "Cancelar" of one reminder pressed at
// the same time: exactly one applies, the other answers that the button is no longer valid.
func TestWhatsAppButtonsConcurrently(t *testing.T) {
	ctx := t.Context()
	w := newButtonsWorld(t)
	for round := range 5 {
		id, _, buttons := w.remind(t, 9+round%3, "boton-carrera-"+strconv.Itoa(round))
		before := len(w.replies(t))
		var wg sync.WaitGroup
		errs := make(chan error, 2)
		for _, p := range buttons {
			wg.Add(1)
			go func() {
				defer wg.Done()
				errs <- w.notif.ProcessWebhookItem(context.WithoutCancel(ctx), button(p))
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("round %d: %v", round, err)
			}
		}
		rs := w.replies(t)[before:]
		valid := 0
		for _, r := range rs {
			if !strings.Contains(r, "ya no es válido") {
				valid++
			}
		}
		st := w.status(t, id)
		if len(rs) != 2 || valid != 1 || (st != "CONFIRMED" && st != "CANCELLED") { //nolint:misspell // stored value
			t.Fatalf("round %d: status %s, replies %q", round, st, rs)
		}
		if st == "CANCELLED" { //nolint:misspell // stored value
			continue
		}
		// Free the slot for the next round.
		if _, err := w.sched.StaffChange(ctx, w.owner, w.biz, id, "cancel", "Prueba"); err != nil {
			t.Fatal(err)
		}
	}
}
