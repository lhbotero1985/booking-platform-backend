package application

// Absence use cases (T5.4). Decision of 2026-11-24: the OWNER manages the absences of every
// professional; a professional registers, changes and deletes its own and reads the others'.

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/audit"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/authz"
)

// Audited actions.
const (
	AuditAbsenceCreated = "professional.absence_created"
	AuditAbsenceUpdated = "professional.absence_updated"
	AuditAbsenceDeleted = "professional.absence_deleted"
)

// ErrAbsenceOverlap means the period overlaps another absence of the same professional.
var ErrAbsenceOverlap = errors.New("professionals: the absence overlaps another one")

// Absence field error codes.
const (
	CodeInvalidPeriod = "INVALID_PERIOD"
	CodePeriodTooLong = "PERIOD_TOO_LONG"
	CodeInvalidReason = "INVALID_REASON"
	CodeInvalidWindow = "INVALID_WINDOW"
)

// Listing limits.
const (
	MaxListedAbsences  = 200
	defaultWindowYears = 2
)

// AbsenceTx is the absence persistence available inside a transaction.
type AbsenceTx interface {
	// InsertAbsence and UpdateAbsence return ErrAbsenceOverlap on overlap.
	InsertAbsence(ctx context.Context, a domain.Absence) error
	UpdateAbsence(ctx context.Context, a domain.Absence) error
	DeleteAbsence(ctx context.Context, a domain.Absence) error
	// AbsenceByID returns ErrNotFound when absent for that professional and business.
	AbsenceByID(ctx context.Context, businessID, professionalID, id uuid.UUID) (domain.Absence, error)
	// ListAbsences returns up to limit absences overlapping [from, to), by start.
	ListAbsences(ctx context.Context, businessID, professionalID uuid.UUID, from, to time.Time, limit int) ([]domain.Absence, error)
}

// AbsenceInput creates an absence.
type AbsenceInput struct {
	Start, End time.Time
	Reason     string
}

// AbsencePatch changes the given fields (nil = unchanged).
type AbsencePatch struct {
	Start, End *time.Time
	Reason     *string
}

// ListAbsences returns the absences of a professional overlapping [from, to) to the staff of
// its business. Defaults: from = now, to = from + 2 years; at most 200, by start.
func (s *Service) ListAbsences(ctx context.Context, p identity.Principal, businessID, profID uuid.UUID, from, to *time.Time) ([]domain.Absence, error) {
	if _, err := s.identity.RequireBusinessRole(ctx, p, businessID, identity.RolesStaff...); err != nil {
		return nil, err
	}
	start := s.clock.Now()
	if from != nil {
		start = *from
	}
	end := start.AddDate(defaultWindowYears, 0, 0)
	if to != nil {
		end = *to
	}
	if !start.Before(end) {
		return nil, &ValidationError{Fields: []FieldError{{"to", CodeInvalidWindow}}}
	}
	var out []domain.Absence
	err := s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		if _, err := tx.ProfessionalByID(ctx, businessID, profID); err != nil {
			return err
		}
		var err error
		out, err = tx.ListAbsences(ctx, businessID, profID, start.UTC(), end.UTC(), MaxListedAbsences)
		return err
	})
	return out, err
}

// CreateAbsence registers an absence (OWNER for anyone, PROFESSIONAL for itself).
func (s *Service) CreateAbsence(ctx context.Context, p identity.Principal, businessID, profID uuid.UUID, in AbsenceInput) (domain.Absence, error) {
	role, err := s.identity.RequireBusinessRole(ctx, p, businessID, identity.RolesStaff...)
	if err != nil {
		return domain.Absence{}, err
	}
	var verr ValidationError
	start, end := s.period(&verr, in.Start, in.End)
	reason, rerr := domain.ParseReason(in.Reason)
	if rerr != nil {
		verr.add("reason", CodeInvalidReason)
	}
	if err := verr.orNil(); err != nil {
		return domain.Absence{}, err
	}
	var a domain.Absence
	err = s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		prof, self, err := s.ownAgenda(ctx, tx, p, role, businessID, profID)
		if err != nil {
			return err
		}
		now := s.clock.Now()
		a = domain.Absence{
			ID: uuid.Must(uuid.NewV7()), BusinessID: businessID, ProfessionalID: profID, Start: start, End: end,
			Reason: reason, CreatedBy: p.UserID, CreatedAt: now, UpdatedAt: now,
		}
		if err := tx.InsertAbsence(ctx, a); err != nil {
			return err
		}
		return tx.AppendAudit(ctx, s.absenceAudit(ctx, p, AuditAbsenceCreated, prof, a, self))
	})
	if err != nil {
		return domain.Absence{}, err
	}
	return a, nil
}

