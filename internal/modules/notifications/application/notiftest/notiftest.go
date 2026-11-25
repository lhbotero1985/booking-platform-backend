// Package notiftest is an in-memory notifications store for tests: a transaction is applied only
// when its function succeeds.
package notiftest

import (
	"context"
	"sync"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/domain"
	scheduling "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
)

// Store keeps messages in memory.
type Store struct {
	mu       sync.Mutex
	msgs     map[uuid.UUID]domain.Message
	order    []uuid.UUID
	enqueued []uuid.UUID
	// FailCommit, when set, makes every transaction fail after its function ran.
	FailCommit error
}

// NewStore builds an empty store.
func NewStore() *Store { return &Store{msgs: map[uuid.UUID]domain.Message{}} }

type tx struct {
	s        *Store
	msgs     []domain.Message
	enqueued []uuid.UUID
}

// WithinTx implements application.Store.
func (s *Store) WithinTx(ctx context.Context, fn func(ctx context.Context, tx application.Tx) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := &tx{s: s}
	if err := fn(ctx, t); err != nil {
		return err
	}
	if s.FailCommit != nil {
		return s.FailCommit
	}
	for _, m := range t.msgs {
		s.msgs[m.ID] = m
		s.order = append(s.order, m.ID)
	}
	s.enqueued = append(s.enqueued, t.enqueued...)
	return nil
}

func (t *tx) InsertMessage(_ context.Context, m domain.Message) (bool, error) {
	for _, old := range t.s.msgs {
		if old.EventID == m.EventID && old.Channel == m.Channel && old.RecipientRole == m.RecipientRole {
			return false, nil
		}
	}
	t.msgs = append(t.msgs, m)
	return true, nil
}

func (t *tx) EnqueueSend(_ context.Context, id uuid.UUID) error {
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
func NewService(appointments scheduling.Appointments, clk clock.Clock) (*application.Service, *Store) {
	st := NewStore()
	return application.NewService(application.Deps{
		Store: st, Appointments: appointments, Senders: map[domain.Channel]application.Sender{domain.ChannelWhatsApp: &Sender{}},
		Policy: application.Policy{WhatsAppEnabled: true}, Clock: clk,
	}), st
}
