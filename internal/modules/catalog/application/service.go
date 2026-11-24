// Package application holds the catalog use cases (T5.5): only the OWNER (or PLATFORM_ADMIN)
// manages categories and services; the staff (OWNER, PROFESSIONAL) reads them.
//
// Rules (enforced by depguard): may import domain, the identity, businesses and professionals
// contracts and
// platform packages; no HTTP, database drivers or River.
package application

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/google/uuid"

	businesses "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/domain"
	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	professionals "github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/audit"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
)

// Audited actions.
const (
	AuditCategoryCreated = "catalog.category_created"
	AuditCategoryUpdated = "catalog.category_updated"
	AuditCategoryDeleted = "catalog.category_deleted"
	AuditServiceCreated  = "catalog.service_created"
	AuditServiceUpdated  = "catalog.service_updated"
)

// Errors.
var (
	ErrNotFound  = errors.New("catalog: not found")
	ErrNameTaken = errors.New("catalog: name already used in this business")
)

// Field error codes.
const (
	CodeRequired           = "REQUIRED"
	CodeInvalidName        = "INVALID_NAME"
	CodeInvalidDescription = "INVALID_DESCRIPTION"
	CodeInvalidDuration    = "INVALID_DURATION"
	CodeInvalidPrice       = "INVALID_PRICE"
	CodeInvalidStatus      = "INVALID_STATUS"
	CodeInvalidSortOrder   = "INVALID_SORT_ORDER"
	CodeCategoryNotFound   = "CATEGORY_NOT_FOUND"
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
	return "catalog: validation failed (" + strings.Join(parts, ", ") + ")"
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

// Store runs a unit of work in a transaction.
type Store interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context, tx Tx) error) error
}

// Tx is the persistence available inside a transaction (always scoped by business).
type Tx interface {
	// InsertCategory and UpdateCategory return ErrNameTaken on a duplicate name.
	InsertCategory(ctx context.Context, c domain.Category) error
	UpdateCategory(ctx context.Context, c domain.Category) error
	DeleteCategory(ctx context.Context, businessID, id uuid.UUID) error
	// CategoryByID and ServiceByID return ErrNotFound when absent in that business.
	CategoryByID(ctx context.Context, businessID, id uuid.UUID) (domain.Category, error)
	ListCategories(ctx context.Context, businessID uuid.UUID) ([]domain.Category, error)
	// InsertService and UpdateService return ErrNameTaken on a duplicate name.
	InsertService(ctx context.Context, s domain.Service) error
	UpdateService(ctx context.Context, s domain.Service) error
	ServiceByID(ctx context.Context, businessID, id uuid.UUID) (domain.Service, error)
	ListServices(ctx context.Context, businessID uuid.UUID) ([]domain.Service, error)
	AppendAudit(ctx context.Context, e *audit.Entry) error
	ProfessionalServicesTx
}

// Identity is what this module needs from the identity contract.
type Identity interface {
	RequireBusinessRole(ctx context.Context, p identity.Principal, businessID uuid.UUID, allowed ...identity.Role) (identity.Role, error)
}

// Service implements the catalog use cases.
type Service struct {
	store         Store
	identity      Identity
	currencies    businesses.Currencies
	professionals professionals.Directory
	clock         clock.Clock
}

// NewService builds the service; a nil clock defaults to the system clock.
func NewService(store Store, id Identity, currencies businesses.Currencies, profs professionals.Directory, clk clock.Clock) *Service {
	if clk == nil {
		clk = clock.System{}
	}
	return &Service{store: store, identity: id, currencies: currencies, professionals: profs, clock: clk}
}

// ---------------------------------------------------------------------------------------------
// Categories

// CategoryInput creates a category.
type CategoryInput struct {
	Name      string
	SortOrder *int
}

// CategoryPatch changes the given fields (nil = unchanged).
type CategoryPatch struct {
	Name, Status *string
	SortOrder    *int
}

// ListCategories returns the categories of a business to its staff, by sort order and name.
func (s *Service) ListCategories(ctx context.Context, p identity.Principal, businessID uuid.UUID) ([]domain.Category, error) {
	if _, err := s.identity.RequireBusinessRole(ctx, p, businessID, identity.RolesStaff...); err != nil {
		return nil, err
	}
	var out []domain.Category
	err := s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) (err error) {
		out, err = tx.ListCategories(ctx, businessID)
		return err
	})
	return out, err
}

