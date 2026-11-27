// Package httpapi exposes the scheduling endpoints: public availability (T6.2) and customer
// bookings (T6.3).
package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/httpx"
)

// Problem codes.
const (
	CodeOutsideBookingWindow = "DATE_OUTSIDE_BOOKING_WINDOW"
	CodeSlotUnavailable      = "SLOT_UNAVAILABLE"
	CodePhoneRequired        = "PHONE_REQUIRED"
	CodeIdempotencyKeyReused = "IDEMPOTENCY_KEY_REUSED"
	CodeBusy                 = "AGENDA_BUSY"
)

// Service is the subset of the application used by these handlers.
type Service interface {
	PublicAvailability(ctx context.Context, slug string, serviceID, professionalID uuid.UUID, date domain.LocalDate) (application.Availability, error)
	Book(ctx context.Context, p contract.Principal, in application.BookingInput) (domain.Appointment, error)
	StaffService
	AgendaService
	ChangeService
}

// Handler serves the scheduling endpoints.
type Handler struct {
	svc         Service
	staff       StaffService
	agendas     AgendaService
	changes     ChangeService
	logger      *slog.Logger
	requireAuth httpx.Middleware
}

// New builds the handler; requireAuth is the identity module's authentication middleware.
func New(svc Service, logger *slog.Logger, requireAuth httpx.Middleware) *Handler {
	return &Handler{svc: svc, staff: svc, agendas: svc, changes: svc, logger: logger, requireAuth: requireAuth}
}

// Routes registers the endpoints.
func (h *Handler) Routes(rt *httpx.Router) {
	rt.HandleFunc("GET /api/v1/businesses/{slug}/availability", h.publicAvailability)
	rt.Handle("POST /api/v1/appointments", h.requireAuth(http.HandlerFunc(h.book)))
	h.staffRoutes(rt)
	h.agendaRoutes(rt)
	h.changeRoutes(rt)
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
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toAvailabilityDTO(a, serviceID, professionalID))
}

func toAvailabilityDTO(a application.Availability, serviceID, professionalID uuid.UUID) availabilityDTO {
	out := availabilityDTO{
		Date: a.Date.String(), TimeZone: a.Location.String(), ServiceID: serviceID, ProfessionalID: professionalID,
		DurationMinutes: a.DurationMinutes, Slots: make([]slotDTO, 0, len(a.Slots)),
	}
	for _, s := range a.Slots {
		out.Slots = append(out.Slots, slotDTO{StartAt: s.Start, EndAt: s.End, StartLocal: s.Start.In(a.Location).Format(time.RFC3339)})
	}
	return out
}

type bookRequest struct {
	BusinessID     uuid.UUID `json:"businessId"`
	ServiceID      uuid.UUID `json:"serviceId"`
	ProfessionalID uuid.UUID `json:"professionalId"`
	StartAt        time.Time `json:"startAt"`
}

// appointmentDTO is an appointment as its customer sees it.
type appointmentDTO struct {
	ID              uuid.UUID `json:"id"`
	BusinessID      uuid.UUID `json:"businessId"`
	ProfessionalID  uuid.UUID `json:"professionalId"`
	ServiceID       uuid.UUID `json:"serviceId"`
	ServiceName     string    `json:"serviceName"`
	DurationMinutes int       `json:"durationMinutes"`
	PriceMinor      int64     `json:"priceMinor"`
	Currency        string    `json:"currency"`
	StartAt         time.Time `json:"startAt"`
	EndAt           time.Time `json:"endAt"`
	Status          string    `json:"status"`
	CustomerName    string    `json:"customerName"`
	Version         int       `json:"version"`
	// CustomerReschedules: times the customer moved it (T7.8; the business's
	// maxCustomerReschedules limits them).
	CustomerReschedules int        `json:"customerReschedules"`
	CancelledAt         *time.Time `json:"cancelledAt"`
	CancelReason        *string    `json:"cancelReason"`
	CancelledBy         *string    `json:"cancelledBy"`
	CreatedAt           time.Time  `json:"createdAt"`
}

func toAppointmentDTO(a domain.Appointment) appointmentDTO {
	out := appointmentDTO{
		ID: a.ID, BusinessID: a.BusinessID, ProfessionalID: a.ProfessionalID, ServiceID: a.Service.ServiceID,
		ServiceName: a.Service.Name, DurationMinutes: a.Service.DurationMinutes, PriceMinor: a.Service.PriceMinor,
		Currency: a.Service.Currency, StartAt: a.Start, EndAt: a.End, Status: string(a.Status),
		CustomerName: a.Customer.Name, Version: a.Version, CustomerReschedules: a.CustomerReschedules, CancelReason: optional(a.CancelReason),
		CancelledBy: optional(string(a.CancelledBy)), CreatedAt: a.CreatedAt,
	}
	if !a.CancelledAt.IsZero() {
		at := a.CancelledAt
		out.CancelledAt = &at
	}
	return out
}

