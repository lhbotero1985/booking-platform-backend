// Package domain holds the notification rules (notifications.md, T7.1): which messages a change
// of an appointment produces, and when a pending message is stale and must not be sent.
package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// Kind is what a message announces.
type Kind string

// Kinds.
const (
	KindCreated Kind = "APPOINTMENT_CREATED"
	// KindCreatedByBusiness: booked by the staff for the customer (P40: carries the app link).
	KindCreatedByBusiness Kind = "APPOINTMENT_CREATED_BY_BUSINESS"
	KindCancelled         Kind = "APPOINTMENT_CANCELLED" //nolint:misspell // stored value
	// KindCancelledByCustomer: the customer canceled (T7.6: an email receipt to the customer and a
	// notice to the professional; no WhatsApp).
	KindCancelledByCustomer Kind = "APPOINTMENT_CANCELLED_BY_CUSTOMER" //nolint:misspell // stored value
	KindRescheduled         Kind = "APPOINTMENT_RESCHEDULED"
	KindReassigned          Kind = "APPOINTMENT_REASSIGNED"
	// KindReminder asks the customer to confirm before the appointment (T7.4).
	KindReminder Kind = "APPOINTMENT_REMINDER"
)

// Channel of delivery.
type Channel string

// Channels.
const (
	ChannelWhatsApp Channel = "WHATSAPP"
	ChannelPush     Channel = "PUSH"
	// ChannelEmail (T7.6, ADR-0019): the address is resolved when the message is sent.
	ChannelEmail Channel = "EMAIL"
)

// Role of the recipient.
type Role string

// Recipient roles.
const (
	RecipientCustomer     Role = "CUSTOMER"
	RecipientProfessional Role = "PROFESSIONAL"
)

// Status of a message. PENDING until a delivery attempt ends it.
type Status string

// Statuses.
const (
	StatusPending   Status = "PENDING"
	StatusSent      Status = "SENT"
	StatusDelivered Status = "DELIVERED"
	StatusRead      Status = "READ"
	StatusFailed    Status = "FAILED"
	StatusSkipped   Status = "SKIPPED"
)

// SkipReason tells why a message was not sent.
type SkipReason string

// Skip reasons.
const (
	SkipChannelDisabled SkipReason = "CHANNEL_DISABLED"
	SkipNotAllowlisted  SkipReason = "NOT_ALLOWLISTED"
	SkipCancelled       SkipReason = "APPOINTMENT_CANCELLED" //nolint:misspell // stored value
	SkipChanged         SkipReason = "APPOINTMENT_CHANGED"
	SkipPassed          SkipReason = "APPOINTMENT_PASSED"
	SkipNotFound        SkipReason = "APPOINTMENT_NOT_FOUND"
	SkipNoRecipient     SkipReason = "NO_RECIPIENT"
	// SkipConfirmed: a reminder whose appointment is no longer pending confirmation.
	SkipConfirmed SkipReason = "APPOINTMENT_CONFIRMED"
	// T7.9: the recipient's preferences and the WhatsApp caps.
	SkipCustomerOptedOut     SkipReason = "CUSTOMER_OPTED_OUT"     // the customer turned the channel off
	SkipRemindersOff         SkipReason = "REMINDERS_OFF"          // the customer wants no reminders
	SkipProfessionalOptedOut SkipReason = "PROFESSIONAL_OPTED_OUT" // the specialist's emails are off
	SkipMonthlyLimit         SkipReason = "MONTHLY_LIMIT_REACHED"  // the business's WhatsApp cap
	SkipDailyLimit           SkipReason = "DAILY_LIMIT_REACHED"    // the platform's daily WhatsApp cap
)

// Values of the scheduling module's events (the domain imports no other module).
const (
	statusCancelled  = "CANCELLED" //nolint:misspell // value of the scheduling module
	statusPending    = "PENDING_CONFIRMATION"
	eventCreated     = "CREATED"
	eventCancelled   = "CANCELLED" //nolint:misspell // value of the scheduling module
	eventRescheduled = "RESCHEDULED"
	eventReassigned  = "REASSIGNED"
	actorBusiness    = "BUSINESS"
	actorCustomer    = "CUSTOMER"
)

// Message is one notification to one recipient through one channel.
type Message struct {
	ID, BusinessID, AppointmentID, EventID uuid.UUID
	Kind                                   Kind
	Channel                                Channel
	RecipientRole                          Role
	// Recipient is the E.164 phone for WhatsApp; for email the address, "" until it is sent.
	Recipient string
	// ProfessionalID and Start are what the message announces.
	ProfessionalID uuid.UUID
	Start          time.Time
	Status         Status
	// SendAt is when the delivery is due: the change itself, or the reminder time.
	SendAt time.Time

	SkipReason        SkipReason
	ErrorCode         string
	ProviderMessageID string
	Attempts          int
	CreatedAt         time.Time
	UpdatedAt         time.Time
	SentAt            time.Time
}

// Change is a recorded change of an appointment (scheduling's event, after the change).
type Change struct {
	EventID, BusinessID, AppointmentID, ProfessionalID uuid.UUID
	Type, ActorRole, Status                            string
	Start                                              time.Time
	CustomerPhone                                      string
	At                                                 time.Time
}

