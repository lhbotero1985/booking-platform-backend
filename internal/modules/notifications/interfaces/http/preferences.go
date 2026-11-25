package httpapi

// Preferences and the WhatsApp cap (T7.9).

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/httpx"
)

// PreferencesService is the subset of the application used by the preference endpoints.
type PreferencesService interface {
	MyPreferences(ctx context.Context, p contract.Principal) (domain.CustomerPreferences, error)
	SetMyPreferences(ctx context.Context, p contract.Principal, prefs domain.CustomerPreferences) error
	ProfessionalEmails(ctx context.Context, p contract.Principal, businessID, professionalID uuid.UUID) (bool, error)
	SetProfessionalEmails(ctx context.Context, p contract.Principal, businessID, professionalID uuid.UUID, on *bool) error
	BusinessWhatsAppLimit(ctx context.Context, p contract.Principal, businessID uuid.UUID) (application.WhatsAppLimit, error)
	SetBusinessWhatsAppLimit(ctx context.Context, p contract.Principal, businessID uuid.UUID, limit *int) error
}

// CodeNoChannel is the problem code of a customer without any channel.
const CodeNoChannel = "AT_LEAST_ONE_CHANNEL"

func (h *Handler) preferenceRoutes(rt *httpx.Router) {
	if h.prefs == nil {
		return
	}
	auth := func(f http.HandlerFunc) http.Handler { return h.requireAuth(f) }
	rt.Handle("GET /api/v1/me/notification-preferences", auth(h.myPreferences))
	rt.Handle("PUT /api/v1/me/notification-preferences", auth(h.setMyPreferences))
	rt.Handle("GET /api/v1/business/{businessId}/professionals/{professionalId}/notification-preferences", auth(h.professionalPreferences))
	rt.Handle("PUT /api/v1/business/{businessId}/professionals/{professionalId}/notification-preferences", auth(h.setProfessionalPreferences))
	rt.Handle("GET /api/v1/admin/businesses/{id}/whatsapp-limit", auth(h.whatsAppLimit))
	rt.Handle("PUT /api/v1/admin/businesses/{id}/whatsapp-limit", auth(h.setWhatsAppLimit))
}

type customerPreferencesDTO struct {
	WhatsApp  *bool `json:"whatsapp"`
	Email     *bool `json:"email"`
	Reminders *bool `json:"reminders"`
}

func toCustomerDTO(p domain.CustomerPreferences) customerPreferencesDTO {
	return customerPreferencesDTO{WhatsApp: &p.WhatsApp, Email: &p.Email, Reminders: &p.Reminders}
}

func (h *Handler) myPreferences(w http.ResponseWriter, r *http.Request) {
	p, _ := contract.PrincipalFrom(r.Context())
	prefs, err := h.prefs.MyPreferences(r.Context(), p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toCustomerDTO(prefs))
}

func (h *Handler) setMyPreferences(w http.ResponseWriter, r *http.Request) {
	var req customerPreferencesDTO
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	var fields []httpx.FieldError
	for _, f := range []struct {
		name string
		v    *bool
	}{{"whatsapp", req.WhatsApp}, {"email", req.Email}, {"reminders", req.Reminders}} {
		if f.v == nil {
			fields = append(fields, httpx.FieldError{Field: f.name, Message: "REQUIRED"})
		}
	}
	if len(fields) > 0 {
		writeFields(w, r, fields)
		return
	}
	prefs := domain.CustomerPreferences{WhatsApp: *req.WhatsApp, Email: *req.Email, Reminders: *req.Reminders}
	p, _ := contract.PrincipalFrom(r.Context())
	err := h.prefs.SetMyPreferences(r.Context(), p, prefs)
	if errors.Is(err, domain.ErrNoChannel) {
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusBadRequest, CodeNoChannel, "keep WhatsApp, email or both"))
		return
	}
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toCustomerDTO(prefs))
}

type professionalPreferencesDTO struct {
	Email *bool `json:"email"`
}

func (h *Handler) professionalIDs(w http.ResponseWriter, r *http.Request) (businessID, professionalID uuid.UUID, ok bool) {
	b, err1 := uuid.Parse(r.PathValue("businessId"))
	pr, err2 := uuid.Parse(r.PathValue("professionalId"))
	if err1 != nil || err2 != nil {
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusNotFound, httpx.CodeNotFound, ""))
		return uuid.Nil, uuid.Nil, false
	}
	return b, pr, true
}

func (h *Handler) professionalPreferences(w http.ResponseWriter, r *http.Request) {
	b, pr, ok := h.professionalIDs(w, r)
	if !ok {
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	on, err := h.prefs.ProfessionalEmails(r.Context(), p, b, pr)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, professionalPreferencesDTO{Email: &on})
}

func (h *Handler) setProfessionalPreferences(w http.ResponseWriter, r *http.Request) {
	b, pr, ok := h.professionalIDs(w, r)
	if !ok {
		return
	}
	var req professionalPreferencesDTO
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	err := h.prefs.SetProfessionalEmails(r.Context(), p, b, pr, req.Email)
	if errors.Is(err, application.ErrEmailRequired) {
		writeFields(w, r, []httpx.FieldError{{Field: "email", Message: "REQUIRED"}})
		return
	}
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, req)
}

type whatsAppLimitDTO struct {
	MonthlyLimit  int       `json:"monthlyLimit"`
	IsDefault     bool      `json:"isDefault"`
	Limited       bool      `json:"limited"`
	UsedThisMonth int       `json:"usedThisMonth"`
	MonthStart    time.Time `json:"monthStart"`
}

type setWhatsAppLimitDTO struct {
	MonthlyLimit *int `json:"monthlyLimit"`
}

func (h *Handler) businessID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusNotFound, httpx.CodeNotFound, ""))
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) whatsAppLimit(w http.ResponseWriter, r *http.Request) {
	id, ok := h.businessID(w, r)
	if !ok {
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	l, err := h.prefs.BusinessWhatsAppLimit(r.Context(), p, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, whatsAppLimitDTO{
		MonthlyLimit: l.MonthlyLimit, IsDefault: l.Default, Limited: l.Limited, UsedThisMonth: l.UsedThisMonth, MonthStart: l.MonthStart,
	})
}

func (h *Handler) setWhatsAppLimit(w http.ResponseWriter, r *http.Request) {
	id, ok := h.businessID(w, r)
	if !ok {
		return
	}
	var req setWhatsAppLimitDTO
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	err := h.prefs.SetBusinessWhatsAppLimit(r.Context(), p, id, req.MonthlyLimit)
	if errors.Is(err, application.ErrInvalidLimit) {
		writeFields(w, r, []httpx.FieldError{{Field: "monthlyLimit", Message: "OUT_OF_RANGE"}})
		return
	}
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.whatsAppLimit(w, r)
}

func writeFields(w http.ResponseWriter, r *http.Request, fields []httpx.FieldError) {
	p := httpx.NewProblem(http.StatusBadRequest, httpx.CodeInvalidRequest, "one or more fields are invalid")
	p.Errors = fields
	httpx.WriteProblem(w, r, p)
}
