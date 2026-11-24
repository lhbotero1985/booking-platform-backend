// Package httpapi exposes the businesses endpoints. T4.3: PLATFORM_ADMIN onboarding under
// /api/v1/admin/businesses.
package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/httpx"
)

// Problem codes of this module.
const (
	// CodeLocationInactive: the business cannot be activated while its city, division or
	// country is inactive.
	CodeLocationInactive = "LOCATION_INACTIVE"
	// CodeOwnerExists: the business already has an OWNER.
	CodeOwnerExists = "OWNER_EXISTS"
)

// Service is the subset of the application used by these handlers.
type Service interface {
	CreateBusiness(ctx context.Context, p contract.Principal, in application.CreateInput) (*domain.Business, error)
	GetBusiness(ctx context.Context, p contract.Principal, id uuid.UUID) (*domain.Business, error)
	ListBusinesses(ctx context.Context, p contract.Principal, status string, cityID, after uuid.UUID, limit int) ([]*domain.Business, uuid.UUID, error)
	ActivateBusiness(ctx context.Context, p contract.Principal, id uuid.UUID) (*domain.Business, error)
	SuspendBusiness(ctx context.Context, p contract.Principal, id uuid.UUID) (*domain.Business, error)
}

// Handler serves the businesses endpoints.
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
	rt.Handle("POST /api/v1/admin/businesses", auth(h.create))
	rt.Handle("GET /api/v1/admin/businesses", auth(h.list))
	rt.Handle("GET /api/v1/admin/businesses/{id}", auth(h.get))
	rt.Handle("POST /api/v1/admin/businesses/{id}/activate", auth(h.activate))
	rt.Handle("POST /api/v1/admin/businesses/{id}/suspend", auth(h.suspend))
}

// --- representation ----------------------------------------------------------------------

type coordinatesDTO struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type locationDTO struct {
	CountryCode string          `json:"countryCode"`
	DivisionID  uuid.UUID       `json:"divisionId"`
	CityID      uuid.UUID       `json:"cityId"`
	AddressLine string          `json:"addressLine"`
	Coordinates *coordinatesDTO `json:"coordinates"`
}

type policyDTO struct {
	SlotIntervalMinutes         int  `json:"slotIntervalMinutes"`
	BufferMinutes               int  `json:"bufferMinutes"`
	MinLeadMinutes              int  `json:"minLeadMinutes"`
	MaxAdvanceDays              int  `json:"maxAdvanceDays"`
	CancellationCutoffHours     int  `json:"cancellationCutoffHours"`
	RescheduleCutoffHours       int  `json:"rescheduleCutoffHours"`
	RequireCustomerConfirmation bool `json:"requireCustomerConfirmation"`
}

type remindersDTO struct {
	Enabled     bool `json:"enabled"`
	HoursBefore int  `json:"hoursBefore"`
}

type businessDTO struct {
	ID            uuid.UUID    `json:"id"`
	Name          string       `json:"name"`
	Slug          string       `json:"slug"`
	Status        string       `json:"status"`
	ContactPhone  string       `json:"contactPhone"`
	Description   string       `json:"description"`
	Location      locationDTO  `json:"location"`
	TimeZone      string       `json:"timeZone"`
	Currency      string       `json:"currency"`
	BookingPolicy policyDTO    `json:"bookingPolicy"`
	Reminders     remindersDTO `json:"reminders"`
	CreatedAt     time.Time    `json:"createdAt"`
	UpdatedAt     time.Time    `json:"updatedAt"`
}

type listDTO struct {
	Items      []businessDTO `json:"items"`
	NextCursor *string       `json:"nextCursor,omitempty"`
}