// CreateCategory adds a category (OWNER). Names are unique per business, ignoring case.
func (s *Service) CreateCategory(ctx context.Context, p identity.Principal, businessID uuid.UUID, in CategoryInput) (domain.Category, error) {
	if _, err := s.identity.RequireBusinessRole(ctx, p, businessID, identity.RolesOwner...); err != nil {
		return domain.Category{}, err
	}
	var verr ValidationError
	name, err := domain.ParseCategoryName(in.Name)
	if err != nil {
		verr.add("name", required(in.Name, CodeInvalidName))
	}
	order := sortOrder(&verr, in.SortOrder, 0)
	if err := verr.orNil(); err != nil {
		return domain.Category{}, err
	}
	now := s.clock.Now()
	c := domain.Category{ID: uuid.Must(uuid.NewV7()), BusinessID: businessID, Name: name, Status: domain.StatusActive, SortOrder: order, CreatedAt: now, UpdatedAt: now}
	err = s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		if err := tx.InsertCategory(ctx, c); err != nil {
			return err
		}
		return tx.AppendAudit(ctx, s.audit(ctx, p, AuditCategoryCreated, businessID, "service_category", c.ID))
	})
	if err != nil {
		return domain.Category{}, err
	}
	return c, nil
}

// UpdateCategory changes a category (OWNER); a request that changes nothing is a no-op.
func (s *Service) UpdateCategory(ctx context.Context, p identity.Principal, businessID, id uuid.UUID, in CategoryPatch) (domain.Category, error) {
	if _, err := s.identity.RequireBusinessRole(ctx, p, businessID, identity.RolesOwner...); err != nil {
		return domain.Category{}, err
	}
	var verr ValidationError
	var name string
	var status domain.Status
	var err error
	if in.Name != nil {
		if name, err = domain.ParseCategoryName(*in.Name); err != nil {
			verr.add("name", required(*in.Name, CodeInvalidName))
		}
	}
	if in.Status != nil {
		if status, err = domain.ParseStatus(*in.Status); err != nil {
			verr.add("status", CodeInvalidStatus)
		}
	}
	sortOrder(&verr, in.SortOrder, 0)
	if err := verr.orNil(); err != nil {
		return domain.Category{}, err
	}
	var c domain.Category
	err = s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		var err error
		if c, err = tx.CategoryByID(ctx, businessID, id); err != nil {
			return err
		}
		var ch changes
		ch.set(in.Name != nil && name != c.Name, "name", func() { c.Name = name })
		ch.set(in.Status != nil && status != c.Status, "status", func() { c.Status = status })
		ch.set(in.SortOrder != nil && *in.SortOrder != c.SortOrder, "sortOrder", func() { c.SortOrder = *in.SortOrder })
		if ch.none() {
			return nil
		}
		c.UpdatedAt = s.clock.Now()
		if err := tx.UpdateCategory(ctx, c); err != nil {
			return err
		}
		return tx.AppendAudit(ctx, s.audit(ctx, p, AuditCategoryUpdated, businessID, "service_category", c.ID).Meta("fields", ch.String()))
	})
	if err != nil {
		return domain.Category{}, err
	}
	return c, nil
}

// DeleteCategory removes a category (OWNER); its services become uncategorized.
func (s *Service) DeleteCategory(ctx context.Context, p identity.Principal, businessID, id uuid.UUID) error {
	if _, err := s.identity.RequireBusinessRole(ctx, p, businessID, identity.RolesOwner...); err != nil {
		return err
	}
	return s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		if _, err := tx.CategoryByID(ctx, businessID, id); err != nil {
			return err
		}
		if err := tx.DeleteCategory(ctx, businessID, id); err != nil {
			return err
		}
		return tx.AppendAudit(ctx, s.audit(ctx, p, AuditCategoryDeleted, businessID, "service_category", id))
	})
}

// ---------------------------------------------------------------------------------------------
// Services

// ServiceInput creates a service. CategoryID Nil = uncategorized.
type ServiceInput struct {
	Name, Description string
	CategoryID        uuid.UUID
	DurationMinutes   *int
	PriceMinor        *int64
	SortOrder         *int
}

