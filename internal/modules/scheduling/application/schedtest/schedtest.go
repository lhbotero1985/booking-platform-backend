// Package schedtest provides an in-memory scheduling store for tests without PostgreSQL. It
// mimics the exclusion constraint and the idempotency key table; transactions are serialized by
// one mutex (PostgreSQL behavior is covered by the integration tests).
package schedtest

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/domain"
)

type key struct {
	user uuid.UUID
	key  string
}

type idem struct {
	fingerprint string
	appointment uuid.UUID
}

// Event is a recorded appointment event.
type Event struct {
	AppointmentID uuid.UUID
	Type          domain.EventType
	Actor         uuid.UUID
}

// Store is the in-memory store.
type Store struct {
	mu           sync.Mutex
	Appointments map[uuid.UUID]domain.Appointment
	Events       []Event
	keys         map[key]idem
}

// NewStore returns an empty store.
func NewStore() *Store {
	return &Store{Appointments: map[uuid.UUID]domain.Appointment{}, keys: map[key]idem{}}
}

// WithinTx implements application.Store: all-or-nothing on a copy, one transaction at a time.
func (s *Store) WithinTx(ctx context.Context, fn func(ctx context.Context, tx application.Tx) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := &tx{s: s, appts: map[uuid.UUID]domain.Appointment{}, keys: map[key]idem{}}
	for id, a := range s.Appointments {
		t.appts[id] = a
	}
	for k, v := range s.keys {
		t.keys[k] = v
	}
	events := append([]Event(nil), s.Events...)
	t.events = events
	if err := fn(ctx, t); err != nil {
		return err
	}
	s.Appointments, s.keys, s.Events = t.appts, t.keys, t.events
	return nil
}

type tx struct {
	s      *Store
	appts  map[uuid.UUID]domain.Appointment
	keys   map[key]idem
	events []Event
}

func (t *tx) SetBookingTimeouts(context.Context) error { return nil }

func (t *tx) ClaimIdempotencyKey(_ context.Context, userID uuid.UUID, k, fp string, _ time.Time) (bool, error) {
	if _, ok := t.keys[key{userID, k}]; ok {
		return false, nil
	}
	t.keys[key{userID, k}] = idem{fingerprint: fp}
	return true, nil
}

func (t *tx) IdempotencyKeyOf(_ context.Context, userID uuid.UUID, k string) (string, uuid.UUID, error) {
	v, ok := t.keys[key{userID, k}]
	if !ok {
		return "", uuid.Nil, errors.New("schedtest: no such key")
	}
	return v.fingerprint, v.appointment, nil
}

func (t *tx) SetIdempotencyAppointment(_ context.Context, userID uuid.UUID, k string, id uuid.UUID) error {
	v := t.keys[key{userID, k}]
	v.appointment = id
	t.keys[key{userID, k}] = v
	return nil
}

func (t *tx) InsertAppointment(_ context.Context, a domain.Appointment) error {
	for _, b := range t.appts {
		if b.ProfessionalID == a.ProfessionalID && b.Status.Occupies() && a.Status.Occupies() &&
			a.Start.Before(b.OccupiedUntil()) && b.Start.Before(a.OccupiedUntil()) {
			return application.ErrSlotUnavailable // the exclusion constraint
		}
	}
	t.appts[a.ID] = a
	return nil
}

func (t *tx) InsertAppointmentEvent(_ context.Context, _ uuid.UUID, a domain.Appointment, typ domain.EventType, actor uuid.UUID, _ time.Time) error {
	t.events = append(t.events, Event{AppointmentID: a.ID, Type: typ, Actor: actor})
	return nil
}

func (t *tx) AppointmentByID(_ context.Context, id uuid.UUID) (domain.Appointment, error) {
	a, ok := t.appts[id]
	if !ok {
		return domain.Appointment{}, errors.New("schedtest: no such appointment")
	}
	return a, nil
}

func (t *tx) ActiveAppointments(_ context.Context, businessID, professionalID uuid.UUID, from, to time.Time) ([]domain.Booked, error) {
	var out []domain.Booked
	for _, a := range t.appts {
		if a.BusinessID == businessID && a.ProfessionalID == professionalID && a.Status.Occupies() &&
			a.Start.Before(to) && a.OccupiedUntil().After(from) {
			out = append(out, domain.Booked{Start: a.Start, End: a.End, Buffer: a.Buffer})
		}
	}
	return out, nil
}
