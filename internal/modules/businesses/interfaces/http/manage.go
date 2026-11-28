package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/httpx"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/media"
)

// manageRoutes are the endpoints of a business's own staff (T4.4). The businessId of the path
// is checked against the caller's memberships on every request; another business is a 404.
func (h *Handler) manageRoutes(rt *httpx.Router) {
	auth := func(f http.HandlerFunc) http.Handler { return h.requireAuth(f) }
	rt.Handle("GET /api/v1/business/{businessId}", auth(h.getManaged))
	rt.Handle("PATCH /api/v1/business/{businessId}", auth(h.updateManaged))
	for _, kind := range []application.ImageKind{application.ImageLogo, application.ImageCover} {
		rt.Handle("PUT /api/v1/business/{businessId}/"+string(kind), auth(h.setImage(kind)))
		rt.Handle("DELETE /api/v1/business/{businessId}/"+string(kind), auth(h.removeImage(kind)))
	}
}

// setImage serves PUT /api/v1/business/{businessId}/logo|cover (multipart/form-data, field
// "image"): the OWNER replaces the image; the response is the business.
func (h *Handler) setImage(kind application.ImageKind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := businessIDParam(r)
		if !ok {
			h.fail(w, r, application.ErrNotFound)
			return
		}
		p, _ := contract.PrincipalFrom(r.Context())
		b, err := h.svc.SetImage(r.Context(), p, id, kind, media.FromRequest(r))
		if err != nil {
			h.fail(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, toDTO(b))
	}
}

// removeImage serves DELETE /api/v1/business/{businessId}/logo|cover.
func (h *Handler) removeImage(kind application.ImageKind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := businessIDParam(r)
		if !ok {
			h.fail(w, r, application.ErrNotFound)
			return
		}
		p, _ := contract.PrincipalFrom(r.Context())
		if _, err := h.svc.RemoveImage(r.Context(), p, id, kind); err != nil {
			h.fail(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

type subscriptionSummaryDTO struct {
	Status      string     `json:"status"`
	TrialEndsAt *time.Time `json:"trialEndsAt"`
	Entitled    bool       `json:"entitled"`
}

type managedDTO struct {
	Business     businessDTO            `json:"business"`
	Subscription subscriptionSummaryDTO `json:"subscription"`
}

func (h *Handler) getManaged(w http.ResponseWriter, r *http.Request) {
	id, ok := businessIDParam(r)
	if !ok {
		h.fail(w, r, application.ErrNotFound)
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	m, err := h.svc.GetManaged(r.Context(), p, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, managedDTO{
		Business: toDTO(m.Business),
		Subscription: subscriptionSummaryDTO{
			Status: m.Subscription.Status, TrialEndsAt: m.Subscription.TrialEndsAt, Entitled: m.Subscription.Entitled,
		},
	})
}

// nullableCoordinates distinguishes an absent field (Present false) from null (remove).
type nullableCoordinates struct {
	Present bool
	Value   *coordinatesDTO
}

func (n *nullableCoordinates) UnmarshalJSON(b []byte) error {
	n.Present = true
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		return nil
	}
	n.Value = &coordinatesDTO{}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	return dec.Decode(n.Value)
}

type policyPatchDTO struct {
	SlotIntervalMinutes         *int  `json:"slotIntervalMinutes"`
	BufferMinutes               *int  `json:"bufferMinutes"`
	MinLeadMinutes              *int  `json:"minLeadMinutes"`
	MaxAdvanceDays              *int  `json:"maxAdvanceDays"`
	CancellationCutoffHours     *int  `json:"cancellationCutoffHours"`
	RescheduleCutoffHours       *int  `json:"rescheduleCutoffHours"`
	MaxCustomerReschedules      *int  `json:"maxCustomerReschedules"`
	RequireCustomerConfirmation *bool `json:"requireCustomerConfirmation"`
}

type remindersPatchDTO struct {
	Enabled     *bool `json:"enabled"`
	HoursBefore *int  `json:"hoursBefore"`
}

type updateRequest struct {
	Name          *string             `json:"name"`
	ContactPhone  *string             `json:"contactPhone"`
	Description   *string             `json:"description"`
	CityID        *uuid.UUID          `json:"cityId"`
	AddressLine   *string             `json:"addressLine"`
	Coordinates   nullableCoordinates `json:"coordinates"`
	BookingPolicy *policyPatchDTO     `json:"bookingPolicy"`
	Reminders     *remindersPatchDTO  `json:"reminders"`
}

func (h *Handler) updateManaged(w http.ResponseWriter, r *http.Request) {
	var req updateRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	id, ok := businessIDParam(r)
	if !ok {
		h.fail(w, r, application.ErrNotFound)
		return
	}
	in := application.UpdateInput{
		Name: req.Name, ContactPhone: req.ContactPhone, Description: req.Description,
		AddressLine: req.AddressLine, CityID: req.CityID,
	}
	if req.Coordinates.Present {
		if req.Coordinates.Value == nil {
			in.Coordinates = &application.CoordinatesPatch{Remove: true}
		} else {
			in.Coordinates = &application.CoordinatesPatch{Lat: req.Coordinates.Value.Latitude, Lng: req.Coordinates.Value.Longitude}
		}
	}
	if p := req.BookingPolicy; p != nil {
		in.Policy = &application.PolicyPatch{
			SlotIntervalMinutes: p.SlotIntervalMinutes, BufferMinutes: p.BufferMinutes, MinLeadMinutes: p.MinLeadMinutes,
			MaxAdvanceDays: p.MaxAdvanceDays, CancellationCutoffHours: p.CancellationCutoffHours,
			RescheduleCutoffHours: p.RescheduleCutoffHours, MaxCustomerReschedules: p.MaxCustomerReschedules,
			RequireCustomerConfirmation: p.RequireCustomerConfirmation,
		}
	}
	if rm := req.Reminders; rm != nil {
		in.Reminders = &application.RemindersPatch{Enabled: rm.Enabled, HoursBefore: rm.HoursBefore}
	}
	p, _ := contract.PrincipalFrom(r.Context())
	b, err := h.svc.UpdateBusiness(r.Context(), p, id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toDTO(b))
}

func businessIDParam(r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("businessId"))
	return id, err == nil && id != uuid.Nil
}