// ServicePatch changes the given fields (nil = unchanged). CategoryID pointing to uuid.Nil
// removes the category.
type ServicePatch struct {
	Name, Description, Status *string
	CategoryID                *uuid.UUID
	DurationMinutes           *int
	PriceMinor                *int64
	SortOrder                 *int
}

// ListServices returns every service of a business (active and inactive) to its staff, by sort
// order and name.
func (s *Service) ListServices(ctx context.Context, p identity.Principal, businessID uuid.UUID) ([]domain.Service, error) {
	if _, err := s.identity.RequireBusinessRole(ctx, p, businessID, identity.RolesStaff...); err != nil {
		return nil, err
	}
	var out []domain.Service
	err := s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) (err error) {
		out, err = tx.ListServices(ctx, businessID)
		return err
	})
	return out, err
}

// GetService returns one service to the staff of its business.
func (s *Service) GetService(ctx context.Context, p identity.Principal, businessID, id uuid.UUID) (domain.Service, error) {
	if _, err := s.identity.RequireBusinessRole(ctx, p, businessID, identity.RolesStaff...); err != nil {
		return domain.Service{}, err
	}
	var out domain.Service
	err := s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) (err error) {
		out, err = tx.ServiceByID(ctx, businessID, id)
		return err
	})
	return out, err
}

// CreateService adds a service (OWNER) priced in the business's currency.
func (s *Service) CreateService(ctx context.Context, p identity.Principal, businessID uuid.UUID, in ServiceInput) (domain.Service, error) {
	if _, err := s.identity.RequireBusinessRole(ctx, p, businessID, identity.RolesOwner...); err != nil {
		return domain.Service{}, err
	}
	var verr ValidationError
	name, err := domain.ParseServiceName(in.Name)
	if err != nil {
		verr.add("name", required(in.Name, CodeInvalidName))
	}
	desc, err := domain.ParseDescription(in.Description)
	if err != nil {
		verr.add("description", CodeInvalidDescription)
	}
	switch {
	case in.DurationMinutes == nil:
		verr.add("durationMinutes", CodeRequired)
	case domain.ValidateDuration(*in.DurationMinutes) != nil:
		verr.add("durationMinutes", CodeInvalidDuration)
	}
	switch {
	case in.PriceMinor == nil:
		verr.add("priceMinor", CodeRequired)
	case domain.ValidatePrice(*in.PriceMinor) != nil:
		verr.add("priceMinor", CodeInvalidPrice)
	}
	order := sortOrder(&verr, in.SortOrder, 0)
	if err := verr.orNil(); err != nil {
		return domain.Service{}, err
	}
	currency, err := s.currencies.CurrencyOf(ctx, businessID)
	if err != nil {
		return domain.Service{}, err
	}
	now := s.clock.Now()
	svc := domain.Service{
		ID: uuid.Must(uuid.NewV7()), BusinessID: businessID, CategoryID: in.CategoryID, Name: name, Description: desc,
		DurationMinutes: *in.DurationMinutes, PriceMinor: *in.PriceMinor, Currency: currency, Status: domain.StatusActive,
		SortOrder: order, CreatedAt: now, UpdatedAt: now,
	}
	err = s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		if err := checkCategory(ctx, tx, businessID, svc.CategoryID); err != nil {
			return err
		}
		if err := tx.InsertService(ctx, svc); err != nil {
			return err
		}
		return tx.AppendAudit(ctx, s.audit(ctx, p, AuditServiceCreated, businessID, "service", svc.ID))
	})
	if err != nil {
		return domain.Service{}, err
	}
	return svc, nil
}