// Reminders is the reminder setting of the business.
type Reminders struct {
	Enabled bool
	Before  time.Duration
}

// Plan returns the messages a change produces (notifications.md §2, §12).
//
// To the customer, by WhatsApp and by email (T7.6, ADR-0019): when the appointment is created (by
// the customer or by the staff), moved to another time or professional, or canceled by the
// business. When the customer cancels, only an email receipt (a WhatsApp would cost for nothing).
// Confirmations and the changes of the service itself (start, completion, no-show) send nothing.
//
// To the professional, by email, only what the customer did (decision of the user 2026-11-25): a
// new booking, a cancellation or a new time.
//
// Reminder (T7.4, decisions of the user 2026-11-25), by both channels: when the change leaves the
// appointment pending confirmation, a reminder is due r.Before the start; if that moment already
// passed it goes at once. Confirmed appointments (manual ones are, by default) get no reminder.
func Plan(c Change, r Reminders, newID func() uuid.UUID) []Message {
	at, start := c.At.UTC(), c.Start.UTC()
	msg := func(k Kind, ch Channel, role Role, to string, sendAt time.Time) Message {
		return Message{
			ID: newID(), BusinessID: c.BusinessID, AppointmentID: c.AppointmentID, EventID: c.EventID,
			Kind: k, Channel: ch, RecipientRole: role, Recipient: to,
			ProfessionalID: c.ProfessionalID, Start: start, Status: StatusPending, SendAt: sendAt, CreatedAt: at, UpdatedAt: at,
		}
	}
	var out []Message
	kind, notify := customerKind(c)
	if notify {
		if kind != KindCancelledByCustomer {
			out = append(out, msg(kind, ChannelWhatsApp, RecipientCustomer, c.CustomerPhone, at))
		}
		out = append(out, msg(kind, ChannelEmail, RecipientCustomer, "", at))
	}
	if notify && kind != KindCancelled && kind != KindCancelledByCustomer && c.Status == statusPending && r.Enabled && at.Before(start) {
		due := start.Add(-r.Before)
		if due.Before(at) {
			due = at
		}
		out = append(out, msg(KindReminder, ChannelWhatsApp, RecipientCustomer, c.CustomerPhone, due),
			msg(KindReminder, ChannelEmail, RecipientCustomer, "", due))
	}
	if c.ActorRole == actorCustomer {
		switch c.Type {
		case eventCreated:
			out = append(out, msg(KindCreated, ChannelEmail, RecipientProfessional, "", at))
		case eventCancelled:
			out = append(out, msg(KindCancelledByCustomer, ChannelEmail, RecipientProfessional, "", at))
		case eventRescheduled:
			out = append(out, msg(KindRescheduled, ChannelEmail, RecipientProfessional, "", at))
		}
	}
	return out
}

// customerKind is what the customer is told of a change, if anything.
func customerKind(c Change) (Kind, bool) {
	switch c.Type {
	case eventCreated:
		if c.ActorRole == actorBusiness {
			return KindCreatedByBusiness, true
		}
		return KindCreated, true
	case eventRescheduled:
		return KindRescheduled, true
	case eventReassigned:
		return KindReassigned, true
	case eventCancelled:
		if c.ActorRole == actorBusiness {
			return KindCancelled, true
		}
		return KindCancelledByCustomer, true
	}
	return "", false
}

// Appointment is the current state of the appointment a message is about.
type Appointment struct {
	Status         string
	ProfessionalID uuid.UUID
	Start          time.Time
}

// Stale tells why a pending message must not be sent at now, or "" if it still holds: the
// appointment already started, or (except for a cancellation notice) it was canceled or moved
// meanwhile — the later change has its own message; a reminder also when the appointment is no
// longer pending confirmation.
func Stale(m Message, current Appointment, now time.Time) SkipReason {
	switch {
	case m.Recipient == "":
		return SkipNoRecipient
	case !now.Before(m.Start):
		return SkipPassed
	case m.Kind == KindCancelled, m.Kind == KindCancelledByCustomer:
		return ""
	case current.Status == statusCancelled:
		return SkipCancelled
	case current.ProfessionalID != m.ProfessionalID || !current.Start.Equal(m.Start):
		return SkipChanged
	case m.Kind == KindReminder && current.Status != statusPending:
		return SkipConfirmed
	}
	return ""
}

// MaskPhone keeps the country prefix and the last four digits: "+57•••••4567". For logs and
// the staff view of the messages.
func MaskPhone(phone string) string {
	const visible = 4
	if len(phone) <= 3+visible {
		return strings.Repeat("•", len(phone))
	}
	return phone[:3] + strings.Repeat("•", len(phone)-3-visible) + phone[len(phone)-visible:]
}

// MaskEmail keeps the first character of the local part and the domain: "l•••@gmail.com".
func MaskEmail(email string) string {
	local, domain, ok := strings.Cut(email, "@")
	if !ok || local == "" {
		return strings.Repeat("•", len(email))
	}
	return local[:1] + "•••@" + domain
}

// MaskRecipient masks the recipient of a message for its channel.
func (m Message) MaskRecipient() string {
	if m.Channel == ChannelEmail {
		return MaskEmail(m.Recipient)
	}
	return MaskPhone(m.Recipient)
}
