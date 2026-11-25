package httpapi

// Agenda of a business and "my appointments" of a customer (T6.5).

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/httpx"
)

// AgendaService is the subset of the application used by the agenda endpoints.
type AgendaService interface {
	Agenda(ctx context.Context, p contract.Principal, businessID uuid.UUID, q application.AgendaQuery) (application.AgendaPage, error)
	MyAppointments(ctx context.Context, p contract.Principal, q application.MyAppointmentsQuery) ([]application.MyAppointment, string, error)
	MyAppointmentDetail(ctx context.Context, p contract.Principal, id uuid.UUID) (application.MyAppointment, error)
	ToReassign(ctx context.Context, p contract.Principal, businessID uuid.UUID) (application.AgendaPage, error)
}

type agendaCustomerDTO struct {
	ID         *uuid.UUID `json:"id"`
	Name       string     `json:"name"`
	Phone      string     `json:"phone"`
	HasAccount bool       `json:"hasAccount"`
}

type agendaItemDTO struct {
	ID                uuid.UUID         `json:"id"`
	ProfessionalID    uuid.UUID         `json:"professionalId"`
	ProfessionalName  string            `json:"professionalName"`
	ServiceID         uuid.UUID         `json:"serviceId"`
	ServiceName       string            `json:"serviceName"`
	DurationMinutes   int               `json:"durationMinutes"`
	PriceMinor        int64             `json:"priceMinor"`
	Currency          string            `json:"currency"`
	StartAt           time.Time         `json:"startAt"`
	EndAt             time.Time         `json:"endAt"`
	StartLocal        string            `json:"startLocal"`
	Status            string            `json:"status"`
	Customer          agendaCustomerDTO `json:"customer"`
	NeedsReassignment bool              `json:"needsReassignment"`
	CreatedAt         time.Time         `json:"createdAt"`
}

type agendaDTO struct {
	View       string          `json:"view"`
	From       string          `json:"from"` // first local day
	To         string          `json:"to"`   // last local day (inclusive)
	TimeZone   string          `json:"timeZone"`
	Items      []agendaItemDTO `json:"items"`
	NextCursor *string         `json:"nextCursor"`
}

type refDTO struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type businessRefDTO struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
	Slug string    `json:"slug"`
}

type myAppointmentDTO struct {
	ID              uuid.UUID      `json:"id"`
	Business        businessRefDTO `json:"business"`
	Professional    refDTO         `json:"professional"`
	ServiceName     string         `json:"serviceName"`
	DurationMinutes int            `json:"durationMinutes"`
	PriceMinor      int64          `json:"priceMinor"`
	Currency        string         `json:"currency"`
	StartAt         time.Time      `json:"startAt"`
	EndAt           time.Time      `json:"endAt"`
	StartLocal      string         `json:"startLocal"`
	Status          string         `json:"status"`
	CreatedAt       time.Time      `json:"createdAt"`
}

type myAppointmentsDTO struct {
	Items      []myAppointmentDTO `json:"items"`
	NextCursor *string            `json:"nextCursor"`
}

func (h *Handler) agendaRoutes(rt *httpx.Router) {
	auth := func(f http.HandlerFunc) http.Handler { return h.requireAuth(f) }
	rt.Handle("GET /api/v1/business/{businessId}/agenda", auth(h.agenda))
	rt.Handle("GET /api/v1/business/{businessId}/appointments/to-reassign", auth(h.toReassign))
	rt.Handle("GET /api/v1/me/appointments", auth(h.myAppointments))
	rt.Handle("GET /api/v1/me/appointments/{appointmentId}", auth(h.myAppointment))
}

func optionalCursor(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func queryLimit(r *http.Request) (int, bool) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return 0, true
	}
	n, err := strconv.Atoi(raw)
	return n, err == nil && n > 0
}

func badQuery(w http.ResponseWriter, r *http.Request, field, code string) {
	p := httpx.NewProblem(http.StatusBadRequest, httpx.CodeInvalidRequest, "one or more query parameters are invalid")
	p.Errors = []httpx.FieldError{{Field: field, Message: code}}
	httpx.WriteProblem(w, r, p)
}

