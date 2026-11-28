// Package httpapi exposes the businesses endpoints: self-service onboarding (POST
// /api/v1/businesses, T4.3b) and PLATFORM_ADMIN onboarding under /api/v1/admin/businesses (T4.3).
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
	scheduling "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/httpx"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/media"
)

// Problem codes of this module.
const (
	// CodeLocationInactive: the business cannot be activated while its city, division or
	// country is inactive.
	CodeLocationInactive = "LOCATION_INACTIVE"
	// CodeAlreadyOwner: the account already owns a business (one demo per account, ADR-0018).
	CodeOwnedBusinessLimit   = "OWNED_BUSINESS_LIMIT"
	CodeIdempotencyKeyReused = "IDEMPOTENCY_KEY_REUSED"
)

// Service is the subset of the application used by these handlers.
type Service interface {
	CreateBusiness(ctx context.Context, p contract.Principal, in application.CreateInput) (*domain.Business, error)
	CreateOwnBusiness(ctx context.Context, p contract.Principal, in application.BusinessInput) (*domain.Business, error)
	GetBusiness(ctx context.Context, p contract.Principal, id uuid.UUID) (*domain.Business, error)
	ListBusinesses(ctx context.Context, p contract.Principal, status string, cityID, after uuid.UUID, limit int) ([]*domain.Business, uuid.UUID, error)
	ActivateBusiness(ctx context.Context, p contract.Principal, id uuid.UUID) (*domain.Business, error)
	SuspendBusiness(ctx context.Context, p contract.Principal, id uuid.UUID) (*domain.Business, error)
	GetManaged(ctx context.Context, p contract.Principal, businessID uuid.UUID) (application.ManagedBusiness, error)
	UpdateBusiness(ctx context.Context, p contract.Principal, businessID uuid.UUID, in application.UpdateInput) (*domain.Business, error)
	SearchPublic(ctx context.Context, q application.SearchQuery) ([]*domain.Business, string, error)
	SetImage(ctx context.Context, p contract.Principal, businessID uuid.UUID, kind application.ImageKind, up media.Upload) (*domain.Business, error)
	RemoveImage(ctx context.Context, p contract.Principal, businessID uuid.UUID, kind application.ImageKind) (*domain.Business, error)
	GetPublic(ctx context.Context, slug string) (*domain.Business, error)
}

// Handler serves the businesses endpoints.
type Handler struct {
	svc           Service
	logger        *slog.Logger
	requireAuth   httpx.Middleware
	createLimiter httpx.Allower
	ratings       scheduling.Ratings
}

// New builds the handler; requireAuth is the identity module's authentication middleware and
// createLimiter the strict per-client limit applied to self-service creation (nil disables it).
func New(svc Service, logger *slog.Logger, requireAuth httpx.Middleware, createLimiter httpx.Allower) *Handler {
	return &Handler{svc: svc, logger: logger, requireAuth: requireAuth, createLimiter: createLimiter}
}

// Routes registers the endpoints.
func (h *Handler) Routes(rt *httpx.Router) {
	auth := func(f http.HandlerFunc) http.Handler { return h.requireAuth(f) }
	rt.Handle("POST /api/v1/businesses", httpx.RateLimit(h.createLimiter)(auth(h.createOwn)))
	rt.Handle("POST /api/v1/admin/businesses", auth(h.create))
	rt.Handle("GET /api/v1/admin/businesses", auth(h.list))
	rt.Handle("GET /api/v1/admin/businesses/{id}", auth(h.get))
	rt.Handle("POST /api/v1/admin/businesses/{id}/activate", auth(h.activate))
	rt.Handle("POST /api/v1/admin/businesses/{id}/suspend", auth(h.suspend))
	h.manageRoutes(rt)
	h.publicRoutes(rt)
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
	MaxCustomerReschedules      int  `json:"maxCustomerReschedules"`
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
	LogoURL       *string      `json:"logoUrl"`
	CoverURL      *string      `json:"coverUrl"`
	CreatedAt     time.Time    `json:"createdAt"`
	UpdatedAt     time.Time    `json:"updatedAt"`
}

type listDTO struct {
	Items      []businessDTO `json:"items"`
	NextCursor *string       `json:"nextCursor,omitempty"`
}

// imageURL is the public address of a stored image, null if none.
func imageURL(key string) *string {
	if key == "" {
		return nil
	}
	u := media.URL(key)
	return &u
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
			MaxCustomerReschedules: p.MaxCustomerReschedules, RequireCustomerConfirmation: p.RequireCustomerConfirmation,
		},
		Reminders: remindersDTO{Enabled: r.Enabled, HoursBefore: r.HoursBefore},
		LogoURL:   imageURL(b.Images().Logo), CoverURL: imageURL(b.Images().Cover),
	}
	if l.Coordinates != nil {
		out.Location.Coordinates = &coordinatesDTO{Latitude: l.Coordinates.Lat, Longitude: l.Coordinates.Lng}
	}
	return out
}

// --- handlers ----------------------------------------------------------------------------

type businessFields struct {
	Name         string          `json:"name"`
	ContactPhone string          `json:"contactPhone"`
	Description  string          `json:"description"`
	CityID       uuid.UUID       `json:"cityId"`
	AddressLine  string          `json:"addressLine"`
	Coordinates  *coordinatesDTO `json:"coordinates"`
}

func (f businessFields) input() application.BusinessInput {
	in := application.BusinessInput{
		Name: f.Name, ContactPhone: f.ContactPhone, Description: f.Description, CityID: f.CityID, AddressLine: f.AddressLine,
	}
	if f.Coordinates != nil {
		in.Latitude, in.Longitude = &f.Coordinates.Latitude, &f.Coordinates.Longitude
	}
	return in
}

type createRequest struct {
	businessFields
	OwnerEmail string `json:"ownerEmail"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	b, err := h.svc.CreateBusiness(r.Context(), p, application.CreateInput{BusinessInput: req.input(), OwnerEmail: req.OwnerEmail})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toDTO(b))
}

// createOwn is the self-service onboarding: the caller becomes OWNER and gets a 7-day demo. The
// optional Idempotency-Key header makes a repeated request return the same business (P-11).
func (h *Handler) createOwn(w http.ResponseWriter, r *http.Request) {
	var req businessFields
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	in := req.input()
	in.IdempotencyKey = r.Header.Get("Idempotency-Key")
	b, err := h.svc.CreateOwnBusiness(r.Context(), p, in)
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
	if p, ok := media.Problem(err); ok {
		httpx.WriteProblem(w, r, p)
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
	case errors.Is(err, contract.ErrOwnedBusinessLimit):
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusConflict, CodeOwnedBusinessLimit, "this account already owns the most businesses it can create"))
	case errors.Is(err, application.ErrInvalidIdempotencyKey):
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusBadRequest, httpx.CodeInvalidRequest, "the Idempotency-Key header must have 8-100 characters: letters, digits, - and _"))
	case errors.Is(err, application.ErrIdempotencyKeyReused):
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusUnprocessableEntity, CodeIdempotencyKeyReused, "this Idempotency-Key was used for another request"))
	default:
		h.logger.ErrorContext(r.Context(), "businesses request failed",
			slog.String("request_id", httpx.RequestIDFrom(r.Context())),
			slog.String("error", err.Error()))
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusInternalServerError, httpx.CodeInternal, ""))
	}
}
