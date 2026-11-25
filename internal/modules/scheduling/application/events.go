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
		ID: e.ID, BusinessID: a.BusinessID, AppointmentID: a.ID, Type: string(e.Type), ActorRole: string(e.ActorRole), Status: string(e.Status),
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
			CustomerName: a.Customer.Name, ServiceName: a.Service.Name, CancelReason: a.CancelReason, CustomerUserID: a.Customer.UserID,
		}
		if a.Customer.CustomerID == uuid.Nil {
			return nil
		}
		c, err := tx.CustomerByID(ctx, a.BusinessID, a.Customer.CustomerID)
		if errors.Is(err, ErrCustomerNotFound) {
			return nil
		}
		out.CustomerEmail = c.Email
		return err
	})
	if err != nil {
		return out, contractError(err)
	}
	return out, s.describeState(ctx, &out)
}

// describeState adds the names of the business and the professional and the business's zone.
func (s *Service) describeState(ctx context.Context, st *contract.AppointmentState) error {
	if s.d.Businesses != nil {
		sums, err := s.d.Businesses.BusinessSummaries(ctx, []uuid.UUID{st.BusinessID})
		if err != nil {
			return err
		}
		b := sums[st.BusinessID]
		st.BusinessName, st.TimeZone = b.Name, b.TimeZone
		st.BusinessAddress, st.BusinessPhone, st.BusinessLatitude, st.BusinessLongitude = b.AddressLine, b.ContactPhone, b.Latitude, b.Longitude
	}
	if s.d.Professionals != nil {
		roster, err := s.d.Professionals.Roster(ctx, st.BusinessID)
		if err != nil {
			return err
		}
		for _, r := range roster {
			if r.ID == st.ProfessionalID {
				st.ProfessionalName = r.DisplayName
			}
		}
	}
	return nil
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

// ConfirmAsCustomer implements contract.CustomerReplies.
func (s *Service) ConfirmAsCustomer(ctx context.Context, businessID, id uuid.UUID) error {
	_, err := s.change(ctx, actor{role: domain.ActorCustomer, businessID: businessID, onBehalf: true}, id, ActionConfirm, "")
	return replyError(err)
}

// CancelAsCustomer implements contract.CustomerReplies (with the customer's cut-off).
func (s *Service) CancelAsCustomer(ctx context.Context, businessID, id uuid.UUID) error {
	_, err := s.change(ctx, actor{role: domain.ActorCustomer, businessID: businessID, onBehalf: true}, id, ActionCancel, "")
	return replyError(err)
}

func replyError(err error) error {
	switch {
	case errors.Is(err, domain.ErrCutoffPassed):
		return errors.Join(contract.ErrCutoffPassed, err)
	case errors.Is(err, domain.ErrInvalidTransition), errors.Is(err, domain.ErrAlreadyStarted):
		return errors.Join(contract.ErrNotActive, err)
	}
	return contractError(err)
}
