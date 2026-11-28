package httpapi

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/application"
	scheduling "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/httpx"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/media"
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
	ID          uuid.UUID           `json:"id"`
	DisplayName string              `json:"displayName"`
	Bio         string              `json:"bio"`
	ServiceIDs  []uuid.UUID         `json:"serviceIds"`
	Portfolio   []portfolioPhotoDTO `json:"portfolio"`
	// Rating of the professional from the customers' reviews (2026-11-28).
	Rating ratingDTO `json:"rating"`
}

// ratingDTO is the average of the visible reviews (one decimal, null without reviews) and how
// many there are.
type ratingDTO struct {
	Average *float64 `json:"average"`
	Count   int      `json:"count"`
}

// WithRatings makes the public catalog show the professionals' ratings (the scheduling
// module's reviews). Call once, before serving.
func (h *Handler) WithRatings(r scheduling.Ratings) *Handler {
	h.ratings = r
	return h
}

// portfolioPhotoDTO is an image of a professional's best work.
type portfolioPhotoDTO struct {
	ID      uuid.UUID `json:"id"`
	URL     string    `json:"url"`
	Caption string    `json:"caption"`
	Width   int       `json:"width"`
	Height  int       `json:"height"`
}

// publicCatalogDTO is what customers see: no status, sort keys, accounts or timestamps.
type publicCatalogDTO struct {
	Categories    []publicCategoryDTO     `json:"categories"`
	Services      []publicServiceDTO      `json:"services"`
	Professionals []publicProfessionalDTO `json:"professionals"`
}

func toPublicCatalogDTO(c application.PublicCatalog, ratings map[uuid.UUID]scheduling.Rating) publicCatalogDTO {
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
		photos := make([]portfolioPhotoDTO, 0, len(p.Portfolio))
		for _, ph := range p.Portfolio {
			photos = append(photos, portfolioPhotoDTO{ID: ph.ID, URL: media.URL(ph.Key), Caption: ph.Caption, Width: ph.Width, Height: ph.Height})
		}
		rt := ratings[p.ID]
		out.Professionals = append(out.Professionals, publicProfessionalDTO{ID: p.ID, DisplayName: p.DisplayName, Bio: p.Bio, ServiceIDs: p.ServiceIDs,
			Portfolio: photos, Rating: ratingDTO{Average: rt.Average, Count: rt.Count}})
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
	ratings := map[uuid.UUID]scheduling.Rating{}
	if h.ratings != nil && len(c.Professionals) > 0 {
		if ratings, err = h.ratings.ProfessionalRatings(r.Context(), c.BusinessID); err != nil {
			h.fail(w, r, err)
			return
		}
	}
	httpx.WriteJSON(w, http.StatusOK, toPublicCatalogDTO(c, ratings))
}
