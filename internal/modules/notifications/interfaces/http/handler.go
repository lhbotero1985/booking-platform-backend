// Package httpapi exposes the notifications endpoints (T7.1): the staff see which messages an
// appointment produced and how each one ended.
package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/domain"
	scheduling "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/httpx"
)

// Service is the subset of the application used by these handlers.
type Service interface {
	AppointmentMessages(ctx context.Context, p contract.Principal, businessID, appointmentID uuid.UUID) ([]domain.Message, error)
}

// Handler serves the notifications endpoints.
type Handler struct {
	svc         Service
	logger      *slog.Logger
	requireAuth httpx.Middleware
}

// New builds the handler; requireAuth is the identity module's authentication middleware.
func New(svc Service, logger *slog.Logger, requireAuth httpx.Middleware) *Handler {
	return &Handler{svc: svc, logger: logger, requireAuth: requireAuth}
}

// Routes registers the endpoints.
func (h *Handler) Routes(rt *httpx.Router) {
	rt.Handle("GET /api/v1/business/{businessId}/appointments/{appointmentId}/notifications", h.requireAuth(http.HandlerFunc(h.list)))
}

type messageDTO struct {
	ID            uuid.UUID  `json:"id"`
	Kind          string     `json:"kind"`
	Channel       string     `json:"channel"`
	RecipientRole string     `json:"recipientRole"`
	To            string     `json:"to"` // masked
	Status        string     `json:"status"`
	SkipReason    *string    `json:"skipReason"`
	ErrorCode     *string    `json:"errorCode"`
	Attempts      int        `json:"attempts"`
	SendAt        time.Time  `json:"sendAt"` // when the delivery is due (reminders: before the appointment)
	CreatedAt     time.Time  `json:"createdAt"`
	SentAt        *time.Time `json:"sentAt"`
}

type listDTO struct {
	Items []messageDTO `json:"items"`
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	businessID, err1 := uuid.Parse(r.PathValue("businessId"))
	appointmentID, err2 := uuid.Parse(r.PathValue("appointmentId"))
	if err1 != nil || err2 != nil {
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusNotFound, httpx.CodeNotFound, ""))
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	msgs, err := h.svc.AppointmentMessages(r.Context(), p, businessID, appointmentID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := listDTO{Items: make([]messageDTO, 0, len(msgs))}
	for _, m := range msgs {
		d := messageDTO{
			ID: m.ID, Kind: string(m.Kind), Channel: string(m.Channel), RecipientRole: string(m.RecipientRole),
			To: m.MaskRecipient(), Status: string(m.Status), SkipReason: optional(string(m.SkipReason)),
			ErrorCode: optional(m.ErrorCode), Attempts: m.Attempts, SendAt: m.SendAt, CreatedAt: m.CreatedAt,
		}
		if !m.SentAt.IsZero() {
			t := m.SentAt
			d.SentAt = &t
		}
		out.Items = append(out.Items, d)
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	if httpx.WriteAuthzProblem(w, r, err) {
		return
	}
	if errors.Is(err, scheduling.ErrAppointmentNotFound) {
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusNotFound, httpx.CodeNotFound, ""))
		return
	}
	h.logger.ErrorContext(r.Context(), "notifications request failed",
		slog.String("request_id", httpx.RequestIDFrom(r.Context())),
		slog.String("error", err.Error()))
	httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusInternalServerError, httpx.CodeInternal, ""))
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
