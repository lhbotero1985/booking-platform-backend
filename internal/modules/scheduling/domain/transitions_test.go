package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/domain"
)

func appointmentAt(start time.Time, status domain.Status, withAccount bool) domain.Appointment {
	c := domain.Customer{Name: "Ana", Phone: "+573001234567"}
	if withAccount {
		c.UserID = uuid.Must(uuid.NewV7())
	}
	return domain.Appointment{
		ID: uuid.Must(uuid.NewV7()), ProfessionalID: uuid.Must(uuid.NewV7()), Start: start, End: start.Add(30 * time.Minute),
		Service: domain.ServiceSnapshot{DurationMinutes: 30}, Customer: c, Status: status, Version: 1,
	}
}

func TestTransitions(t *testing.T) {
	start := time.Date(2030, 3, 5, 14, 0, 0, 0, time.UTC)
	before, after := start.Add(-3*time.Hour), start.Add(5*time.Minute)
	all := []domain.Status{domain.StatusPendingConfirmation, domain.StatusConfirmed, domain.StatusInProgress, domain.StatusCompleted, domain.StatusCancelled, domain.StatusNoShow}

	type outcome struct {
		status domain.Status
		err    error
	}
	cases := map[string]struct {
		apply func(a *domain.Appointment) error
		want  map[domain.Status]outcome
	}{
		"confirm": {func(a *domain.Appointment) error { _, err := a.Confirm(before); return err }, map[domain.Status]outcome{
			domain.StatusPendingConfirmation: {domain.StatusConfirmed, nil}, domain.StatusConfirmed: {domain.StatusConfirmed, nil},
		}},
		"cancel (business, before)": {func(a *domain.Appointment) error {
			_, err := a.Cancel(domain.ActorBusiness, "Enfermo", 0, before)
			return err
		}, map[domain.Status]outcome{
			domain.StatusPendingConfirmation: {domain.StatusCancelled, nil}, domain.StatusConfirmed: {domain.StatusCancelled, nil}, domain.StatusCancelled: {domain.StatusCancelled, nil},
		}},
		"start (10 min before)": {func(a *domain.Appointment) error { return a.Begin(start.Add(-10 * time.Minute)) }, map[domain.Status]outcome{
			domain.StatusPendingConfirmation: {domain.StatusInProgress, nil}, domain.StatusConfirmed: {domain.StatusInProgress, nil},
		}},
		"complete (after the start)": {func(a *domain.Appointment) error { return a.Complete(after) }, map[domain.Status]outcome{
			domain.StatusConfirmed: {domain.StatusCompleted, nil}, domain.StatusInProgress: {domain.StatusCompleted, nil},
		}},
		"no-show (after the start)": {func(a *domain.Appointment) error { return a.MarkNoShow(after) }, map[domain.Status]outcome{
			domain.StatusPendingConfirmation: {domain.StatusNoShow, nil}, domain.StatusConfirmed: {domain.StatusNoShow, nil},
		}},
	}
	for name, c := range cases {
		for _, from := range all {
			a := appointmentAt(start, from, true)
			err := c.apply(&a)
			want, allowed := c.want[from]
			switch {
			case allowed && (err != nil || a.Status != want.status):
				t.Errorf("%s from %s: %s %v, want %s", name, from, a.Status, err, want.status)
			case !allowed && !errors.Is(err, domain.ErrInvalidTransition):
				t.Errorf("%s from %s: %v, want ErrInvalidTransition", name, from, err)
			}
		}
	}

	// Time guards.
	a := appointmentAt(start, domain.StatusConfirmed, true)
	if err := a.Begin(start.Add(-16 * time.Minute)); !errors.Is(err, domain.ErrTooEarly) {
		t.Errorf("start 16 min before: %v", err)
	}
	if err := a.Complete(start.Add(-time.Minute)); !errors.Is(err, domain.ErrTooEarly) {
		t.Errorf("complete before the start: %v", err)
	}
	if err := a.MarkNoShow(start.Add(-time.Minute)); !errors.Is(err, domain.ErrTooEarly) {
		t.Errorf("no-show before the start: %v", err)
	}
	if _, err := a.Cancel(domain.ActorBusiness, "x", 0, start); !errors.Is(err, domain.ErrAlreadyStarted) {
		t.Errorf("cancel at the start: %v", err)
	}
	if _, err := a.Cancel(domain.ActorCustomer, "", 2*time.Hour, start.Add(-119*time.Minute)); !errors.Is(err, domain.ErrCutoffPassed) {
		t.Errorf("customer inside the cut-off: %v", err)
	}
	if changed, err := a.Cancel(domain.ActorCustomer, "", 2*time.Hour, start.Add(-2*time.Hour)); err != nil || !changed ||
		a.CancelledBy != domain.ActorCustomer || !a.CancelledAt.Equal(start.Add(-2*time.Hour)) {
		t.Errorf("customer exactly at the cut-off: %v %+v", err, a)
	}
	if a.Status.Occupies() {
		t.Error("a canceled appointment still occupies")
	}

	// Moves.
	m := appointmentAt(start, domain.StatusConfirmed, true)
	if err := m.CanMove(domain.ActorCustomer, 2*time.Hour, 0, start.Add(-time.Hour)); !errors.Is(err, domain.ErrCutoffPassed) {
		t.Errorf("customer move inside the cut-off: %v", err)
	}
	if err := m.CanMove(domain.ActorBusiness, 2*time.Hour, 0, start.Add(time.Minute)); !errors.Is(err, domain.ErrAlreadyStarted) {
		t.Errorf("move after the start: %v", err)
	}
	newPro := uuid.Must(uuid.NewV7())
	m.MoveTo(newPro, start.Add(2*time.Hour), domain.ActorCustomer, before)
	if m.Status != domain.StatusConfirmed || m.ProfessionalID != newPro || !m.End.Equal(start.Add(150*time.Minute)) {
		t.Errorf("moved by the customer: %+v", m)
	}
	m.MoveTo(newPro, start, domain.ActorBusiness, before)
	if m.Status != domain.StatusPendingConfirmation {
		t.Errorf("moved by the business, customer with account: %s", m.Status)
	}
	walkIn := appointmentAt(start, domain.StatusPendingConfirmation, false)
	walkIn.MoveTo(walkIn.ProfessionalID, start.Add(time.Hour), domain.ActorBusiness, before)
	if walkIn.Status != domain.StatusConfirmed {
		t.Errorf("moved by the business, customer without account: %s", walkIn.Status)
	}
	done := appointmentAt(start, domain.StatusCompleted, true)
	if err := done.CanMove(domain.ActorBusiness, 0, 0, before); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("move a completed one: %v", err)
	}

	// Reasons.
	if r, err := domain.ParseReason("  me   enfermé "); err != nil || r != "me enfermé" {
		t.Errorf("reason = %q %v", r, err)
	}
	for _, bad := range []string{strings.Repeat("a", 301), "con\x00nulo"} {
		if _, err := domain.ParseReason(bad); !errors.Is(err, domain.ErrInvalidReason) {
			t.Errorf("reason %q: %v", bad[:5], err)
		}
	}
}

