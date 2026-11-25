//go:build integration

package app

import (
	"testing"

	"github.com/google/uuid"

	identitycontract "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	notifapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/application"
	notifdomain "github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/domain"
)

// TestPreferencesOnPostgres (T7.9): the preferences and the cap with the real SQL; the counter
// of sent WhatsApp messages; the constraints (at least one channel, an existing profile).
func TestPreferencesOnPostgres(t *testing.T) {
	ctx := t.Context()
	w := newNotifWorld(t, func(w *notifWorld, d *notifapp.Deps) {
		d.Policy.WhatsAppMonthlyLimit = 1
		d.Authz, d.Recipients = w.identity, NewEmailRecipients(w.identity, w.profs)
	})
	sendWhatsApp := func(id uuid.UUID) notifdomain.Message {
		t.Helper()
		for _, m := range onWhatsApp(w.messages(t, id)) {
			if m.Kind == notifdomain.KindCreated {
				if err := w.notif.Send(ctx, m.ID, false); err != nil {
					t.Fatal(err)
				}
				for _, got := range w.messages(t, id) {
					if got.ID == m.ID {
						return got
					}
				}
			}
		}
		t.Fatal("no WhatsApp message")
		return notifdomain.Message{}
	}
	ana := w.customer(t)
	a, err := w.sched.Book(ctx, ana, w.in(w.at(9, 0), "preferencias-1"))
	if err != nil {
		t.Fatal(err)
	}
	if m := sendWhatsApp(a.ID); m.Status != notifdomain.StatusSent {
		t.Fatalf("first WhatsApp: %s %s", m.Status, m.SkipReason)
	}
	b, err := w.sched.Book(ctx, ana, w.in(w.at(10, 0), "preferencias-2"))
	if err != nil {
		t.Fatal(err)
	}
	if m := sendWhatsApp(b.ID); m.SkipReason != notifdomain.SkipMonthlyLimit {
		t.Errorf("over the default cap of 1: %s %s", m.Status, m.SkipReason)
	}

	// The platform raises the business's cap; the customer turns WhatsApp off.
	adminID, _ := activeUser(t, w.pool, "ACTIVE")
	admin := identitycontract.Principal{UserID: adminID, IsPlatformAdmin: true} // as the middleware loads it
	limit := 10
	if err := w.notif.SetBusinessWhatsAppLimit(ctx, admin, w.biz, &limit); err != nil {
		t.Fatal(err)
	}
	if l, err := w.notif.BusinessWhatsAppLimit(ctx, admin, w.biz); err != nil || l.MonthlyLimit != 10 || l.Default || l.UsedThisMonth != 1 {
		t.Errorf("usage: %+v %v", l, err)
	}
	if err := w.notif.SetMyPreferences(ctx, ana, notifdomain.CustomerPreferences{Email: true, Reminders: true}); err != nil {
		t.Fatal(err)
	}
	c, err := w.sched.Book(ctx, ana, w.in(w.at(11, 0), "preferencias-3"))
	if err != nil {
		t.Fatal(err)
	}
	if m := sendWhatsApp(c.ID); m.SkipReason != notifdomain.SkipCustomerOptedOut {
		t.Errorf("WhatsApp off: %s %s", m.Status, m.SkipReason)
	}
	if p, err := w.notif.MyPreferences(ctx, ana); err != nil || p.WhatsApp || !p.Email {
		t.Errorf("read back: %+v %v", p, err)
	}

	// The specialist's emails: stored per profile.
	off := false
	if err := w.notif.SetProfessionalEmails(ctx, w.owner, w.biz, w.pro, &off); err != nil {
		t.Fatal(err)
	}
	if on, err := w.notif.ProfessionalEmails(ctx, w.barber, w.biz, w.pro); err != nil || on {
		t.Errorf("the specialist reads: %v %v", on, err)
	}

	// Constraints.
	if _, err := w.pool.Exec(ctx, `UPDATE customer_notification_preferences SET whatsapp = false, email = false WHERE user_id = $1`, ana.UserID); err == nil {
		t.Error("a customer without channels was stored")
	}
	if _, err := w.pool.Exec(ctx, `INSERT INTO professional_notification_preferences VALUES ($1, $2, false, now())`, w.biz, uuid.Must(uuid.NewV7())); err == nil {
		t.Error("preferences of an unknown profile were stored")
	}
	if _, err := w.pool.Exec(ctx, `INSERT INTO business_whatsapp_limits VALUES ($1, 100001, now()) ON CONFLICT (business_id) DO UPDATE SET monthly_limit = 100001`, w.biz); err == nil {
		t.Error("a cap out of range was stored")
	}
}
