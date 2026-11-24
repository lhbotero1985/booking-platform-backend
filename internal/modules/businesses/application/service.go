package application

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"

	bizcontract "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/domain"
	geography "github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/contract"
	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	subscriptions "github.com/lhbotero1985/booking-platform-backend/internal/modules/subscriptions/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/audit"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
)

// Audited actions (audit_log.action). Stable identifiers.
const (
	AuditBusinessCreated   = "business.created"
	AuditBusinessActivated = "business.activated"
	AuditBusinessSuspended = "business.suspended"
)

// ErrUnauthenticated is returned when a self-service request has no authenticated account.
var ErrUnauthenticated = errors.New("businesses: authentication required")

// ErrLocationInactive is returned when the business's city, division or country is inactive, so it cannot be
// activated (it would be unreachable).
var ErrLocationInactive = errors.New("businesses: location is inactive")

// Field error codes exposed in Problem.errors[].message.
const (
	CodeRequired           = "REQUIRED"
	CodeInvalidName        = "INVALID_NAME"
	CodeInvalidPhone       = "INVALID_PHONE"
	CodeInvalidDescription = "INVALID_DESCRIPTION"
	CodeInvalidAddress     = "INVALID_ADDRESS"
	CodeInvalidCoordinates = "INVALID_COORDINATES"
	CodeUnknownCity        = "UNKNOWN_CITY"
	CodeCityInactive       = "CITY_INACTIVE"
	CodeAccountNotEligible = "ACCOUNT_NOT_ELIGIBLE"
	CodeInvalidStatus      = "INVALID_STATUS"
)

// FieldError is a validation failure on one input field.
type FieldError struct {
	Field string
	Code  string
}

// ValidationError groups every invalid field of a request.
type ValidationError struct {
	Fields []FieldError
}

