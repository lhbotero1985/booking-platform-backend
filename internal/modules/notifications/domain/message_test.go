package domain_test

import (
	"errors"
	"slices"
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

// keys describes each message as kind/channel/recipient role.
func keys(ms []domain.Message) []string {
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, string(m.Kind)+"/"+string(m.Channel)+"/"+string(m.RecipientRole))
	}
	return out
}

// TestPlan (T7.1, T7.6): to the customer by WhatsApp and email; to the professional by email only
// what the customer did; a customer who cancels gets an email receipt, not a WhatsApp.
func TestPlan(t *testing.T) {
	const (
		wa     = "/WHATSAPP/CUSTOMER"
		email  = "/EMAIL/CUSTOMER"
		barber = "/EMAIL/PROFESSIONAL"
	)
	cases := []struct {
		typ, actor string
		want       []string
	}{
		{"CREATED", "CUSTOMER", []string{"APPOINTMENT_CREATED" + wa, "APPOINTMENT_CREATED" + email, "APPOINTMENT_CREATED" + barber}},
		{"CREATED", "BUSINESS", []string{"APPOINTMENT_CREATED_BY_BUSINESS" + wa, "APPOINTMENT_CREATED_BY_BUSINESS" + email}}, // P40
		{"RESCHEDULED", "CUSTOMER", []string{"APPOINTMENT_RESCHEDULED" + wa, "APPOINTMENT_RESCHEDULED" + email, "APPOINTMENT_RESCHEDULED" + barber}},
		{"RESCHEDULED", "BUSINESS", []string{"APPOINTMENT_RESCHEDULED" + wa, "APPOINTMENT_RESCHEDULED" + email}},
		{"REASSIGNED", "BUSINESS", []string{"APPOINTMENT_REASSIGNED" + wa, "APPOINTMENT_REASSIGNED" + email}},
		{"CANCELLED", "BUSINESS", []string{"APPOINTMENT_CANCELLED" + wa, "APPOINTMENT_CANCELLED" + email}},                             //nolint:misspell // stored value
		{"CANCELLED", "CUSTOMER", []string{"APPOINTMENT_CANCELLED_BY_CUSTOMER" + email, "APPOINTMENT_CANCELLED_BY_CUSTOMER" + barber}}, //nolint:misspell // stored value
		{"CONFIRMED", "CUSTOMER", nil},
		{"CONFIRMED", "BUSINESS", nil},
		{"STARTED", "BUSINESS", nil},
		{"COMPLETED", "BUSINESS", nil},
		{"NO_SHOW", "BUSINESS", nil},
		{"UNKNOWN", "CUSTOMER", nil},
	}
	for _, c := range cases {
		ch := change(c.typ, c.actor)
		got := domain.Plan(ch, domain.Reminders{}, func() uuid.UUID { return uuid.Must(uuid.NewV7()) })
		if k := keys(got); !slices.Equal(k, c.want) {
			t.Errorf("%s by %s: %v, want %v", c.typ, c.actor, k, c.want)
			continue
		}
		ids := map[uuid.UUID]bool{}
		for _, m := range got {
			ids[m.ID] = true
			to := ""
			if m.Channel == domain.ChannelWhatsApp {
				to = ch.CustomerPhone // emails: resolved when sent
			}
			if !m.SendAt.Equal(ch.At) || m.Recipient != to || m.EventID != ch.EventID || m.BusinessID != ch.BusinessID ||
				m.AppointmentID != ch.AppointmentID || m.ProfessionalID != pro || !m.Start.Equal(start) || m.Status != domain.StatusPending ||
				!m.CreatedAt.Equal(ch.At) {
				t.Errorf("%s by %s: %+v", c.typ, c.actor, m)
			}
		}
		if len(ids) != len(got) {
			t.Errorf("%s by %s: repeated ids", c.typ, c.actor)
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
		{"receipt of the customer's cancellation", msg(domain.KindCancelledByCustomer), domain.Appointment{Status: canceled, ProfessionalID: pro, Start: start}, now, ""},
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
			if ms[i].Kind == domain.KindReminder && ms[i].Channel == domain.ChannelWhatsApp {
				return &ms[i]
			}
		}
		return nil
	}
	twoDaysBefore := start.Add(-48 * time.Hour)

	ms := plan("CREATED", "CUSTOMER", "PENDING_CONFIRMATION", twoDaysBefore, day)
	want := []string{"APPOINTMENT_CREATED/WHATSAPP/CUSTOMER", "APPOINTMENT_CREATED/EMAIL/CUSTOMER", "APPOINTMENT_REMINDER/WHATSAPP/CUSTOMER",
		"APPOINTMENT_REMINDER/EMAIL/CUSTOMER", "APPOINTMENT_CREATED/EMAIL/PROFESSIONAL"}
	if r := reminder(ms); !slices.Equal(keys(ms), want) || r == nil || !r.SendAt.Equal(start.Add(-24*time.Hour)) ||
		r.Recipient != "+573001234567" || !r.Start.Equal(start) || r.ID == ms[0].ID || !ms[3].SendAt.Equal(r.SendAt) {
		t.Fatalf("pending, two days before: %v", keys(ms))
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
		{"canceled", "CANCELLED", "BUSINESS", "CANCELLED", twoDaysBefore, day},                 //nolint:misspell // stored value
		{"canceled by the customer", "CANCELLED", "CUSTOMER", "CANCELLED", twoDaysBefore, day}, //nolint:misspell // stored value
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

func TestMaskEmail(t *testing.T) {
	for in, want := range map[string]string{
		"laura.gomez@gmail.com": "l•••@gmail.com",
		"a@b.co":                "a•••@b.co",
		"@b.co":                 "•••••",
		"nope":                  "••••",
	} {
		if got := domain.MaskEmail(in); got != want {
			t.Errorf("MaskEmail(%q) = %q, want %q", in, got, want)
		}
	}
	if got := (domain.Message{Channel: domain.ChannelEmail, Recipient: "ana@x.co"}).MaskRecipient(); got != "a•••@x.co" {
		t.Errorf("email: %q", got)
	}
	if got := (domain.Message{Channel: domain.ChannelWhatsApp, Recipient: "+573001234567"}).MaskRecipient(); got != "+57••••••4567" {
		t.Errorf("whatsapp: %q", got)
	}
}

func TestDateAndTimeES(t *testing.T) {
	bogota, _ := time.LoadLocation("America/Bogota")
	c := domain.Content{Start: time.Date(2026, 12, 2, 2, 0, 0, 0, time.UTC), Location: bogota} // the day before locally
	if got := c.DateES(); got != "martes 1 de diciembre de 2026" {
		t.Errorf("DateES %q", got)
	}
	if got := c.TimeES(); got != "9:00 p. m." {
		t.Errorf("TimeES %q", got)
	}
	if got := (domain.Content{Start: time.Date(2027, 1, 1, 0, 5, 0, 0, time.UTC)}).DateES(); got != "viernes 1 de enero de 2027" {
		t.Errorf("no zone: %q", got)
	}
}

func TestRescheduleLimitES(t *testing.T) {
	for _, tc := range []struct {
		max, done int
		can       bool
		want      string
	}{
		{0, 7, true, ""},
		{2, 0, true, "Te quedan 2 cambios de hora para esta cita (máximo 2 veces)."},
		{2, 1, true, "Te queda 1 cambio de hora para esta cita (máximo 2 veces)."},
		{1, 1, false, "Ya usaste los cambios de hora que permite el negocio para esta cita (1 vez)."},
		{2, 3, false, "Ya usaste los cambios de hora que permite el negocio para esta cita (2 veces)."},
	} {
		c := domain.Content{MaxReschedules: tc.max, Reschedules: tc.done}
		if c.CanReschedule() != tc.can || c.RescheduleLimitES() != tc.want {
			t.Errorf("%d/%d: %v %q", tc.done, tc.max, c.CanReschedule(), c.RescheduleLimitES())
		}
	}
}

func TestCustomerPreferences(t *testing.T) {
	if domain.DefaultCustomerPreferences().Validate() != nil || !errors.Is((domain.CustomerPreferences{Reminders: true}).Validate(), domain.ErrNoChannel) {
		t.Error("validate")
	}
	onlyEmail := domain.CustomerPreferences{Email: true}
	for _, tc := range []struct {
		ch   domain.Channel
		kind domain.Kind
		want domain.SkipReason
	}{
		{domain.ChannelWhatsApp, domain.KindCreated, domain.SkipCustomerOptedOut},
		{domain.ChannelEmail, domain.KindCreated, ""},
		{domain.ChannelEmail, domain.KindReminder, domain.SkipRemindersOff},
		{domain.ChannelWhatsApp, domain.KindReminder, domain.SkipCustomerOptedOut},
	} {
		if got := onlyEmail.SkipFor(domain.Message{Channel: tc.ch, Kind: tc.kind}); got != tc.want {
			t.Errorf("%s %s: %q", tc.ch, tc.kind, got)
		}
	}
	if got := domain.DefaultCustomerPreferences().SkipFor(domain.Message{Channel: domain.ChannelWhatsApp, Kind: domain.KindReminder}); got != "" {
		t.Errorf("defaults: %q", got)
	}
}

func TestMonthAndDayOf(t *testing.T) {
	bogota, _ := time.LoadLocation("America/Bogota")
	// 31 Dec 2030, 21:00 in Bogotá is already January in UTC.
	from, to := domain.MonthOf(time.Date(2030, 12, 31, 21, 0, 0, 0, bogota))
	if !from.Equal(time.Date(2031, 1, 1, 0, 0, 0, 0, time.UTC)) || !to.Equal(time.Date(2031, 2, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("month %s %s", from, to)
	}
	from, to = domain.DayOf(time.Date(2030, 3, 5, 23, 59, 0, 0, time.UTC))
	if !from.Equal(time.Date(2030, 3, 5, 0, 0, 0, 0, time.UTC)) || to.Sub(from) != 24*time.Hour {
		t.Errorf("day %s %s", from, to)
	}
}
