// Package application holds the notification use cases (T7.1): queue the messages of every
// change of an appointment inside its transaction, send them from the worker re-checking the
// appointment, and show the staff what was sent.
package application

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"

	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/domain"
	scheduling "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
)

// ErrMessageNotFound means there is no message with that id.
var ErrMessageNotFound = errors.New("notifications: message not found")

// Store is the persistence of the module.
type Store interface {
	// WithinTx runs fn in a transaction, joining the one ctx carries (db.ContextWithTx): the
	// messages of a change commit or roll back with it.
	WithinTx(ctx context.Context, fn func(ctx context.Context, tx Tx) error) error
	// Message returns ErrMessageNotFound when absent.
	Message(ctx context.Context, id uuid.UUID) (domain.Message, error)
	// FinishAttempt stores the status, skip reason, error code, provider id and sent time of m;
	// attempted adds one to its attempts.
	FinishAttempt(ctx context.Context, m domain.Message, attempted bool) error
	AppointmentMessages(ctx context.Context, businessID, appointmentID uuid.UUID) ([]domain.Message, error)
}

// Tx is the persistence inside a transaction.
type Tx interface {
	// InsertMessage reports false when the message already exists (same event, channel and
	// recipient role).
	InsertMessage(ctx context.Context, m domain.Message) (bool, error)
	// EnqueueSend queues the delivery job of a message in the transaction.
	EnqueueSend(ctx context.Context, messageID uuid.UUID) error
}

// Outgoing is what a Sender delivers.
type Outgoing struct {
	MessageID uuid.UUID
	Kind      domain.Kind
	Channel   domain.Channel
	To        string
	Content   domain.Content
}

// Sender delivers a message through its channel and returns the provider's message id.
// Failures should be a *DeliveryError.
type Sender interface {
	Send(ctx context.Context, m Outgoing) (string, error)
}

// DeliveryError is a failed delivery. Permanent ones (invalid number, rejected template) are not
// retried.
type DeliveryError struct {
	Code      string
	Permanent bool
	Err       error
}

func (e *DeliveryError) Error() string {
	return fmt.Sprintf("notifications: delivery failed (%s): %v", e.Code, e.Err)
}

func (e *DeliveryError) Unwrap() error { return e.Err }

// Policy is the per-environment switch of each channel (notifications.md §3, cost control).
type Policy struct {
	WhatsAppEnabled bool
	// WhatsAppAllowlist, when not empty, is the only numbers WhatsApp may write to (QA).
	WhatsAppAllowlist []string
}

func (p Policy) gate(m domain.Message) domain.SkipReason {
	if m.Channel != domain.ChannelWhatsApp {
		return ""
	}
	switch {
	case !p.WhatsAppEnabled:
		return domain.SkipChannelDisabled
	case len(p.WhatsAppAllowlist) > 0 && !slices.Contains(p.WhatsAppAllowlist, m.Recipient):
		return domain.SkipNotAllowlisted
	}
	return ""
}

// Deps of the service.
type Deps struct {
	Store        Store
	Appointments scheduling.Appointments
	Senders      map[domain.Channel]Sender
	Policy       Policy
	Clock        clock.Clock
}

// Service implements the notification use cases.
type Service struct{ d Deps }

// NewService builds the service.
func NewService(d Deps) *Service { return &Service{d: d} }

// AppointmentEventRecorded implements scheduling.EventListener: it stores the messages of the
// change and queues their delivery, in the change's transaction.
func (s *Service) AppointmentEventRecorded(ctx context.Context, e scheduling.AppointmentEvent) error {
	msgs := domain.Plan(domain.Change{
		EventID: e.ID, BusinessID: e.BusinessID, AppointmentID: e.AppointmentID, ProfessionalID: e.ProfessionalID,
		Type: e.Type, ActorRole: e.ActorRole, Start: e.Start, CustomerPhone: e.CustomerPhone, At: e.At,
	}, func() uuid.UUID { return uuid.Must(uuid.NewV7()) })
	if len(msgs) == 0 {
		return nil
	}
	return s.d.Store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		for _, m := range msgs {
			inserted, err := tx.InsertMessage(ctx, m)
			if err != nil {
				return fmt.Errorf("notifications: queue message: %w", err)
			}
			if !inserted {
				continue
			}
			if err := tx.EnqueueSend(ctx, m.ID); err != nil {
				return fmt.Errorf("notifications: queue delivery: %w", err)
			}
		}
		return nil
	})
}

