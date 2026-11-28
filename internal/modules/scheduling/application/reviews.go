package application

// Reviews of appointments (2026-11-28, decisions of the user): the customer of a COMPLETED
// appointment rates the business and the professional with an optional comment, once per
// appointment and up to 30 days after it (it may change it meanwhile and delete it any time);
// the OWNER replies in public and cannot hide it; a PROFESSIONAL reads its own; only a platform
// administrator hides one. The public sees the visible ones and the averages.

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"

	businesses "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/contract"
	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/authz"
)

// Page sizes of the reviews.
const (
	DefaultReviewPage = 20
	MaxReviewPage     = 50
)

// Field error codes of reviews.
const (
	CodeInvalidRating = "INVALID_RATING"
	CodeTooLong       = "TOO_LONG"
)

// ErrReviewNotFound means there is no such review for the caller.
var ErrReviewNotFound = errors.New("scheduling: review not found")

// PlatformAdmins is the part of identity's authorizer that hides reviews.
type PlatformAdmins interface {
	RequirePlatformAdmin(ctx context.Context, p identity.Principal) error
}

// ReviewTx is the persistence of reviews inside a transaction. The reads return
// ErrReviewNotFound when there is none.
type ReviewTx interface {
	InsertReview(ctx context.Context, r domain.Review) error
	ReviewOfAppointment(ctx context.Context, businessID, appointmentID uuid.UUID) (domain.Review, error)
	ReviewByID(ctx context.Context, businessID, id uuid.UUID) (domain.Review, error)
	// AnyReviewByID reads a review of any business (platform administrator).
	AnyReviewByID(ctx context.Context, id uuid.UUID) (domain.Review, error)
	LockReview(ctx context.Context, businessID, id uuid.UUID) (domain.Review, error)
	// UpdateReview writes the ratings and the comment; SetReviewReply the reply;
	// SetReviewHidden the hiding.
	UpdateReview(ctx context.Context, r domain.Review) error
	SetReviewReply(ctx context.Context, r domain.Review) error
	SetReviewHidden(ctx context.Context, r domain.Review) error
	DeleteReview(ctx context.Context, businessID, id uuid.UUID) error
	// BusinessReviews returns reviews newest first; professionalIDs nil = every professional.
	BusinessReviews(ctx context.Context, businessID uuid.UUID, professionalIDs []uuid.UUID, withHidden bool, after *Cursor, limit int) ([]domain.Review, error)
	ReviewsOfAppointments(ctx context.Context, userID uuid.UUID, appointmentIDs []uuid.UUID) ([]domain.Review, error)
	// BusinessRatings and ProfessionalRatings count the visible reviews.
	BusinessRatings(ctx context.Context, businessIDs []uuid.UUID) (map[uuid.UUID]domain.RatingSummary, error)
	ProfessionalRatings(ctx context.Context, businessID uuid.UUID) (map[uuid.UUID]domain.RatingSummary, error)
}

// ReviewInput is what the customer writes.
type ReviewInput struct {
	BusinessRating, ProfessionalRating int
	Comment                            string
}

func (in ReviewInput) parse() (ReviewInput, error) {
	verr := &ValidationError{}
	if !domain.ValidRating(in.BusinessRating) {
		verr.add("businessRating", CodeInvalidRating)
	}
	if !domain.ValidRating(in.ProfessionalRating) {
		verr.add("professionalRating", CodeInvalidRating)
	}
	comment, ok := domain.CleanText(in.Comment, domain.MaxReviewComment)
	if !ok {
		verr.add("comment", CodeTooLong)
	}
	in.Comment = comment
	return in, verr.orNil()
}

// ownAppointment locks the caller's appointment; ErrAppointmentNotFound for anyone else's.
func ownAppointment(ctx context.Context, tx Tx, p identity.Principal, id uuid.UUID) (domain.Appointment, error) {
	a, err := tx.LockAppointment(ctx, id)
	if err == nil && (p.UserID == uuid.Nil || a.Customer.UserID != p.UserID) {
		err = ErrAppointmentNotFound
	}
	return a, err
}

