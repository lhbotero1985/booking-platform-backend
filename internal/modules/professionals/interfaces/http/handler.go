// Package httpapi exposes the professionals endpoints under /api/v1/business/{businessId}
// (T5.2; weekly schedule T5.3; absences T5.4). The businessId is checked against the caller's memberships on every request; another
// business is a 404 identical to a missing one.
package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/httpx"
)

// CodeAlreadyProfessional is returned when the account already has a profile in this business.
const CodeAlreadyProfessional = "ALREADY_PROFESSIONAL"

// Service is the subset of the application used by these handlers.
type Service interface {
	Create(ctx context.Context, p contract.Principal, businessID uuid.UUID, in application.CreateInput) (application.View, error)
	List(ctx context.Context, p contract.Principal, businessID uuid.UUID) ([]application.View, error)
	Get(ctx context.Context, p contract.Principal, businessID, id uuid.UUID) (application.View, error)
	Update(ctx context.Context, p contract.Principal, businessID, id uuid.UUID, in application.UpdateInput) (application.View, error)
	ScheduleService
	AbsenceService
}

// Handler serves the professionals endpoints.
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
	auth := func(f http.HandlerFunc) http.Handler { return h.requireAuth(f) }
	rt.Handle("GET /api/v1/business/{businessId}/professionals", auth(h.list))
	rt.Handle("POST /api/v1/business/{businessId}/professionals", auth(h.create))
	rt.Handle("GET /api/v1/business/{businessId}/professionals/{professionalId}", auth(h.get))
	rt.Handle("PATCH /api/v1/business/{businessId}/professionals/{professionalId}", auth(h.update))
	rt.Handle("GET /api/v1/business/{businessId}/professionals/{professionalId}/schedule", auth(h.getSchedule))
	rt.Handle("PUT /api/v1/business/{businessId}/professionals/{professionalId}/schedule", auth(h.replaceSchedule))
	rt.Handle("GET /api/v1/business/{businessId}/professionals/{professionalId}/absences", auth(h.listAbsences))
	rt.Handle("POST /api/v1/business/{businessId}/professionals/{professionalId}/absences", auth(h.createAbsence))
	rt.Handle("PATCH /api/v1/business/{businessId}/professionals/{professionalId}/absences/{absenceId}", auth(h.updateAbsence))
	rt.Handle("DELETE /api/v1/business/{businessId}/professionals/{professionalId}/absences/{absenceId}", auth(h.deleteAbsence))
}

type professionalDTO struct {
	ID           uuid.UUID `json:"id"`
	UserID       uuid.UUID `json:"userId"`
	DisplayName  string    `json:"displayName"`
	Bio          string    `json:"bio"`
	Status       string    `json:"status"`
	SortOrder    int       `json:"sortOrder"`
	MemberActive bool      `json:"memberActive"`
	Bookable     bool      `json:"bookable"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type listDTO struct {
	Items []professionalDTO `json:"items"`
}

func toDTO(v application.View) professionalDTO {
	return professionalDTO{
		ID: v.ID, UserID: v.UserID, DisplayName: v.DisplayName, Bio: v.Bio, Status: string(v.Status),
		SortOrder: v.SortOrder, MemberActive: v.MemberActive, Bookable: v.Bookable, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt,
	}
}

type createRequest struct {
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	Bio         string `json:"bio"`
	SortOrder   *int   `json:"sortOrder"`
}

type updateRequest struct {
	DisplayName *string `json:"displayName"`
	Bio         *string `json:"bio"`
	Status      *string `json:"status"`
	SortOrder   *int    `json:"sortOrder"`
}

func ids(r *http.Request, names ...string) ([]uuid.UUID, bool) {
	out := make([]uuid.UUID, len(names))
	for i, n := range names {
		id, err := uuid.Parse(r.PathValue(n))
		if err != nil || id == uuid.Nil {
			return nil, false
		}
		out[i] = id
	}
	return out, true
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	v, ok := ids(r, "businessId")
	if !ok {
		h.fail(w, r, contract.ErrNotFound)
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	views, err := h.svc.List(r.Context(), p, v[0])
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := listDTO{Items: make([]professionalDTO, 0, len(views))}
	for _, view := range views {
		out.Items = append(out.Items, toDTO(view))
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	v, ok := ids(r, "businessId")
	if !ok {
		h.fail(w, r, contract.ErrNotFound)
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	view, err := h.svc.Create(r.Context(), p, v[0], application.CreateInput{
		Email: req.Email, DisplayName: req.DisplayName, Bio: req.Bio, SortOrder: req.SortOrder,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toDTO(view))
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	v, ok := ids(r, "businessId", "professionalId")
	if !ok {
		h.fail(w, r, contract.ErrNotFound)
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	view, err := h.svc.Get(r.Context(), p, v[0], v[1])
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toDTO(view))
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	var req updateRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	v, ok := ids(r, "businessId", "professionalId")
	if !ok {
		h.fail(w, r, contract.ErrNotFound)
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	view, err := h.svc.Update(r.Context(), p, v[0], v[1], application.UpdateInput{
		DisplayName: req.DisplayName, Bio: req.Bio, Status: req.Status, SortOrder: req.SortOrder,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toDTO(view))
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	if httpx.WriteAuthzProblem(w, r, err) {
		return
	}
	var verr *application.ValidationError
	switch {
	case errors.As(err, &verr):
		p := httpx.NewProblem(http.StatusBadRequest, httpx.CodeInvalidRequest, "one or more fields are invalid")
		for _, f := range verr.Fields {
			p.Errors = append(p.Errors, httpx.FieldError{Field: f.Field, Message: f.Code})
		}
		httpx.WriteProblem(w, r, p)
	case errors.Is(err, application.ErrNotFound):
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusNotFound, httpx.CodeNotFound, ""))
	case errors.Is(err, application.ErrAbsenceOverlap):
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusConflict, CodeAbsenceOverlap, "the absence overlaps another absence of the professional"))
	case errors.Is(err, application.ErrAlreadyExists):
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusConflict, CodeAlreadyProfessional, "that account already has a professional profile in this business"))
	default:
		h.logger.ErrorContext(r.Context(), "professionals request failed",
			slog.String("request_id", httpx.RequestIDFrom(r.Context())),
			slog.String("error", err.Error()))
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusInternalServerError, httpx.CodeInternal, ""))
	}
}
