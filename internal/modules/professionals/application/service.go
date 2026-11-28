// Package application holds the professionals use cases (T5.2, schedules T5.3).
//
// Rules (enforced by depguard): may import domain, the identity contract and platform packages;
// no HTTP, database drivers or River.
package application

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	profcontract "github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/domain"
	scheduling "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/audit"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/media"
)

// Audited actions.
const (
	AuditProfessionalCreated = "professional.created"
	AuditProfessionalUpdated = "professional.updated"
)

// Errors.
var (
	ErrNotFound      = errors.New("professionals: not found")
	ErrAlreadyExists = errors.New("professionals: the account already has a profile in this business")
)

// Field error codes.
const (
	CodeRequired           = "REQUIRED"
	CodeInvalidDisplayName = "INVALID_DISPLAY_NAME"
	CodeInvalidBio         = "INVALID_BIO"
	CodeInvalidStatus      = "INVALID_STATUS"
	CodeInvalidSortOrder   = "INVALID_SORT_ORDER"
	CodeAccountNotEligible = "ACCOUNT_NOT_ELIGIBLE"
)

// FieldError is a validation failure on one input field.
type FieldError struct{ Field, Code string }

// ValidationError groups every invalid field of a request.
type ValidationError struct{ Fields []FieldError }

func (e *ValidationError) Error() string {
	parts := make([]string, 0, len(e.Fields))
	for _, f := range e.Fields {
		parts = append(parts, f.Field+":"+f.Code)
	}
	return "professionals: validation failed (" + strings.Join(parts, ", ") + ")"
}

func (e *ValidationError) add(field, code string) {
	e.Fields = append(e.Fields, FieldError{field, code})
}

func (e *ValidationError) orNil() error {
	if len(e.Fields) == 0 {
		return nil
	}
	return e
}

// Store runs a unit of work; fn receives a context carrying the transaction so other modules'
// contracts called with it join it.
type Store interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context, tx Tx) error) error
}

// Tx is the persistence available inside a transaction (always scoped by business).
type Tx interface {
	// Portfolio (2026-11-27). PortfolioImageByID returns ErrNotFound when absent.
	InsertPortfolioImage(ctx context.Context, img domain.PortfolioImage) error
	ListPortfolio(ctx context.Context, businessID, professionalID uuid.UUID) ([]domain.PortfolioImage, error)
	PortfoliosOf(ctx context.Context, professionalIDs []uuid.UUID) ([]domain.PortfolioImage, error)
	PortfolioImageByID(ctx context.Context, businessID, professionalID, id uuid.UUID) (domain.PortfolioImage, error)
	UpdatePortfolioImage(ctx context.Context, img domain.PortfolioImage) error
	DeletePortfolioImage(ctx context.Context, businessID, id uuid.UUID) error
	// InsertProfessional returns ErrAlreadyExists if the account already has a profile here.
	InsertProfessional(ctx context.Context, p domain.Professional) error
	UpdateProfessional(ctx context.Context, p domain.Professional) error
	// ProfessionalByID returns ErrNotFound when absent in that business.
	ProfessionalByID(ctx context.Context, businessID, id uuid.UUID) (domain.Professional, error)
	ListProfessionals(ctx context.Context, businessID uuid.UUID) ([]domain.Professional, error)
	// ActiveProfessionalsIn returns the ACTIVE profiles of several businesses (public view).
	ActiveProfessionalsIn(ctx context.Context, businessIDs []uuid.UUID) ([]domain.Professional, error)
	AppendAudit(ctx context.Context, e *audit.Entry) error
	ScheduleTx
	AbsenceTx
}

// Identity is what this module needs from the identity contract.
type Identity interface {
	RequireBusinessRole(ctx context.Context, p identity.Principal, businessID uuid.UUID, allowed ...identity.Role) (identity.Role, error)
	identity.MemberEnsurer
	identity.MembershipChecker
}

// Service implements the professionals use cases.
type Service struct {
	store    Store
	identity Identity
	clock    clock.Clock
	guard    scheduling.AgendaGuard // nil: no appointment checks (set by the composition root)
	media    media.Store            // portfolio images, see SetMedia
}