// SubmitReview writes or changes the caller's review of its appointment; created is true for a
// new one. ErrAppointmentNotFound (not the caller's), *ValidationError, domain.ErrNotReviewable
// (not COMPLETED) and domain.ErrReviewWindowClosed. The appointment's lock serializes two
// submissions of the same review.
func (s *Service) SubmitReview(ctx context.Context, p identity.Principal, appointmentID uuid.UUID, in ReviewInput) (domain.Review, bool, error) {
	var out domain.Review
	created := false
	err := s.d.Store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		a, err := ownAppointment(ctx, tx, p, appointmentID)
		if err != nil {
			return err
		}
		if in, err = in.parse(); err != nil {
			return err
		}
		now := s.d.Clock.Now()
		if err := domain.CanReview(a, p.UserID, now); err != nil {
			return err
		}
		r, err := tx.ReviewOfAppointment(ctx, a.BusinessID, a.ID)
		switch {
		case errors.Is(err, ErrReviewNotFound):
			created = true
			out = domain.Review{
				ID: uuid.Must(uuid.NewV7()), BusinessID: a.BusinessID, AppointmentID: a.ID, ProfessionalID: a.ProfessionalID,
				CustomerUserID: p.UserID, AuthorName: domain.AuthorName(a.Customer.Name), ServiceName: a.Service.Name,
				BusinessRating: in.BusinessRating, ProfessionalRating: in.ProfessionalRating, Comment: in.Comment,
				CreatedAt: now, UpdatedAt: now,
			}
			return tx.InsertReview(ctx, out)
		case err != nil:
			return err
		}
		r.BusinessRating, r.ProfessionalRating, r.Comment, r.UpdatedAt = in.BusinessRating, in.ProfessionalRating, in.Comment, now
		out = r
		return tx.UpdateReview(ctx, r)
	})
	return out, created, err
}

// MyReview returns the caller's review of its appointment; ErrAppointmentNotFound for anyone
// else's appointment, ErrReviewNotFound without a review.
func (s *Service) MyReview(ctx context.Context, p identity.Principal, appointmentID uuid.UUID) (domain.Review, error) {
	var out domain.Review
	err := s.d.Store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		a, err := tx.AppointmentByID(ctx, appointmentID)
		if err == nil && (p.UserID == uuid.Nil || a.Customer.UserID != p.UserID) {
			err = ErrAppointmentNotFound
		}
		if err != nil {
			return err
		}
		out, err = tx.ReviewOfAppointment(ctx, a.BusinessID, a.ID)
		return err
	})
	return out, err
}

// DeleteMyReview deletes the caller's review of its appointment (any time).
func (s *Service) DeleteMyReview(ctx context.Context, p identity.Principal, appointmentID uuid.UUID) error {
	return s.d.Store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		a, err := ownAppointment(ctx, tx, p, appointmentID)
		if err != nil {
			return err
		}
		r, err := tx.ReviewOfAppointment(ctx, a.BusinessID, a.ID)
		if err != nil {
			return err
		}
		return tx.DeleteReview(ctx, r.BusinessID, r.ID)
	})
}

// ReviewsQuery selects a page of reviews, as the query string has it (parsed after authorizing).
type ReviewsQuery struct {
	ProfessionalID string // "" = every professional
	Cursor         string
	Limit          string
}

type reviewsQuery struct {
	professionalID uuid.UUID
	after          *Cursor
	limit          int
}

func (q ReviewsQuery) parse() (reviewsQuery, error) {
	verr := &ValidationError{}
	out := reviewsQuery{limit: DefaultReviewPage}
	if q.ProfessionalID != "" {
		id, err := uuid.Parse(q.ProfessionalID)
		if err != nil {
			verr.add("professionalId", "INVALID_ID")
		}
		out.professionalID = id
	}
	if q.Limit != "" {
		n, err := strconv.Atoi(q.Limit)
		if err != nil || n < 1 || n > MaxReviewPage {
			verr.add("limit", "OUT_OF_RANGE")
		}
		out.limit = n
	}
	after, ok := DecodeCursor(q.Cursor)
	if !ok {
		verr.add("cursor", "INVALID_CURSOR")
	}
	out.after = after
	return out, verr.orNil()
}

// ReviewPage is a page of reviews with the rating of what was asked: the business's, or the
// professional's when the page is of one professional.
type ReviewPage struct {
	Rating     domain.RatingSummary
	Items      []domain.Review
	NextCursor string
	// Names are the display names of the professionals of the business.
	Names map[uuid.UUID]string
}

