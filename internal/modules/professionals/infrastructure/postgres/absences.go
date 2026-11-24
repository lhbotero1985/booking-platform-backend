package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/infrastructure/sqlcgen"
)

// overlapErr maps the exclusion constraint violation to application.ErrAbsenceOverlap.
func overlapErr(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23P01" && pgErr.ConstraintName == "professional_absences_no_overlap" {
		return application.ErrAbsenceOverlap
	}
	return err
}

func (t *txStore) InsertAbsence(ctx context.Context, a domain.Absence) error {
	return overlapErr(t.q.InsertAbsence(ctx, sqlcgen.InsertAbsenceParams{
		ID: a.ID, BusinessID: a.BusinessID, ProfessionalID: a.ProfessionalID, StartsAt: a.Start, EndsAt: a.End,
		Reason: a.Reason, CreatedBy: a.CreatedBy, CreatedAt: a.CreatedAt,
	}))
}

func (t *txStore) UpdateAbsence(ctx context.Context, a domain.Absence) error {
	return overlapErr(t.q.UpdateAbsence(ctx, sqlcgen.UpdateAbsenceParams{
		BusinessID: a.BusinessID, ProfessionalID: a.ProfessionalID, ID: a.ID, StartsAt: a.Start, EndsAt: a.End,
		Reason: a.Reason, UpdatedAt: a.UpdatedAt,
	}))
}

func (t *txStore) DeleteAbsence(ctx context.Context, a domain.Absence) error {
	return t.q.DeleteAbsence(ctx, sqlcgen.DeleteAbsenceParams{BusinessID: a.BusinessID, ProfessionalID: a.ProfessionalID, ID: a.ID})
}

func (t *txStore) AbsenceByID(ctx context.Context, businessID, professionalID, id uuid.UUID) (domain.Absence, error) {
	r, err := t.q.AbsenceByID(ctx, sqlcgen.AbsenceByIDParams{BusinessID: businessID, ProfessionalID: professionalID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Absence{}, application.ErrNotFound
	}
	if err != nil {
		return domain.Absence{}, err
	}
	return absenceToDomain(r), nil
}

func (t *txStore) ListAbsences(ctx context.Context, businessID, professionalID uuid.UUID, from, to time.Time, limit int) ([]domain.Absence, error) {
	rows, err := t.q.ListAbsences(ctx, sqlcgen.ListAbsencesParams{
		BusinessID: businessID, ProfessionalID: professionalID, FromTime: from, ToTime: to, MaxRows: int32(limit), //nolint:gosec // small constant
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.Absence, len(rows))
	for i, r := range rows {
		out[i] = absenceToDomain(r)
	}
	return out, nil
}

func absenceToDomain(r sqlcgen.ProfessionalAbsence) domain.Absence {
	return domain.Absence{
		ID: r.ID, BusinessID: r.BusinessID, ProfessionalID: r.ProfessionalID, Start: r.StartsAt.UTC(), End: r.EndsAt.UTC(),
		Reason: r.Reason, CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}