// UpdateService changes a service (OWNER); audited with the changed fields (price changes do not
// affect appointments already booked: they keep a snapshot, Phase 6).
func (s *Service) UpdateService(ctx context.Context, p identity.Principal, businessID, id uuid.UUID, in ServicePatch) (domain.Service, error) {
	if _, err := s.identity.RequireBusinessRole(ctx, p, businessID, identity.RolesOwner...); err != nil {
		return domain.Service{}, err
	}
	var verr ValidationError
	var name, desc string
	var status domain.Status
	var err error
	if in.Name != nil {
		if name, err = domain.ParseServiceName(*in.Name); err != nil {
			verr.add("name", required(*in.Name, CodeInvalidName))
		}
	}
	if in.Description != nil {
		if desc, err = domain.ParseDescription(*in.Description); err != nil {
			verr.add("description", CodeInvalidDescription)
		}
	}
	if in.Status != nil {
		if status, err = domain.ParseStatus(*in.Status); err != nil {
			verr.add("status", CodeInvalidStatus)
		}
	}
	if in.DurationMinutes != nil && domain.ValidateDuration(*in.DurationMinutes) != nil {
		verr.add("durationMinutes", CodeInvalidDuration)
	}
	if in.PriceMinor != nil && domain.ValidatePrice(*in.PriceMinor) != nil {
		verr.add("priceMinor", CodeInvalidPrice)
	}
	sortOrder(&verr, in.SortOrder, 0)
	if err := verr.orNil(); err != nil {
		return domain.Service{}, err
	}
	var svc domain.Service
	err = s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		var err error
		if svc, err = tx.ServiceByID(ctx, businessID, id); err != nil {
			return err
		}
		if in.CategoryID != nil && *in.CategoryID != svc.CategoryID {
			if err := checkCategory(ctx, tx, businessID, *in.CategoryID); err != nil {
				return err
			}
		}
		var ch changes
		ch.set(in.Name != nil && name != svc.Name, "name", func() { svc.Name = name })
		ch.set(in.Description != nil && desc != svc.Description, "description", func() { svc.Description = desc })
		ch.set(in.Status != nil && status != svc.Status, "status", func() { svc.Status = status })
		ch.set(in.CategoryID != nil && *in.CategoryID != svc.CategoryID, "categoryId", func() { svc.CategoryID = *in.CategoryID })
		ch.set(in.DurationMinutes != nil && *in.DurationMinutes != svc.DurationMinutes, "durationMinutes", func() { svc.DurationMinutes = *in.DurationMinutes })
		ch.set(in.PriceMinor != nil && *in.PriceMinor != svc.PriceMinor, "priceMinor", func() { svc.PriceMinor = *in.PriceMinor })
		ch.set(in.SortOrder != nil && *in.SortOrder != svc.SortOrder, "sortOrder", func() { svc.SortOrder = *in.SortOrder })
		if ch.none() {
			return nil
		}
		svc.UpdatedAt = s.clock.Now()
		if err := tx.UpdateService(ctx, svc); err != nil {
			return err
		}
		return tx.AppendAudit(ctx, s.audit(ctx, p, AuditServiceUpdated, businessID, "service", svc.ID).Meta("fields", ch.String()))
	})
	if err != nil {
		return domain.Service{}, err
	}
	return svc, nil
}

// checkCategory verifies that a (non-Nil) category belongs to the business.
func checkCategory(ctx context.Context, tx Tx, businessID, categoryID uuid.UUID) error {
	if categoryID == uuid.Nil {
		return nil
	}
	_, err := tx.CategoryByID(ctx, businessID, categoryID)
	if errors.Is(err, ErrNotFound) {
		return &ValidationError{Fields: []FieldError{{"categoryId", CodeCategoryNotFound}}}
	}
	return err
}

// ---------------------------------------------------------------------------------------------
// Helpers

type changes []string

func (c *changes) set(cond bool, field string, apply func()) {
	if cond {
		apply()
		*c = append(*c, field)
	}
}

func (c changes) none() bool { return len(c) == 0 }

func (c changes) String() string {
	out := append([]string(nil), c...)
	sort.Strings(out)
	return strings.Join(out, ",")
}

func sortOrder(verr *ValidationError, n *int, def int) int {
	if n == nil {
		return def
	}
	if domain.ValidateSortOrder(*n) != nil {
		verr.add("sortOrder", CodeInvalidSortOrder)
	}
	return *n
}

func (s *Service) audit(ctx context.Context, p identity.Principal, action string, businessID uuid.UUID, kind string, id uuid.UUID) *audit.Entry {
	return audit.New(ctx, s.clock.Now(), action, audit.Success).Actor(p.UserID).Business(businessID).Resource(kind, id.String())
}

func required(raw, code string) string {
	if strings.TrimSpace(raw) == "" {
		return CodeRequired
	}
	return code
}
