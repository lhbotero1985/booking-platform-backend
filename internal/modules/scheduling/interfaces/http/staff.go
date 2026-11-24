package httpapi

// Staff endpoints under /api/v1/business/{businessId} (T6.4): customers of the business and
// manual appointments. The businessId is checked against the caller's memberships on every
// request; another business is a 404 identical to a missing one.

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/httpx"
)

// CodeCustomerPhoneTaken is the 409 code for a duplicate customer mobile in a business.
const CodeCustomerPhoneTaken = "CUSTOMER_PHONE_TAKEN"

// StaffService is the subset of the application used by the staff endpoints.
type StaffService interface {
	ListCustomers(ctx context.Context, p contract.Principal, businessID uuid.UUID, q string) ([]domain.BusinessCustomer, error)
	CreateCustomer(ctx context.Context, p contract.Principal, businessID uuid.UUID, in application.CustomerInput) (domain.BusinessCustomer, error)
	UpdateCustomer(ctx context.Context, p contract.Principal, businessID, id uuid.UUID, patch application.CustomerPatch) (domain.BusinessCustomer, error)
	BookManual(ctx context.Context, p contract.Principal, businessID uuid.UUID, in application.ManualInput) (domain.Appointment, error)
}

type customerDTO struct {
	ID         uuid.UUID `json:"id"`
	FullName   string    `json:"fullName"`
	Phone      string    `json:"phone"`
	HasAccount bool      `json:"hasAccount"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

func toCustomerDTO(c domain.BusinessCustomer) customerDTO {
	return customerDTO{ID: c.ID, FullName: c.FullName, Phone: c.Phone, HasAccount: c.UserID != uuid.Nil, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt}
}

type customerListDTO struct {
	Items []customerDTO `json:"items"`
}

type createCustomerRequest struct {
	FullName string `json:"fullName"`
	Phone    string `json:"phone"`
}

type updateCustomerRequest struct {
	FullName *string `json:"fullName"`
	Phone    *string `json:"phone"`
}

type manualAppointmentRequest struct {
	ProfessionalID uuid.UUID `json:"professionalId"`
	ServiceID      uuid.UUID `json:"serviceId"`
	CustomerID     uuid.UUID `json:"customerId"`
	StartAt        time.Time `json:"startAt"`
	Confirmed      *bool     `json:"confirmed"`
}

func (h *Handler) staffRoutes(rt *httpx.Router) {
	auth := func(f http.HandlerFunc) http.Handler { return h.requireAuth(f) }
	rt.Handle("GET /api/v1/business/{businessId}/customers", auth(h.listCustomers))
	rt.Handle("POST /api/v1/business/{businessId}/customers", auth(h.createCustomer))
	rt.Handle("PATCH /api/v1/business/{businessId}/customers/{customerId}", auth(h.updateCustomer))
	rt.Handle("POST /api/v1/business/{businessId}/appointments", auth(h.bookManual))
}

// pathIDs parses the UUID path values; an invalid one is a 404 like a missing resource.
func (h *Handler) pathIDs(w http.ResponseWriter, r *http.Request, names ...string) ([]uuid.UUID, bool) {
	out := make([]uuid.UUID, len(names))
	for i, n := range names {
		id, err := uuid.Parse(r.PathValue(n))
		if err != nil {
			httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusNotFound, httpx.CodeNotFound, ""))
			return nil, false
		}
		out[i] = id
	}
	return out, true
}

func (h *Handler) listCustomers(w http.ResponseWriter, r *http.Request) {
	ids, ok := h.pathIDs(w, r, "businessId")
	if !ok {
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	cs, err := h.staff.ListCustomers(r.Context(), p, ids[0], r.URL.Query().Get("q"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := customerListDTO{Items: make([]customerDTO, 0, len(cs))}
	for _, c := range cs {
		out.Items = append(out.Items, toCustomerDTO(c))
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) createCustomer(w http.ResponseWriter, r *http.Request) {
	ids, ok := h.pathIDs(w, r, "businessId")
	if !ok {
		return
	}
	var req createCustomerRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	c, err := h.staff.CreateCustomer(r.Context(), p, ids[0], application.CustomerInput{FullName: req.FullName, Phone: req.Phone})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toCustomerDTO(c))
}

func (h *Handler) updateCustomer(w http.ResponseWriter, r *http.Request) {
	ids, ok := h.pathIDs(w, r, "businessId", "customerId")
	if !ok {
		return
	}
	var req updateCustomerRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	c, err := h.staff.UpdateCustomer(r.Context(), p, ids[0], ids[1], application.CustomerPatch{FullName: req.FullName, Phone: req.Phone})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toCustomerDTO(c))
}

// bookManual serves POST /api/v1/business/{businessId}/appointments: the staff books a
// customer of the business. Idempotency-Key mandatory; confirmed defaults to true.
func (h *Handler) bookManual(w http.ResponseWriter, r *http.Request) {
	ids, ok := h.pathIDs(w, r, "businessId")
	if !ok {
		return
	}
	var req manualAppointmentRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	confirmed := req.Confirmed == nil || *req.Confirmed
	p, _ := contract.PrincipalFrom(r.Context())
	a, err := h.staff.BookManual(r.Context(), p, ids[0], application.ManualInput{
		ProfessionalID: req.ProfessionalID, ServiceID: req.ServiceID, CustomerID: req.CustomerID, Start: req.StartAt,
		Confirmed: confirmed, IdempotencyKey: r.Header.Get("Idempotency-Key"),
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toAppointmentDTO(a))
}
