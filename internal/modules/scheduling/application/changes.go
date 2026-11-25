package application

// Changes of an appointment (T6.6, booking-lifecycle.md, decisions of 2026-11-24). The customer
// confirms, cancels and reschedules its own appointments (cancel and reschedule before the
// business's cut-offs); the staff confirms, cancels (with a reason), reschedules, starts,
// completes and marks no-shows (a PROFESSIONAL only in its own agenda); only the OWNER moves an
// appointment to another professional. Every change locks the appointment row, keeps a version
// and writes an appointment_events row; moves also lock the agendas involved (ADR-0004).

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	businesses "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/contract"
	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	professionals "github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/authz"
)

// ErrAppointmentChanged means the appointment changed while it was being moved: read it again.
var ErrAppointmentChanged = errors.New("scheduling: the appointment changed meanwhile")

// Action is a status change of an appointment.
type Action string

// Actions.
const (
	ActionConfirm  Action = "confirm"
	ActionCancel   Action = "cancel"
	ActionStart    Action = "start"
	ActionComplete Action = "complete"
	ActionNoShow   Action = "no-show"
)

// actor is who changes an appointment and how it may see it.
type actor struct {
	p          identity.Principal
	role       domain.ActorRole
	businessID uuid.UUID     // staff: the business in the path
	staffRole  identity.Role // staff: OWNER or PROFESSIONAL
	// onBehalf: the customer of the appointment answering a message (T7.5), already
	// authenticated by the caller; it acts in businessID only.
	onBehalf bool
}

// CustomerChange applies a status change of one of the caller's appointments (confirm or cancel;
// reason optional). Anyone else's appointment is ErrAppointmentNotFound.
func (s *Service) CustomerChange(ctx context.Context, p identity.Principal, id uuid.UUID, action Action, reason string) (domain.Appointment, error) {
	if action != ActionConfirm && action != ActionCancel {
		return domain.Appointment{}, ErrAppointmentNotFound
	}
	return s.change(ctx, actor{p: p, role: domain.ActorCustomer}, id, action, reason)
}

// StaffChange applies a status change to an appointment of the business. A cancel needs a reason.
func (s *Service) StaffChange(ctx context.Context, p identity.Principal, businessID, id uuid.UUID, action Action, reason string) (domain.Appointment, error) {
	role, err := s.d.Authz.RequireBusinessRole(ctx, p, businessID, identity.RolesStaff...)
	if err != nil {
		return domain.Appointment{}, err
	}
	if action == ActionCancel && strings.TrimSpace(reason) == "" {
		return domain.Appointment{}, &ValidationError{Fields: []FieldError{{"reason", CodeRequired}}}
	}
	return s.change(ctx, actor{p: p, role: domain.ActorBusiness, businessID: businessID, staffRole: role}, id, action, reason)
}

// authorize checks that the actor may change a (it must be read first).
func (s *Service) authorize(ctx context.Context, who actor, a domain.Appointment) error {
	if who.onBehalf {
		if a.BusinessID != who.businessID {
			return ErrAppointmentNotFound
		}
		return nil
	}
	if who.role == domain.ActorCustomer {
		if a.Customer.UserID != who.p.UserID {
			return ErrAppointmentNotFound
		}
		return nil
	}
	if a.BusinessID != who.businessID {
		return ErrAppointmentNotFound
	}
	if who.staffRole == identity.RoleOwner {
		return nil
	}
	owner, err := s.d.Professionals.ProfessionalUserID(ctx, a.BusinessID, a.ProfessionalID)
	if err != nil && !errors.Is(err, professionals.ErrProfessionalNotFound) {
		return err
	}
	if owner != who.p.UserID {
		return authz.ErrForbidden
	}
	return nil
}

