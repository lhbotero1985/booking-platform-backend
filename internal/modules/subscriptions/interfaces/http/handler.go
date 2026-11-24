// Package httpapi exposes the subscriptions endpoints (PLATFORM_ADMIN, T4.3b).
package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/subscriptions/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/subscriptions/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/httpx"
)

// CodeNotExtendable is returned when the subscription is not a (running or expired) demo.
const CodeNotExtendable = "NOT_EXTENDABLE"

// Service is the subset of the application used by these handlers.
type Service interface {
	Get(ctx context.Context, p contract.Principal, businessID uuid.UUID) (domain.Subscription, error)
	ExtendTrial(ctx context.Context, p contract.Principal, businessID uuid.UUID, days int) (domain.Subscription, error)
	ActivateCourtesy(ctx context.Context, p contract.Principal, businessID uuid.UUID) (domain.Subscription, error)
}

// Handler serves the subscriptions endpoints.
type Handler struct {
	svc         Service
	logger      *slog.Logger
	requireAuth httpx.Middleware
	clock       func() time.Time
}

// New builds the handler; requireAuth is the identity module's authentication middleware.
func New(svc Service, logger *slog.Logger, requireAuth httpx.Middleware) *Handler {
	return &Handler{svc: svc, logger: logger, requireAuth: requireAuth, clock: time.Now}
}

// Routes registers the endpoints.
func (h *Handler) Routes(rt *httpx.Router) {
	auth := func(f http.HandlerFunc) http.Handler { return h.requireAuth(f) }
	rt.Handle("GET /api/v1/admin/businesses/{id}/subscription", auth(h.get))
	rt.Handle("POST /api/v1/admin/businesses/{id}/subscription/extend-trial", auth(h.extend))
	rt.Handle("POST /api/v1/admin/businesses/{id}/subscription/activate", auth(h.activate))
}

// SubscriptionDTO is the representation of a subscription; Entitled tells whether the business
// may be visible and take bookings right now.
type SubscriptionDTO struct {
	BusinessID  uuid.UUID  `json:"businessId"`
	Status      string     `json:"status"`
	TrialEndsAt *time.Time `json:"trialEndsAt"`
	ExpiredAt   *time.Time `json:"expiredAt"`
	Entitled    bool       `json:"entitled"`
}

// ToDTO converts a subscription evaluated at now.
func ToDTO(s domain.Subscription, now time.Time) SubscriptionDTO {
	return SubscriptionDTO{
		BusinessID: s.BusinessID, Status: string(s.Status), TrialEndsAt: s.TrialEndsAt, ExpiredAt: s.ExpiredAt,
		Entitled: s.Entitled(now),
	}
}

type extendRequest struct {
	Days int `json:"days"`
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	h.byID(w, r, func(ctx context.Context, p contract.Principal, id uuid.UUID) (domain.Subscription, error) {
		return h.svc.Get(ctx, p, id)
	})
}

func (h *Handler) extend(w http.ResponseWriter, r *http.Request) {
	var req extendRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	h.byID(w, r, func(ctx context.Context, p contract.Principal, id uuid.UUID) (domain.Subscription, error) {
		return h.svc.ExtendTrial(ctx, p, id, req.Days)
	})
}

func (h *Handler) activate(w http.ResponseWriter, r *http.Request) {
	h.byID(w, r, h.svc.ActivateCourtesy)
}

func (h *Handler) byID(w http.ResponseWriter, r *http.Request, f func(context.Context, contract.Principal, uuid.UUID) (domain.Subscription, error)) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil || id == uuid.Nil {
		h.fail(w, r, application.ErrNotFound)
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	s, err := f(r.Context(), p, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, ToDTO(s, h.clock()))
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	if httpx.WriteAuthzProblem(w, r, err) {
		return
	}
	switch {
	case errors.Is(err, application.ErrNotFound):
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusNotFound, httpx.CodeNotFound, ""))
	case errors.Is(err, domain.ErrInvalidExtension):
		p := httpx.NewProblem(http.StatusBadRequest, httpx.CodeInvalidRequest, "one or more fields are invalid")
		p.Errors = []httpx.FieldError{{Field: "days", Message: "INVALID_DAYS"}}
		httpx.WriteProblem(w, r, p)
	case errors.Is(err, domain.ErrNotExtendable):
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusConflict, CodeNotExtendable, "only running or expired demos can be extended"))
	default:
		h.logger.ErrorContext(r.Context(), "subscriptions request failed",
			slog.String("request_id", httpx.RequestIDFrom(r.Context())),
			slog.String("error", err.Error()))
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusInternalServerError, httpx.CodeInternal, ""))
	}
}
