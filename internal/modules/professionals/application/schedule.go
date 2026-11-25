package application

// Weekly schedule use cases (T5.3). Decision of 2026-11-24: the OWNER manages the schedule of
// every professional; a professional manages its own (working ranges and breaks) and reads the
// others'.

import (
	"context"
	"strconv"
	"strings"

	"github.com/google/uuid"

	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/domain"
	scheduling "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/authz"
)

// AuditScheduleReplaced is audited when a weekly schedule changes.
const AuditScheduleReplaced = "professional.schedule_replaced"

// ScheduleTx is the schedule persistence available inside a transaction.
type ScheduleTx interface {
	// LockProfessional is ProfessionalByID taking a row lock: concurrent replacements of the same
	// schedule run one after the other.
	LockProfessional(ctx context.Context, businessID, id uuid.UUID) (domain.Professional, error)
	WeeklySchedule(ctx context.Context, businessID, professionalID uuid.UUID) (domain.WeeklySchedule, error)
	ReplaceWeeklySchedule(ctx context.Context, businessID, professionalID uuid.UUID, w domain.WeeklySchedule) error
}

// GetSchedule returns the weekly schedule of a professional to the staff of its business (or
// PLATFORM_ADMIN).
func (s *Service) GetSchedule(ctx context.Context, p identity.Principal, businessID, id uuid.UUID) (domain.WeeklySchedule, error) {
	if _, err := s.identity.RequireBusinessRole(ctx, p, businessID, identity.RolesStaff...); err != nil {
		return domain.WeeklySchedule{}, err
	}
	var w domain.WeeklySchedule
	err := s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		if _, err := tx.ProfessionalByID(ctx, businessID, id); err != nil {
			return err
		}
		var err error
		w, err = tx.WeeklySchedule(ctx, businessID, id)
		return err
	})
	return w, err
}

// ReplaceSchedule replaces the whole week (omitted days are days off). OWNER or PLATFORM_ADMIN
// for any professional; a PROFESSIONAL only for its own profile (authz.ErrForbidden otherwise).
// Audited; an identical schedule is a no-op.
func (s *Service) ReplaceSchedule(ctx context.Context, p identity.Principal, businessID, id uuid.UUID, days []domain.DayInput) (domain.WeeklySchedule, error) {
	role, err := s.identity.RequireBusinessRole(ctx, p, businessID, identity.RolesStaff...)
	if err != nil {
		return domain.WeeklySchedule{}, err
	}
	w, violations := domain.NewWeeklySchedule(days)
	if len(violations) > 0 {
		verr := &ValidationError{}
		for _, v := range violations {
			verr.add(v.Path, v.Code)
		}
		return domain.WeeklySchedule{}, verr
	}
	err = s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		prof, err := tx.LockProfessional(ctx, businessID, id)
		if err != nil {
			return err
		}
		self := prof.UserID == p.UserID
		if role != identity.RoleOwner && !self {
			return authz.ErrForbidden
		}
		current, err := tx.WeeklySchedule(ctx, businessID, id)
		if err != nil {
			return err
		}
		if current.Equal(w) {
			return nil
		}
		if s.guard != nil {
			if err := conflictsOrNil(s.guard.AppointmentsOutside(ctx, businessID, id, toGuardWeek(w))); err != nil {
				return err
			}
		}
		if err := tx.ReplaceWeeklySchedule(ctx, businessID, id, w); err != nil {
			return err
		}
		return tx.AppendAudit(ctx, s.audit(ctx, p, AuditScheduleReplaced, prof).
			Meta("workingDays", workingDays(w)).Meta("self", strconv.FormatBool(self)))
	})
	if err != nil {
		return domain.WeeklySchedule{}, err
	}
	return w, nil
}

func workingDays(w domain.WeeklySchedule) string {
	var out []string
	for _, d := range w.Days {
		if len(d.WorkingHours) > 0 {
			out = append(out, domain.WeekdayName(d.Weekday))
		}
	}
	return strings.Join(out, ",")
}

func toGuardWeek(w domain.WeeklySchedule) [7]scheduling.Day {
	var out [7]scheduling.Day
	for i, d := range w.Days {
		for _, r := range d.WorkingHours {
			out[i].WorkingHours = append(out[i].WorkingHours, scheduling.Range{Start: int(r.Start), End: int(r.End)})
		}
		for _, r := range d.Breaks {
			out[i].Breaks = append(out[i].Breaks, scheduling.Range{Start: int(r.Start), End: int(r.End)})
		}
	}
	return out
}