func (e *ValidationError) Error() string {
	parts := make([]string, 0, len(e.Fields))
	for _, f := range e.Fields {
		parts = append(parts, f.Field+":"+f.Code)
	}
	return "businesses: validation failed (" + strings.Join(parts, ", ") + ")"
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

func invalid(field, code string) error { return &ValidationError{Fields: []FieldError{{field, code}}} }

// Authorizer is the part of the identity contract these use cases need.
type Authorizer interface {
	RequirePlatformAdmin(ctx context.Context, p identity.Principal) error
	RequireBusinessRole(ctx context.Context, p identity.Principal, businessID uuid.UUID, allowed ...identity.Role) (identity.Role, error)
}

// Service implements the businesses use cases.
type Service struct {
	store        Store
	authz        Authorizer
	members      identity.MemberAdder
	owners       identity.OwnershipClaimer
	trials       subscriptions.Trials
	summaries    subscriptions.Summaries
	entitlements subscriptions.Entitlements
	places       geography.Places
	clock        clock.Clock
}

// Deps are the collaborators of the service.
type Deps struct {
	Store        Store
	Authz        Authorizer
	Members      identity.MemberAdder
	Owners       identity.OwnershipClaimer
	Trials       subscriptions.Trials
	Summaries    subscriptions.Summaries
	Entitlements subscriptions.Entitlements
	Places       geography.Places
	Clock        clock.Clock
}

// NewService builds the service; a nil clock defaults to the system clock.
func NewService(d Deps) *Service {
	if d.Clock == nil {
		d.Clock = clock.System{}
	}
	return &Service{
		store: d.Store, authz: d.Authz, members: d.Members, owners: d.Owners, trials: d.Trials,
		summaries: d.Summaries, entitlements: d.Entitlements, places: d.Places, clock: d.Clock,
	}
}

// BusinessInput are the data of a new business.
type BusinessInput struct {
	Name, ContactPhone, Description string
	CityID                          uuid.UUID
	AddressLine                     string
	Latitude, Longitude             *float64
}

// CreateInput creates a business with its OWNER (PLATFORM_ADMIN, T4.3).
type CreateInput struct {
	BusinessInput
	OwnerEmail string
}

// maxInsertAttempts retries when a concurrent creation takes the allocated slug.
const maxInsertAttempts = 3

// CreateBusiness is the support path (PLATFORM_ADMIN) to create a business on behalf of an
// owner: like self-service it is ACTIVE at once with a 7-day demo, so the administrator never
// has to activate anything (decision of 2026-11-24); the account with OwnerEmail becomes its
// OWNER. Business, membership, subscription and audit entries commit atomically.
func (s *Service) CreateBusiness(ctx context.Context, p identity.Principal, in CreateInput) (*domain.Business, error) {
	if err := s.authz.RequirePlatformAdmin(ctx, p); err != nil {
		return nil, err
	}
	var verr ValidationError
	if strings.TrimSpace(in.OwnerEmail) == "" {
		verr.add("ownerEmail", CodeRequired)
	}
	v, err := s.validate(ctx, in.BusinessInput, &verr)
	if err != nil {
		return nil, err
	}
	return s.create(ctx, p, v, domain.StatusActive, "admin", func(ctx context.Context, businessID uuid.UUID) error {
		if _, err := s.members.AddMember(ctx, p, businessID, in.OwnerEmail, identity.RoleOwner); err != nil {
			if errors.Is(err, identity.ErrAccountNotEligible) {
				return invalid("ownerEmail", CodeAccountNotEligible)
			}
			return err
		}
		return nil
	})
}

// CreateOwnBusiness is the self-service onboarding (ADR-0018): the authenticated account
// creates its business, which is ACTIVE at once (it can take real bookings during the demo),
// becomes its OWNER and gets a 7-day demo. One per account (identity.ErrAlreadyOwner).
func (s *Service) CreateOwnBusiness(ctx context.Context, p identity.Principal, in BusinessInput) (*domain.Business, error) {
	if p.UserID == uuid.Nil {
		return nil, ErrUnauthenticated
	}
	var verr ValidationError
	v, err := s.validate(ctx, in, &verr)
	if err != nil {
		return nil, err
	}
	return s.create(ctx, p, v, domain.StatusActive, "self_service", func(ctx context.Context, businessID uuid.UUID) error {
		return s.owners.ClaimOwnership(ctx, p, businessID)
	})
}

// validated are the parsed inputs of a new business.
type validated struct {
	name, description, address, phone string
	coords                            *domain.Coordinates
	place                             geography.Place
}

// validate parses in, adding field errors to verr (which may already hold some) and returning
// it if any; other errors (geography unavailable) are returned as they are.
func (s *Service) validate(ctx context.Context, in BusinessInput, verr *ValidationError) (validated, error) {
	var v validated
	var err error
	if v.name, err = domain.ParseName(in.Name); err != nil {
		verr.add("name", required(in.Name, CodeInvalidName))
	}
	if v.description, err = domain.ParseDescription(in.Description); err != nil {
		verr.add("description", CodeInvalidDescription)
	}
	if v.address, err = domain.ParseAddress(in.AddressLine); err != nil {
		verr.add("addressLine", required(in.AddressLine, CodeInvalidAddress))
	}
	switch {
	case in.Latitude == nil && in.Longitude == nil:
	case in.Latitude == nil || in.Longitude == nil:
		verr.add("coordinates", CodeInvalidCoordinates)
	default:
		c, err := domain.NewCoordinates(*in.Latitude, *in.Longitude)
		if err != nil {
			verr.add("coordinates", CodeInvalidCoordinates)
		}
		v.coords = &c
	}
	if in.CityID == uuid.Nil {
		verr.add("cityId", CodeRequired)
	} else if v.place, err = s.places.PlaceOfCity(ctx, in.CityID); errors.Is(err, geography.ErrCityNotFound) {
		verr.add("cityId", CodeUnknownCity)
	} else if err != nil {
		return validated{}, err
	} else if !v.place.Active {
		verr.add("cityId", CodeCityInactive)
	}
	if v.place.CountryCode != "" {
		if v.phone, err = domain.ParsePhone(in.ContactPhone, v.place.CountryCode); err != nil {
			verr.add("contactPhone", required(in.ContactPhone, CodeInvalidPhone))
		}
	}
	return v, verr.orNil()
}

// create inserts the business, attaches its OWNER and starts the demo in one transaction (the
// other modules join it through ctx), retrying if a concurrent creation takes the slug.
func (s *Service) create(ctx context.Context, p identity.Principal, v validated, status domain.Status, via string,
	attachOwner func(ctx context.Context, businessID uuid.UUID) error) (*domain.Business, error) {
	var b *domain.Business
	var err error
	for attempt := 1; ; attempt++ {
		err = s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
			slug, err := AllocateSlug(ctx, tx, v.name)
			if err != nil {
				return err
			}
			b, err = domain.NewBusiness(domain.NewBusinessParams{
				ID: uuid.Must(uuid.NewV7()), Name: v.name, Slug: slug, Status: status,
				ContactPhone: v.phone, Description: v.description,
				Location: domain.Location{
					CountryCode: v.place.CountryCode, DivisionID: v.place.DivisionID, CityID: v.place.CityID,
					AddressLine: v.address, Coordinates: v.coords,
				},
				TimeZone: v.place.TimeZone, Currency: v.place.Currency,
				Policy: domain.DefaultBookingPolicy(), Reminders: domain.DefaultReminders(), Now: s.clock.Now(),
			})
			if err != nil {
				return err
			}
			if err := tx.InsertBusiness(ctx, b); err != nil {
				return err
			}
			// Same transaction (ctx carries it): no business without its OWNER and its demo.
			if err := attachOwner(ctx, b.ID()); err != nil {
				return err
			}
			if err := s.trials.StartTrial(ctx, b.ID()); err != nil {
				return err
			}
			return tx.AppendAudit(ctx, s.audit(ctx, p, AuditBusinessCreated, b).
				Meta("city", v.place.CityID.String()).Meta("via", via))
		})
		if !errors.Is(err, ErrSlugTaken) || attempt == maxInsertAttempts {
			break
		}
	}
	if err != nil {
		return nil, err
	}
	return b, nil
}

