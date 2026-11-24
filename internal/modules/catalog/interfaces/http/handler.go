// Package httpapi exposes the catalog endpoints under /api/v1/business/{businessId} (T5.5). The
// businessId is checked against the caller's memberships on every request; another business is
// a 404 identical to a missing one.
package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/httpx"
)

// Problem codes.
const (
	CodeCategoryNameTaken = "CATEGORY_NAME_TAKEN"
	CodeServiceNameTaken  = "SERVICE_NAME_TAKEN"
	codeInvalidID         = "INVALID_ID"
)

// Service is the subset of the application used by these handlers.
type Service interface {
	ListCategories(ctx context.Context, p contract.Principal, businessID uuid.UUID) ([]domain.Category, error)
	CreateCategory(ctx context.Context, p contract.Principal, businessID uuid.UUID, in application.CategoryInput) (domain.Category, error)
	UpdateCategory(ctx context.Context, p contract.Principal, businessID, id uuid.UUID, in application.CategoryPatch) (domain.Category, error)
	DeleteCategory(ctx context.Context, p contract.Principal, businessID, id uuid.UUID) error
	ListServices(ctx context.Context, p contract.Principal, businessID uuid.UUID) ([]domain.Service, error)
	GetService(ctx context.Context, p contract.Principal, businessID, id uuid.UUID) (domain.Service, error)
	CreateService(ctx context.Context, p contract.Principal, businessID uuid.UUID, in application.ServiceInput) (domain.Service, error)
	UpdateService(ctx context.Context, p contract.Principal, businessID, id uuid.UUID, in application.ServicePatch) (domain.Service, error)
}

// Handler serves the catalog endpoints.
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
	rt.Handle("GET /api/v1/business/{businessId}/service-categories", auth(h.listCategories))
	rt.Handle("POST /api/v1/business/{businessId}/service-categories", auth(h.createCategory))
	rt.Handle("PATCH /api/v1/business/{businessId}/service-categories/{categoryId}", auth(h.updateCategory))
	rt.Handle("DELETE /api/v1/business/{businessId}/service-categories/{categoryId}", auth(h.deleteCategory))
	rt.Handle("GET /api/v1/business/{businessId}/services", auth(h.listServices))
	rt.Handle("POST /api/v1/business/{businessId}/services", auth(h.createService))
	rt.Handle("GET /api/v1/business/{businessId}/services/{serviceId}", auth(h.getService))
	rt.Handle("PATCH /api/v1/business/{businessId}/services/{serviceId}", auth(h.updateService))
}

// --- DTOs ---------------------------------------------------------------------------------------

type categoryDTO struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	SortOrder int       `json:"sortOrder"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type serviceDTO struct {
	ID              uuid.UUID  `json:"id"`
	CategoryID      *uuid.UUID `json:"categoryId"`
	Name            string     `json:"name"`
	Description     string     `json:"description"`
	DurationMinutes int        `json:"durationMinutes"`
	PriceMinor      int64      `json:"priceMinor"`
	Currency        string     `json:"currency"`
	Status          string     `json:"status"`
	SortOrder       int        `json:"sortOrder"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

func toCategoryDTO(c domain.Category) categoryDTO {
	return categoryDTO{ID: c.ID, Name: c.Name, Status: string(c.Status), SortOrder: c.SortOrder, CreatedAt: c.CreatedAt.UTC(), UpdatedAt: c.UpdatedAt.UTC()}
}

func toServiceDTO(s domain.Service) serviceDTO {
	out := serviceDTO{
		ID: s.ID, Name: s.Name, Description: s.Description, DurationMinutes: s.DurationMinutes, PriceMinor: s.PriceMinor,
		Currency: s.Currency, Status: string(s.Status), SortOrder: s.SortOrder, CreatedAt: s.CreatedAt.UTC(), UpdatedAt: s.UpdatedAt.UTC(),
	}
	if s.CategoryID != uuid.Nil {
		id := s.CategoryID
		out.CategoryID = &id
	}
	return out
}

// optionalID distinguishes an absent "categoryId" (Set false) from null (Set, Nil) and a UUID.
type optionalID struct {
	Set, Invalid bool
	Value        uuid.UUID
}

// UnmarshalJSON implements json.Unmarshaler (called only when the key is present).
func (o *optionalID) UnmarshalJSON(b []byte) error {
	o.Set = true
	if bytes.Equal(b, []byte("null")) {
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		o.Invalid = true
		return nil //nolint:nilerr // reported as a field error
	}
	id, err := uuid.Parse(s)
	if err != nil || id == uuid.Nil {
		o.Invalid = true
		return nil //nolint:nilerr // reported as a field error
	}
	o.Value = id
	return nil
}

type categoryRequest struct {
	Name      *string `json:"name"`
	Status    *string `json:"status"`
	SortOrder *int    `json:"sortOrder"`
}

type serviceRequest struct {
	CategoryID      optionalID `json:"categoryId"`
	Name            *string    `json:"name"`
	Description     *string    `json:"description"`
	DurationMinutes *int       `json:"durationMinutes"`
	PriceMinor      *int64     `json:"priceMinor"`
	Status          *string    `json:"status"`
	SortOrder       *int       `json:"sortOrder"`
}

