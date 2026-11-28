// Package application holds the notification use cases (T7.1): queue the messages of every
// change of an appointment inside its transaction, send them from the worker re-checking the
// appointment, and show the staff what was sent.
package application

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/google/uuid"

	businesses "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/contract"
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
	// UpdateDeliveryStatus moves a sent message forward with a status from the provider; false
	// when it changed nothing (unknown id, late or repeated status).
	UpdateDeliveryStatus(ctx context.Context, providerMessageID string, status domain.Status, errorCode string, at time.Time) (bool, error)
	PreferencesStore
}

// Tx is the persistence inside a transaction.
type Tx interface {
	// InsertMessage reports false when the message already exists (same event, channel and
	// recipient role).
	InsertMessage(ctx context.Context, m domain.Message) (bool, error)
	// EnqueueSend queues the delivery job of a message in the transaction, due at at (zero: now).
	EnqueueSend(ctx context.Context, messageID uuid.UUID, at time.Time) error
	WebhookTx
}

// Outgoing is what a Sender delivers.
type Outgoing struct {
	MessageID uuid.UUID
	// AppointmentID is the appointment the message is about (the review request's link).
	AppointmentID uuid.UUID
	Kind          domain.Kind
	Channel       domain.Channel
	Role          domain.Role
	To            string
	Content       domain.Content
	// ButtonPayloads are the payloads of the quick-reply buttons, in order (T7.5: the reminder's
	// single-use tokens for "Confirmar cita" and "Cancelar cita").
	ButtonPayloads []string
	// Links are the single-use codes of an email's links, by action (T7.7).
	Links map[domain.Action]string
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
	// EmailEnabled turns the appointment emails on (T7.6).
	EmailEnabled    bool
	WhatsAppEnabled bool
	// WhatsAppAllowlist, when not empty, is the only numbers WhatsApp may write to (QA).
	WhatsAppAllowlist []string
	// T7.9 (P16): WhatsApp messages a business may send per month unless the platform set its
	// own cap (0 here = no default cap), and the platform's cap per day (0 = none).
	WhatsAppMonthlyLimit int
	WhatsAppDailyLimit   int
}

func (p Policy) gate(m domain.Message) domain.SkipReason {
	if m.Channel == domain.ChannelEmail && !p.EmailEnabled {
		return domain.SkipChannelDisabled
	}
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

// Recipients resolves the email addresses of the people a message goes to (T7.6). "" when there
// is none (no account, disabled account, no professional).
type Recipients interface {
	AccountEmail(ctx context.Context, userID uuid.UUID) (string, error)
	ProfessionalEmail(ctx context.Context, businessID, professionalID uuid.UUID) (string, error)
	// ProfessionalAccount is the account of a professional profile (T7.9: it sets its own
	// preferences); uuid.Nil when the profile does not exist in the business.
	ProfessionalAccount(ctx context.Context, businessID, professionalID uuid.UUID) (uuid.UUID, error)
}

// ReminderSettings reads the booking policy of a business: its reminder setting (T7.4) and the
// cut-offs the emails show (T7.6).
type ReminderSettings interface {
	BookingPolicyOf(ctx context.Context, businessID uuid.UUID) (businesses.BookingPolicy, error)
}

// Deps of the service.
type Deps struct {
	Store        Store
	Appointments scheduling.Appointments
	// Replies applies the answers to the reminder's buttons (T7.5); TextSender answers back.
	Replies    scheduling.CustomerReplies
	TextSender TextSender
	// Random is the source of the buttons' tokens (crypto/rand.Reader when nil).
	Random io.Reader
	// Reminders: nil sends no reminders and shows no cut-offs.
	Reminders ReminderSettings
	// Recipients: nil leaves only the email the business registered for the customer.
	Recipients Recipients
	Senders    map[domain.Channel]Sender
	Policy     Policy
	Clock      clock.Clock
	// Authz authorizes the preference endpoints (T7.9).
	Authz identity.Authorizer
}

// Service implements the notification use cases.
type Service struct{ d Deps }

// NewService builds the service.
func NewService(d Deps) *Service { return &Service{d: d} }

// AppointmentEventRecorded implements scheduling.EventListener: it stores the messages of the
// change and queues their delivery, in the change's transaction.
func (s *Service) AppointmentEventRecorded(ctx context.Context, e scheduling.AppointmentEvent) error {
	rem, err := s.reminders(ctx, e)
	if err != nil {
		return fmt.Errorf("notifications: reminder setting: %w", err)
	}
	msgs := domain.Plan(domain.Change{
		EventID: e.ID, BusinessID: e.BusinessID, AppointmentID: e.AppointmentID, ProfessionalID: e.ProfessionalID,
		Type: e.Type, ActorRole: e.ActorRole, Status: e.Status, Start: e.Start, CustomerPhone: e.CustomerPhone,
		CustomerHasAccount: e.CustomerHasAccount, At: e.At,
	}, rem, func() uuid.UUID { return uuid.Must(uuid.NewV7()) })
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
			// Only a delivery due later (a reminder) is scheduled; the rest go at once, whatever
			// the clock of the change.
			var at time.Time
			if m.SendAt.After(m.CreatedAt) {
				at = m.SendAt
			}
			if err := tx.EnqueueSend(ctx, m.ID, at); err != nil {
				return fmt.Errorf("notifications: queue delivery: %w", err)
			}
		}
		return nil
	})
}