func (s *Service) change(ctx context.Context, who actor, id uuid.UUID, action Action, rawReason string) (domain.Appointment, error) {
	reason, err := domain.ParseReason(rawReason)
	if err != nil {
		return domain.Appointment{}, &ValidationError{Fields: []FieldError{{"reason", "INVALID_REASON"}}}
	}
	var out domain.Appointment
	err = s.d.Store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		a, err := tx.LockAppointment(ctx, id)
		if err != nil {
			return err
		}
		if err := s.authorize(ctx, who, a); err != nil {
			return err
		}
		if who.onBehalf {
			who.p.UserID = a.Customer.UserID // the event's actor: the customer's account, if any
		}
		before := a
		now := s.d.Clock.Now()
		changed, typ := true, domain.EventType("")
		switch action {
		case ActionConfirm:
			typ = domain.EventConfirmed
			changed, err = a.Confirm(now)
		case ActionCancel:
			typ = domain.EventCancelled
			cutoff := time.Duration(0)
			if who.role == domain.ActorCustomer {
				pol, perr := s.d.Businesses.BookingPolicyOf(ctx, a.BusinessID)
				if perr != nil {
					return perr
				}
				cutoff = time.Duration(pol.CancellationCutoffHours) * time.Hour
			}
			changed, err = a.Cancel(who.role, reason, cutoff, now)
		case ActionStart:
			typ, err = domain.EventStarted, a.Begin(now)
		case ActionComplete:
			typ, err = domain.EventCompleted, a.Complete(now)
		case ActionNoShow:
			typ, err = domain.EventNoShow, a.MarkNoShow(now)
		default:
			return domain.ErrInvalidTransition
		}
		if err != nil {
			return err
		}
		out = a
		if !changed {
			return nil
		}
		if err := tx.UpdateAppointment(ctx, a, before.Version); err != nil {
			return err
		}
		out.Version = before.Version + 1
		return s.record(ctx, tx, a, domain.Event{
			ID: uuid.Must(uuid.NewV7()), Type: typ, Status: a.Status, FromStatus: before.Status, ProfessionalID: a.ProfessionalID,
			Start: a.Start, Reason: reason, Actor: who.p.UserID, ActorRole: who.role, At: now,
		})
	})
	return out, err
}

// CustomerReschedule moves one of the caller's appointments to another start time (same
// professional), before the business's cut-off, if the business is still public.
func (s *Service) CustomerReschedule(ctx context.Context, p identity.Principal, id uuid.UUID, start time.Time) (domain.Appointment, error) {
	return s.move(ctx, actor{p: p, role: domain.ActorCustomer}, id, start, uuid.Nil)
}

// StaffReschedule moves an appointment of the business to another start time and, for the
// OWNER only, to another professional (uuid.Nil = the same).
func (s *Service) StaffReschedule(ctx context.Context, p identity.Principal, businessID, id uuid.UUID, start time.Time, professionalID uuid.UUID) (domain.Appointment, error) {
	role, err := s.d.Authz.RequireBusinessRole(ctx, p, businessID, identity.RolesStaff...)
	if err != nil {
		return domain.Appointment{}, err
	}
	if start.IsZero() {
		return domain.Appointment{}, &ValidationError{Fields: []FieldError{{"startAt", CodeRequired}}}
	}
	return s.move(ctx, actor{p: p, role: domain.ActorBusiness, businessID: businessID, staffRole: role}, id, start, professionalID)
}