// SetAgendaGuard plugs the scheduling module's guard (composition root only, before serving):
// absences and schedules that would break active appointments are refused (T6.7, P11).
func (s *Service) SetAgendaGuard(g scheduling.AgendaGuard) { s.guard = g }

// ConflictError refuses a change of the agenda that would break active appointments; they must
// be rescheduled or canceled first (P11).
type ConflictError struct {
	Appointments []scheduling.ConflictingAppointment
}

func (e *ConflictError) Error() string {
	return "professionals: the change conflicts with appointments"
}

func conflictsOrNil(list []scheduling.ConflictingAppointment, err error) error {
	if err != nil {
		return err
	}
	if len(list) > 0 {
		return &ConflictError{Appointments: list}
	}
	return nil
}

// NewService builds the service; a nil clock defaults to the system clock.
func NewService(store Store, id Identity, clk clock.Clock) *Service {
	if clk == nil {
		clk = clock.System{}
	}
	return &Service{store: store, identity: id, clock: clk}
}

// View is a professional with its effective state: Bookable is false when the profile is
// INACTIVE or the account is no longer a member of the business.
type View struct {
	domain.Professional
	MemberActive bool
	Bookable     bool
}

// CreateInput creates a professional for the account with Email (T5.2).
type CreateInput struct {
	Email, DisplayName, Bio string
	SortOrder               *int
}

// Create adds a professional (OWNER or PLATFORM_ADMIN). If the account is not a member yet it
// becomes a PROFESSIONAL member in the same transaction; an existing member (e.g. an OWNER who
// also serves customers) keeps its role. One profile per account and business.
func (s *Service) Create(ctx context.Context, p identity.Principal, businessID uuid.UUID, in CreateInput) (View, error) {
	if _, err := s.identity.RequireBusinessRole(ctx, p, businessID, identity.RolesOwner...); err != nil {
		return View{}, err
	}
	var verr ValidationError
	if strings.TrimSpace(in.Email) == "" {
		verr.add("email", CodeRequired)
	}
	name, err := domain.ParseDisplayName(in.DisplayName)
	if err != nil {
		verr.add("displayName", required(in.DisplayName, CodeInvalidDisplayName))
	}
	bio, err := domain.ParseBio(in.Bio)
	if err != nil {
		verr.add("bio", CodeInvalidBio)
	}
	order := 0
	if in.SortOrder != nil {
		if domain.ValidateSortOrder(*in.SortOrder) != nil {
			verr.add("sortOrder", CodeInvalidSortOrder)
		}
		order = *in.SortOrder
	}
	if err := verr.orNil(); err != nil {
		return View{}, err
	}
	var prof domain.Professional
	err = s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		userID, err := s.identity.EnsureMember(ctx, p, businessID, in.Email, identity.RoleProfessional)
		if errors.Is(err, identity.ErrAccountNotEligible) {
			return &ValidationError{Fields: []FieldError{{"email", CodeAccountNotEligible}}}
		}
		if err != nil {
			return err
		}
		now := s.clock.Now()
		prof = domain.Professional{
			ID: uuid.Must(uuid.NewV7()), BusinessID: businessID, UserID: userID, DisplayName: name, Bio: bio,
			Status: domain.StatusActive, SortOrder: order, CreatedAt: now, UpdatedAt: now,
		}
		if err := tx.InsertProfessional(ctx, prof); err != nil {
			return err
		}
		return tx.AppendAudit(ctx, s.audit(ctx, p, AuditProfessionalCreated, prof).Meta("user", userID.String()))
	})
	if err != nil {
		return View{}, err
	}
	return View{Professional: prof, MemberActive: true, Bookable: true}, nil
}

