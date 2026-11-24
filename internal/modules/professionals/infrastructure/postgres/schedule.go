package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/infrastructure/sqlcgen"
)

func (t *txStore) LockProfessional(ctx context.Context, businessID, id uuid.UUID) (domain.Professional, error) {
	r, err := t.q.LockProfessional(ctx, sqlcgen.LockProfessionalParams{BusinessID: businessID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Professional{}, application.ErrNotFound
	}
	if err != nil {
		return domain.Professional{}, err
	}
	return toDomain(r)
}

func (t *txStore) WeeklySchedule(ctx context.Context, businessID, professionalID uuid.UUID) (domain.WeeklySchedule, error) {
	w := domain.EmptyWeek()
	work, err := t.q.WorkingHoursOf(ctx, sqlcgen.WorkingHoursOfParams{BusinessID: businessID, ProfessionalID: professionalID})
	if err != nil {
		return w, err
	}
	for _, r := range work {
		d, err := day(&w, r.Weekday)
		if err != nil {
			return w, err
		}
		d.WorkingHours = append(d.WorkingHours, domain.TimeRange{Start: domain.LocalTime(r.StartMinute), End: domain.LocalTime(r.EndMinute)})
	}
	breaks, err := t.q.BreaksOf(ctx, sqlcgen.BreaksOfParams{BusinessID: businessID, ProfessionalID: professionalID})
	if err != nil {
		return w, err
	}
	for _, r := range breaks {
		d, err := day(&w, r.Weekday)
		if err != nil {
			return w, err
		}
		d.Breaks = append(d.Breaks, domain.TimeRange{Start: domain.LocalTime(r.StartMinute), End: domain.LocalTime(r.EndMinute)})
	}
	return w, nil
}

func day(w *domain.WeeklySchedule, weekday int16) (*domain.Day, error) {
	if weekday < 0 || int(weekday) >= len(w.Days) {
		return nil, fmt.Errorf("professionals: stored weekday %d out of range", weekday)
	}
	return &w.Days[weekday], nil
}

func (t *txStore) ReplaceWeeklySchedule(ctx context.Context, businessID, professionalID uuid.UUID, w domain.WeeklySchedule) error {
	key := sqlcgen.DeleteWorkingHoursParams{BusinessID: businessID, ProfessionalID: professionalID}
	if err := t.q.DeleteWorkingHours(ctx, key); err != nil {
		return err
	}
	if err := t.q.DeleteBreaks(ctx, sqlcgen.DeleteBreaksParams(key)); err != nil {
		return err
	}
	for _, d := range w.Days {
		weekday := int16(d.Weekday) //nolint:gosec // 0–6
		for _, r := range d.WorkingHours {
			if err := t.q.InsertWorkingHours(ctx, sqlcgen.InsertWorkingHoursParams{
				ID: uuid.Must(uuid.NewV7()), BusinessID: businessID, ProfessionalID: professionalID,
				Weekday: weekday, StartMinute: int32(r.Start), EndMinute: int32(r.End), //nolint:gosec // 0–1439
			}); err != nil {
				return err
			}
		}
		for _, r := range d.Breaks {
			if err := t.q.InsertBreak(ctx, sqlcgen.InsertBreakParams{
				ID: uuid.Must(uuid.NewV7()), BusinessID: businessID, ProfessionalID: professionalID,
				Weekday: weekday, StartMinute: int32(r.Start), EndMinute: int32(r.End), //nolint:gosec // 0–1439
			}); err != nil {
				return err
			}
		}
	}
	return nil
}
