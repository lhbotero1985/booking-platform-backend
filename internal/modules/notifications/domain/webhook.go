package domain

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
)

// Action is what a button of a reminder does (T7.5).
type Action string

// Actions.
const (
	ActionConfirm Action = "CONFIRM"
	ActionCancel  Action = "CANCEL"
	// ActionReschedule: the email's "Reagendar" link (T7.7).
	ActionReschedule Action = "RESCHEDULE"
)

// ActionToken authorizes one answer to a reminder: the opaque payload of a button. Only the
// SHA-256 of the token is stored.
type ActionToken struct {
	Hash                                 []byte
	MessageID, BusinessID, AppointmentID uuid.UUID
	Action                               Action
	Recipient                            string    // the number the reminder went to (E.164)
	ExpiresAt                            time.Time // the start of the appointment
	UsedAt                               time.Time // zero while unused
	CreatedAt                            time.Time
}

// NewActionToken returns a random token (43 characters, URL-safe) and its hash.
func NewActionToken(random io.Reader) (string, []byte, error) {
	b := make([]byte, 32)
	if _, err := io.ReadFull(random, b); err != nil {
		return "", nil, fmt.Errorf("notifications: token: %w", err)
	}
	raw := base64.RawURLEncoding.EncodeToString(b)
	return raw, HashActionToken(raw), nil
}

// HashActionToken is the stored form of a token.
func HashActionToken(raw string) []byte {
	h := sha256.Sum256([]byte(raw))
	return h[:]
}

// WebhookKind is the kind of an item of a WhatsApp webhook.
type WebhookKind string

// Webhook kinds.
const (
	WebhookStatus WebhookKind = "STATUS" // delivery status of a message we sent
	WebhookButton WebhookKind = "BUTTON" // a customer pressed a quick-reply button
)

// WebhookItem is one thing Meta reports, processed once (EventID).
type WebhookItem struct {
	Kind    WebhookKind `json:"kind"`
	EventID string      `json:"event_id"`
	// STATUS
	ProviderMessageID string `json:"provider_message_id,omitempty"`
	Status            Status `json:"status,omitempty"` // DELIVERED, READ or FAILED
	ErrorCode         string `json:"error_code,omitempty"`
	// BUTTON
	From    string `json:"from,omitempty"` // E.164
	Payload string `json:"payload,omitempty"`
}

// Outcome of an answer to a reminder.
type Outcome string

// Outcomes.
const (
	OutcomeConfirmed Outcome = "CONFIRMED"
	OutcomeCancelled Outcome = "CANCELLED" //nolint:misspell // stored value
	OutcomeCutoff    Outcome = "CUTOFF_PASSED"
	OutcomeNotActive Outcome = "NOT_ACTIVE"
	OutcomeInvalid   Outcome = "INVALID" // unknown, used, expired token, or the appointment changed
	// T7.7: the email's reschedule link.
	OutcomeRescheduled     Outcome = "RESCHEDULED"
	OutcomeSlotUnavailable Outcome = "SLOT_UNAVAILABLE"
	// T7.8: the customer already moved it as many times as the business allows.
	OutcomeRescheduleLimit Outcome = "RESCHEDULE_LIMIT_REACHED"
)

// ReplyText is the answer sent back to the customer (a session message: no template needed
// within 24 hours of the customer's message).
func ReplyText(o Outcome, c Content) string {
	switch o {
	case OutcomeConfirmed:
		return fmt.Sprintf("¡Listo, %s! Tu cita de %s en %s quedó confirmada para el %s. Te esperamos.",
			c.CustomerName, c.ServiceName, c.BusinessName, c.WhenES())
	case OutcomeCancelled:
		return fmt.Sprintf("Tu cita de %s en %s del %s quedó cancelada. Si quieres, puedes agendar otra desde la app.",
			c.ServiceName, c.BusinessName, c.WhenES())
	case OutcomeCutoff:
		return fmt.Sprintf("Ya no es posible cancelar tu cita por aquí porque falta muy poco para la hora. Por favor comunícate con %s.",
			c.BusinessName)
	case OutcomeNotActive:
		return "Esta cita ya no está activa. Si quieres, puedes agendar una nueva desde la app."
	default:
		return "Este botón ya no es válido: ya respondiste o tu cita cambió. Revisa tu cita en la app o comunícate con el negocio."
	}
}
