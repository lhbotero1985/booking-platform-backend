package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	businesses "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/contract"
	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/domain"
	scheduling "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
)

var customerAccount = uuid.MustParse("01900000-0000-7000-8000-0000000000e1")

// fakeAuthz: roles by (business, user); admins by user.
type fakeAuthz struct {
	roles  map[[2]uuid.UUID]identity.Role
	admins map[uuid.UUID]bool
}

func (f fakeAuthz) RequireBusinessRole(_ context.Context, p identity.Principal, businessID uuid.UUID, allowed ...identity.Role) (identity.Role, error) {
	role, ok := f.roles[[2]uuid.UUID{businessID, p.UserID}]
	if !ok {
		return "", identity.ErrNotFound
	}
	for _, a := range allowed {
		if a == role {
			return role, nil
		}
	}
	return "", identity.ErrForbidden
}

func (f fakeAuthz) RequirePlatformAdmin(_ context.Context, p identity.Principal) error {
	if f.admins[p.UserID] {
		return nil
	}
	return identity.ErrForbidden
}

// prefsWorld: a customer with an account and its email, every channel on.
func prefsWorld(p application.Policy) emailWorld {
	p.EmailEnabled, p.WhatsAppEnabled = true, true
	w := newEmailWorld(p)
	w.appts.state.CustomerUserID, w.appts.state.CustomerEmail = customerAccount, "laura@negocio.co"
	w.rcpts.pros[pro] = "andres@barberia.co"
	return w
}

// sendAll records e and sends every message it planned (reminders included); it returns them by
// channel and recipient.
func (w emailWorld) sendAll(t *testing.T, e scheduling.AppointmentEvent) map[string]domain.Message {
	t.Helper()
	ctx := context.Background()
	if err := w.svc.AppointmentEventRecorded(ctx, e); err != nil {
		t.Fatal(err)
	}
	out := map[string]domain.Message{}
	for _, m := range w.st.Messages() {
		if m.EventID != e.ID {
			continue
		}
		if err := w.svc.Send(ctx, m.ID, false); err != nil {
			t.Fatal(err)
		}
		key := string(m.Channel) + " " + string(m.RecipientRole)
		if m.Kind == domain.KindReminder {
			key += " reminder"
		}
		out[key] = w.msg(m.ID)
	}
	return out
}

func skipped(m domain.Message) domain.SkipReason {
	if m.Status != domain.StatusSkipped {
		return domain.SkipReason("sent:" + string(m.Status))
	}
	return m.SkipReason
}

// TestPreferencesAtSend (T7.9): the customer's channels and reminders, the specialist's emails;
// read when sending, so a reminder already planned follows a later change.
func TestPreferencesAtSend(t *testing.T) {
	ctx := context.Background()
	w := prefsWorld(application.Policy{})
	w.rs.pol.RemindersEnabled, w.rs.pol.ReminderHoursBefore = true, 24
	w.appts.state.Status = scheduling.StatusPendingConfirmation
	if err := w.st.SetCustomerPreferences(ctx, customerAccount, domain.CustomerPreferences{Email: true, Reminders: false}, now); err != nil {
		t.Fatal(err)
	}
	if err := w.st.SetProfessionalEmailEnabled(ctx, biz, pro, false, now); err != nil {
		t.Fatal(err)
	}
	e := event("CREATED", "CUSTOMER")
	e.Status = scheduling.StatusPendingConfirmation
	got := w.sendAll(t, e)
	for key, want := range map[string]domain.SkipReason{
		"WHATSAPP CUSTOMER":          domain.SkipCustomerOptedOut,
		"EMAIL CUSTOMER":             "sent:SENT",
		"EMAIL PROFESSIONAL":         domain.SkipProfessionalOptedOut,
		"WHATSAPP CUSTOMER reminder": domain.SkipCustomerOptedOut,
		"EMAIL CUSTOMER reminder":    domain.SkipRemindersOff,
	} {
		if m, ok := got[key]; !ok || skipped(m) != want {
			t.Errorf("%s: %s, want %s", key, skipped(m), want)
		}
	}

	// Defaults: a customer who never chose gets everything; one booked by hand without an
	// account too, whatever another account chose.
	for _, account := range []uuid.UUID{uuid.Must(uuid.NewV7()), uuid.Nil} {
		w := prefsWorld(application.Policy{})
		w.appts.state.CustomerUserID = account
		_ = w.st.SetCustomerPreferences(ctx, customerAccount, domain.CustomerPreferences{Email: true}, now)
		got := w.sendAll(t, event("CREATED", "CUSTOMER"))
		for _, key := range []string{"WHATSAPP CUSTOMER", "EMAIL CUSTOMER", "EMAIL PROFESSIONAL"} {
			if skipped(got[key]) != "sent:SENT" {
				t.Errorf("account %s, %s: %s", account, key, skipped(got[key]))
			}
		}
	}
}

