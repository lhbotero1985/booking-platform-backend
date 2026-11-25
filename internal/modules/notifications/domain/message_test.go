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
		got := domain.Plan(ch, func() uuid.UUID { return id })
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