// List returns the professionals of a business to its staff (OWNER, PROFESSIONAL) and
// PLATFORM_ADMIN, ordered by sort order and name.
func (s *Service) List(ctx context.Context, p identity.Principal, businessID uuid.UUID) ([]View, error) {
	if _, err := s.identity.RequireBusinessRole(ctx, p, businessID, identity.RolesStaff...); err != nil {
		return nil, err
	}
	var profs []domain.Professional
	err := s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) (err error) {
		profs, err = tx.ListProfessionals(ctx, businessID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return s.views(ctx, businessID, profs)
}

// Get returns one professional to the staff of its business (or PLATFORM_ADMIN).
func (s *Service) Get(ctx context.Context, p identity.Principal, businessID, id uuid.UUID) (View, error) {
	if _, err := s.identity.RequireBusinessRole(ctx, p, businessID, identity.RolesStaff...); err != nil {
		return View{}, err
	}
	var prof domain.Professional
	err := s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) (err error) {
		prof, err = tx.ProfessionalByID(ctx, businessID, id)
		return err
	})
	if err != nil {
		return View{}, err
	}
	vs, err := s.views(ctx, businessID, []domain.Professional{prof})
	if err != nil {
		return View{}, err
	}
	return vs[0], nil
}

// UpdateInput changes the given fields (nil = unchanged).
type UpdateInput struct {
	DisplayName, Bio, Status *string
	SortOrder                *int
}

// Update changes a professional (OWNER or PLATFORM_ADMIN). Audited with the changed fields; a
// request that changes nothing is a no-op.
func (s *Service) Update(ctx context.Context, p identity.Principal, businessID, id uuid.UUID, in UpdateInput) (View, error) {
	if _, err := s.identity.RequireBusinessRole(ctx, p, businessID, identity.RolesOwner...); err != nil {
		return View{}, err
	}
	var verr ValidationError
	var name, bio string
	var status domain.Status
	var err error
	if in.DisplayName != nil {
		if name, err = domain.ParseDisplayName(*in.DisplayName); err != nil {
			verr.add("displayName", required(*in.DisplayName, CodeInvalidDisplayName))
		}
	}
	if in.Bio != nil {
		if bio, err = domain.ParseBio(*in.Bio); err != nil {
			verr.add("bio", CodeInvalidBio)
		}
	}
	if in.Status != nil {
		if status, err = domain.ParseStatus(*in.Status); err != nil {
			verr.add("status", CodeInvalidStatus)
		}
	}
	if in.SortOrder != nil && domain.ValidateSortOrder(*in.SortOrder) != nil {
		verr.add("sortOrder", CodeInvalidSortOrder)
	}
	if err := verr.orNil(); err != nil {
		return View{}, err
	}
	var prof domain.Professional
	err = s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		var err error
		if prof, err = tx.ProfessionalByID(ctx, businessID, id); err != nil {
			return err
		}
		var changed []string
		set := func(cond bool, field string, apply func()) {
			if cond {
				apply()
				changed = append(changed, field)
			}
		}
		set(in.DisplayName != nil && name != prof.DisplayName, "displayName", func() { prof.DisplayName = name })
		set(in.Bio != nil && bio != prof.Bio, "bio", func() { prof.Bio = bio })
		set(in.Status != nil && status != prof.Status, "status", func() { prof.Status = status })
		set(in.SortOrder != nil && *in.SortOrder != prof.SortOrder, "sortOrder", func() { prof.SortOrder = *in.SortOrder })
		if len(changed) == 0 {
			return nil
		}
		prof.UpdatedAt = s.clock.Now()
		if err := tx.UpdateProfessional(ctx, prof); err != nil {
			return err
		}
		sort.Strings(changed)
		return tx.AppendAudit(ctx, s.audit(ctx, p, AuditProfessionalUpdated, prof).Meta("fields", strings.Join(changed, ",")))
	})
	if err != nil {
		return View{}, err
	}
	vs, err := s.views(ctx, businessID, []domain.Professional{prof})
	if err != nil {
		return View{}, err
	}
	return vs[0], nil
}

// views adds the membership state (one query to identity).
func (s *Service) views(ctx context.Context, businessID uuid.UUID, profs []domain.Professional) ([]View, error) {
	ids := make([]uuid.UUID, len(profs))
	for i, p := range profs {
		ids[i] = p.UserID
	}
	active, err := s.identity.ActiveMembersAmong(ctx, businessID, ids)
	if err != nil {
		return nil, err
	}
	out := make([]View, len(profs))
	for i, p := range profs {
		out[i] = View{Professional: p, MemberActive: active[p.UserID], Bookable: p.Bookable(active[p.UserID])}
	}
	return out, nil
}

