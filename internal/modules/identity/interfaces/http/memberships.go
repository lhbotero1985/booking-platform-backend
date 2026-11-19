package httpapi

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/httpx"
)

type membershipItem struct {
	BusinessID uuid.UUID `json:"businessId"`
	Role       string    `json:"role"`
}

// membershipsResponse uses the list envelope of api-design.md. A user belongs to a handful of
// businesses at most, so the list is not paginated (nextCursor is always absent).
type membershipsResponse struct {
	Items []membershipItem `json:"items"`
}

func (h *Handler) myMemberships(w http.ResponseWriter, r *http.Request) {
	p, _ := contract.PrincipalFrom(r.Context())
	ms, err := h.svc.MyMemberships(r.Context(), p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := membershipsResponse{Items: make([]membershipItem, 0, len(ms))}
	for _, m := range ms {
		out.Items = append(out.Items, membershipItem{BusinessID: m.BusinessID, Role: string(m.Role)})
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
