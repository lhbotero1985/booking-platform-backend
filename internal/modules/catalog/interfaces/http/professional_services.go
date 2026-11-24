package httpapi

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/httpx"
)

// ProfessionalServices is the services-per-professional part of the application (T5.6).
type ProfessionalServices interface {
	ProfessionalServices(ctx context.Context, p contract.Principal, businessID, professionalID uuid.UUID) ([]domain.Service, error)
	ReplaceProfessionalServices(ctx context.Context, p contract.Principal, businessID, professionalID uuid.UUID, serviceIDs []uuid.UUID) ([]domain.Service, error)
}

type professionalServicesRequest struct {
	ServiceIDs []string `json:"serviceIds"`
}

func (h *Handler) writeServices(w http.ResponseWriter, list []domain.Service) {
	items := make([]serviceDTO, 0, len(list))
	for _, s := range list {
		items = append(items, toServiceDTO(s))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) listProfessionalServices(w http.ResponseWriter, r *http.Request) {
	v, ok := ids(r, "businessId", "professionalId")
	if !ok {
		h.fail(w, r, contract.ErrNotFound)
		return
	}
	list, err := h.svc.ProfessionalServices(r.Context(), principal(r), v[0], v[1])
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeServices(w, list)
}

func (h *Handler) replaceProfessionalServices(w http.ResponseWriter, r *http.Request) {
	var req professionalServicesRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	v, ok := ids(r, "businessId", "professionalId")
	if !ok {
		h.fail(w, r, contract.ErrNotFound)
		return
	}
	var verr application.ValidationError
	serviceIDs := make([]uuid.UUID, 0, len(req.ServiceIDs))
	for i, raw := range req.ServiceIDs {
		id, err := uuid.Parse(raw)
		if err != nil || id == uuid.Nil {
			verr.Fields = append(verr.Fields, application.FieldError{Field: fmt.Sprintf("serviceIds[%d]", i), Code: codeInvalidID})
			continue
		}
		serviceIDs = append(serviceIDs, id)
	}
	if len(verr.Fields) > 0 {
		h.fail(w, r, &verr)
		return
	}
	list, err := h.svc.ReplaceProfessionalServices(r.Context(), principal(r), v[0], v[1], serviceIDs)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeServices(w, list)
}