func (s *Service) audit(ctx context.Context, p identity.Principal, action string, prof domain.Professional) *audit.Entry {
	return audit.New(ctx, s.clock.Now(), action, audit.Success).Actor(p.UserID).Business(prof.BusinessID).
		Resource("professional", prof.ID.String())
}

func required(raw, code string) string {
	if strings.TrimSpace(raw) == "" {
		return CodeRequired
	}
	return code
}

// IsProfessionalOf implements contract.Directory (no authorization; see the contract).
func (s *Service) IsProfessionalOf(ctx context.Context, businessID, professionalID uuid.UUID) (bool, error) {
	found := false
	err := s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		_, err := tx.ProfessionalByID(ctx, businessID, professionalID)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		found = err == nil
		return err
	})
	return found, err
}

// BookableProfessionals implements contract.Bookable: ACTIVE profiles whose account is still a
// member, per business, ordered by sort order and name. One query here plus one identity query
// per business with active profiles.
func (s *Service) BookableProfessionals(ctx context.Context, businessIDs []uuid.UUID) (map[uuid.UUID][]profcontract.PublicProfessional, error) {
	out := map[uuid.UUID][]profcontract.PublicProfessional{}
	if len(businessIDs) == 0 {
		return out, nil
	}
	var profs []domain.Professional
	err := s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) (err error) {
		profs, err = tx.ActiveProfessionalsIn(ctx, businessIDs)
		return err
	})
	if err != nil {
		return nil, err
	}
	byBusiness := map[uuid.UUID][]domain.Professional{}
	var order []uuid.UUID
	for _, p := range profs {
		if _, seen := byBusiness[p.BusinessID]; !seen {
			order = append(order, p.BusinessID)
		}
		byBusiness[p.BusinessID] = append(byBusiness[p.BusinessID], p)
	}
	for _, biz := range order {
		list := byBusiness[biz]
		users := make([]uuid.UUID, len(list))
		for i, p := range list {
			users[i] = p.UserID
		}
		active, err := s.identity.ActiveMembersAmong(ctx, biz, users)
		if err != nil {
			return nil, err
		}
		for _, p := range list {
			if p.Bookable(active[p.UserID]) {
				out[biz] = append(out[biz], profcontract.PublicProfessional{ID: p.ID, BusinessID: biz, DisplayName: p.DisplayName, Bio: p.Bio, SortOrder: p.SortOrder})
			}
		}
	}
	return out, s.attachPortfolios(ctx, out)
}

// attachPortfolios adds the portfolio of every listed professional (one query).
func (s *Service) attachPortfolios(ctx context.Context, byBusiness map[uuid.UUID][]profcontract.PublicProfessional) error {
	var ids []uuid.UUID
	for _, list := range byBusiness {
		for _, p := range list {
			ids = append(ids, p.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	var imgs []domain.PortfolioImage
	if err := s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) (err error) {
		imgs, err = tx.PortfoliosOf(ctx, ids)
		return err
	}); err != nil {
		return err
	}
	photos := map[uuid.UUID][]profcontract.PortfolioPhoto{}
	for _, img := range imgs {
		photos[img.ProfessionalID] = append(photos[img.ProfessionalID], profcontract.PortfolioPhoto{
			ID: img.ID, Key: img.Key, Caption: img.Caption, Width: img.Width, Height: img.Height})
	}
	for biz, list := range byBusiness {
		for i := range list {
			list[i].Portfolio = photos[list[i].ID]
		}
		byBusiness[biz] = list
	}
	return nil
}

// LockAgenda implements the professionals contract Agenda (T6.3): the same row lock as
// ReplaceSchedule, inside the caller's transaction.
func (s *Service) LockAgenda(ctx context.Context, businessID, professionalID uuid.UUID) error {
	err := s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		_, err := tx.LockProfessional(ctx, businessID, professionalID)
		return err
	})
	if errors.Is(err, ErrNotFound) {
		return profcontract.ErrProfessionalNotFound
	}
	return err
}

