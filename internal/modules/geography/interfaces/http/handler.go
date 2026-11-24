// Package httpapi exposes the geography endpoints: public reads of active reference data and
// PLATFORM_ADMIN writes under /api/v1/admin (api-design.md).
package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/httpx"
)

// Problem codes of this module.
const (
	// CodeAlreadyExists is returned when a code or name is already taken.
	CodeAlreadyExists = "ALREADY_EXISTS"
	// CodeInUse is returned when deactivating an area that still has active businesses.
	CodeInUse = "IN_USE"
)

// Service is the subset of the application used by these handlers.
type Service interface {
	ListCountries(ctx context.Context) ([]domain.Country, error)
	ListDivisions(ctx context.Context, countryCode string) ([]domain.Division, error)
	ListCities(ctx context.Context, divisionID uuid.UUID) ([]domain.City, error)
	CreateCountry(ctx context.Context, p contract.Principal, in application.CountryInput) (domain.Country, error)
	UpdateCountry(ctx context.Context, p contract.Principal, code string, in application.CountryPatch) (domain.Country, error)
	CreateDivision(ctx context.Context, p contract.Principal, in application.DivisionInput) (domain.Division, error)
	UpdateDivision(ctx context.Context, p contract.Principal, id uuid.UUID, in application.DivisionPatch) (domain.Division, error)
	CreateCity(ctx context.Context, p contract.Principal, in application.CityInput) (domain.City, error)
	UpdateCity(ctx context.Context, p contract.Principal, id uuid.UUID, in application.CityPatch) (domain.City, error)
}

// Handler serves the geography endpoints.
type Handler struct {
	svc         Service
	logger      *slog.Logger
	requireAuth httpx.Middleware
}

// New builds the handler. requireAuth is the identity module's authentication middleware
// (it puts the contract.Principal in the request context).
func New(svc Service, logger *slog.Logger, requireAuth httpx.Middleware) *Handler {
	return &Handler{svc: svc, logger: logger, requireAuth: requireAuth}
}

// Routes registers the endpoints.
func (h *Handler) Routes(rt *httpx.Router) {
	rt.HandleFunc("GET /api/v1/countries", h.listCountries)
	rt.HandleFunc("GET /api/v1/countries/{code}/divisions", h.listDivisions)
	rt.HandleFunc("GET /api/v1/divisions/{id}/cities", h.listCities)

	admin := func(f http.HandlerFunc) http.Handler { return h.requireAuth(f) }
	rt.Handle("POST /api/v1/admin/countries", admin(h.createCountry))
	rt.Handle("PATCH /api/v1/admin/countries/{code}", admin(h.updateCountry))
	rt.Handle("POST /api/v1/admin/divisions", admin(h.createDivision))
	rt.Handle("PATCH /api/v1/admin/divisions/{id}", admin(h.updateDivision))
	rt.Handle("POST /api/v1/admin/cities", admin(h.createCity))
	rt.Handle("PATCH /api/v1/admin/cities/{id}", admin(h.updateCity))
}

// --- representations ---------------------------------------------------------------------

type countryDTO struct {
	Code            string `json:"code"`
	Name            string `json:"name"`
	PhoneCode       string `json:"phoneCode"`
	DefaultCurrency string `json:"defaultCurrency"`
	Active          bool   `json:"active"`
}

type divisionDTO struct {
	ID          uuid.UUID `json:"id"`
	CountryCode string    `json:"countryCode"`
	Code        *string   `json:"code"`
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	Active      bool      `json:"active"`
}

type cityDTO struct {
	ID         uuid.UUID `json:"id"`
	DivisionID uuid.UUID `json:"divisionId"`
	Name       string    `json:"name"`
	TimeZone   string    `json:"timeZone"`
	Active     bool      `json:"active"`
}

type list[T any] struct {
	Items []T `json:"items"`
}

func toCountry(c domain.Country) countryDTO {
	return countryDTO{Code: string(c.Code), Name: c.Name, PhoneCode: c.PhoneCode, DefaultCurrency: c.DefaultCurrency, Active: c.Active}
}

func toDivision(d domain.Division) divisionDTO {
	out := divisionDTO{ID: d.ID, CountryCode: string(d.CountryCode), Name: d.Name, Type: string(d.Type), Active: d.Active}
	if d.Code != "" {
		code := d.Code
		out.Code = &code
	}
	return out
}

func toCity(c domain.City) cityDTO {
	return cityDTO{ID: c.ID, DivisionID: c.DivisionID, Name: c.Name, TimeZone: c.TimeZone, Active: c.Active}
}

func mapAll[S, D any](in []S, f func(S) D) list[D] {
	out := list[D]{Items: make([]D, 0, len(in))}
	for _, v := range in {
		out.Items = append(out.Items, f(v))
	}
	return out
}