func toDTO(b *domain.Business) businessDTO {
	l, p, r := b.Location(), b.Policy(), b.Reminders()
	out := businessDTO{
		ID: b.ID(), Name: b.Name(), Slug: string(b.Slug()), Status: string(b.Status()),
		ContactPhone: b.ContactPhone(), Description: b.Description(),
		Location: locationDTO{CountryCode: l.CountryCode, DivisionID: l.DivisionID, CityID: l.CityID, AddressLine: l.AddressLine},
		TimeZone: b.TimeZone(), Currency: b.Currency(),
		CreatedAt: b.CreatedAt(), UpdatedAt: b.UpdatedAt(),
		BookingPolicy: policyDTO{
			SlotIntervalMinutes: p.SlotIntervalMinutes, BufferMinutes: p.BufferMinutes,
			MinLeadMinutes: p.MinLeadMinutes, MaxAdvanceDays: p.MaxAdvanceDays,
			CancellationCutoffHours: p.CancellationCutoffHours, RescheduleCutoffHours: p.RescheduleCutoffHours,
			RequireCustomerConfirmation: p.RequireCustomerConfirmation,
		},
		Reminders: remindersDTO{Enabled: r.Enabled, HoursBefore: r.HoursBefore},
	}
	if l.Coordinates != nil {
		out.Location.Coordinates = &coordinatesDTO{Latitude: l.Coordinates.Lat, Longitude: l.Coordinates.Lng}
	}
	return out
}

// --- handlers ----------------------------------------------------------------------------

type createRequest struct {
	Name         string          `json:"name"`
	ContactPhone string          `json:"contactPhone"`
	Description  string          `json:"description"`
	CityID       uuid.UUID       `json:"cityId"`
	AddressLine  string          `json:"addressLine"`
	Coordinates  *coordinatesDTO `json:"coordinates"`
	OwnerEmail   string          `json:"ownerEmail"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	in := application.CreateInput{
		Name: req.Name, ContactPhone: req.ContactPhone, Description: req.Description, CityID: req.CityID,
		AddressLine: req.AddressLine, OwnerEmail: req.OwnerEmail,
	}
	if req.Coordinates != nil {
		in.Latitude, in.Longitude = &req.Coordinates.Latitude, &req.Coordinates.Longitude
	}
	p, _ := contract.PrincipalFrom(r.Context())
	b, err := h.svc.CreateBusiness(r.Context(), p, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toDTO(b))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var cityID, after uuid.UUID
	var err error
	if v := q.Get("cityId"); v != "" {
		if cityID, err = uuid.Parse(v); err != nil {
			h.fail(w, r, &application.ValidationError{Fields: []application.FieldError{{Field: "cityId", Code: "INVALID_ID"}}})
			return
		}
	}
	if v := q.Get("cursor"); v != "" {
		if after, err = uuid.Parse(v); err != nil {
			h.fail(w, r, &application.ValidationError{Fields: []application.FieldError{{Field: "cursor", Code: "INVALID_CURSOR"}}})
			return
		}
	}
	limit := 0
	if v := q.Get("limit"); v != "" {
		if limit, err = strconv.Atoi(v); err != nil || limit < 1 || limit > application.MaxListLimit {
			h.fail(w, r, &application.ValidationError{Fields: []application.FieldError{{Field: "limit", Code: "INVALID_LIMIT"}}})
			return
		}
	}
	p, _ := contract.PrincipalFrom(r.Context())
	items, next, err := h.svc.ListBusinesses(r.Context(), p, q.Get("status"), cityID, after, limit)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := listDTO{Items: make([]businessDTO, 0, len(items))}
	for _, b := range items {
		out.Items = append(out.Items, toDTO(b))
	}
	if next != uuid.Nil {
		c := next.String()
		out.NextCursor = &c
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	h.byID(w, r, h.svc.GetBusiness)
}

func (h *Handler) activate(w http.ResponseWriter, r *http.Request) {
	h.byID(w, r, h.svc.ActivateBusiness)
}

func (h *Handler) suspend(w http.ResponseWriter, r *http.Request) {
	h.byID(w, r, h.svc.SuspendBusiness)
}

func (h *Handler) byID(w http.ResponseWriter, r *http.Request, f func(context.Context, contract.Principal, uuid.UUID) (*domain.Business, error)) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil || id == uuid.Nil {
		h.fail(w, r, application.ErrNotFound)
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	b, err := f(r.Context(), p, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toDTO(b))
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
	case errors.Is(err, application.ErrLocationInactive):
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusConflict, CodeLocationInactive, "activate the business's city, division and country first"))
	case errors.Is(err, contract.ErrOwnerExists):
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusConflict, CodeOwnerExists, "the business already has an owner"))
	default:
		h.logger.ErrorContext(r.Context(), "businesses request failed",
			slog.String("request_id", httpx.RequestIDFrom(r.Context())),
			slog.String("error", err.Error()))
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusInternalServerError, httpx.CodeInternal, ""))
	}
}