// agenda serves GET /api/v1/business/{businessId}/agenda?view=&date=&professionalId=&cursor=&limit=.
func (h *Handler) agenda(w http.ResponseWriter, r *http.Request) {
	ids, ok := h.pathIDs(w, r, "businessId")
	if !ok {
		return
	}
	q := r.URL.Query()
	in := application.AgendaQuery{Cursor: q.Get("cursor")}
	if v := q.Get("view"); v != "" {
		if in.View, ok = domain.ParseView(v); !ok {
			badQuery(w, r, "view", "INVALID_VIEW")
			return
		}
	}
	if d := q.Get("date"); d != "" {
		if in.Date, ok = domain.ParseLocalDate(d); !ok {
			badQuery(w, r, "date", "INVALID_DATE")
			return
		}
	}
	if pro := q.Get("professionalId"); pro != "" {
		id, err := uuid.Parse(pro)
		if err != nil {
			badQuery(w, r, "professionalId", "INVALID_ID")
			return
		}
		in.ProfessionalID = id
	}
	if in.Limit, ok = queryLimit(r); !ok {
		badQuery(w, r, "limit", "OUT_OF_RANGE")
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	page, err := h.agendas.Agenda(r.Context(), p, ids[0], in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	view := in.View
	if view == "" {
		view = domain.ViewDay
	}
	httpx.WriteJSON(w, http.StatusOK, toAgendaDTO(view, page))
}

func toAgendaDTO(view domain.View, page application.AgendaPage) agendaDTO {
	out := agendaDTO{
		View: string(view), From: page.From.String(), To: page.To.AddDays(-1).String(), TimeZone: page.Location.String(),
		Items: make([]agendaItemDTO, 0, len(page.Items)), NextCursor: optionalCursor(page.NextCursor),
	}
	for _, a := range page.Items {
		c := agendaCustomerDTO{Name: a.Customer.Name, Phone: a.Customer.Phone, HasAccount: a.Customer.UserID != uuid.Nil}
		if a.Customer.CustomerID != uuid.Nil {
			id := a.Customer.CustomerID
			c.ID = &id
		}
		out.Items = append(out.Items, agendaItemDTO{
			ID: a.ID, ProfessionalID: a.ProfessionalID, ProfessionalName: a.ProfessionalName, ServiceID: a.Service.ServiceID,
			ServiceName: a.Service.Name, DurationMinutes: a.Service.DurationMinutes, PriceMinor: a.Service.PriceMinor,
			Currency: a.Service.Currency, StartAt: a.Start, EndAt: a.End, StartLocal: a.Start.In(page.Location).Format(time.RFC3339),
			Status: string(a.Status), Customer: c, NeedsReassignment: a.NeedsReassignment, CreatedAt: a.CreatedAt,
		})
	}
	return out
}

// toReassign serves GET /api/v1/business/{businessId}/appointments/to-reassign (OWNER, T6.7).
func (h *Handler) toReassign(w http.ResponseWriter, r *http.Request) {
	ids, ok := h.pathIDs(w, r, "businessId")
	if !ok {
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	page, err := h.agendas.ToReassign(r.Context(), p, ids[0])
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toAgendaDTO(domain.ViewYear, page))
}

func toMyAppointmentDTO(a application.MyAppointment) myAppointmentDTO {
	local := a.Start.Format(time.RFC3339)
	if loc, err := time.LoadLocation(a.Business.TimeZone); err == nil && a.Business.TimeZone != "" {
		local = a.Start.In(loc).Format(time.RFC3339)
	}
	return myAppointmentDTO{
		ID: a.ID, Business: businessRefDTO{ID: a.BusinessID, Name: a.Business.Name, Slug: a.Business.Slug},
		Professional: refDTO{ID: a.ProfessionalID, Name: a.ProfessionalName}, ServiceName: a.Service.Name,
		DurationMinutes: a.Service.DurationMinutes, PriceMinor: a.Service.PriceMinor, Currency: a.Service.Currency,
		StartAt: a.Start, EndAt: a.End, StartLocal: local, Status: string(a.Status), CreatedAt: a.CreatedAt,
	}
}

// myAppointments serves GET /api/v1/me/appointments?scope=upcoming|past&cursor=&limit=.
func (h *Handler) myAppointments(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	in := application.MyAppointmentsQuery{Upcoming: true, Cursor: q.Get("cursor")}
	switch q.Get("scope") {
	case "", "upcoming":
	case "past":
		in.Upcoming = false
	default:
		badQuery(w, r, "scope", "INVALID_SCOPE")
		return
	}
	var ok bool
	if in.Limit, ok = queryLimit(r); !ok {
		badQuery(w, r, "limit", "OUT_OF_RANGE")
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	items, next, err := h.agendas.MyAppointments(r.Context(), p, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := myAppointmentsDTO{Items: make([]myAppointmentDTO, 0, len(items)), NextCursor: optionalCursor(next)}
	for _, a := range items {
		out.Items = append(out.Items, toMyAppointmentDTO(a))
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// myAppointment serves GET /api/v1/me/appointments/{appointmentId}: anyone else's is a 404.
func (h *Handler) myAppointment(w http.ResponseWriter, r *http.Request) {
	ids, ok := h.pathIDs(w, r, "appointmentId")
	if !ok {
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	a, err := h.agendas.MyAppointmentDetail(r.Context(), p, ids[0])
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toMyAppointmentDTO(a))
}