// UpdateAbsence changes an absence (same permissions as CreateAbsence). A request that changes
// nothing is a no-op.
func (s *Service) UpdateAbsence(ctx context.Context, p identity.Principal, businessID, profID, id uuid.UUID, in AbsencePatch) (domain.Absence, error) {
	role, err := s.identity.RequireBusinessRole(ctx, p, businessID, identity.RolesStaff...)
	if err != nil {
		return domain.Absence{}, err
	}
	var reason string
	if in.Reason != nil {
		var rerr error
		if reason, rerr = domain.ParseReason(*in.Reason); rerr != nil {
			return domain.Absence{}, &ValidationError{Fields: []FieldError{{"reason", CodeInvalidReason}}}
		}
	}
	var a domain.Absence
	err = s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		prof, self, err := s.ownAgenda(ctx, tx, p, role, businessID, profID)
		if err != nil {
			return err
		}
		if a, err = tx.AbsenceByID(ctx, businessID, profID, id); err != nil {
			return err
		}
		start, end := a.Start, a.End
		if in.Start != nil {
			start = *in.Start
		}
		if in.End != nil {
			end = *in.End
		}
		var verr ValidationError
		start, end = s.period(&verr, start, end)
		if err := verr.orNil(); err != nil {
			return err
		}
		var changed []string
		if !start.Equal(a.Start) {
			a.Start, changed = start, append(changed, "start")
		}
		if !end.Equal(a.End) {
			a.End, changed = end, append(changed, "end")
		}
		if in.Reason != nil && reason != a.Reason {
			a.Reason, changed = reason, append(changed, "reason")
		}
		if len(changed) == 0 {
			return nil
		}
		a.UpdatedAt = s.clock.Now()
		if err := tx.UpdateAbsence(ctx, a); err != nil {
			return err
		}
		sort.Strings(changed)
		return tx.AppendAudit(ctx, s.absenceAudit(ctx, p, AuditAbsenceUpdated, prof, a, self).Meta("fields", strings.Join(changed, ",")))
	})
	if err != nil {
		return domain.Absence{}, err
	}
	return a, nil
}

// DeleteAbsence removes an absence (same permissions as CreateAbsence).
func (s *Service) DeleteAbsence(ctx context.Context, p identity.Principal, businessID, profID, id uuid.UUID) error {
	role, err := s.identity.RequireBusinessRole(ctx, p, businessID, identity.RolesStaff...)
	if err != nil {
		return err
	}
	return s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		prof, self, err := s.ownAgenda(ctx, tx, p, role, businessID, profID)
		if err != nil {
			return err
		}
		a, err := tx.AbsenceByID(ctx, businessID, profID, id)
		if err != nil {
			return err
		}
		if err := tx.DeleteAbsence(ctx, a); err != nil {
			return err
		}
		return tx.AppendAudit(ctx, s.absenceAudit(ctx, p, AuditAbsenceDeleted, prof, a, self))
	})
}

// ownAgenda loads the professional and checks that the caller may manage its agenda: the OWNER
// (or PLATFORM_ADMIN) any, a PROFESSIONAL only its own (authz.ErrForbidden).
func (s *Service) ownAgenda(ctx context.Context, tx Tx, p identity.Principal, role identity.Role, businessID, profID uuid.UUID) (domain.Professional, bool, error) {
	prof, err := tx.ProfessionalByID(ctx, businessID, profID)
	if err != nil {
		return prof, false, err
	}
	self := prof.UserID == p.UserID
	if role != identity.RoleOwner && !self {
		return prof, self, authz.ErrForbidden
	}
	return prof, self, nil
}

func (s *Service) period(verr *ValidationError, start, end time.Time) (time.Time, time.Time) {
	if start.IsZero() {
		verr.add("start", CodeRequired)
	}
	if end.IsZero() {
		verr.add("end", CodeRequired)
	}
	if start.IsZero() || end.IsZero() {
		return start, end
	}
	start, end, err := domain.ValidatePeriod(start, end)
	switch {
	case errors.Is(err, domain.ErrPeriodTooLong):
		verr.add("end", CodePeriodTooLong)
	case err != nil:
		verr.add("end", CodeInvalidPeriod)
	}
	return start, end
}

func (s *Service) absenceAudit(ctx context.Context, p identity.Principal, action string, prof domain.Professional, a domain.Absence, self bool) *audit.Entry {
	return s.audit(ctx, p, action, prof).Meta("absence", a.ID.String()).Meta("self", strconv.FormatBool(self))
}
