// Package httpapi exposes the scheduling endpoints (T6.2: public availability).
package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/httpx"
)

// Problem codes.
const (
	CodeOutsideBookingWindow = "DATE_OUTSIDE_BOOKING_WINDOW"
)

// Service is the subset of the application used by these handlers.
type Service interface {
	PublicAvailability(ctx context.Context, slug string, serviceID, professionalID uuid.UUID, date domain.LocalDate) (application.Availability, error)
}

// Handler serves the scheduling endpoints.
type Handler struct {
	svc    Service
	logger *slog.Logger
}

// New builds the handler.
func New(svc Service, logger *slog.Logger) *Handler { return &Handler{svc: svc, logger: logger} }

// Routes registers the endpoints.
func (h *Handler) Routes(rt *httpx.Router) {
	rt.HandleFunc("GET /api/v1/businesses/{slug}/availability", h.publicAvailability)
}

type slotDTO struct {
	StartAt    time.Time `json:"startAt"`    // UTC
	EndAt      time.Time `json:"endAt"`      // UTC
	StartLocal string    `json:"startLocal"` // RFC 3339 with the business's offset
}

type availabilityDTO struct {
	Date            string    `json:"date"`
	TimeZone        string    `json:"timeZone"`
	ServiceID       uuid.UUID `json:"serviceId"`
	ProfessionalID  uuid.UUID `json:"professionalId"`
	DurationMinutes int       `json:"durationMinutes"`
	Slots           []slotDTO `json:"slots"`
}

// publicAvailability serves GET /api/v1/businesses/{slug}/availability?serviceId=&professionalId=&date=
// (no authentication, general rate limit). A business customers cannot see, a service that is
// not bookable with that professional, and an unknown professional are the same 404.
func (h *Handler) publicAvailability(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var fields []httpx.FieldError
	serviceID, err := uuid.Parse(q.Get("serviceId"))
	if err != nil {
		fields = append(fields, httpx.FieldError{Field: "serviceId", Message: "INVALID_ID"})
	}
	professionalID, err := uuid.Parse(q.Get("professionalId"))
	if err != nil {
		fields = append(fields, httpx.FieldError{Field: "professionalId", Message: "INVALID_ID"})
	}
	date, ok := domain.ParseLocalDate(q.Get("date"))
	if !ok {
		fields = append(fields, httpx.FieldError{Field: "date", Message: "INVALID_DATE"})
	}
	if len(fields) > 0 {
		p := httpx.NewProblem(http.StatusBadRequest, httpx.CodeInvalidRequest, "one or more query parameters are invalid")
		p.Errors = fields
		httpx.WriteProblem(w, r, p)
		return
	}
	a, err := h.svc.PublicAvailability(r.Context(), r.PathValue("slug"), serviceID, professionalID, date)
	switch {
	case err == nil:
	case errors.Is(err, application.ErrNotFound):
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusNotFound, httpx.CodeNotFound, ""))
		return
	case errors.Is(err, domain.ErrOutsideBookingWindow):
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusBadRequest, CodeOutsideBookingWindow, "the date is before today or beyond the business's booking horizon"))
		return
	default:
		h.logger.ErrorContext(r.Context(), "availability request failed",
			slog.String("request_id", httpx.RequestIDFrom(r.Context())),
			slog.String("error", err.Error()))
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusInternalServerError, httpx.CodeInternal, ""))
		return
	}
	out := availabilityDTO{
		Date: a.Date.String(), TimeZone: a.Location.String(), ServiceID: serviceID, ProfessionalID: professionalID,
		DurationMinutes: a.DurationMinutes, Slots: make([]slotDTO, 0, len(a.Slots)),
	}
	for _, s := range a.Slots {
		out.Slots = append(out.Slots, slotDTO{StartAt: s.Start, EndAt: s.End, StartLocal: s.Start.In(a.Location).Format(time.RFC3339)})
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
