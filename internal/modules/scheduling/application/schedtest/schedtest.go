// Package schedtest provides an in-memory scheduling store for tests without PostgreSQL. It
// mimics the exclusion constraint and the idempotency key table; transactions are serialized by
// one mutex (PostgreSQL behavior is covered by the integration tests).
package schedtest

import (
	"context"
	"errors"
	"slices"
	"sort"
	"strings"
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
	Customers    map[uuid.UUID]domain.BusinessCustomer
	Events       []Event
	keys         map[key]idem
}

// NewStore returns an empty store.
func NewStore() *Store {
	return &Store{Appointments: map[uuid.UUID]domain.Appointment{}, Customers: map[uuid.UUID]domain.BusinessCustomer{}, keys: map[key]idem{}}
}

// WithinTx implements application.Store: all-or-nothing on a copy, one transaction at a time.
func (s *Store) WithinTx(ctx context.Context, fn func(ctx context.Context, tx application.Tx) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := &tx{s: s, appts: map[uuid.UUID]domain.Appointment{}, customers: map[uuid.UUID]domain.BusinessCustomer{}, keys: map[key]idem{}}
	for id, a := range s.Appointments {
		t.appts[id] = a
	}
	for id, c := range s.Customers {
		t.customers[id] = c
	}
	for k, v := range s.keys {
		t.keys[k] = v
	}
	events := append([]Event(nil), s.Events...)
	t.events = events
	if err := fn(ctx, t); err != nil {
		return err
	}
	s.Appointments, s.Customers, s.keys, s.Events = t.appts, t.customers, t.keys, t.events
	return nil
}

type tx struct {
	s         *Store
	customers map[uuid.UUID]domain.BusinessCustomer
	appts     map[uuid.UUID]domain.Appointment
	keys      map[key]idem
	events    []Event
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
		return domain.Appointment{}, application.ErrAppointmentNotFound
	}
	return a, nil
}

func after(a domain.Appointment, c *application.Cursor, ascending bool) bool {
	if c == nil {
		return true
	}
	if !a.Start.Equal(c.Start) {
		return a.Start.After(c.Start) == ascending
	}
	return (a.ID.String() > c.ID.String()) == ascending
}

func sortAppointments(out []domain.Appointment, ascending bool) {
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Start.Equal(out[j].Start) {
			return out[i].Start.Before(out[j].Start) == ascending
		}
		return (out[i].ID.String() < out[j].ID.String()) == ascending
	})
}

func (t *tx) AgendaAppointments(_ context.Context, businessID uuid.UUID, professionalIDs []uuid.UUID, from, to time.Time, c *application.Cursor, limit int) ([]domain.Appointment, error) {
	var out []domain.Appointment
	for _, a := range t.appts {
		if a.BusinessID != businessID || a.Start.Before(from) || !a.Start.Before(to) || !after(a, c, true) {
			continue
		}
		if professionalIDs != nil && !slices.Contains(professionalIDs, a.ProfessionalID) {
			continue
		}
		out = append(out, a)
	}
	sortAppointments(out, true)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (t *tx) CustomerAppointments(_ context.Context, userID uuid.UUID, now time.Time, upcoming bool, c *application.Cursor, limit int) ([]domain.Appointment, error) {
	var out []domain.Appointment
	for _, a := range t.appts {
		if a.Customer.UserID != userID || a.Start.Before(now) != !upcoming || !after(a, c, upcoming) {
			continue
		}
		out = append(out, a)
	}
	sortAppointments(out, upcoming)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
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

func (t *tx) phoneTaken(c domain.BusinessCustomer) bool {
	for _, o := range t.customers {
		if o.ID != c.ID && o.BusinessID == c.BusinessID && o.Phone == c.Phone {
			return true
		}
	}
	return false
}

func (t *tx) InsertCustomer(_ context.Context, c domain.BusinessCustomer) error {
	if t.phoneTaken(c) {
		return application.ErrCustomerPhoneTaken
	}
	t.customers[c.ID] = c
	return nil
}

func (t *tx) UpdateCustomer(_ context.Context, c domain.BusinessCustomer) error {
	if _, ok := t.customers[c.ID]; !ok {
		return application.ErrCustomerNotFound
	}
	if t.phoneTaken(c) {
		return application.ErrCustomerPhoneTaken
	}
	t.customers[c.ID] = c
	return nil
}

func (t *tx) CustomerByID(_ context.Context, businessID, id uuid.UUID) (domain.BusinessCustomer, error) {
	c, ok := t.customers[id]
	if !ok || c.BusinessID != businessID {
		return domain.BusinessCustomer{}, application.ErrCustomerNotFound
	}
	return c, nil
}

func (t *tx) CustomerByPhone(_ context.Context, businessID uuid.UUID, phone string) (domain.BusinessCustomer, error) {
	for _, c := range t.customers {
		if c.BusinessID == businessID && c.Phone == phone {
			return c, nil
		}
	}
	return domain.BusinessCustomer{}, application.ErrCustomerNotFound
}

// ListCustomers unescapes the LIKE pattern and matches like the SQL query.
func (t *tx) ListCustomers(_ context.Context, businessID uuid.UUID, q string, limit int) ([]domain.BusinessCustomer, error) {
	q = strings.NewReplacer(`\%`, `%`, `\_`, `_`, `\`, `\`).Replace(q)
	var out []domain.BusinessCustomer
	for _, c := range t.customers {
		if c.BusinessID == businessID && (q == "" || strings.Contains(strings.ToLower(c.FullName), strings.ToLower(q)) || strings.Contains(c.Phone, q)) {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].FullName) < strings.ToLower(out[j].FullName) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
