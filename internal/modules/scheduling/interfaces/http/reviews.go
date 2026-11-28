package httpapi

// Reviews of appointments (2026-11-28, decisions of the user).

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	contract "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/httpx"
)

// Problem codes of reviews.
const (
	CodeNotReviewable      = "NOT_REVIEWABLE"
	CodeReviewWindowClosed = "REVIEW_WINDOW_CLOSED"
)

// ReviewService is the part of the application the review endpoints use.
type ReviewService interface {
	SubmitReview(ctx context.Context, p contract.Principal, appointmentID uuid.UUID, in application.ReviewInput) (domain.Review, bool, error)
	MyReview(ctx context.Context, p contract.Principal, appointmentID uuid.UUID) (domain.Review, error)
	DeleteMyReview(ctx context.Context, p contract.Principal, appointmentID uuid.UUID) error
	PublicReviews(ctx context.Context, slug string, q application.ReviewsQuery) (application.ReviewPage, error)
	BusinessReviews(ctx context.Context, p contract.Principal, businessID uuid.UUID, q application.ReviewsQuery) (application.ReviewPage, error)
	ReplyToReview(ctx context.Context, p contract.Principal, businessID, reviewID uuid.UUID, reply string) (domain.Review, error)
	RemoveReviewReply(ctx context.Context, p contract.Principal, businessID, reviewID uuid.UUID) (domain.Review, error)
	SetReviewHidden(ctx context.Context, p contract.Principal, reviewID uuid.UUID, hidden bool, reason string) (domain.Review, error)
}

func (h *Handler) reviewRoutes(rt *httpx.Router) {
	auth := func(f http.HandlerFunc) http.Handler { return h.requireAuth(f) }
	rt.HandleFunc("GET /api/v1/businesses/{slug}/reviews", h.publicReviews)
	rt.Handle("GET /api/v1/me/appointments/{appointmentId}/review", auth(h.myReview))
	rt.Handle("PUT /api/v1/me/appointments/{appointmentId}/review", auth(h.submitReview))
	rt.Handle("DELETE /api/v1/me/appointments/{appointmentId}/review", auth(h.deleteMyReview))
	rt.Handle("GET /api/v1/business/{businessId}/reviews", auth(h.businessReviews))
	rt.Handle("PUT /api/v1/business/{businessId}/reviews/{reviewId}/reply", auth(h.replyToReview))
	rt.Handle("DELETE /api/v1/business/{businessId}/reviews/{reviewId}/reply", auth(h.removeReply))
	rt.Handle("POST /api/v1/admin/reviews/{reviewId}/hide", auth(h.hideReview))
	rt.Handle("POST /api/v1/admin/reviews/{reviewId}/unhide", auth(h.unhideReview))
}

type ratingDTO struct {
	Average *float64 `json:"average"`
	Count   int      `json:"count"`
}

func toRatingDTO(s domain.RatingSummary) ratingDTO {
	return ratingDTO{Average: s.Average(), Count: s.Count}
}

type replyDTO struct {
	Text      string    `json:"text"`
	RepliedAt time.Time `json:"repliedAt"`
}

func toReplyDTO(r domain.Review) *replyDTO {
	if r.Reply == "" {
		return nil
	}
	return &replyDTO{Text: r.Reply, RepliedAt: r.RepliedAt}
}