// ProfessionalUserID implements the professionals contract Agenda (T6.4).
func (s *Service) ProfessionalUserID(ctx context.Context, businessID, professionalID uuid.UUID) (uuid.UUID, error) {
	var user uuid.UUID
	err := s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		p, err := tx.ProfessionalByID(ctx, businessID, professionalID)
		user = p.UserID
		return err
	})
	if errors.Is(err, ErrNotFound) {
		return uuid.Nil, profcontract.ErrProfessionalNotFound
	}
	return user, err
}

// Roster implements the professionals contract Agenda (T6.5).
func (s *Service) Roster(ctx context.Context, businessID uuid.UUID) ([]profcontract.RosterEntry, error) {
	var profs []domain.Professional
	err := s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) (err error) {
		profs, err = tx.ListProfessionals(ctx, businessID)
		return err
	})
	if err != nil {
		return nil, err
	}
	out := make([]profcontract.RosterEntry, 0, len(profs))
	for _, p := range profs {
		out = append(out, profcontract.RosterEntry{ID: p.ID, UserID: p.UserID, DisplayName: p.DisplayName})
	}
	return out, nil
}

// maxStatsAbsences bounds the absences of one professional read for the statistics (5 years).
const maxStatsAbsences = 5000

// WeekSchedules implements the professionals contract Schedules (P-12): the current weekly
// schedule of every profile of the business, its absences overlapping [from, to) and its
// creation time. No authorization.
func (s *Service) WeekSchedules(ctx context.Context, businessID uuid.UUID, from, to time.Time) (map[uuid.UUID]profcontract.WeekSchedule, error) {
	out := map[uuid.UUID]profcontract.WeekSchedule{}
	err := s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		profs, err := tx.ListProfessionals(ctx, businessID)
		if err != nil {
			return err
		}
		for _, p := range profs {
			w, err := tx.WeeklySchedule(ctx, businessID, p.ID)
			if err != nil {
				return err
			}
			ws := profcontract.WeekSchedule{CreatedAt: p.CreatedAt}
			for d, day := range w.Days {
				for _, r := range day.WorkingHours {
					ws.Days[d].WorkingHours = append(ws.Days[d].WorkingHours, profcontract.MinuteRange{Start: int(r.Start), End: int(r.End)})
				}
				for _, r := range day.Breaks {
					ws.Days[d].Breaks = append(ws.Days[d].Breaks, profcontract.MinuteRange{Start: int(r.Start), End: int(r.End)})
				}
			}
			absences, err := tx.ListAbsences(ctx, businessID, p.ID, from.UTC(), to.UTC(), maxStatsAbsences)
			if err != nil {
				return err
			}
			for _, a := range absences {
				ws.Absences = append(ws.Absences, profcontract.Interval{Start: a.Start, End: a.End})
			}
			out[p.ID] = ws
		}
		return nil
	})
	return out, err
}

// DayAgenda implements the professionals contract Agenda (T6.2): working ranges and breaks of
// the weekday and the absences overlapping [from, to). No authorization.
func (s *Service) DayAgenda(ctx context.Context, businessID, professionalID uuid.UUID, weekday time.Weekday, from, to time.Time) (profcontract.DayAgenda, error) {
	var out profcontract.DayAgenda
	err := s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		if _, err := tx.ProfessionalByID(ctx, businessID, professionalID); err != nil {
			return err
		}
		w, err := tx.WeeklySchedule(ctx, businessID, professionalID)
		if err != nil {
			return err
		}
		day := w.Days[weekday]
		for _, r := range day.WorkingHours {
			out.WorkingHours = append(out.WorkingHours, profcontract.MinuteRange{Start: int(r.Start), End: int(r.End)})
		}
		for _, r := range day.Breaks {
			out.Breaks = append(out.Breaks, profcontract.MinuteRange{Start: int(r.Start), End: int(r.End)})
		}
		absences, err := tx.ListAbsences(ctx, businessID, professionalID, from.UTC(), to.UTC(), MaxListedAbsences)
		for _, a := range absences {
			out.Absences = append(out.Absences, profcontract.Interval{Start: a.Start, End: a.End})
		}
		return err
	})
	if errors.Is(err, ErrNotFound) {
		return profcontract.DayAgenda{}, profcontract.ErrProfessionalNotFound
	}
	return out, err
}