// reminders reads the business's setting only for changes that may need a reminder.
func (s *Service) reminders(ctx context.Context, e scheduling.AppointmentEvent) (domain.Reminders, error) {
	switch {
	case s.d.Reminders == nil, e.Status != scheduling.StatusPendingConfirmation:
		return domain.Reminders{}, nil
	case e.Type != scheduling.EventCreated && e.Type != scheduling.EventRescheduled && e.Type != scheduling.EventReassigned:
		return domain.Reminders{}, nil
	}
	pol, err := s.d.Reminders.BookingPolicyOf(ctx, e.BusinessID)
	if err != nil {
		return domain.Reminders{}, err
	}
	return domain.Reminders{Enabled: pol.RemindersEnabled, Before: time.Duration(pol.ReminderHoursBefore) * time.Hour}, nil
}

// Send delivers a pending message (worker). It is skipped when its channel is off, the number is
// not allowed, the appointment changed since, the recipient does not want it or the WhatsApp cap
// was reached (T7.9); a temporary failure is returned for a retry
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
	if m.Channel == domain.ChannelEmail && m.Recipient == "" {
		if m.Recipient, err = s.emailOf(ctx, m, st); err != nil {
			return err
		}
	}
	now := s.d.Clock.Now()
	if reason := domain.Stale(m, domain.Appointment{Status: st.Status, ProfessionalID: st.ProfessionalID, Start: st.Start}, now); reason != "" {
		return s.skip(ctx, m, reason)
	}
	if reason, err := s.unwanted(ctx, m, st); err != nil || reason != "" {
		if err != nil {
			return err
		}
		return s.skip(ctx, m, reason)
	}
	sender, ok := s.d.Senders[m.Channel]
	if !ok {
		return s.skip(ctx, m, domain.SkipChannelDisabled)
	}
	out := Outgoing{MessageID: m.ID, AppointmentID: m.AppointmentID, Kind: m.Kind, Channel: m.Channel, Role: m.RecipientRole, To: m.Recipient, Content: contentOf(st)}
	if m.Channel == domain.ChannelEmail {
		if err := s.addPolicy(ctx, m.BusinessID, &out.Content); err != nil {
			return err
		}
	}
	if m.Kind == domain.KindReminder && m.Channel == domain.ChannelWhatsApp {
		if out.ButtonPayloads, err = s.issueTokens(ctx, m, []domain.Action{domain.ActionConfirm, domain.ActionCancel}); err != nil {
			return err
		}
	}
	if actions := emailActions(m.Kind, m.RecipientRole, out.Content); m.Channel == domain.ChannelEmail && len(actions) > 0 {
		raws, err := s.issueTokens(ctx, m, actions)
		if err != nil {
			return err
		}
		out.Links = make(map[domain.Action]string, len(actions))
		for i, a := range actions {
			out.Links[a] = raws[i]
		}
	}
	providerID, sendErr := sender.Send(ctx, out)
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

// contentOf is what a message says about an appointment.
func contentOf(st scheduling.AppointmentState) domain.Content {
	loc, err := time.LoadLocation(st.TimeZone)
	if err != nil || st.TimeZone == "" {
		loc = time.UTC // never expected: the zone is validated when the business is created
	}
	return domain.Content{
		CustomerName: st.CustomerName, BusinessName: st.BusinessName, ServiceName: st.ServiceName,
		ProfessionalName: st.ProfessionalName, Start: st.Start, Location: loc, CancelReason: st.CancelReason,
		Status: st.Status, BusinessAddress: st.BusinessAddress, BusinessPhone: st.BusinessPhone,
		Latitude: st.BusinessLatitude, Longitude: st.BusinessLongitude, Reschedules: st.CustomerReschedules,
	}
}

// emailOf is the address of the recipient of an email (T7.6): for the customer, the one the
// business registered or else the one of its account; for the professional, its account's.
func (s *Service) emailOf(ctx context.Context, m domain.Message, st scheduling.AppointmentState) (string, error) {
	switch {
	case m.RecipientRole == domain.RecipientCustomer && st.CustomerEmail != "":
		return st.CustomerEmail, nil
	case s.d.Recipients == nil:
		return "", nil
	case m.RecipientRole == domain.RecipientCustomer && st.CustomerUserID != uuid.Nil:
		return s.d.Recipients.AccountEmail(ctx, st.CustomerUserID)
	case m.RecipientRole == domain.RecipientProfessional:
		return s.d.Recipients.ProfessionalEmail(ctx, m.BusinessID, m.ProfessionalID)
	}
	return "", nil
}

// addPolicy adds the business's cut-offs to what an email shows.
func (s *Service) addPolicy(ctx context.Context, businessID uuid.UUID, c *domain.Content) error {
	if s.d.Reminders == nil {
		return nil
	}
	pol, err := s.d.Reminders.BookingPolicyOf(ctx, businessID)
	if err != nil {
		return err
	}
	c.CancelCutoffHours, c.RescheduleCutoffHours = pol.CancellationCutoffHours, pol.RescheduleCutoffHours
	c.MaxReschedules = pol.MaxCustomerReschedules
	return nil
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
