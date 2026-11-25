package application

// WhatsApp webhook (T7.5, notifications.md §3): Meta reports delivery statuses and the answers to
// the reminder's buttons. Each item is stored once (webhook_events) and queued; the worker
// processes it. An answer is authenticated by its single-use token and the sender's number, and
// applied in one transaction with the use of the token.

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/domain"
	scheduling "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/contract"
)

// ErrTokenNotFound means there is no action token with that hash.
var ErrTokenNotFound = errors.New("notifications: action token not found")

// WebhookTx is the persistence of the webhook inside a transaction.
type WebhookTx interface {
	// ClaimWebhookEvent reports true only the first time an event id arrives.
	ClaimWebhookEvent(ctx context.Context, eventID string, at time.Time) (bool, error)
	EnqueueWebhookItem(ctx context.Context, item domain.WebhookItem) error
	InsertActionTokens(ctx context.Context, tokens []domain.ActionToken) error
	// ActionToken returns ErrTokenNotFound when absent.
	ActionToken(ctx context.Context, hash []byte) (domain.ActionToken, error)
	// LockMessageTokens locks every token of a message (always in the same order).
	LockMessageTokens(ctx context.Context, messageID uuid.UUID) error
	UseMessageTokens(ctx context.Context, messageID uuid.UUID, at time.Time) error
	EnqueueReply(ctx context.Context, to, text string) error
}

// TextSender sends a free-text message (an answer inside the 24-hour window the customer opened).
type TextSender interface {
	SendText(ctx context.Context, to, text string) (string, error)
}

// ReceiveWebhook stores each item once and queues it (called by the HTTP handler, which already
// checked Meta's signature). Repeated items are ignored.
func (s *Service) ReceiveWebhook(ctx context.Context, items []domain.WebhookItem) error {
	if len(items) == 0 {
		return nil
	}
	now := s.d.Clock.Now()
	return s.d.Store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		for _, it := range items {
			first, err := tx.ClaimWebhookEvent(ctx, it.EventID, now)
			if err != nil {
				return fmt.Errorf("notifications: webhook event: %w", err)
			}
			if first {
				if err := tx.EnqueueWebhookItem(ctx, it); err != nil {
					return fmt.Errorf("notifications: queue webhook item: %w", err)
				}
			}
		}
		return nil
	})
}

// ProcessWebhookItem applies one item (worker).
func (s *Service) ProcessWebhookItem(ctx context.Context, it domain.WebhookItem) error {
	switch it.Kind {
	case domain.WebhookStatus:
		_, err := s.d.Store.UpdateDeliveryStatus(ctx, it.ProviderMessageID, it.Status, it.ErrorCode, s.d.Clock.Now())
		return err
	case domain.WebhookButton:
		return s.answer(ctx, it)
	}
	return nil
}

// answer applies a button of a reminder. Whatever happens the customer gets one answer back.
func (s *Service) answer(ctx context.Context, it domain.WebhookItem) error {
	hash := domain.HashActionToken(it.Payload)
	return s.d.Store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		outcome, content, err := s.apply(ctx, tx, hash, it.From)
		if err != nil {
			return err
		}
		return tx.EnqueueReply(ctx, it.From, domain.ReplyText(outcome, content))
	})
}

func (s *Service) apply(ctx context.Context, tx Tx, hash []byte, from string) (domain.Outcome, domain.Content, error) {
	invalid := domain.OutcomeInvalid
	tok, err := tx.ActionToken(ctx, hash)
	if errors.Is(err, ErrTokenNotFound) {
		return invalid, domain.Content{}, nil
	}
	if err != nil {
		return "", domain.Content{}, err
	}
	if tok.Recipient != from {
		return invalid, domain.Content{}, nil // someone else's button: say nothing about the appointment
	}
	// Both buttons of a reminder answer once: lock them all, then read the token again.
	if err := tx.LockMessageTokens(ctx, tok.MessageID); err != nil {
		return "", domain.Content{}, err
	}
	if tok, err = tx.ActionToken(ctx, hash); err != nil {
		return "", domain.Content{}, err
	}
	now := s.d.Clock.Now()
	if !tok.UsedAt.IsZero() || !now.Before(tok.ExpiresAt) {
		return invalid, domain.Content{}, nil
	}
	m, err := s.d.Store.Message(ctx, tok.MessageID)
	if err != nil {
		return "", domain.Content{}, err
	}
	st, err := s.d.Appointments.AppointmentState(ctx, tok.BusinessID, tok.AppointmentID)
	if errors.Is(err, scheduling.ErrAppointmentNotFound) {
		return invalid, domain.Content{}, nil
	}
	if err != nil {
		return "", domain.Content{}, err
	}
	content := contentOf(st)
	if st.ProfessionalID != m.ProfessionalID || !st.Start.Equal(m.Start) {
		return invalid, content, tx.UseMessageTokens(ctx, tok.MessageID, now) // moved since
	}
	outcome := domain.OutcomeConfirmed
	if tok.Action == domain.ActionCancel {
		outcome = domain.OutcomeCancelled
		err = s.d.Replies.CancelAsCustomer(ctx, tok.BusinessID, tok.AppointmentID)
	} else {
		err = s.d.Replies.ConfirmAsCustomer(ctx, tok.BusinessID, tok.AppointmentID)
	}
	switch {
	case errors.Is(err, scheduling.ErrCutoffPassed):
		outcome = domain.OutcomeCutoff
	case errors.Is(err, scheduling.ErrNotActive):
		outcome = domain.OutcomeNotActive
	case errors.Is(err, scheduling.ErrAppointmentNotFound):
		outcome = invalid
	case err != nil:
		return "", domain.Content{}, err
	}
	return outcome, content, tx.UseMessageTokens(ctx, tok.MessageID, now)
}

// issueTokens stores the two tokens of a reminder's buttons (confirm, cancel) and returns them in
// the order of the buttons. A new attempt issues new tokens; the old ones were never delivered.
func (s *Service) issueTokens(ctx context.Context, m domain.Message) ([]string, error) {
	random := s.d.Random
	if random == nil {
		random = rand.Reader
	}
	now := s.d.Clock.Now()
	raws := make([]string, 0, 2)
	toks := make([]domain.ActionToken, 0, 2)
	for _, a := range []domain.Action{domain.ActionConfirm, domain.ActionCancel} {
		raw, hash, err := domain.NewActionToken(random)
		if err != nil {
			return nil, err
		}
		raws = append(raws, raw)
		toks = append(toks, domain.ActionToken{
			Hash: hash, MessageID: m.ID, BusinessID: m.BusinessID, AppointmentID: m.AppointmentID, Action: a,
			Recipient: m.Recipient, ExpiresAt: m.Start, CreatedAt: now,
		})
	}
	err := s.d.Store.WithinTx(ctx, func(ctx context.Context, tx Tx) error { return tx.InsertActionTokens(ctx, toks) })
	if err != nil {
		return nil, fmt.Errorf("notifications: issue tokens: %w", err)
	}
	return raws, nil
}

// SendReply sends an answer to a customer (worker). Off or not allowlisted: dropped. A permanent
// failure is dropped; a temporary one is retried.
func (s *Service) SendReply(ctx context.Context, to, text string) error {
	if s.d.TextSender == nil || s.d.Policy.gate(domain.Message{Channel: domain.ChannelWhatsApp, Recipient: to}) != "" {
		return nil
	}
	_, err := s.d.TextSender.SendText(ctx, to, text)
	var de *DeliveryError
	if errors.As(err, &de) && de.Permanent {
		return nil
	}
	return err
}
