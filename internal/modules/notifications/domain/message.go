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
	KindRescheduled       Kind = "APPOINTMENT_RESCHEDULED"
	KindReassigned        Kind = "APPOINTMENT_REASSIGNED"
	// KindReminder asks the customer to confirm before the appointment (T7.4).
	KindReminder Kind = "APPOINTMENT_REMINDER"
)

// Channel of delivery.
type Channel string

// Channels.
const (
	ChannelWhatsApp Channel = "WHATSAPP"
	ChannelPush     Channel = "PUSH"
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
)

// Message is one notification to one recipient through one channel.
type Message struct {
	ID, BusinessID, AppointmentID, EventID uuid.UUID
	Kind                                   Kind
	Channel                                Channel
	RecipientRole                          Role
	Recipient                              string // E.164 phone for WhatsApp
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

// Plan returns the messages a change produces (notifications.md §2): to the customer by
// WhatsApp when the appointment is created (by the customer or by the staff), moved to another
// time or professional, or canceled by the business (a customer who cancels knows it).
// Confirmations and the changes of the service itself (start, completion, no-show) send nothing.
// Push to the professional: T7.6.
//
// Reminder (T7.4, decisions of the user 2026-11-25): when the change leaves the appointment
// pending confirmation, a reminder is due r.Before the start; if that moment already passed it
// goes at once. Confirmed appointments (manual ones are, by default) get no reminder.
func Plan(c Change, r Reminders, newID func() uuid.UUID) []Message {
	var kind Kind
	switch c.Type {
	case eventCreated:
		kind = KindCreated
		if c.ActorRole == actorBusiness {
			kind = KindCreatedByBusiness
		}
	case eventRescheduled:
		kind = KindRescheduled
	case eventReassigned:
		kind = KindReassigned
	case eventCancelled:
		if c.ActorRole != actorBusiness {
			return nil
		}
		kind = KindCancelled
	default:
		return nil
	}
	at, start := c.At.UTC(), c.Start.UTC()
	msg := func(k Kind, sendAt time.Time) Message {
		return Message{
			ID: newID(), BusinessID: c.BusinessID, AppointmentID: c.AppointmentID, EventID: c.EventID,
			Kind: k, Channel: ChannelWhatsApp, RecipientRole: RecipientCustomer, Recipient: c.CustomerPhone,
			ProfessionalID: c.ProfessionalID, Start: start, Status: StatusPending, SendAt: sendAt, CreatedAt: at, UpdatedAt: at,
		}
	}
	out := []Message{msg(kind, at)}
	if kind != KindCancelled && c.Status == statusPending && r.Enabled && at.Before(start) {
		due := start.Add(-r.Before)
		if due.Before(at) {
			due = at
		}
		out = append(out, msg(KindReminder, due))
	}
	return out
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
	case m.Kind == KindCancelled:
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