// --- handlers -----------------------------------------------------------------------------------

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

func principal(r *http.Request) contract.Principal {
	p, _ := contract.PrincipalFrom(r.Context())
	return p
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (h *Handler) listCategories(w http.ResponseWriter, r *http.Request) {
	v, ok := ids(r, "businessId")
	if !ok {
		h.fail(w, r, contract.ErrNotFound)
		return
	}
	list, err := h.svc.ListCategories(r.Context(), principal(r), v[0])
	if err != nil {
		h.fail(w, r, err)
		return
	}
	items := make([]categoryDTO, 0, len(list))
	for _, c := range list {
		items = append(items, toCategoryDTO(c))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) createCategory(w http.ResponseWriter, r *http.Request) {
	var req categoryRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	v, ok := ids(r, "businessId")
	if !ok {
		h.fail(w, r, contract.ErrNotFound)
		return
	}
	c, err := h.svc.CreateCategory(r.Context(), principal(r), v[0], application.CategoryInput{Name: deref(req.Name), SortOrder: req.SortOrder})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toCategoryDTO(c))
}

func (h *Handler) updateCategory(w http.ResponseWriter, r *http.Request) {
	var req categoryRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	v, ok := ids(r, "businessId", "categoryId")
	if !ok {
		h.fail(w, r, contract.ErrNotFound)
		return
	}
	c, err := h.svc.UpdateCategory(r.Context(), principal(r), v[0], v[1], application.CategoryPatch{Name: req.Name, Status: req.Status, SortOrder: req.SortOrder})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toCategoryDTO(c))
}

func (h *Handler) deleteCategory(w http.ResponseWriter, r *http.Request) {
	v, ok := ids(r, "businessId", "categoryId")
	if !ok {
		h.fail(w, r, contract.ErrNotFound)
		return
	}
	if err := h.svc.DeleteCategory(r.Context(), principal(r), v[0], v[1]); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listServices(w http.ResponseWriter, r *http.Request) {
	v, ok := ids(r, "businessId")
	if !ok {
		h.fail(w, r, contract.ErrNotFound)
		return
	}
	list, err := h.svc.ListServices(r.Context(), principal(r), v[0])
	if err != nil {
		h.fail(w, r, err)
		return
	}
	items := make([]serviceDTO, 0, len(list))
	for _, s := range list {
		items = append(items, toServiceDTO(s))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) getService(w http.ResponseWriter, r *http.Request) {
	v, ok := ids(r, "businessId", "serviceId")
	if !ok {
		h.fail(w, r, contract.ErrNotFound)
		return
	}
	s, err := h.svc.GetService(r.Context(), principal(r), v[0], v[1])
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toServiceDTO(s))
}

func (h *Handler) createService(w http.ResponseWriter, r *http.Request) {
	var req serviceRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	v, ok := ids(r, "businessId")
	if !ok {
		h.fail(w, r, contract.ErrNotFound)
		return
	}
	if req.CategoryID.Invalid {
		h.fail(w, r, &application.ValidationError{Fields: []application.FieldError{{Field: "categoryId", Code: codeInvalidID}}})
		return
	}
	s, err := h.svc.CreateService(r.Context(), principal(r), v[0], application.ServiceInput{
		Name: deref(req.Name), Description: deref(req.Description), CategoryID: req.CategoryID.Value,
		DurationMinutes: req.DurationMinutes, PriceMinor: req.PriceMinor, SortOrder: req.SortOrder,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toServiceDTO(s))
}

func (h *Handler) updateService(w http.ResponseWriter, r *http.Request) {
	var req serviceRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	v, ok := ids(r, "businessId", "serviceId")
	if !ok {
		h.fail(w, r, contract.ErrNotFound)
		return
	}
	if req.CategoryID.Invalid {
		h.fail(w, r, &application.ValidationError{Fields: []application.FieldError{{Field: "categoryId", Code: codeInvalidID}}})
		return
	}
	patch := application.ServicePatch{
		Name: req.Name, Description: req.Description, Status: req.Status,
		DurationMinutes: req.DurationMinutes, PriceMinor: req.PriceMinor, SortOrder: req.SortOrder,
	}
	if req.CategoryID.Set {
		patch.CategoryID = &req.CategoryID.Value
	}
	s, err := h.svc.UpdateService(r.Context(), principal(r), v[0], v[1], patch)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toServiceDTO(s))
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
	case errors.Is(err, application.ErrNameTaken):
		code, detail := CodeServiceNameTaken, "another service of this business already has that name"
		if strings.Contains(r.URL.Path, "/service-categories") {
			code, detail = CodeCategoryNameTaken, "another category of this business already has that name"
		}
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusConflict, code, detail))
	default:
		h.logger.ErrorContext(r.Context(), "catalog request failed",
			slog.String("request_id", httpx.RequestIDFrom(r.Context())),
			slog.String("error", err.Error()))
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusInternalServerError, httpx.CodeInternal, ""))
	}
}
