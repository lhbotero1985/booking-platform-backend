package httpapi

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/httpx"
)

// PublicCatalogService is the public view of the catalog (T5.7).
type PublicCatalogService interface {
	PublicCatalog(ctx context.Context, slug string) (application.PublicCatalog, error)
}

type publicCategoryDTO struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type publicServiceDTO struct {
	ID              uuid.UUID   `json:"id"`
	CategoryID      *uuid.UUID  `json:"categoryId"`
	Name            string      `json:"name"`
	Description     string      `json:"description"`
	DurationMinutes int         `json:"durationMinutes"`
	PriceMinor      int64       `json:"priceMinor"`
	Currency        string      `json:"currency"`
	ProfessionalIDs []uuid.UUID `json:"professionalIds"`
}

type publicProfessionalDTO struct {
	ID          uuid.UUID   `json:"id"`
	DisplayName string      `json:"displayName"`
	Bio         string      `json:"bio"`
	ServiceIDs  []uuid.UUID `json:"serviceIds"`
}

// publicCatalogDTO is what customers see: no status, sort keys, accounts or timestamps.
type publicCatalogDTO struct {
	Categories    []publicCategoryDTO     `json:"categories"`
	Services      []publicServiceDTO      `json:"services"`
	Professionals []publicProfessionalDTO `json:"professionals"`
}

func toPublicCatalogDTO(c application.PublicCatalog) publicCatalogDTO {
	out := publicCatalogDTO{
		Categories:    make([]publicCategoryDTO, 0, len(c.Categories)),
		Services:      make([]publicServiceDTO, 0, len(c.Services)),
		Professionals: make([]publicProfessionalDTO, 0, len(c.Professionals)),
	}
	for _, cat := range c.Categories {
		out.Categories = append(out.Categories, publicCategoryDTO{ID: cat.ID, Name: cat.Name})
	}
	for _, s := range c.Services {
		dto := publicServiceDTO{
			ID: s.ID, Name: s.Name, Description: s.Description, DurationMinutes: s.DurationMinutes,
			PriceMinor: s.PriceMinor, Currency: s.Currency, ProfessionalIDs: s.ProfessionalIDs,
		}
		if s.CategoryID != uuid.Nil {
			id := s.CategoryID
			dto.CategoryID = &id
		}
		out.Services = append(out.Services, dto)
	}
	for _, p := range c.Professionals {
		out.Professionals = append(out.Professionals, publicProfessionalDTO{ID: p.ID, DisplayName: p.DisplayName, Bio: p.Bio, ServiceIDs: p.ServiceIDs})
	}
	return out
}

// publicCatalog serves GET /api/v1/businesses/{slug}/catalog (no authentication, general rate
// limit). A business customers cannot see is a 404.
func (h *Handler) publicCatalog(w http.ResponseWriter, r *http.Request) {
	c, err := h.svc.PublicCatalog(r.Context(), r.PathValue("slug"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toPublicCatalogDTO(c))
}
