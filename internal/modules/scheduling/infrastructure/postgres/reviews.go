package postgres

// Reviews of appointments (2026-11-28).

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/infrastructure/sqlcgen"
)

func toReview(r sqlcgen.AppointmentReview) domain.Review {
	out := domain.Review{
		ID: r.ID, BusinessID: r.BusinessID, AppointmentID: r.AppointmentID, ProfessionalID: r.ProfessionalID,
		CustomerUserID: r.CustomerUserID, AuthorName: r.AuthorName, ServiceName: r.ServiceName,
		BusinessRating: int(r.BusinessRating), ProfessionalRating: int(r.ProfessionalRating), Comment: r.Comment,
		Reply: r.Reply, HiddenReason: r.HiddenReason, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
	if r.RepliedAt != nil {
		out.RepliedAt = *r.RepliedAt
	}
	if r.RepliedBy != nil {
		out.RepliedBy = *r.RepliedBy
	}
	if r.HiddenAt != nil {
		out.HiddenAt = *r.HiddenAt
	}
	if r.HiddenBy != nil {
		out.HiddenBy = *r.HiddenBy
	}
	return out
}

func oneReview(r sqlcgen.AppointmentReview, err error) (domain.Review, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Review{}, application.ErrReviewNotFound
	}
	if err != nil {
		return domain.Review{}, err
	}
	return toReview(r), nil
}

func toReviews(rows []sqlcgen.AppointmentReview, err error) ([]domain.Review, error) {
	if err != nil {
		return nil, err
	}
	out := make([]domain.Review, 0, len(rows))
	for _, r := range rows {
		out = append(out, toReview(r))
	}
	return out, nil
}

func timeOrNil(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func idOrNil(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}

func (t *txStore) InsertReview(ctx context.Context, r domain.Review) error {
	return t.q.InsertReview(ctx, sqlcgen.InsertReviewParams{
		ID: r.ID, BusinessID: r.BusinessID, AppointmentID: r.AppointmentID, ProfessionalID: r.ProfessionalID,
		CustomerUserID: r.CustomerUserID, AuthorName: r.AuthorName, ServiceName: r.ServiceName,
		BusinessRating: int16(r.BusinessRating), ProfessionalRating: int16(r.ProfessionalRating), //nolint:gosec // 1-5, validated
		Comment: r.Comment, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	})
}

func (t *txStore) ReviewOfAppointment(ctx context.Context, businessID, appointmentID uuid.UUID) (domain.Review, error) {
	return oneReview(t.q.ReviewOfAppointment(ctx, sqlcgen.ReviewOfAppointmentParams{BusinessID: businessID, AppointmentID: appointmentID}))
}

func (t *txStore) ReviewByID(ctx context.Context, businessID, id uuid.UUID) (domain.Review, error) {
	return oneReview(t.q.ReviewByID(ctx, sqlcgen.ReviewByIDParams{BusinessID: businessID, ID: id}))
}

func (t *txStore) AnyReviewByID(ctx context.Context, id uuid.UUID) (domain.Review, error) {
	return oneReview(t.q.AnyReviewByID(ctx, id))
}

func (t *txStore) LockReview(ctx context.Context, businessID, id uuid.UUID) (domain.Review, error) {
	return oneReview(t.q.LockReview(ctx, sqlcgen.LockReviewParams{BusinessID: businessID, ID: id}))
}

func (t *txStore) UpdateReview(ctx context.Context, r domain.Review) error {
	return t.q.UpdateReview(ctx, sqlcgen.UpdateReviewParams{
		BusinessRating: int16(r.BusinessRating), ProfessionalRating: int16(r.ProfessionalRating), //nolint:gosec // 1-5, validated
		Comment: r.Comment, UpdatedAt: r.UpdatedAt, BusinessID: r.BusinessID, ID: r.ID,
	})
}

func (t *txStore) SetReviewReply(ctx context.Context, r domain.Review) error {
	return t.q.SetReviewReply(ctx, sqlcgen.SetReviewReplyParams{
		Reply: r.Reply, RepliedAt: timeOrNil(r.RepliedAt), RepliedBy: idOrNil(r.RepliedBy), UpdatedAt: r.UpdatedAt,
		BusinessID: r.BusinessID, ID: r.ID,
	})
}

func (t *txStore) SetReviewHidden(ctx context.Context, r domain.Review) error {
	return t.q.SetReviewHidden(ctx, sqlcgen.SetReviewHiddenParams{
		HiddenAt: timeOrNil(r.HiddenAt), HiddenBy: idOrNil(r.HiddenBy), HiddenReason: r.HiddenReason, UpdatedAt: r.UpdatedAt,
		BusinessID: r.BusinessID, ID: r.ID,
	})
}

func (t *txStore) DeleteReview(ctx context.Context, businessID, id uuid.UUID) error {
	return t.q.DeleteReview(ctx, sqlcgen.DeleteReviewParams{BusinessID: businessID, ID: id})
}

func (t *txStore) BusinessReviews(ctx context.Context, businessID uuid.UUID, professionalIDs []uuid.UUID, withHidden bool, after *application.Cursor, limit int) ([]domain.Review, error) {
	has, at, id := cursorArgs(after)
	return toReviews(t.q.BusinessReviews(ctx, sqlcgen.BusinessReviewsParams{
		BusinessID: businessID, AllProfessionals: professionalIDs == nil, ProfessionalIds: professionalIDs, WithHidden: withHidden,
		HasCursor: has, AfterAt: at, AfterID: id, MaxRows: int32(limit), //nolint:gosec // bounded by the application
	}))
}

func (t *txStore) ReviewsOfAppointments(ctx context.Context, userID uuid.UUID, appointmentIDs []uuid.UUID) ([]domain.Review, error) {
	return toReviews(t.q.ReviewsOfAppointments(ctx, sqlcgen.ReviewsOfAppointmentsParams{CustomerUserID: userID, AppointmentIds: appointmentIDs}))
}

func (t *txStore) BusinessRatings(ctx context.Context, businessIDs []uuid.UUID) (map[uuid.UUID]domain.RatingSummary, error) {
	rows, err := t.q.BusinessRatings(ctx, businessIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]domain.RatingSummary, len(rows))
	for _, r := range rows {
		out[r.BusinessID] = domain.RatingSummary{Count: int(r.Reviews), Sum: int(r.Stars)}
	}
	return out, nil
}

func (t *txStore) ProfessionalRatings(ctx context.Context, businessID uuid.UUID) (map[uuid.UUID]domain.RatingSummary, error) {
	rows, err := t.q.ProfessionalRatings(ctx, businessID)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]domain.RatingSummary, len(rows))
	for _, r := range rows {
		out[r.ProfessionalID] = domain.RatingSummary{Count: int(r.Reviews), Sum: int(r.Stars)}
	}
	return out, nil
}