// PublicReviews returns the visible reviews of a public business (no authentication).
// ErrNotFound when customers cannot see the business.
func (s *Service) PublicReviews(ctx context.Context, slug string, q ReviewsQuery) (ReviewPage, error) {
	in, err := q.parse()
	if err != nil {
		return ReviewPage{}, err
	}
	pol, err := s.d.Businesses.PublicBookingPolicy(ctx, slug)
	if errors.Is(err, businesses.ErrBusinessNotFound) {
		return ReviewPage{}, ErrNotFound
	}
	if err != nil {
		return ReviewPage{}, err
	}
	var filter []uuid.UUID
	if in.professionalID != uuid.Nil {
		filter = []uuid.UUID{in.professionalID}
	}
	return s.reviewPage(ctx, pol.BusinessID, filter, false, in.after, in.limit)
}

// BusinessReviews returns the reviews of the business to its staff, hidden ones included (marked):
// the OWNER every professional's (or one's), a PROFESSIONAL only its own (another professional →
// authz.ErrForbidden).
func (s *Service) BusinessReviews(ctx context.Context, p identity.Principal, businessID uuid.UUID, q ReviewsQuery) (ReviewPage, error) {
	role, err := s.d.Authz.RequireBusinessRole(ctx, p, businessID, identity.RolesStaff...)
	if err != nil {
		return ReviewPage{}, err
	}
	in, err := q.parse()
	if err != nil {
		return ReviewPage{}, err
	}
	var filter []uuid.UUID
	switch {
	case role == identity.RoleOwner && in.professionalID != uuid.Nil:
		filter = []uuid.UUID{in.professionalID}
	case role == identity.RoleOwner:
	default:
		own, err := s.ownProfiles(ctx, businessID, p.UserID)
		if err != nil {
			return ReviewPage{}, err
		}
		if in.professionalID != uuid.Nil {
			if !contains(own, in.professionalID) {
				return ReviewPage{}, authz.ErrForbidden
			}
			own = []uuid.UUID{in.professionalID}
		}
		filter = append([]uuid.UUID{}, own...) // none → no reviews
	}
	return s.reviewPage(ctx, businessID, filter, true, in.after, in.limit)
}

func (s *Service) reviewPage(ctx context.Context, businessID uuid.UUID, filter []uuid.UUID, withHidden bool, after *Cursor, limit int) (ReviewPage, error) {
	var out ReviewPage
	err := s.d.Store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		var err error
		if out.Items, err = tx.BusinessReviews(ctx, businessID, filter, withHidden, after, limit+1); err != nil {
			return err
		}
		if len(filter) == 1 {
			pros, err := tx.ProfessionalRatings(ctx, businessID)
			out.Rating = pros[filter[0]]
			return err
		}
		biz, err := tx.BusinessRatings(ctx, []uuid.UUID{businessID})
		out.Rating = biz[businessID]
		return err
	})
	if err != nil {
		return ReviewPage{}, err
	}
	roster, err := s.d.Professionals.Roster(ctx, businessID)
	if err != nil {
		return ReviewPage{}, err
	}
	out.Names = make(map[uuid.UUID]string, len(roster))
	for _, r := range roster {
		out.Names[r.ID] = r.DisplayName
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		last := out.Items[limit-1]
		out.NextCursor = Cursor{Start: last.CreatedAt, ID: last.ID}.Encode()
	}
	return out, nil
}

// ReplyToReview writes the business's public reply (OWNER), replacing the previous one.
func (s *Service) ReplyToReview(ctx context.Context, p identity.Principal, businessID, reviewID uuid.UUID, reply string) (domain.Review, error) {
	if _, err := s.d.Authz.RequireBusinessRole(ctx, p, businessID, identity.RolesOwner...); err != nil {
		return domain.Review{}, err
	}
	text, ok := domain.CleanText(reply, domain.MaxReviewReply)
	switch {
	case !ok:
		return domain.Review{}, &ValidationError{Fields: []FieldError{{"reply", CodeTooLong}}}
	case text == "":
		return domain.Review{}, &ValidationError{Fields: []FieldError{{"reply", CodeRequired}}}
	}
	return s.setReply(ctx, p, businessID, reviewID, text)
}

// RemoveReviewReply removes the business's reply (OWNER); removing none is not an error.
func (s *Service) RemoveReviewReply(ctx context.Context, p identity.Principal, businessID, reviewID uuid.UUID) (domain.Review, error) {
	if _, err := s.d.Authz.RequireBusinessRole(ctx, p, businessID, identity.RolesOwner...); err != nil {
		return domain.Review{}, err
	}
	return s.setReply(ctx, p, businessID, reviewID, "")
}

