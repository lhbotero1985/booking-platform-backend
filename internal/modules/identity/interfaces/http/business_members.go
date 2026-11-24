package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/httpx"
)

// Problem codes of the members endpoints (T4.5).
const (
	CodeLastOwner          = "LAST_OWNER"
	CodeAlreadyMember      = "ALREADY_MEMBER"
	CodeAccountNotEligible = "ACCOUNT_NOT_ELIGIBLE"
)

// memberRoutes manage the members of a business (T4.5). The person in charge of a business is the
// OWNER role; there may be several and there is always at least one.
func (h *Handler) memberRoutes(rt *httpx.Router) {
	rt.Handle("GET /api/v1/business/{businessId}/members", h.RequireAuth(http.HandlerFunc(h.listMembers)))
	rt.Handle("POST /api/v1/business/{businessId}/members", h.RequireAuth(http.HandlerFunc(h.addMember)))
	rt.Handle("PATCH /api/v1/business/{businessId}/members/{userId}", h.RequireAuth(http.HandlerFunc(h.changeMemberRole)))
	rt.Handle("DELETE /api/v1/business/{businessId}/members/{userId}", h.RequireAuth(http.HandlerFunc(h.removeMember)))
}

type memberDTO struct {
	UserID   uuid.UUID `json:"userId"`
	FullName string    `json:"fullName"`
	Email    string    `json:"email"`
	Role     string    `json:"role"`
	Since    time.Time `json:"since"`
}

type membersResponse struct {
	Items []memberDTO `json:"items"`
}

type addMemberRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

type changeRoleRequest struct {
	Role string `json:"role"`
}

type addMemberResponse struct {
	UserID uuid.UUID `json:"userId"`
	Role   string    `json:"role"`
}

// pathUUID parses a UUID path value; a malformed one is a plain 404.
func pathUUID(r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	return id, err == nil && id != uuid.Nil
}

func (h *Handler) listMembers(w http.ResponseWriter, r *http.Request) {
	biz, ok := pathUUID(r, "businessId")
	if !ok {
		httpx.WriteAuthzProblem(w, r, contract.ErrNotFound)
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	ms, err := h.svc.ListMembers(r.Context(), p, biz)
	if err != nil {
		h.failMembers(w, r, err)
		return
	}
	out := membersResponse{Items: make([]memberDTO, 0, len(ms))}
	for _, m := range ms {
		out.Items = append(out.Items, memberDTO{UserID: m.UserID, FullName: m.FullName, Email: m.Email, Role: string(m.Role), Since: m.Since})
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) addMember(w http.ResponseWriter, r *http.Request) {
	var req addMemberRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	biz, ok := pathUUID(r, "businessId")
	if !ok {
		httpx.WriteAuthzProblem(w, r, contract.ErrNotFound)
		return
	}
	role, ok := parseRole(req.Role)
	if !ok {
		h.invalidRole(w, r)
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	userID, err := h.svc.AddMember(r.Context(), p, biz, req.Email, role)
	if err != nil {
		h.failMembers(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, addMemberResponse{UserID: userID, Role: string(role)})
}

func (h *Handler) changeMemberRole(w http.ResponseWriter, r *http.Request) {
	var req changeRoleRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	biz, okB := pathUUID(r, "businessId")
	user, okU := pathUUID(r, "userId")
	if !okB || !okU {
		httpx.WriteAuthzProblem(w, r, contract.ErrNotFound)
		return
	}
	role, ok := parseRole(req.Role)
	if !ok {
		h.invalidRole(w, r)
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	if err := h.svc.ChangeMemberRole(r.Context(), p, biz, user, role); err != nil {
		h.failMembers(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) removeMember(w http.ResponseWriter, r *http.Request) {
	biz, okB := pathUUID(r, "businessId")
	user, okU := pathUUID(r, "userId")
	if !okB || !okU {
		httpx.WriteAuthzProblem(w, r, contract.ErrNotFound)
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	if err := h.svc.RemoveMember(r.Context(), p, biz, user); err != nil {
		h.failMembers(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func parseRole(s string) (contract.Role, bool) {
	switch r := contract.Role(s); r {
	case contract.RoleOwner, contract.RoleProfessional:
		return r, true
	default:
		return "", false
	}
}

func (h *Handler) invalidRole(w http.ResponseWriter, r *http.Request) {
	p := httpx.NewProblem(http.StatusBadRequest, httpx.CodeInvalidRequest, "one or more fields are invalid")
	p.Errors = []httpx.FieldError{{Field: "role", Message: "INVALID_ROLE"}}
	httpx.WriteProblem(w, r, p)
}

func (h *Handler) failMembers(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, contract.ErrMemberNotFound):
		// Same as an unknown route: does not reveal whether the user exists.
		httpx.WriteAuthzProblem(w, r, contract.ErrNotFound)
	case errors.Is(err, contract.ErrLastOwner):
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusConflict, CodeLastOwner, "the business must keep at least one owner"))
	case errors.Is(err, contract.ErrAlreadyMember):
		httpx.WriteProblem(w, r, httpx.NewProblem(http.StatusConflict, CodeAlreadyMember, "that account is already a member"))
	case errors.Is(err, contract.ErrAccountNotEligible):
		p := httpx.NewProblem(http.StatusBadRequest, httpx.CodeInvalidRequest, "one or more fields are invalid")
		p.Errors = []httpx.FieldError{{Field: "email", Message: CodeAccountNotEligible}}
		httpx.WriteProblem(w, r, p)
	default:
		h.fail(w, r, err)
	}
}