// book serves POST /api/v1/appointments (T6.3): the authenticated customer books a slot. The
// Idempotency-Key header is mandatory; repeating the request with it returns the same
// appointment.
func (h *Handler) book(w http.ResponseWriter, r *http.Request) {
	var req bookRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	var fields []httpx.FieldError
	for _, f := range []struct {
		name string
		id   uuid.UUID
	}{{"businessId", req.BusinessID}, {"serviceId", req.ServiceID}, {"professionalId", req.ProfessionalID}} {
		if f.id == uuid.Nil {
			fields = append(fields, httpx.FieldError{Field: f.name, Message: "REQUIRED"})
		}
	}
	if req.StartAt.IsZero() {
		fields = append(fields, httpx.FieldError{Field: "startAt", Message: "REQUIRED"})
	}
	if len(fields) > 0 {
		p := httpx.NewProblem(http.StatusBadRequest, httpx.CodeInvalidRequest, "one or more fields are invalid")
		p.Errors = fields
		httpx.WriteProblem(w, r, p)
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	a, err := h.svc.Book(r.Context(), p, application.BookingInput{
		BusinessID: req.BusinessID, ServiceID: req.ServiceID, ProfessionalID: req.ProfessionalID,
		Start: req.StartAt, IdempotencyKey: r.Header.Get("Idempotency-Key"),
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toAppointmentDTO(a))
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
	case errors.Is(err, application.ErrCustomerNotFound), errors.Is(err, application.ErrAppointmentNotFound):
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusNotFound, httpx.CodeNotFound, ""))
	case errors.Is(err, application.ErrCustomerPhoneTaken):
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusConflict, CodeCustomerPhoneTaken, "another customer of this business has that mobile number"))
	case errors.Is(err, application.ErrNotFound):
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusNotFound, httpx.CodeNotFound, ""))
	case errors.Is(err, domain.ErrOutsideBookingWindow):
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusBadRequest, CodeOutsideBookingWindow, "the date is before today or beyond the business's booking horizon"))
	case errors.Is(err, application.ErrInvalidIdempotencyKey):
		p := httpx.NewProblem(http.StatusBadRequest, httpx.CodeInvalidRequest, "the Idempotency-Key header is required (8-100 characters: letters, digits, - and _)")
		p.Errors = []httpx.FieldError{{Field: "Idempotency-Key", Message: "INVALID"}}
		httpx.WriteProblem(w, r, p)
	case errors.Is(err, application.ErrSlotUnavailable):
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusConflict, CodeSlotUnavailable, "that start time is not available; ask for the availability again"))
	case errors.Is(err, application.ErrPhoneRequired):
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusUnprocessableEntity, CodePhoneRequired, "set your mobile number (PUT /api/v1/me/phone) before booking"))
	case errors.Is(err, application.ErrIdempotencyKeyReused):
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusUnprocessableEntity, CodeIdempotencyKeyReused, "this Idempotency-Key was used for a different booking"))
	case errors.Is(err, domain.ErrInvalidTransition):
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusConflict, CodeInvalidTransition, "the appointment does not allow that change in its current status"))
	case errors.Is(err, domain.ErrTooEarly):
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusConflict, CodeTooEarly, "too early for that change"))
	case errors.Is(err, domain.ErrAlreadyStarted):
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusConflict, CodeAlreadyStarted, "the appointment time has passed: it can only be completed or marked as no-show"))
	case errors.Is(err, domain.ErrCutoffPassed):
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusConflict, CodeCutoffPassed, "too close to the appointment to change it from the app; contact the business"))
	case errors.Is(err, domain.ErrRescheduleLimit):
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusConflict, CodeRescheduleLimit, "the appointment was already moved as many times as the business allows; contact the business"))
	case errors.Is(err, application.ErrAppointmentChanged):
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusConflict, CodeAppointmentChanged, "the appointment changed meanwhile; read it again"))
	case errors.Is(err, application.ErrBusy):
		w.Header().Set("Retry-After", "1")
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusServiceUnavailable, CodeBusy, "the agenda is busy, retry"))
	default:
		h.logger.ErrorContext(r.Context(), "scheduling request failed",
			slog.String("request_id", httpx.RequestIDFrom(r.Context())),
			slog.String("error", err.Error()))
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusInternalServerError, httpx.CodeInternal, ""))
	}
}