// Send delivers a pending message (worker). It is skipped when its channel is off, the number is
// not allowed, or the appointment changed since; a temporary failure is returned for a retry
// unless lastAttempt, a permanent one or the last attempt marks it FAILED. A message that is no
// longer pending is left alone, so a repeated job sends nothing twice.
func (s *Service) Send(ctx context.Context, id uuid.UUID, lastAttempt bool) error {
	m, err := s.d.Store.Message(ctx, id)
	if errors.Is(err, ErrMessageNotFound) {
		return nil // rolled back with its change
	}
	if err != nil {
		return err
	}
	if m.Status != domain.StatusPending {
		return nil
	}
	if reason := s.d.Policy.gate(m); reason != "" {
		return s.skip(ctx, m, reason)
	}
	st, err := s.d.Appointments.AppointmentState(ctx, m.BusinessID, m.AppointmentID)
	if errors.Is(err, scheduling.ErrAppointmentNotFound) {
		return s.skip(ctx, m, domain.SkipNotFound)
	}
	if err != nil {
		return err
	}
	now := s.d.Clock.Now()
	if reason := domain.Stale(m, domain.Appointment{Status: st.Status, ProfessionalID: st.ProfessionalID, Start: st.Start}, now); reason != "" {
		return s.skip(ctx, m, reason)
	}
	sender, ok := s.d.Senders[m.Channel]
	if !ok {
		return s.skip(ctx, m, domain.SkipChannelDisabled)
	}
	loc, err := time.LoadLocation(st.TimeZone)
	if err != nil || st.TimeZone == "" {
		loc = time.UTC // never expected: the zone is validated when the business is created
	}
	content := domain.Content{
		CustomerName: st.CustomerName, BusinessName: st.BusinessName, ServiceName: st.ServiceName,
		ProfessionalName: st.ProfessionalName, Start: st.Start, Location: loc, CancelReason: st.CancelReason,
	}
	providerID, sendErr := sender.Send(ctx, Outgoing{MessageID: m.ID, Kind: m.Kind, Channel: m.Channel, To: m.Recipient, Content: content})
	m.UpdatedAt = s.d.Clock.Now()
	if sendErr != nil {
		m.ErrorCode = "UNKNOWN"
		var de *DeliveryError
		permanent := false
		if errors.As(sendErr, &de) {
			m.ErrorCode, permanent = de.Code, de.Permanent
		}
		if permanent || lastAttempt {
			m.Status = domain.StatusFailed
			return s.d.Store.FinishAttempt(ctx, m, true)
		}
		if err := s.d.Store.FinishAttempt(ctx, m, true); err != nil {
			return errors.Join(sendErr, err)
		}
		return sendErr
	}
	m.Status, m.ProviderMessageID, m.SentAt, m.ErrorCode = domain.StatusSent, providerID, m.UpdatedAt, ""
	return s.d.Store.FinishAttempt(ctx, m, true)
}

func (s *Service) skip(ctx context.Context, m domain.Message, reason domain.SkipReason) error {
	m.Status, m.SkipReason, m.UpdatedAt = domain.StatusSkipped, reason, s.d.Clock.Now()
	return s.d.Store.FinishAttempt(ctx, m, false)
}

// AppointmentMessages returns the messages of an appointment of the business, oldest first, to
// the staff who can see the appointment.
func (s *Service) AppointmentMessages(ctx context.Context, p identity.Principal, businessID, appointmentID uuid.UUID) ([]domain.Message, error) {
	if err := s.d.Appointments.AuthorizeAppointmentView(ctx, p, businessID, appointmentID); err != nil {
		return nil, err
	}
	return s.d.Store.AppointmentMessages(ctx, businessID, appointmentID)
}