// TestWhatsAppCaps (T7.9, P16): the business's monthly cap (its own or the default), 0 = none
// for the business, a default of 0 = no cap; the platform's daily cap. Emails always go.
func TestWhatsAppCaps(t *testing.T) {
	ctx := context.Background()
	sendTwo := func(w emailWorld) (first, second domain.Message, mail domain.Message) {
		a := w.sendAll(t, event("CREATED", "CUSTOMER"))
		b := w.sendAll(t, event("RESCHEDULED", "CUSTOMER"))
		return a["WHATSAPP CUSTOMER"], b["WHATSAPP CUSTOMER"], b["EMAIL CUSTOMER"]
	}
	w := prefsWorld(application.Policy{WhatsAppMonthlyLimit: 1})
	a, b, mail := sendTwo(w)
	if skipped(a) != "sent:SENT" || skipped(b) != domain.SkipMonthlyLimit || skipped(mail) != "sent:SENT" {
		t.Errorf("default cap 1: %s %s, email %s", skipped(a), skipped(b), skipped(mail))
	}
	// The business's own cap wins over the default.
	w = prefsWorld(application.Policy{WhatsAppMonthlyLimit: 1})
	_ = w.st.SetWhatsAppMonthlyLimit(ctx, biz, 5, now)
	if a, b, _ := sendTwo(w); skipped(a) != "sent:SENT" || skipped(b) != "sent:SENT" {
		t.Errorf("own cap 5: %s %s", skipped(a), skipped(b))
	}
	w = prefsWorld(application.Policy{WhatsAppMonthlyLimit: 500})
	_ = w.st.SetWhatsAppMonthlyLimit(ctx, biz, 0, now)
	if a, _, _ := sendTwo(w); skipped(a) != domain.SkipMonthlyLimit {
		t.Errorf("own cap 0: %s", skipped(a))
	}
	// A default of 0: no cap in this environment.
	w = prefsWorld(application.Policy{})
	if a, b, _ := sendTwo(w); skipped(a) != "sent:SENT" || skipped(b) != "sent:SENT" {
		t.Errorf("no default cap: %s %s", skipped(a), skipped(b))
	}
	w = prefsWorld(application.Policy{WhatsAppDailyLimit: 1})
	if a, b, _ := sendTwo(w); skipped(a) != "sent:SENT" || skipped(b) != domain.SkipDailyLimit {
		t.Errorf("daily cap 1: %s %s", skipped(a), skipped(b))
	}
}

func prefsService(w emailWorld, authz fakeAuthz) *application.Service {
	return application.NewService(application.Deps{
		Store: w.st, Appointments: w.appts, Recipients: w.rcpts, Reminders: w.rs, Authz: authz,
		Senders: map[domain.Channel]application.Sender{domain.ChannelEmail: w.mails}, Clock: clock.NewFixed(now),
		Policy: application.Policy{EmailEnabled: true, WhatsAppMonthlyLimit: 500},
	})
}

