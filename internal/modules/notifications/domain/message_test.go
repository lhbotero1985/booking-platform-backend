package domain_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/domain"
)

var (
	start = time.Date(2030, 3, 5, 15, 0, 0, 0, time.UTC)
	pro   = uuid.MustParse("01900000-0000-7000-8000-000000000001")
)

func change(typ, actor string) domain.Change {
	return domain.Change{
		EventID: uuid.Must(uuid.NewV7()), BusinessID: uuid.Must(uuid.NewV7()), AppointmentID: uuid.Must(uuid.NewV7()),
		ProfessionalID: pro, Type: typ, ActorRole: actor, Start: start, CustomerPhone: "+573001234567",
		At: start.Add(-48 * time.Hour),
	}
}

func TestPlan(t *testing.T) {
	cases := []struct {
		typ, actor string
		want       domain.Kind // "" = no message
	}{
		{"CREATED", "CUSTOMER", domain.KindCreated},
		{"CREATED", "BUSINESS", domain.KindCreatedByBusiness}, // P40: booked by the staff
		{"RESCHEDULED", "CUSTOMER", domain.KindRescheduled},
		{"RESCHEDULED", "BUSINESS", domain.KindRescheduled},
		{"REASSIGNED", "BUSINESS", domain.KindReassigned},
		{"CANCELLED", "BUSINESS", domain.KindCancelled}, //nolint:misspell // stored value
		{"CANCELLED", "CUSTOMER", ""},                   //nolint:misspell // the customer knows
		{"CONFIRMED", "CUSTOMER", ""},
		{"CONFIRMED", "BUSINESS", ""},
		{"STARTED", "BUSINESS", ""},
		{"COMPLETED", "BUSINESS", ""},
		{"NO_SHOW", "BUSINESS", ""},
		{"UNKNOWN", "BUSINESS", ""},
	}
	for _, c := range cases {
		ch := change(c.typ, c.actor)
		id := uuid.Must(uuid.NewV7())
		got := domain.Plan(ch, domain.Reminders{}, func() uuid.UUID { return id })
		if c.want == "" {
			if len(got) != 0 {
				t.Errorf("%s by %s: %v, want none", c.typ, c.actor, got)
			}
			continue
		}
		if len(got) != 1 {
			t.Fatalf("%s by %s: %d messages", c.typ, c.actor, len(got))
		}
		m := got[0]
		if !m.SendAt.Equal(ch.At) {
			t.Errorf("%s: due %s, want at once", c.typ, m.SendAt)
		}
		if m.ID != id || m.Kind != c.want || m.Channel != domain.ChannelWhatsApp || m.RecipientRole != domain.RecipientCustomer ||
			m.Recipient != ch.CustomerPhone || m.EventID != ch.EventID || m.BusinessID != ch.BusinessID || m.AppointmentID != ch.AppointmentID ||
			m.ProfessionalID != pro || !m.Start.Equal(start) || m.Status != domain.StatusPending || !m.CreatedAt.Equal(ch.At) {
			t.Errorf("%s by %s: %+v", c.typ, c.actor, m)
		}
	}
}

const canceled = "CANCELLED" //nolint:misspell // value of the scheduling module

