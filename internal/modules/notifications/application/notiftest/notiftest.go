// Package notiftest is an in-memory notifications store for tests: a transaction is applied only
// when its function succeeds.
package notiftest

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/domain"
	scheduling "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
)

// Store keeps messages in memory.
type Store struct {
	txMu     sync.Mutex // one transaction at a time (stands for the row locks)
	mu       sync.Mutex // the data
	msgs     map[uuid.UUID]domain.Message
	order    []uuid.UUID
	enqueued []uuid.UUID
	events   map[string]bool
	tokens   map[string]domain.ActionToken
	items    []domain.WebhookItem
	replies  []Reply
	// FailCommit, when set, makes every transaction fail after its function ran.
	FailCommit error
}

// Reply is an answer queued for a customer.
type Reply struct{ To, Text string }

// NewStore builds an empty store.
func NewStore() *Store {
	return &Store{msgs: map[uuid.UUID]domain.Message{}, events: map[string]bool{}, tokens: map[string]domain.ActionToken{}}
}

type tx struct {
	s        *Store
	msgs     []domain.Message
	enqueued []uuid.UUID
	events   []string
	items    []domain.WebhookItem
	tokens   []domain.ActionToken
	used     map[uuid.UUID]time.Time
	replies  []Reply
}

// WithinTx implements application.Store.
func (s *Store) WithinTx(ctx context.Context, fn func(ctx context.Context, tx application.Tx) error) error {
	s.txMu.Lock()
	defer s.txMu.Unlock()
	t := &tx{s: s, used: map[uuid.UUID]time.Time{}}
	if err := fn(ctx, t); err != nil {
		return err
	}
	if s.FailCommit != nil {
		return s.FailCommit
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range t.msgs {
		s.msgs[m.ID] = m
		s.order = append(s.order, m.ID)
	}
	s.enqueued = append(s.enqueued, t.enqueued...)
	for _, e := range t.events {
		s.events[e] = true
	}
	s.items = append(s.items, t.items...)
	for _, k := range t.tokens {
		s.tokens[string(k.Hash)] = k
	}
	for id, at := range t.used {
		for h, k := range s.tokens {
			if k.MessageID == id && k.UsedAt.IsZero() {
				k.UsedAt = at
				s.tokens[h] = k
			}
		}
	}
	s.replies = append(s.replies, t.replies...)
	return nil
}

func (t *tx) InsertMessage(_ context.Context, m domain.Message) (bool, error) {
	t.s.mu.Lock()
	defer t.s.mu.Unlock()
	for _, old := range t.s.msgs {
		if old.EventID == m.EventID && old.Kind == m.Kind && old.Channel == m.Channel && old.RecipientRole == m.RecipientRole {
			return false, nil
		}
	}
	t.msgs = append(t.msgs, m)
	return true, nil
}

func (t *tx) EnqueueSend(_ context.Context, id uuid.UUID, _ time.Time) error {
	t.enqueued = append(t.enqueued, id)
	return nil
}

// Message implements application.Store.
func (s *Store) Message(_ context.Context, id uuid.UUID) (domain.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.msgs[id]
	if !ok {
		return domain.Message{}, application.ErrMessageNotFound
	}
	return m, nil
}

// FinishAttempt implements application.Store; a message no longer pending is left alone.
func (s *Store) FinishAttempt(_ context.Context, m domain.Message, attempted bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.msgs[m.ID]
	if !ok || cur.Status != domain.StatusPending {
		return nil
	}
	m.Attempts = cur.Attempts
	if attempted {
		m.Attempts++
	}
	s.msgs[m.ID] = m
	return nil
}

// AppointmentMessages implements application.Store.
func (s *Store) AppointmentMessages(_ context.Context, businessID, appointmentID uuid.UUID) ([]domain.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []domain.Message{}
	for _, id := range s.order {
		if m := s.msgs[id]; m.BusinessID == businessID && m.AppointmentID == appointmentID {
			out = append(out, m)
		}
	}
	return out, nil
}

// Messages returns every message, in insertion order.
func (s *Store) Messages() []domain.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.Message, 0, len(s.order))
	for _, id := range s.order {
		out = append(out, s.msgs[id])
	}
	return out
}

