package schedtest

import (
	"context"
	"errors"
	"slices"
	"sort"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/domain"
)

// errDuplicateReview mimics the unique constraint of a review per appointment.
var errDuplicateReview = errors.New("schedtest: the appointment already has a review")

func (t *tx) InsertReview(_ context.Context, r domain.Review) error {
	for _, o := range t.reviews {
		if o.AppointmentID == r.AppointmentID {
			return errDuplicateReview
		}
	}
	t.reviews[r.ID] = r
	return nil
}

func (t *tx) ReviewOfAppointment(_ context.Context, businessID, appointmentID uuid.UUID) (domain.Review, error) {
	for _, r := range t.reviews {
		if r.BusinessID == businessID && r.AppointmentID == appointmentID {
			return r, nil
		}
	}
	return domain.Review{}, application.ErrReviewNotFound
}

func (t *tx) ReviewByID(_ context.Context, businessID, id uuid.UUID) (domain.Review, error) {
	r, ok := t.reviews[id]
	if !ok || r.BusinessID != businessID {
		return domain.Review{}, application.ErrReviewNotFound
	}
	return r, nil
}

func (t *tx) AnyReviewByID(_ context.Context, id uuid.UUID) (domain.Review, error) {
	r, ok := t.reviews[id]
	if !ok {
		return domain.Review{}, application.ErrReviewNotFound
	}
	return r, nil
}

func (t *tx) LockReview(ctx context.Context, businessID, id uuid.UUID) (domain.Review, error) {
	return t.ReviewByID(ctx, businessID, id)
}

func (t *tx) put(r domain.Review, apply func(cur *domain.Review)) error {
	cur, ok := t.reviews[r.ID]
	if !ok || cur.BusinessID != r.BusinessID {
		return nil // UPDATE ... WHERE: no row
	}
	apply(&cur)
	cur.UpdatedAt = r.UpdatedAt
	t.reviews[r.ID] = cur
	return nil
}

func (t *tx) UpdateReview(_ context.Context, r domain.Review) error {
	return t.put(r, func(c *domain.Review) {
		c.BusinessRating, c.ProfessionalRating, c.Comment = r.BusinessRating, r.ProfessionalRating, r.Comment
	})
}

func (t *tx) SetReviewReply(_ context.Context, r domain.Review) error {
	return t.put(r, func(c *domain.Review) { c.Reply, c.RepliedAt, c.RepliedBy = r.Reply, r.RepliedAt, r.RepliedBy })
}

func (t *tx) SetReviewHidden(_ context.Context, r domain.Review) error {
	return t.put(r, func(c *domain.Review) {
		c.HiddenAt, c.HiddenBy, c.HiddenReason = r.HiddenAt, r.HiddenBy, r.HiddenReason
	})
}

func (t *tx) DeleteReview(_ context.Context, businessID, id uuid.UUID) error {
	if r, ok := t.reviews[id]; ok && r.BusinessID == businessID {
		delete(t.reviews, id)
	}
	return nil
}

func (t *tx) BusinessReviews(_ context.Context, businessID uuid.UUID, professionalIDs []uuid.UUID, withHidden bool, c *application.Cursor, limit int) ([]domain.Review, error) {
	var out []domain.Review
	for _, r := range t.reviews {
		switch {
		case r.BusinessID != businessID,
			professionalIDs != nil && !slices.Contains(professionalIDs, r.ProfessionalID),
			!withHidden && r.Hidden():
			continue
		case c != nil && !before(r, c):
			continue
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID.String() > out[j].ID.String()
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (t *tx) ReviewsOfAppointments(_ context.Context, userID uuid.UUID, ids []uuid.UUID) ([]domain.Review, error) {
	var out []domain.Review
	for _, r := range t.reviews {
		if r.CustomerUserID == userID && slices.Contains(ids, r.AppointmentID) {
			out = append(out, r)
		}
	}
	return out, nil
}

func (t *tx) BusinessRatings(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]domain.RatingSummary, error) {
	out := map[uuid.UUID]domain.RatingSummary{}
	for _, r := range t.reviews {
		if slices.Contains(ids, r.BusinessID) && !r.Hidden() {
			s := out[r.BusinessID]
			s.Count, s.Sum = s.Count+1, s.Sum+r.BusinessRating
			out[r.BusinessID] = s
		}
	}
	return out, nil
}

func (t *tx) ProfessionalRatings(_ context.Context, businessID uuid.UUID) (map[uuid.UUID]domain.RatingSummary, error) {
	out := map[uuid.UUID]domain.RatingSummary{}
	for _, r := range t.reviews {
		if r.BusinessID == businessID && !r.Hidden() {
			s := out[r.ProfessionalID]
			s.Count, s.Sum = s.Count+1, s.Sum+r.ProfessionalRating
			out[r.ProfessionalID] = s
		}
	}
	return out, nil
}

// before tells whether r comes after the cursor c in the order newest first.
func before(r domain.Review, c *application.Cursor) bool {
	if r.CreatedAt.Equal(c.Start) {
		return r.ID.String() < c.ID.String()
	}
	return r.CreatedAt.Before(c.Start)
}