// TestPreferenceUseCases: the customer's own preferences; the specialist's emails by the owner
// or the specialist; the cap by a platform administrator.
func TestPreferenceUseCases(t *testing.T) {
	ctx := context.Background()
	owner, juan, other, stranger, admin := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	w := prefsWorld(application.Policy{})
	w.rcpts.proAccounts = map[uuid.UUID]uuid.UUID{pro: juan}
	svc := prefsService(w, fakeAuthz{
		roles: map[[2]uuid.UUID]identity.Role{
			{biz, owner}: identity.RoleOwner, {biz, juan}: identity.RoleProfessional, {biz, other}: identity.RoleProfessional,
		},
		admins: map[uuid.UUID]bool{admin: true},
	})
	me := identity.Principal{UserID: customerAccount}
	if p, err := svc.MyPreferences(ctx, me); err != nil || p != domain.DefaultCustomerPreferences() {
		t.Errorf("defaults: %+v %v", p, err)
	}
	if err := svc.SetMyPreferences(ctx, me, domain.CustomerPreferences{Reminders: true}); !errors.Is(err, domain.ErrNoChannel) {
		t.Errorf("no channel: %v", err)
	}
	want := domain.CustomerPreferences{WhatsApp: true, Reminders: false}
	if err := svc.SetMyPreferences(ctx, me, want); err != nil {
		t.Fatal(err)
	}
	if p, _ := svc.MyPreferences(ctx, me); p != want {
		t.Errorf("stored %+v", p)
	}

	off := false
	for name, tc := range map[string]struct {
		who  uuid.UUID
		pro  uuid.UUID
		want error
	}{
		"owner":              {owner, pro, nil},
		"the specialist":     {juan, pro, nil},
		"another specialist": {other, pro, identity.ErrForbidden},
		"not a member":       {stranger, pro, identity.ErrNotFound},
		"unknown profile":    {owner, uuid.Must(uuid.NewV7()), identity.ErrNotFound},
	} {
		p := identity.Principal{UserID: tc.who}
		if _, err := svc.ProfessionalEmails(ctx, p, biz, tc.pro); !errors.Is(err, tc.want) {
			t.Errorf("%s reads: %v", name, err)
		}
		if err := svc.SetProfessionalEmails(ctx, p, biz, tc.pro, &off); !errors.Is(err, tc.want) {
			t.Errorf("%s sets: %v", name, err)
		}
	}
	if on, _ := svc.ProfessionalEmails(ctx, identity.Principal{UserID: juan}, biz, pro); on {
		t.Error("still on")
	}
	if err := svc.SetProfessionalEmails(ctx, identity.Principal{UserID: stranger}, biz, pro, nil); !errors.Is(err, identity.ErrNotFound) {
		t.Errorf("a stranger without a body learns nothing: %v", err)
	}
	if err := svc.SetProfessionalEmails(ctx, identity.Principal{UserID: juan}, biz, pro, nil); !errors.Is(err, application.ErrEmailRequired) {
		t.Errorf("no body: %v", err)
	}

	// The cap: administrators only, an existing business, 0–100000.
	ad := identity.Principal{UserID: admin}
	if l, err := svc.BusinessWhatsAppLimit(ctx, ad, biz); err != nil || l.MonthlyLimit != 500 || !l.Default || !l.Limited || l.UsedThisMonth != 0 ||
		l.MonthStart.Day() != 1 {
		t.Errorf("default: %+v %v", l, err)
	}
	if _, err := svc.BusinessWhatsAppLimit(ctx, identity.Principal{UserID: owner}, biz); !errors.Is(err, identity.ErrForbidden) {
		t.Errorf("the owner: %v", err)
	}
	for _, bad := range []int{-1, 100001} {
		if err := svc.SetBusinessWhatsAppLimit(ctx, ad, biz, &bad); !errors.Is(err, application.ErrInvalidLimit) {
			t.Errorf("%d: %v", bad, err)
		}
	}
	limit := 50
	if err := svc.SetBusinessWhatsAppLimit(ctx, ad, biz, &limit); err != nil {
		t.Fatal(err)
	}
	if l, _ := svc.BusinessWhatsAppLimit(ctx, ad, biz); l.MonthlyLimit != 50 || l.Default {
		t.Errorf("own cap: %+v", l)
	}
	w.rs.err = businesses.ErrBusinessNotFound
	if _, err := svc.BusinessWhatsAppLimit(ctx, ad, uuid.Must(uuid.NewV7())); !errors.Is(err, identity.ErrNotFound) {
		t.Errorf("unknown business: %v", err)
	}
}
