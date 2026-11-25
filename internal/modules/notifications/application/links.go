package application

// Links of the appointment emails (T7.7, ADR-0019): "Confirmar cita", "Reagendar" and "Cancelar
// cita" carry single-use codes like the WhatsApp buttons (only their SHA-256 is stored; they
// expire at the start). Two taps: opening a link only shows the appointment (Link); the change
// happens when the customer presses the page's button (UseLink), because mail scanners open the
// links of every email. Whoever holds the code acts as the customer, with the customer's rules.

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/domain"
	scheduling "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/contract"
)

// LinkView is what the page of an email link shows.
type LinkView struct {
	Action domain.Action
	// Outcome is "" while the link can be used (Link) and the result once used (UseLink):
	// confirmed, canceled, rescheduled, cut-off passed, not active, slot unavailable or INVALID
	// (unknown, used or expired code, or the appointment changed since the email).
	Outcome domain.Outcome
	// Content is empty for an INVALID link (it says nothing about the appointment).
	Content domain.Content
	// What a reschedule page needs to ask the public availability.
	BusinessID, ServiceID, ProfessionalID uuid.UUID
	BusinessSlug                          string
}

// emailActions are the links of an email to the customer about an active appointment: confirm
// while pending, reschedule while the business's limit allows it (T7.8), cancel.
func emailActions(kind domain.Kind, role domain.Role, c domain.Content) []domain.Action {
	if role != domain.RecipientCustomer || kind == domain.KindCancelled || kind == domain.KindCancelledByCustomer {
		return nil
	}
	var out []domain.Action
	if c.Status == scheduling.StatusPendingConfirmation {
		out = append(out, domain.ActionConfirm)
	}
	if c.CanReschedule() {
		out = append(out, domain.ActionReschedule)
	}
	return append(out, domain.ActionCancel)
}

// Link reads what an email link would do, changing nothing.
func (s *Service) Link(ctx context.Context, raw string, action domain.Action) (LinkView, error) {
	var view LinkView
	err := s.d.Store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		var err error
		view, _, err = s.checkLink(ctx, tx, raw, action)
		return err
	})
	return view, err
}

// UseLink applies an email link (the second tap). A confirmation spends only its own link; a
// cancellation or a new time spends every link of the email. start is the new time (reschedule).
func (s *Service) UseLink(ctx context.Context, raw string, action domain.Action, start time.Time) (LinkView, error) {
	var view LinkView
	err := s.d.Store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		tok, err := tx.ActionToken(ctx, domain.HashActionToken(raw))
		if errors.Is(err, ErrTokenNotFound) {
			view = LinkView{Action: action, Outcome: domain.OutcomeInvalid}
			return nil
		}
		if err != nil {
			return err
		}
		// The links of one email change the appointment one at a time: lock them all first.
		if err := tx.LockMessageTokens(ctx, tok.MessageID); err != nil {
			return err
		}
		var ok bool
		if view, ok, err = s.checkLink(ctx, tx, raw, action); err != nil || !ok {
			return err
		}
		now := s.d.Clock.Now()
		switch action {
		case domain.ActionConfirm:
			view.Outcome, err = domain.OutcomeConfirmed, s.d.Replies.ConfirmAsCustomer(ctx, tok.BusinessID, tok.AppointmentID)
		case domain.ActionCancel:
			view.Outcome, err = domain.OutcomeCancelled, s.d.Replies.CancelAsCustomer(ctx, tok.BusinessID, tok.AppointmentID)
		case domain.ActionReschedule:
			view.Outcome, err = domain.OutcomeRescheduled, s.d.Replies.RescheduleAsCustomer(ctx, tok.BusinessID, tok.AppointmentID, start)
		}
		switch {
		case errors.Is(err, scheduling.ErrCutoffPassed):
			view.Outcome = domain.OutcomeCutoff
			return nil
		case errors.Is(err, scheduling.ErrRescheduleLimit):
			view.Outcome = domain.OutcomeRescheduleLimit // not spent: it may still confirm or cancel
			return nil
		case errors.Is(err, scheduling.ErrSlotUnavailable):
			view.Outcome = domain.OutcomeSlotUnavailable // pick another time: the link stays valid
			return nil
		case errors.Is(err, scheduling.ErrNotActive):
			view.Outcome = domain.OutcomeNotActive
		case errors.Is(err, scheduling.ErrAppointmentNotFound):
			view = LinkView{Action: action, Outcome: domain.OutcomeInvalid}
		case err != nil:
			return err
		}
		if view.Outcome == domain.OutcomeConfirmed {
			return tx.UseActionToken(ctx, tok.Hash, now)
		}
		if view.Outcome == domain.OutcomeRescheduled || view.Outcome == domain.OutcomeCancelled {
			if st, err := s.d.Appointments.AppointmentState(ctx, tok.BusinessID, tok.AppointmentID); err == nil {
				view.Content = contentOf(st) // the new time or the canceled status
			}
		}
		return tx.UseMessageTokens(ctx, tok.MessageID, now)
	})
	return view, err
}

// checkLink: ok when the link can be used now. Otherwise the view says why (an unknown, used or
// expired code, a code of another action, or an appointment moved since the email: INVALID; an
// appointment no longer active: NOT_ACTIVE).
func (s *Service) checkLink(ctx context.Context, tx Tx, raw string, action domain.Action) (LinkView, bool, error) {
	invalid := LinkView{Action: action, Outcome: domain.OutcomeInvalid}
	tok, err := tx.ActionToken(ctx, domain.HashActionToken(raw))
	if errors.Is(err, ErrTokenNotFound) {
		return invalid, false, nil
	}
	if err != nil {
		return LinkView{}, false, err
	}
	if tok.Action != action || !tok.UsedAt.IsZero() || !s.d.Clock.Now().Before(tok.ExpiresAt) {
		return invalid, false, nil
	}
	m, err := s.d.Store.Message(ctx, tok.MessageID)
	if err != nil {
		return LinkView{}, false, err
	}
	st, err := s.d.Appointments.AppointmentState(ctx, tok.BusinessID, tok.AppointmentID)
	if errors.Is(err, scheduling.ErrAppointmentNotFound) {
		return invalid, false, nil
	}
	if err != nil {
		return LinkView{}, false, err
	}
	if st.ProfessionalID != m.ProfessionalID || !st.Start.Equal(m.Start) {
		return invalid, false, nil // moved since: a newer email has the links of the new time
	}
	view := LinkView{
		Action: action, Content: contentOf(st), BusinessID: st.BusinessID, ServiceID: st.ServiceID,
		ProfessionalID: st.ProfessionalID, BusinessSlug: st.BusinessSlug,
	}
	if err := s.addPolicy(ctx, st.BusinessID, &view.Content); err != nil {
		return LinkView{}, false, err
	}
	if st.Status != scheduling.StatusPendingConfirmation && st.Status != statusConfirmed {
		view.Outcome = domain.OutcomeNotActive
		return view, false, nil
	}
	if action == domain.ActionReschedule && !view.Content.CanReschedule() {
		view.Outcome = domain.OutcomeRescheduleLimit // e.g. the business lowered its limit after the email
		return view, false, nil
	}
	return view, true, nil
}

const statusConfirmed = "CONFIRMED"