func (s *Service) move(ctx context.Context, who actor, id uuid.UUID, start time.Time, target uuid.UUID) (domain.Appointment, error) {
	var out domain.Appointment
	err := s.d.Store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		if err := tx.SetBookingTimeouts(ctx); err != nil {
			return err
		}
		current, err := tx.AppointmentByID(ctx, id)
		if err != nil {
			return err
		}
		if err := s.authorize(ctx, who, current); err != nil {
			return err
		}
		if target == uuid.Nil {
			target = current.ProfessionalID
		}
		if target != current.ProfessionalID && (who.role != domain.ActorBusiness || who.staffRole != identity.RoleOwner) {
			return authz.ErrForbidden // only the OWNER moves an appointment to another professional
		}
		pol, err := s.policyFor(ctx, who, current.BusinessID)
		if err != nil {
			return err
		}
		loc, err := location(pol)
		if err != nil {
			return err
		}
		// Lock both agendas in a fixed order (no deadlock between crossed reassignments), then the
		// appointment, and check nothing moved in between.
		agendas := []uuid.UUID{current.ProfessionalID, target}
		sort.Slice(agendas, func(i, j int) bool { return agendas[i].String() < agendas[j].String() })
		for i, pro := range agendas {
			if i > 0 && pro == agendas[i-1] {
				continue
			}
			if err := s.d.Professionals.LockAgenda(ctx, current.BusinessID, pro); err != nil {
				return mapProfessional(err)
			}
		}
		a, err := tx.LockAppointment(ctx, id)
		if err != nil {
			return err
		}
		if a.Version != current.Version {
			return ErrAppointmentChanged
		}
		if who.onBehalf {
			who.p.UserID = a.Customer.UserID // the event's actor: the customer's account, if any
		}
		now := s.d.Clock.Now()
		if err := a.CanMove(who.role, time.Duration(pol.RescheduleCutoffHours)*time.Hour, now); err != nil {
			return err
		}
		if _, err := s.bookableService(ctx, a.BusinessID, a.Service.ServiceID, target); err != nil {
			if errors.Is(err, ErrNotFound) && who.role == domain.ActorBusiness {
				return &ValidationError{Fields: []FieldError{{"professionalId", CodeProfessionalInvalid}}}
			}
			return err
		}
		if target == a.ProfessionalID && start.Equal(a.Start) {
			out = a // nothing to move
			return nil
		}
		slots, err := s.slotsExcept(ctx, tx, pol, loc, a.Service.DurationMinutes, target, domain.LocalDateOf(start, loc), a.ID)
		if err != nil {
			return err
		}
		if !offered(slots, start) {
			return ErrSlotUnavailable
		}
		before := a
		a.MoveTo(target, start, who.role, now)
		if err := tx.UpdateAppointment(ctx, a, before.Version); err != nil {
			return err
		}
		a.Version = before.Version + 1
		e := domain.Event{
			ID: uuid.Must(uuid.NewV7()), Type: domain.EventRescheduled, Status: a.Status, FromStatus: before.Status,
			ProfessionalID: a.ProfessionalID, Start: a.Start, PreviousStart: before.Start, Actor: who.p.UserID, ActorRole: who.role, At: now,
		}
		if target != before.ProfessionalID {
			e.Type, e.PreviousProfessionalID = domain.EventReassigned, before.ProfessionalID
		}
		out = a
		return s.record(ctx, tx, a, e)
	})
	return out, err
}

// policyFor: the customer only while the business is public (with the minimum notice); the staff
// any time, without the minimum notice.
func (s *Service) policyFor(ctx context.Context, who actor, businessID uuid.UUID) (businesses.BookingPolicy, error) {
	if who.role == domain.ActorCustomer {
		pol, err := s.d.Businesses.PublicBookingPolicyOf(ctx, businessID)
		if errors.Is(err, businesses.ErrBusinessNotFound) {
			return pol, ErrNotFound
		}
		return pol, err
	}
	pol, err := s.d.Businesses.BookingPolicyOf(ctx, businessID)
	if errors.Is(err, businesses.ErrBusinessNotFound) {
		return pol, ErrNotFound
	}
	pol.MinLeadMinutes = 0
	return pol, err
}

func mapProfessional(err error) error {
	if errors.Is(err, professionals.ErrProfessionalNotFound) {
		return &ValidationError{Fields: []FieldError{{"professionalId", CodeProfessionalInvalid}}}
	}
	return err
}

// History returns the changes of an appointment of the business, oldest first (staff; a
// PROFESSIONAL only for its own agenda).
func (s *Service) History(ctx context.Context, p identity.Principal, businessID, id uuid.UUID) ([]domain.Event, error) {
	role, err := s.d.Authz.RequireBusinessRole(ctx, p, businessID, identity.RolesStaff...)
	if err != nil {
		return nil, err
	}
	var out []domain.Event
	err = s.d.Store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		a, err := tx.AppointmentByID(ctx, id)
		if err != nil {
			return err
		}
		if err := s.authorize(ctx, actor{p: p, role: domain.ActorBusiness, businessID: businessID, staffRole: role}, a); err != nil {
			return err
		}
		out, err = tx.AppointmentEvents(ctx, id)
		return err
	})
	return out, err
}