// GetBusiness returns any business (PLATFORM_ADMIN).
func (s *Service) GetBusiness(ctx context.Context, p identity.Principal, id uuid.UUID) (*domain.Business, error) {
	if err := s.authz.RequirePlatformAdmin(ctx, p); err != nil {
		return nil, err
	}
	var b *domain.Business
	err := s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) (err error) {
		b, err = tx.BusinessByID(ctx, id)
		return err
	})
	return b, err
}

// MaxListLimit bounds a page of the administrator's list.
const MaxListLimit = 100

// ListBusinesses pages through businesses (PLATFORM_ADMIN). next is the cursor of the following
// page (Nil when there are no more).
func (s *Service) ListBusinesses(ctx context.Context, p identity.Principal, status string, cityID, after uuid.UUID, limit int) (items []*domain.Business, next uuid.UUID, err error) {
	if err := s.authz.RequirePlatformAdmin(ctx, p); err != nil {
		return nil, uuid.Nil, err
	}
	f := ListFilter{CityID: cityID, After: after, Limit: limit}
	if status != "" {
		if f.Status, err = domain.ParseStatus(status); err != nil {
			return nil, uuid.Nil, invalid("status", CodeInvalidStatus)
		}
	}
	if f.Limit <= 0 || f.Limit > MaxListLimit {
		f.Limit = MaxListLimit
	}
	f.Limit++ // one extra row tells whether there is a next page
	err = s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) (err error) {
		items, err = tx.ListBusinesses(ctx, f)
		return err
	})
	if err != nil {
		return nil, uuid.Nil, err
	}
	if len(items) == f.Limit {
		items = items[:f.Limit-1]
		next = items[len(items)-1].ID()
	}
	return items, next, nil
}

// ActivateBusiness makes a business visible and bookable (PLATFORM_ADMIN). Its location must
// be active.
func (s *Service) ActivateBusiness(ctx context.Context, p identity.Principal, id uuid.UUID) (*domain.Business, error) {
	return s.changeStatus(ctx, p, id, domain.StatusActive)
}

// SuspendBusiness hides a business and stops new bookings (PLATFORM_ADMIN).
func (s *Service) SuspendBusiness(ctx context.Context, p identity.Principal, id uuid.UUID) (*domain.Business, error) {
	return s.changeStatus(ctx, p, id, domain.StatusSuspended)
}

func (s *Service) changeStatus(ctx context.Context, p identity.Principal, id uuid.UUID, to domain.Status) (*domain.Business, error) {
	if err := s.authz.RequirePlatformAdmin(ctx, p); err != nil {
		return nil, err
	}
	var b *domain.Business
	err := s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		var err error
		if b, err = tx.BusinessByID(ctx, id); err != nil {
			return err
		}
		if b.Status() == to {
			return nil // idempotent, nothing to audit
		}
		now := s.clock.Now()
		action := AuditBusinessSuspended
		if to == domain.StatusActive {
			place, err := s.places.PlaceOfCity(ctx, b.Location().CityID)
			if err != nil {
				return err
			}
			if !place.Active {
				return ErrLocationInactive
			}
			b.Activate(now)
			action = AuditBusinessActivated
		} else {
			b.Suspend(now)
		}
		if err := tx.UpdateBusiness(ctx, b); err != nil {
			return err
		}
		return tx.AppendAudit(ctx, s.audit(ctx, p, action, b))
	})
	return b, err
}

func (s *Service) audit(ctx context.Context, p identity.Principal, action string, b *domain.Business) *audit.Entry {
	return audit.New(ctx, s.clock.Now(), action, audit.Success).Actor(p.UserID).Business(b.ID()).
		Resource("business", b.ID().String())
}

func required(raw, code string) string {
	if strings.TrimSpace(raw) == "" {
		return CodeRequired
	}
	return code
}

// CurrencyOf implements contract.Currencies (no authorization; see the contract).
func (s *Service) CurrencyOf(ctx context.Context, businessID uuid.UUID) (string, error) {
	var currency string
	err := s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		b, err := tx.BusinessByID(ctx, businessID)
		if errors.Is(err, ErrNotFound) {
			return bizcontract.ErrBusinessNotFound
		}
		if err != nil {
			return err
		}
		currency = b.Currency()
		return nil
	})
	return currency, err
}