// Enqueued returns the ids of the messages whose delivery was queued, in order.
func (s *Store) Enqueued() []uuid.UUID {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]uuid.UUID(nil), s.enqueued...)
}

// Sender records what it delivers.
type Sender struct {
	mu   sync.Mutex
	Sent []application.Outgoing
}

// Send implements application.Sender.
func (s *Sender) Send(_ context.Context, m application.Outgoing) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Sent = append(s.Sent, m)
	return "test-" + m.MessageID.String(), nil
}

// NewService is the notifications service over an in-memory store, with WhatsApp on and a
// recording sender.
func NewService(appointments scheduling.Appointments, reminders application.ReminderSettings, clk clock.Clock) (*application.Service, *Store) {
	st := NewStore()
	return application.NewService(application.Deps{
		Store: st, Appointments: appointments, Reminders: reminders, Senders: map[domain.Channel]application.Sender{domain.ChannelWhatsApp: &Sender{}},
		Policy: application.Policy{WhatsAppEnabled: true}, Clock: clk,
	}), st
}

func (t *tx) ClaimWebhookEvent(_ context.Context, id string, _ time.Time) (bool, error) {
	t.s.mu.Lock()
	defer t.s.mu.Unlock()
	if t.s.events[id] || slices.Contains(t.events, id) {
		return false, nil
	}
	t.events = append(t.events, id)
	return true, nil
}

func (t *tx) EnqueueWebhookItem(_ context.Context, it domain.WebhookItem) error {
	t.items = append(t.items, it)
	return nil
}

func (t *tx) InsertActionTokens(_ context.Context, ks []domain.ActionToken) error {
	t.tokens = append(t.tokens, ks...)
	return nil
}

func (t *tx) ActionToken(_ context.Context, hash []byte) (domain.ActionToken, error) {
	t.s.mu.Lock()
	defer t.s.mu.Unlock()
	k, ok := t.s.tokens[string(hash)]
	if !ok {
		return domain.ActionToken{}, application.ErrTokenNotFound
	}
	if at, used := t.used[k.MessageID]; used && k.UsedAt.IsZero() {
		k.UsedAt = at
	}
	return k, nil
}

func (t *tx) LockMessageTokens(context.Context, uuid.UUID) error { return nil } // one transaction at a time

func (t *tx) UseMessageTokens(_ context.Context, id uuid.UUID, at time.Time) error {
	t.used[id] = at
	return nil
}

func (t *tx) EnqueueReply(_ context.Context, to, text string) error {
	t.replies = append(t.replies, Reply{To: to, Text: text})
	return nil
}

// UpdateDeliveryStatus implements application.Store with the forward-only rule.
func (s *Store) UpdateDeliveryStatus(_ context.Context, providerID string, st domain.Status, code string, at time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rank := map[domain.Status]int{domain.StatusSent: 1, domain.StatusDelivered: 2, domain.StatusRead: 3}
	for id, m := range s.msgs {
		if m.ProviderMessageID != providerID || providerID == "" {
			continue
		}
		cur, ok := rank[m.Status]
		switch {
		case !ok, st == domain.StatusFailed && cur > 2, st != domain.StatusFailed && rank[st] <= cur:
			return false, nil
		}
		m.Status, m.UpdatedAt = st, at
		if code != "" {
			m.ErrorCode = code
		}
		s.msgs[id] = m
		return true, nil
	}
	return false, nil
}

// WebhookItems returns the queued webhook items, in order.
func (s *Store) WebhookItems() []domain.WebhookItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]domain.WebhookItem(nil), s.items...)
}

// Tokens returns the stored action tokens of a message.
func (s *Store) Tokens(messageID uuid.UUID) []domain.ActionToken {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []domain.ActionToken
	for _, k := range s.tokens {
		if k.MessageID == messageID {
			out = append(out, k)
		}
	}
	return out
}

// Replies returns the queued answers, in order.
func (s *Store) Replies() []Reply {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Reply(nil), s.replies...)
}
