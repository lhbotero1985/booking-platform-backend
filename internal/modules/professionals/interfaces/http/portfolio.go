package httpapi

// Portfolio of a professional (2026-11-27): up to 10 images of its best work.

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/httpx"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/media"
)

// CodePortfolioFull is returned when the professional already shows 10 images.
const CodePortfolioFull = "PORTFOLIO_FULL"

// PortfolioService is the portfolio part of the application.
type PortfolioService interface {
	ListPortfolio(ctx context.Context, p contract.Principal, businessID, profID uuid.UUID) ([]domain.PortfolioImage, error)
	AddPortfolioImage(ctx context.Context, p contract.Principal, businessID, profID uuid.UUID, up media.Upload) (domain.PortfolioImage, error)
	UpdatePortfolioImage(ctx context.Context, p contract.Principal, businessID, profID, id uuid.UUID, in application.PortfolioPatch) (domain.PortfolioImage, error)
	DeletePortfolioImage(ctx context.Context, p contract.Principal, businessID, profID, id uuid.UUID) error
}

type portfolioImageDTO struct {
	ID             uuid.UUID `json:"id"`
	ProfessionalID uuid.UUID `json:"professionalId"`
	URL            string    `json:"url"`
	Caption        string    `json:"caption"`
	SortOrder      int       `json:"sortOrder"`
	Width          int       `json:"width"`
	Height         int       `json:"height"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type portfolioListDTO struct {
	Items []portfolioImageDTO `json:"items"`
	Max   int                 `json:"max"`
}

func toPortfolioDTO(img domain.PortfolioImage) portfolioImageDTO {
	return portfolioImageDTO{
		ID: img.ID, ProfessionalID: img.ProfessionalID, URL: media.URL(img.Key), Caption: img.Caption,
		SortOrder: img.SortOrder, Width: img.Width, Height: img.Height, CreatedAt: img.CreatedAt.UTC(), UpdatedAt: img.UpdatedAt.UTC(),
	}
}

type portfolioPatchRequest struct {
	Caption   *string `json:"caption"`
	SortOrder *int    `json:"sortOrder"`
}

func (h *Handler) portfolioRoutes(rt *httpx.Router) {
	auth := func(f http.HandlerFunc) http.Handler { return h.requireAuth(f) }
	base := "/api/v1/business/{businessId}/professionals/{professionalId}/portfolio"
	rt.Handle("GET "+base, auth(h.listPortfolio))
	rt.Handle("POST "+base, auth(h.addPortfolioImage))
	rt.Handle("PATCH "+base+"/{imageId}", auth(h.updatePortfolioImage))
	rt.Handle("DELETE "+base+"/{imageId}", auth(h.deletePortfolioImage))
}

func (h *Handler) listPortfolio(w http.ResponseWriter, r *http.Request) {
	pids, ok := ids(r, "businessId", "professionalId")
	if !ok {
		h.fail(w, r, application.ErrNotFound)
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	imgs, err := h.svc.ListPortfolio(r.Context(), p, pids[0], pids[1])
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := portfolioListDTO{Items: make([]portfolioImageDTO, 0, len(imgs)), Max: domain.MaxPortfolioImages}
	for _, img := range imgs {
		out.Items = append(out.Items, toPortfolioDTO(img))
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// addPortfolioImage serves POST …/portfolio (multipart/form-data: "image" and optional
// "caption").
func (h *Handler) addPortfolioImage(w http.ResponseWriter, r *http.Request) {
	pids, ok := ids(r, "businessId", "professionalId")
	if !ok {
		h.fail(w, r, application.ErrNotFound)
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	img, err := h.svc.AddPortfolioImage(r.Context(), p, pids[0], pids[1], media.FromRequest(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toPortfolioDTO(img))
}

func (h *Handler) updatePortfolioImage(w http.ResponseWriter, r *http.Request) {
	pids, ok := ids(r, "businessId", "professionalId", "imageId")
	if !ok {
		h.fail(w, r, application.ErrNotFound)
		return
	}
	var req portfolioPatchRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	img, err := h.svc.UpdatePortfolioImage(r.Context(), p, pids[0], pids[1], pids[2], application.PortfolioPatch{Caption: req.Caption, SortOrder: req.SortOrder})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toPortfolioDTO(img))
}

func (h *Handler) deletePortfolioImage(w http.ResponseWriter, r *http.Request) {
	pids, ok := ids(r, "businessId", "professionalId", "imageId")
	if !ok {
		h.fail(w, r, application.ErrNotFound)
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	if err := h.svc.DeletePortfolioImage(r.Context(), p, pids[0], pids[1], pids[2]); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
