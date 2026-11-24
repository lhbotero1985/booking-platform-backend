package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/httpx"
)

// CodeAbsenceOverlap is returned when an absence overlaps another of the same professional.
const CodeAbsenceOverlap = "ABSENCE_OVERLAP"

// AbsenceService is the absences part of the application (T5.4).
type AbsenceService interface {
	ListAbsences(ctx context.Context, p contract.Principal, businessID, profID uuid.UUID, from, to *time.Time) ([]domain.Absence, error)
	CreateAbsence(ctx context.Context, p contract.Principal, businessID, profID uuid.UUID, in application.AbsenceInput) (domain.Absence, error)
	UpdateAbsence(ctx context.Context, p contract.Principal, businessID, profID, id uuid.UUID, in application.AbsencePatch) (domain.Absence, error)
	DeleteAbsence(ctx context.Context, p contract.Principal, businessID, profID, id uuid.UUID) error
}

type absenceDTO struct {
	ID             uuid.UUID `json:"id"`
	ProfessionalID uuid.UUID `json:"professionalId"`
	Start          time.Time `json:"start"`
	End            time.Time `json:"end"`
	Reason         string    `json:"reason"`
	CreatedBy      uuid.UUID `json:"createdBy"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type absenceListDTO struct {
	Items []absenceDTO `json:"items"`
}

type absenceRequest struct {
	Start  *time.Time `json:"start"`
	End    *time.Time `json:"end"`
	Reason *string    `json:"reason"`
}

func toAbsenceDTO(a domain.Absence) absenceDTO {
	return absenceDTO{
		ID: a.ID, ProfessionalID: a.ProfessionalID, Start: a.Start.UTC(), End: a.End.UTC(), Reason: a.Reason,
		CreatedBy: a.CreatedBy, CreatedAt: a.CreatedAt.UTC(), UpdatedAt: a.UpdatedAt.UTC(),
	}
}

// queryTime parses an optional RFC 3339 query parameter.
func queryTime(r *http.Request, name string, verr *application.ValidationError) *time.Time {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		verr.Fields = append(verr.Fields, application.FieldError{Field: name, Code: domain.CodeInvalidTime})
		return nil
	}
	return &t
}

func (h *Handler) listAbsences(w http.ResponseWriter, r *http.Request) {
	v, ok := ids(r, "businessId", "professionalId")
	if !ok {
		h.fail(w, r, contract.ErrNotFound)
		return
	}
	var verr application.ValidationError
	from, to := queryTime(r, "from", &verr), queryTime(r, "to", &verr)
	if len(verr.Fields) > 0 {
		h.fail(w, r, &verr)
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	list, err := h.svc.ListAbsences(r.Context(), p, v[0], v[1], from, to)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := absenceListDTO{Items: make([]absenceDTO, 0, len(list))}
	for _, a := range list {
		out.Items = append(out.Items, toAbsenceDTO(a))
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) createAbsence(w http.ResponseWriter, r *http.Request) {
	var req absenceRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	v, ok := ids(r, "businessId", "professionalId")
	if !ok {
		h.fail(w, r, contract.ErrNotFound)
		return
	}
	in := application.AbsenceInput{}
	if req.Start != nil {
		in.Start = *req.Start
	}
	if req.End != nil {
		in.End = *req.End
	}
	if req.Reason != nil {
		in.Reason = *req.Reason
	}
	p, _ := contract.PrincipalFrom(r.Context())
	a, err := h.svc.CreateAbsence(r.Context(), p, v[0], v[1], in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toAbsenceDTO(a))
}

func (h *Handler) updateAbsence(w http.ResponseWriter, r *http.Request) {
	var req absenceRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	v, ok := ids(r, "businessId", "professionalId", "absenceId")
	if !ok {
		h.fail(w, r, contract.ErrNotFound)
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	a, err := h.svc.UpdateAbsence(r.Context(), p, v[0], v[1], v[2], application.AbsencePatch{Start: req.Start, End: req.End, Reason: req.Reason})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toAbsenceDTO(a))
}

func (h *Handler) deleteAbsence(w http.ResponseWriter, r *http.Request) {
	v, ok := ids(r, "businessId", "professionalId", "absenceId")
	if !ok {
		h.fail(w, r, contract.ErrNotFound)
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	if err := h.svc.DeleteAbsence(r.Context(), p, v[0], v[1], v[2]); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