func (s *Service) setReply(ctx context.Context, p identity.Principal, businessID, reviewID uuid.UUID, text string) (domain.Review, error) {
	var out domain.Review
	err := s.d.Store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		r, err := tx.LockReview(ctx, businessID, reviewID)
		if err != nil {
			return err
		}
		now := s.d.Clock.Now()
		r.Reply, r.RepliedAt, r.RepliedBy, r.UpdatedAt = text, now, p.UserID, now
		if text == "" {
			r.RepliedAt, r.RepliedBy = time.Time{}, uuid.Nil
		}
		out = r
		return tx.SetReviewReply(ctx, r)
	})
	return out, err
}

// SetReviewHidden hides (hidden true, with a reason) or shows again a review of any business
// (platform administrator). The review keeps who hid it and when.
func (s *Service) SetReviewHidden(ctx context.Context, p identity.Principal, reviewID uuid.UUID, hidden bool, reason string) (domain.Review, error) {
	if s.d.Admins == nil {
		return domain.Review{}, authz.ErrForbidden
	}
	if err := s.d.Admins.RequirePlatformAdmin(ctx, p); err != nil {
		return domain.Review{}, err
	}
	text, ok := domain.CleanText(reason, domain.MaxHideReason)
	switch {
	case !ok:
		return domain.Review{}, &ValidationError{Fields: []FieldError{{"reason", CodeTooLong}}}
	case hidden && text == "":
		return domain.Review{}, &ValidationError{Fields: []FieldError{{"reason", CodeRequired}}}
	}
	var out domain.Review
	err := s.d.Store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		found, err := tx.AnyReviewByID(ctx, reviewID)
		if err != nil {
			return err
		}
		r, err := tx.LockReview(ctx, found.BusinessID, found.ID)
		if err != nil {
			return err
		}
		now := s.d.Clock.Now()
		if hidden {
			r.HiddenAt, r.HiddenBy, r.HiddenReason = now, p.UserID, text
		} else {
			r.HiddenAt, r.HiddenBy, r.HiddenReason = time.Time{}, uuid.Nil, ""
		}
		r.UpdatedAt = now
		out = r
		return tx.SetReviewHidden(ctx, r)
	})
	return out, err
}

// BusinessRatings implements contract.Ratings.
func (s *Service) BusinessRatings(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]contract.Rating, error) {
	var sums map[uuid.UUID]domain.RatingSummary
	if len(ids) == 0 {
		return map[uuid.UUID]contract.Rating{}, nil
	}
	err := s.d.Store.WithinTx(ctx, func(ctx context.Context, tx Tx) (err error) {
		sums, err = tx.BusinessRatings(ctx, ids)
		return err
	})
	return toRatings(sums), err
}

// ProfessionalRatings implements contract.Ratings.
func (s *Service) ProfessionalRatings(ctx context.Context, businessID uuid.UUID) (map[uuid.UUID]contract.Rating, error) {
	var sums map[uuid.UUID]domain.RatingSummary
	err := s.d.Store.WithinTx(ctx, func(ctx context.Context, tx Tx) (err error) {
		sums, err = tx.ProfessionalRatings(ctx, businessID)
		return err
	})
	return toRatings(sums), err
}

func toRatings(sums map[uuid.UUID]domain.RatingSummary) map[uuid.UUID]contract.Rating {
	out := make(map[uuid.UUID]contract.Rating, len(sums))
	for id, s := range sums {
		out[id] = contract.Rating{Average: s.Average(), Count: s.Count}
	}
	return out
}

// reviewsOf returns the caller's reviews of some of its appointments, by appointment.
func (s *Service) reviewsOf(ctx context.Context, userID uuid.UUID, rows []domain.Appointment) (map[uuid.UUID]domain.Review, error) {
	var ids []uuid.UUID
	for _, a := range rows {
		if a.Status == domain.StatusCompleted {
			ids = append(ids, a.ID)
		}
	}
	out := map[uuid.UUID]domain.Review{}
	if len(ids) == 0 || userID == uuid.Nil {
		return out, nil
	}
	err := s.d.Store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		list, err := tx.ReviewsOfAppointments(ctx, userID, ids)
		for _, r := range list {
			out[r.AppointmentID] = r
		}
		return err
	})
	return out, err
}