func TestStale(t *testing.T) {
	now := start.Add(-time.Hour)
	msg := func(k domain.Kind) domain.Message {
		return domain.Message{Kind: k, Recipient: "+573001234567", ProfessionalID: pro, Start: start}
	}
	same := domain.Appointment{Status: "CONFIRMED", ProfessionalID: pro, Start: start}
	other := uuid.Must(uuid.NewV7())
	cases := []struct {
		name string
		m    domain.Message
		cur  domain.Appointment
		now  time.Time
		want domain.SkipReason
	}{
		{"still valid", msg(domain.KindCreated), same, now, ""},
		{"confirmed meanwhile is still valid", msg(domain.KindRescheduled), domain.Appointment{Status: "PENDING_CONFIRMATION", ProfessionalID: pro, Start: start}, now, ""},
		{"no recipient", domain.Message{Kind: domain.KindCreated, ProfessionalID: pro, Start: start}, same, now, domain.SkipNoRecipient},
		{"started", msg(domain.KindCreated), same, start, domain.SkipPassed},
		{"canceled meanwhile", msg(domain.KindCreated), domain.Appointment{Status: canceled, ProfessionalID: pro, Start: start}, now, domain.SkipCancelled},
		{"moved meanwhile", msg(domain.KindCreated), domain.Appointment{Status: "CONFIRMED", ProfessionalID: pro, Start: start.Add(time.Hour)}, now, domain.SkipChanged},
		{"reassigned meanwhile", msg(domain.KindRescheduled), domain.Appointment{Status: "CONFIRMED", ProfessionalID: other, Start: start}, now, domain.SkipChanged},
		{"cancellation notice", msg(domain.KindCancelled), domain.Appointment{Status: canceled, ProfessionalID: pro, Start: start}, now, ""},
		{"cancellation of a past appointment", msg(domain.KindCancelled), domain.Appointment{Status: canceled, ProfessionalID: pro, Start: start}, start.Add(time.Minute), domain.SkipPassed},
	}
	for _, c := range cases {
		if got := domain.Stale(c.m, c.cur, c.now); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

func TestMaskPhone(t *testing.T) {
	for in, want := range map[string]string{
		"+573001234567": "+57••••••4567",
		"+12125550100":  "+12•••••0100",
		"+1234567":      "+12•4567",
		"1234":          "••••",
		"":              "",
	} {
		if got := domain.MaskPhone(in); got != want {
			t.Errorf("MaskPhone(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWhenES(t *testing.T) {
	bogota, _ := time.LoadLocation("America/Bogota")
	cases := []struct {
		utc  time.Time
		loc  *time.Location
		want string
	}{
		{time.Date(2026, 12, 2, 15, 0, 0, 0, time.UTC), bogota, "miércoles 2 de diciembre, 10:00 a. m."},
		{time.Date(2026, 12, 2, 17, 0, 0, 0, time.UTC), bogota, "miércoles 2 de diciembre, 12:00 p. m."},
		{time.Date(2026, 12, 2, 21, 30, 0, 0, time.UTC), bogota, "miércoles 2 de diciembre, 4:30 p. m."},
		{time.Date(2027, 1, 1, 5, 5, 0, 0, time.UTC), bogota, "viernes 1 de enero, 12:05 a. m."},     // midnight local
		{time.Date(2026, 12, 2, 2, 0, 0, 0, time.UTC), bogota, "martes 1 de diciembre, 9:00 p. m."},  // the day before locally
		{time.Date(2026, 9, 6, 23, 59, 0, 0, time.UTC), nil, "domingo 6 de septiembre, 11:59 p. m."}, // no zone: UTC
	}
	for _, c := range cases {
		if got := (domain.Content{Start: c.utc, Location: c.loc}).WhenES(); got != c.want {
			t.Errorf("%s: %q, want %q", c.utc, got, c.want)
		}
	}
}

// TestPlanReminder (T7.4, decisions of 2026-11-25): a reminder only while the appointment is
// pending confirmation, r.Before the start, or at once when that moment already passed.
func TestPlanReminder(t *testing.T) {
	day := domain.Reminders{Enabled: true, Before: 24 * time.Hour}
	id := func() uuid.UUID { return uuid.Must(uuid.NewV7()) }
	plan := func(typ, actor, status string, at time.Time, r domain.Reminders) []domain.Message {
		c := change(typ, actor)
		c.Status, c.At = status, at
		return domain.Plan(c, r, id)
	}
	reminder := func(ms []domain.Message) *domain.Message {
		for i := range ms {
			if ms[i].Kind == domain.KindReminder {
				return &ms[i]
			}
		}
		return nil
	}
	twoDaysBefore := start.Add(-48 * time.Hour)

	ms := plan("CREATED", "CUSTOMER", "PENDING_CONFIRMATION", twoDaysBefore, day)
	if r := reminder(ms); len(ms) != 2 || ms[0].Kind != domain.KindCreated || r == nil || !r.SendAt.Equal(start.Add(-24*time.Hour)) ||
		r.Recipient != "+573001234567" || !r.Start.Equal(start) || r.ID == ms[0].ID {
		t.Fatalf("pending, two days before: %+v", ms)
	}
	// Booked inside the window: the reminder goes at once (user's decision).
	threeHoursBefore := start.Add(-3 * time.Hour)
	if r := reminder(plan("CREATED", "CUSTOMER", "PENDING_CONFIRMATION", threeHoursBefore, day)); r == nil || !r.SendAt.Equal(threeHoursBefore) {
		t.Errorf("inside the window: %+v", r)
	}
	// Moved or reassigned and still pending: a reminder for the new time.
	for _, typ := range []string{"RESCHEDULED", "REASSIGNED"} {
		if reminder(plan(typ, "BUSINESS", "PENDING_CONFIRMATION", twoDaysBefore, day)) == nil {
			t.Errorf("%s pending: no reminder", typ)
		}
	}
	// No reminder: confirmed (manual bookings are), disabled, canceled, already started.
	none := []struct {
		name, typ, actor, status string
		at                       time.Time
		r                        domain.Reminders
	}{
		{"confirmed", "CREATED", "BUSINESS", "CONFIRMED", twoDaysBefore, day},
		{"disabled", "CREATED", "CUSTOMER", "PENDING_CONFIRMATION", twoDaysBefore, domain.Reminders{Before: 24 * time.Hour}},
		{"canceled", "CANCELLED", "BUSINESS", "CANCELLED", twoDaysBefore, day}, //nolint:misspell // stored value
		{"confirmation", "CONFIRMED", "CUSTOMER", "PENDING_CONFIRMATION", twoDaysBefore, day},
		{"after the start", "CREATED", "BUSINESS", "PENDING_CONFIRMATION", start.Add(time.Minute), day},
	}
	for _, c := range none {
		if r := reminder(plan(c.typ, c.actor, c.status, c.at, c.r)); r != nil {
			t.Errorf("%s: reminder %+v", c.name, r)
		}
	}
}

func TestStaleReminder(t *testing.T) {
	m := domain.Message{Kind: domain.KindReminder, Recipient: "+573001234567", ProfessionalID: pro, Start: start}
	now := start.Add(-24 * time.Hour)
	for status, want := range map[string]domain.SkipReason{
		"PENDING_CONFIRMATION": "",
		"CONFIRMED":            domain.SkipConfirmed,
		canceled:               domain.SkipCancelled,
	} {
		if got := domain.Stale(m, domain.Appointment{Status: status, ProfessionalID: pro, Start: start}, now); got != want {
			t.Errorf("%s: %q, want %q", status, got, want)
		}
	}
	if got := domain.Stale(m, domain.Appointment{Status: "PENDING_CONFIRMATION", ProfessionalID: pro, Start: start.Add(time.Hour)}, now); got != domain.SkipChanged {
		t.Errorf("moved: %q", got)
	}
}
