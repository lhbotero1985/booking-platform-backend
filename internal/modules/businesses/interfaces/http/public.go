package httpapi

import (
	"context"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/domain"
	scheduling "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/httpx"
)

// publicRoutes are the customer-facing endpoints (T4.6): no authentication, general rate limit.
func (h *Handler) publicRoutes(rt *httpx.Router) {
	rt.HandleFunc("GET /api/v1/businesses", h.search)
	rt.HandleFunc("GET /api/v1/businesses/{slug}", h.publicDetail)
}

// publicPolicyDTO are the booking rules a customer needs to know.
type publicPolicyDTO struct {
	MinLeadMinutes          int `json:"minLeadMinutes"`
	MaxAdvanceDays          int `json:"maxAdvanceDays"`
	CancellationCutoffHours int `json:"cancellationCutoffHours"`
	RescheduleCutoffHours   int `json:"rescheduleCutoffHours"`
	MaxCustomerReschedules  int `json:"maxCustomerReschedules"`
}

// publicBusinessDTO is what customers see: no status, internal settings or timestamps.
type publicBusinessDTO struct {
	ID            uuid.UUID       `json:"id"`
	Name          string          `json:"name"`
	Slug          string          `json:"slug"`
	Description   string          `json:"description"`
	ContactPhone  string          `json:"contactPhone"`
	Location      locationDTO     `json:"location"`
	TimeZone      string          `json:"timeZone"`
	Currency      string          `json:"currency"`
	BookingPolicy publicPolicyDTO `json:"bookingPolicy"`
	LogoURL       *string         `json:"logoUrl"`
	CoverURL      *string         `json:"coverUrl"`
	// Rating of the business from its customers' reviews (2026-11-28).
	Rating ratingDTO `json:"rating"`
}

// ratingDTO is the average of the visible reviews (one decimal, null without reviews) and how
// many there are.
type ratingDTO struct {
	Average *float64 `json:"average"`
	Count   int      `json:"count"`
}

// WithRatings makes the public views show the businesses' ratings (the scheduling module's
// reviews). Call once, before serving.
func (h *Handler) WithRatings(r scheduling.Ratings) *Handler {
	h.ratings = r
	return h
}

// ratingsOf reads the ratings of some businesses; without a source they all have none.
func (h *Handler) ratingsOf(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]scheduling.Rating, error) {
	if h.ratings == nil || len(ids) == 0 {
		return map[uuid.UUID]scheduling.Rating{}, nil
	}
	return h.ratings.BusinessRatings(ctx, ids)
}

type publicListDTO struct {
	Items      []publicBusinessDTO `json:"items"`
	NextCursor *string             `json:"nextCursor,omitempty"`
}

func toPublicDTO(b *domain.Business, rating scheduling.Rating) publicBusinessDTO {
	full, p := toDTO(b), b.Policy()
	return publicBusinessDTO{
		ID: full.ID, Name: full.Name, Slug: full.Slug, Description: full.Description, ContactPhone: full.ContactPhone,
		Location: full.Location, TimeZone: full.TimeZone, Currency: full.Currency,
		BookingPolicy: publicPolicyDTO{
			MinLeadMinutes: p.MinLeadMinutes, MaxAdvanceDays: p.MaxAdvanceDays,
			CancellationCutoffHours: p.CancellationCutoffHours, RescheduleCutoffHours: p.RescheduleCutoffHours,
			MaxCustomerReschedules: p.MaxCustomerReschedules,
		},
		LogoURL: full.LogoURL, CoverURL: full.CoverURL, Rating: ratingDTO{Average: rating.Average, Count: rating.Count},
	}
}

func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	in := application.SearchQuery{Text: q.Get("q"), Cursor: q.Get("cursor")}
	if v := q.Get("cityId"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			h.fail(w, r, &application.ValidationError{Fields: []application.FieldError{{Field: "cityId", Code: "INVALID_ID"}}})
			return
		}
		in.CityID = id
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			h.fail(w, r, &application.ValidationError{Fields: []application.FieldError{{Field: "limit", Code: application.CodeInvalidLimit}}})
			return
		}
		in.Limit = n
	}
	items, next, err := h.svc.SearchPublic(r.Context(), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	ids := make([]uuid.UUID, 0, len(items))
	for _, b := range items {
		ids = append(ids, b.ID())
	}
	ratings, err := h.ratingsOf(r.Context(), ids)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := publicListDTO{Items: make([]publicBusinessDTO, 0, len(items))}
	for _, b := range items {
		out.Items = append(out.Items, toPublicDTO(b, ratings[b.ID()]))
	}
	if next != "" {
		out.NextCursor = &next
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) publicDetail(w http.ResponseWriter, r *http.Request) {
	b, err := h.svc.GetPublic(r.Context(), r.PathValue("slug"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	ratings, err := h.ratingsOf(r.Context(), []uuid.UUID{b.ID()})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toPublicDTO(b, ratings[b.ID()]))
}
