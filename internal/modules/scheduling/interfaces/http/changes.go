package httpapi

// Changes of an appointment (T6.6): the customer on its own appointments, the staff on the
// business's.

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/httpx"
)

// Problem codes of the changes.
const (
	CodeInvalidTransition  = "INVALID_TRANSITION"
	CodeTooEarly           = "TOO_EARLY"
	CodeAlreadyStarted     = "APPOINTMENT_STARTED"
	CodeCutoffPassed       = "CUTOFF_PASSED"
	CodeAppointmentChanged = "APPOINTMENT_CHANGED"
)

// ChangeService is the subset of the application used by the change endpoints.
type ChangeService interface {
	CustomerChange(ctx context.Context, p contract.Principal, id uuid.UUID, action application.Action, reason string) (domain.Appointment, error)
	CustomerReschedule(ctx context.Context, p contract.Principal, id uuid.UUID, start time.Time) (domain.Appointment, error)
	StaffChange(ctx context.Context, p contract.Principal, businessID, id uuid.UUID, action application.Action, reason string) (domain.Appointment, error)
	StaffReschedule(ctx context.Context, p contract.Principal, businessID, id uuid.UUID, start time.Time, professionalID uuid.UUID) (domain.Appointment, error)
	History(ctx context.Context, p contract.Principal, businessID, id uuid.UUID) ([]domain.Event, error)
}

type reasonRequest struct {
	Reason string `json:"reason"`
}

type rescheduleRequest struct {
	StartAt        time.Time `json:"startAt"`
	ProfessionalID uuid.UUID `json:"professionalId"`
}

type eventDTO struct {
	Type                   string     `json:"type"`
	Status                 string     `json:"status"`
	FromStatus             *string    `json:"fromStatus"`
	ProfessionalID         uuid.UUID  `json:"professionalId"`
	StartAt                time.Time  `json:"startAt"`
	PreviousStartAt        *time.Time `json:"previousStartAt"`
	PreviousProfessionalID *uuid.UUID `json:"previousProfessionalId"`
	Reason                 *string    `json:"reason"`
	ActorRole              *string    `json:"actorRole"`
	OccurredAt             time.Time  `json:"occurredAt"`
}

type historyDTO struct {
	Items []eventDTO `json:"items"`
}

func optional[T comparable](v T) *T {
	var zero T
	if v == zero {
		return nil
	}
	return &v
}

func (h *Handler) changeRoutes(rt *httpx.Router) {
	auth := func(f http.HandlerFunc) http.Handler { return h.requireAuth(f) }
	for _, a := range []application.Action{application.ActionConfirm, application.ActionCancel} {
		rt.Handle("POST /api/v1/me/appointments/{appointmentId}/"+string(a), auth(h.customerChange(a)))
	}
	rt.Handle("POST /api/v1/me/appointments/{appointmentId}/reschedule", auth(h.customerReschedule))
	for _, a := range []application.Action{application.ActionConfirm, application.ActionCancel, application.ActionStart, application.ActionComplete, application.ActionNoShow} {
		rt.Handle("POST /api/v1/business/{businessId}/appointments/{appointmentId}/"+string(a), auth(h.staffChange(a)))
	}
	rt.Handle("POST /api/v1/business/{businessId}/appointments/{appointmentId}/reschedule", auth(h.staffReschedule))
	rt.Handle("GET /api/v1/business/{businessId}/appointments/{appointmentId}/history", auth(h.history))
}

// optionalBody decodes a JSON body when there is one (confirm, start… take none; a reason is
// optional for the customer).
func optionalBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	if r.ContentLength == 0 {
		return true
	}
	return httpx.DecodeJSON(w, r, dst)
}

func (h *Handler) customerChange(action application.Action) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ids, ok := h.pathIDs(w, r, "appointmentId")
		if !ok {
			return
		}
		var req reasonRequest
		if !optionalBody(w, r, &req) {
			return
		}
		p, _ := contract.PrincipalFrom(r.Context())
		a, err := h.changes.CustomerChange(r.Context(), p, ids[0], action, req.Reason)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, toAppointmentDTO(a))
	}
}

func (h *Handler) customerReschedule(w http.ResponseWriter, r *http.Request) {
	ids, ok := h.pathIDs(w, r, "appointmentId")
	if !ok {
		return
	}
	var req struct {
		StartAt time.Time `json:"startAt"`
	}
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if req.StartAt.IsZero() {
		badQuery(w, r, "startAt", application.CodeRequired)
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	a, err := h.changes.CustomerReschedule(r.Context(), p, ids[0], req.StartAt)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toAppointmentDTO(a))
}

func (h *Handler) staffChange(action application.Action) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ids, ok := h.pathIDs(w, r, "businessId", "appointmentId")
		if !ok {
			return
		}
		var req reasonRequest
		if !optionalBody(w, r, &req) {
			return
		}
		p, _ := contract.PrincipalFrom(r.Context())
		a, err := h.changes.StaffChange(r.Context(), p, ids[0], ids[1], action, req.Reason)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, toAppointmentDTO(a))
	}
}

func (h *Handler) staffReschedule(w http.ResponseWriter, r *http.Request) {
	ids, ok := h.pathIDs(w, r, "businessId", "appointmentId")
	if !ok {
		return
	}
	var req rescheduleRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	a, err := h.changes.StaffReschedule(r.Context(), p, ids[0], ids[1], req.StartAt, req.ProfessionalID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toAppointmentDTO(a))
}

func (h *Handler) history(w http.ResponseWriter, r *http.Request) {
	ids, ok := h.pathIDs(w, r, "businessId", "appointmentId")
	if !ok {
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	events, err := h.changes.History(r.Context(), p, ids[0], ids[1])
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := historyDTO{Items: make([]eventDTO, 0, len(events))}
	for _, e := range events {
		d := eventDTO{
			Type: string(e.Type), Status: string(e.Status), FromStatus: optional(string(e.FromStatus)), ProfessionalID: e.ProfessionalID,
			StartAt: e.Start, PreviousProfessionalID: optional(e.PreviousProfessionalID), Reason: optional(e.Reason),
			ActorRole: optional(string(e.ActorRole)), OccurredAt: e.At,
		}
		if !e.PreviousStart.IsZero() {
			ps := e.PreviousStart
			d.PreviousStartAt = &ps
		}
		out.Items = append(out.Items, d)
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