// TestRescheduleLimit (T7.8): the business limits the customer's moves; its own moves do not
// count and are never limited; 0 = no limit.
func TestRescheduleLimit(t *testing.T) {
	start := time.Date(2030, 3, 5, 14, 0, 0, 0, time.UTC)
	now := start.Add(-48 * time.Hour)
	a := appointmentAt(start, domain.StatusConfirmed, true)
	for i := range 2 {
		if err := a.CanMove(domain.ActorCustomer, 2*time.Hour, 2, now); err != nil {
			t.Fatalf("move %d: %v", i+1, err)
		}
		a.MoveTo(a.ProfessionalID, a.Start.Add(time.Hour), domain.ActorCustomer, now)
	}
	if a.CustomerReschedules != 2 {
		t.Fatalf("counted %d", a.CustomerReschedules)
	}
	if err := a.CanMove(domain.ActorCustomer, 2*time.Hour, 2, now); !errors.Is(err, domain.ErrRescheduleLimit) {
		t.Errorf("third move by the customer: %v", err)
	}
	if err := a.CanMove(domain.ActorCustomer, 2*time.Hour, 3, now); err != nil {
		t.Errorf("the business raised its limit: %v", err)
	}
	if err := a.CanMove(domain.ActorCustomer, 2*time.Hour, 0, now); err != nil {
		t.Errorf("no limit: %v", err)
	}
	if err := a.CanMove(domain.ActorBusiness, 0, 2, now); err != nil {
		t.Errorf("the business moves it: %v", err)
	}
	a.MoveTo(a.ProfessionalID, a.Start.Add(time.Hour), domain.ActorBusiness, now)
	if a.CustomerReschedules != 2 {
		t.Errorf("a move by the business counted: %d", a.CustomerReschedules)
	}
	// The cut-off is reported first: it is what the customer can act on.
	late := appointmentAt(start, domain.StatusConfirmed, true)
	late.CustomerReschedules = 5
	if err := late.CanMove(domain.ActorCustomer, 2*time.Hour, 1, start.Add(-time.Hour)); !errors.Is(err, domain.ErrCutoffPassed) {
		t.Errorf("inside the cut-off and over the limit: %v", err)
	}
}