// reviewDTO is a review as the public sees it; the staff also get the hidden ones, marked.
type reviewDTO struct {
	ID                 uuid.UUID `json:"id"`
	AuthorName         string    `json:"authorName"`
	ServiceName        string    `json:"serviceName"`
	Professional       refDTO    `json:"professional"`
	BusinessRating     int       `json:"businessRating"`
	ProfessionalRating int       `json:"professionalRating"`
	Comment            *string   `json:"comment"`
	Reply              *replyDTO `json:"reply"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
	// Staff only.
	AppointmentID *uuid.UUID `json:"appointmentId,omitempty"`
	Hidden        *bool      `json:"hidden,omitempty"`
	HiddenReason  *string    `json:"hiddenReason,omitempty"`
}

type reviewPageDTO struct {
	Rating     ratingDTO   `json:"rating"`
	Items      []reviewDTO `json:"items"`
	NextCursor *string     `json:"nextCursor"`
}

func toReviewDTO(r domain.Review, names map[uuid.UUID]string, staff bool) reviewDTO {
	out := reviewDTO{
		ID: r.ID, AuthorName: r.AuthorName, ServiceName: r.ServiceName,
		Professional: refDTO{ID: r.ProfessionalID, Name: names[r.ProfessionalID]}, BusinessRating: r.BusinessRating,
		ProfessionalRating: r.ProfessionalRating, Comment: optional(r.Comment), Reply: toReplyDTO(r),
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
	if staff {
		id, hidden := r.AppointmentID, r.Hidden()
		out.AppointmentID, out.Hidden, out.HiddenReason = &id, &hidden, optional(r.HiddenReason)
	}
	return out
}

func toReviewPageDTO(p application.ReviewPage, staff bool) reviewPageDTO {
	out := reviewPageDTO{Rating: toRatingDTO(p.Rating), Items: make([]reviewDTO, 0, len(p.Items)), NextCursor: optionalCursor(p.NextCursor)}
	for _, r := range p.Items {
		out.Items = append(out.Items, toReviewDTO(r, p.Names, staff))
	}
	return out
}

// myReviewDTO is the customer's own review.
type myReviewDTO struct {
	ID                 uuid.UUID `json:"id"`
	AppointmentID      uuid.UUID `json:"appointmentId"`
	BusinessRating     int       `json:"businessRating"`
	ProfessionalRating int       `json:"professionalRating"`
	Comment            *string   `json:"comment"`
	Reply              *replyDTO `json:"reply"`
	Hidden             bool      `json:"hidden"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

func toMyReviewDTO(r domain.Review) myReviewDTO {
	return myReviewDTO{
		ID: r.ID, AppointmentID: r.AppointmentID, BusinessRating: r.BusinessRating, ProfessionalRating: r.ProfessionalRating,
		Comment: optional(r.Comment), Reply: toReplyDTO(r), Hidden: r.Hidden(), CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

// reviewsQuery reads ?professionalId=&cursor=&limit= (validated by the application, after
// authorizing).
func reviewsQuery(r *http.Request) application.ReviewsQuery {
	q := r.URL.Query()
	return application.ReviewsQuery{ProfessionalID: q.Get("professionalId"), Cursor: q.Get("cursor"), Limit: q.Get("limit")}
}

// pathID parses one id of the path (an invalid one is 404).
func (h *Handler) pathID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	ids, ok := h.pathIDs(w, r, name)
	if !ok {
		return uuid.Nil, false
	}
	return ids[0], true
}

// publicReviews serves GET /api/v1/businesses/{slug}/reviews (no authentication).
func (h *Handler) publicReviews(w http.ResponseWriter, r *http.Request) {
	page, err := h.reviews.PublicReviews(r.Context(), r.PathValue("slug"), reviewsQuery(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toReviewPageDTO(page, false))
}

func (h *Handler) businessReviews(w http.ResponseWriter, r *http.Request) {
	businessID, ok := h.pathID(w, r, "businessId")
	if !ok {
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	page, err := h.reviews.BusinessReviews(r.Context(), p, businessID, reviewsQuery(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toReviewPageDTO(page, true))
}

func (h *Handler) myReview(w http.ResponseWriter, r *http.Request) {
	id, ok := h.pathID(w, r, "appointmentId")
	if !ok {
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	rev, err := h.reviews.MyReview(r.Context(), p, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toMyReviewDTO(rev))
}

type reviewRequest struct {
	BusinessRating     int    `json:"businessRating"`
	ProfessionalRating int    `json:"professionalRating"`
	Comment            string `json:"comment"`
}

func (h *Handler) submitReview(w http.ResponseWriter, r *http.Request) {
	id, ok := h.pathID(w, r, "appointmentId")
	if !ok {
		return
	}
	var req reviewRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	rev, created, err := h.reviews.SubmitReview(r.Context(), p, id, application.ReviewInput{
		BusinessRating: req.BusinessRating, ProfessionalRating: req.ProfessionalRating, Comment: req.Comment,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	httpx.WriteJSON(w, status, toMyReviewDTO(rev))
}

func (h *Handler) deleteMyReview(w http.ResponseWriter, r *http.Request) {
	id, ok := h.pathID(w, r, "appointmentId")
	if !ok {
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	if err := h.reviews.DeleteMyReview(r.Context(), p, id); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// reply writes (text != nil) or removes the reply of a review.
func (h *Handler) reply(w http.ResponseWriter, r *http.Request, text *string) {
	businessID, ok := h.pathID(w, r, "businessId")
	if !ok {
		return
	}
	reviewID, ok := h.pathID(w, r, "reviewId")
	if !ok {
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	var rev domain.Review
	var err error
	if text != nil {
		rev, err = h.reviews.ReplyToReview(r.Context(), p, businessID, reviewID, *text)
	} else {
		rev, err = h.reviews.RemoveReviewReply(r.Context(), p, businessID, reviewID)
	}
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toReviewDTO(rev, nil, true))
}

type replyRequest struct {
	Reply string `json:"reply"`
}

// replyToReview serves PUT …/reviews/{reviewId}/reply: the OWNER's public reply.
func (h *Handler) replyToReview(w http.ResponseWriter, r *http.Request) {
	var req replyRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	h.reply(w, r, &req.Reply)
}

func (h *Handler) removeReply(w http.ResponseWriter, r *http.Request) { h.reply(w, r, nil) }

type hideRequest struct {
	Reason string `json:"reason"`
}

func (h *Handler) setHidden(w http.ResponseWriter, r *http.Request, hidden bool, reason string) {
	id, ok := h.pathID(w, r, "reviewId")
	if !ok {
		return
	}
	p, _ := contract.PrincipalFrom(r.Context())
	rev, err := h.reviews.SetReviewHidden(r.Context(), p, id, hidden, reason)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toReviewDTO(rev, nil, true))
}

// hideReview serves POST /api/v1/admin/reviews/{reviewId}/hide (platform administrator).
func (h *Handler) hideReview(w http.ResponseWriter, r *http.Request) {
	var req hideRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	h.setHidden(w, r, true, req.Reason)
}

func (h *Handler) unhideReview(w http.ResponseWriter, r *http.Request) { h.setHidden(w, r, false, "") }