// --- public ------------------------------------------------------------------------------

func (h *Handler) listCountries(w http.ResponseWriter, r *http.Request) {
	cs, err := h.svc.ListCountries(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, mapAll(cs, toCountry))
}

func (h *Handler) listDivisions(w http.ResponseWriter, r *http.Request) {
	ds, err := h.svc.ListDivisions(r.Context(), r.PathValue("code"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, mapAll(ds, toDivision))
}

func (h *Handler) listCities(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		h.fail(w, r, application.ErrNotFound)
		return
	}
	cs, err := h.svc.ListCities(r.Context(), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, mapAll(cs, toCity))
}

// --- admin -------------------------------------------------------------------------------

type createCountryRequest struct {
	Code            string `json:"code"`
	Name            string `json:"name"`
	PhoneCode       string `json:"phoneCode"`
	DefaultCurrency string `json:"defaultCurrency"`
	Active          *bool  `json:"active"`
}

type updateCountryRequest struct {
	Name            *string `json:"name"`
	PhoneCode       *string `json:"phoneCode"`
	DefaultCurrency *string `json:"defaultCurrency"`
	Active          *bool   `json:"active"`
}

func (h *Handler) createCountry(w http.ResponseWriter, r *http.Request) {
	var req createCountryRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	c, err := h.svc.CreateCountry(r.Context(), p, application.CountryInput{
		Code: req.Code, Name: req.Name, PhoneCode: req.PhoneCode, DefaultCurrency: req.DefaultCurrency, Active: req.Active,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toCountry(c))
}

func (h *Handler) updateCountry(w http.ResponseWriter, r *http.Request) {
	var req updateCountryRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	c, err := h.svc.UpdateCountry(r.Context(), p, r.PathValue("code"), application.CountryPatch{
		Name: req.Name, PhoneCode: req.PhoneCode, DefaultCurrency: req.DefaultCurrency, Active: req.Active,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toCountry(c))
}

type createDivisionRequest struct {
	CountryCode string `json:"countryCode"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Active      *bool  `json:"active"`
}

type updateDivisionRequest struct {
	Code   *string `json:"code"`
	Name   *string `json:"name"`
	Type   *string `json:"type"`
	Active *bool   `json:"active"`
}

func (h *Handler) createDivision(w http.ResponseWriter, r *http.Request) {
	var req createDivisionRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	d, err := h.svc.CreateDivision(r.Context(), p, application.DivisionInput{
		CountryCode: req.CountryCode, Code: req.Code, Name: req.Name, Type: req.Type, Active: req.Active,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toDivision(d))
}

func (h *Handler) updateDivision(w http.ResponseWriter, r *http.Request) {
	var req updateDivisionRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	id, ok := pathID(r)
	if !ok {
		h.fail(w, r, application.ErrNotFound)
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	d, err := h.svc.UpdateDivision(r.Context(), p, id, application.DivisionPatch{
		Code: req.Code, Name: req.Name, Type: req.Type, Active: req.Active,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toDivision(d))
}

type createCityRequest struct {
	DivisionID uuid.UUID `json:"divisionId"`
	Name       string    `json:"name"`
	TimeZone   string    `json:"timeZone"`
	Active     *bool     `json:"active"`
}

type updateCityRequest struct {
	Name     *string `json:"name"`
	TimeZone *string `json:"timeZone"`
	Active   *bool   `json:"active"`
}

func (h *Handler) createCity(w http.ResponseWriter, r *http.Request) {
	var req createCityRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	c, err := h.svc.CreateCity(r.Context(), p, application.CityInput{
		DivisionID: req.DivisionID, Name: req.Name, TimeZone: req.TimeZone, Active: req.Active,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toCity(c))
}

func (h *Handler) updateCity(w http.ResponseWriter, r *http.Request) {
	var req updateCityRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	id, ok := pathID(r)
	if !ok {
		h.fail(w, r, application.ErrNotFound)
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	c, err := h.svc.UpdateCity(r.Context(), p, id, application.CityPatch{
		Name: req.Name, TimeZone: req.TimeZone, Active: req.Active,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toCity(c))
}

// --- errors ------------------------------------------------------------------------------

func pathID(r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	return id, err == nil && id != uuid.Nil
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
		// Same problem as an unknown route.
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusNotFound, httpx.CodeNotFound, ""))
	case errors.Is(err, application.ErrInUse):
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusConflict, CodeInUse, "it has active businesses; move or suspend them first"))
	case errors.Is(err, application.ErrConflict):
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusConflict, CodeAlreadyExists, "a record with that code or name already exists"))
	default:
		h.logger.ErrorContext(r.Context(), "geography request failed",
			slog.String("request_id", httpx.RequestIDFrom(r.Context())),
			slog.String("error", err.Error()))
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusInternalServerError, httpx.CodeInternal, ""))
	}
}
