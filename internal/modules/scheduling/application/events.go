package application

import (
	"context"
	"errors"

	"github.com/google/uuid"

	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/domain"
)

// SetEventListener tells l of every recorded change of an appointment, inside its transaction
// (T7.1: the notifications module queues its messages there). Call once, before serving.
func (s *Service) SetEventListener(l contract.EventListener) { s.events = l }

// record stores the event of a change and tells the listener, in the caller's transaction.
func (s *Service) record(ctx context.Context, tx Tx, a domain.Appointment, e domain.Event) error {
	if err := tx.InsertAppointmentEvent(ctx, a.BusinessID, a.ID, e); err != nil {
		return err
	}
	if s.events == nil {
		return nil
	}
	return s.events.AppointmentEventRecorded(ctx, contract.AppointmentEvent{
		ID: e.ID, BusinessID: a.BusinessID, AppointmentID: a.ID, Type: string(e.Type), ActorRole: string(e.ActorRole),
		ProfessionalID: e.ProfessionalID, Start: e.Start, CustomerPhone: a.Customer.Phone,
		CustomerHasAccount: a.Customer.UserID != uuid.Nil, At: e.At,
	})
}

// AppointmentState implements contract.Appointments.
func (s *Service) AppointmentState(ctx context.Context, businessID, id uuid.UUID) (contract.AppointmentState, error) {
	var out contract.AppointmentState
	err := s.d.Store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		a, err := tx.AppointmentByID(ctx, id)
		if err != nil {
			return err
		}
		if a.BusinessID != businessID {
			return ErrAppointmentNotFound
		}
		out = contract.AppointmentState{
			ID: a.ID, BusinessID: a.BusinessID, ProfessionalID: a.ProfessionalID, Status: string(a.Status), Start: a.Start, Version: a.Version,
		}
		return nil
	})
	return out, contractError(err)
}

// AuthorizeAppointmentView implements contract.Appointments with the rule of History.
func (s *Service) AuthorizeAppointmentView(ctx context.Context, p identity.Principal, businessID, id uuid.UUID) error {
	_, err := s.History(ctx, p, businessID, id)
	return contractError(err)
}

func contractError(err error) error {
	if errors.Is(err, ErrAppointmentNotFound) {
		return errors.Join(contract.ErrAppointmentNotFound, err)
	}
	return err
}
