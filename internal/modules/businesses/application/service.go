package application

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/domain"
	geography "github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/contract"
	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/audit"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
)

// Audited actions (audit_log.action). Stable identifiers.
const (
	AuditBusinessCreated   = "business.created"
	AuditBusinessActivated = "business.activated"
	AuditBusinessSuspended = "business.suspended"
)

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
}

// Service implements the businesses use cases.
type Service struct {
	store   Store
	authz   Authorizer
	members identity.MemberAdder
	places  geography.Places
	clock   clock.Clock
}

// Deps are the collaborators of the service.
type Deps struct {
	Store   Store
	Authz   Authorizer
	Members identity.MemberAdder
	Places  geography.Places
	Clock   clock.Clock
}

// NewService builds the service; a nil clock defaults to the system clock.
func NewService(d Deps) *Service {
	if d.Clock == nil {
		d.Clock = clock.System{}
	}
	return &Service{store: d.Store, authz: d.Authz, members: d.Members, places: d.Places, clock: d.Clock}
}

// CreateInput creates a business with its OWNER (PLATFORM_ADMIN, T4.3).
type CreateInput struct {
	Name, ContactPhone, Description string
	CityID                          uuid.UUID
	AddressLine                     string
	Latitude, Longitude             *float64
	OwnerEmail                      string
}

// maxInsertAttempts retries when a concurrent creation takes the allocated slug.
const maxInsertAttempts = 3

// CreateBusiness creates a business in PENDING_REVIEW (invisible until activated, decision of
// 2026-11-24) and makes the account with OwnerEmail its OWNER. Business, membership and audit
// entries commit atomically. Time zone and currency come from the city and its country.
func (s *Service) CreateBusiness(ctx context.Context, p identity.Principal, in CreateInput) (*domain.Business, error) {
	if err := s.authz.RequirePlatformAdmin(ctx, p); err != nil {
		return nil, err
	}
	var verr ValidationError
	name, err := domain.ParseName(in.Name)
	if err != nil {
		verr.add("name", required(in.Name, CodeInvalidName))
	}
	description, err := domain.ParseDescription(in.Description)
	if err != nil {
		verr.add("description", CodeInvalidDescription)
	}
	address, err := domain.ParseAddress(in.AddressLine)
	if err != nil {
		verr.add("addressLine", required(in.AddressLine, CodeInvalidAddress))
	}
	var coords *domain.Coordinates
	switch {
	case in.Latitude == nil && in.Longitude == nil:
	case in.Latitude == nil || in.Longitude == nil:
		verr.add("coordinates", CodeInvalidCoordinates)
	default:
		c, err := domain.NewCoordinates(*in.Latitude, *in.Longitude)
		if err != nil {
			verr.add("coordinates", CodeInvalidCoordinates)
		}
		coords = &c
	}
	if strings.TrimSpace(in.OwnerEmail) == "" {
		verr.add("ownerEmail", CodeRequired)
	}
	var place geography.Place
	if in.CityID == uuid.Nil {
		verr.add("cityId", CodeRequired)
	} else if place, err = s.places.PlaceOfCity(ctx, in.CityID); errors.Is(err, geography.ErrCityNotFound) {
		verr.add("cityId", CodeUnknownCity)
	} else if err != nil {
		return nil, err
	} else if !place.Active {
		verr.add("cityId", CodeCityInactive)
	}
	var phone string
	if place.CountryCode != "" {
		if phone, err = domain.ParsePhone(in.ContactPhone, place.CountryCode); err != nil {
			verr.add("contactPhone", required(in.ContactPhone, CodeInvalidPhone))
		}
	}
	if err := verr.orNil(); err != nil {
		return nil, err
	}

	var b *domain.Business
	for attempt := 1; ; attempt++ {
		err = s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
			slug, err := AllocateSlug(ctx, tx, name)
			if err != nil {
				return err
			}
			b, err = domain.NewBusiness(domain.NewBusinessParams{
				ID: uuid.Must(uuid.NewV7()), Name: name, Slug: slug, Status: domain.StatusPendingReview,
				ContactPhone: phone, Description: description,
				Location: domain.Location{
					CountryCode: place.CountryCode, DivisionID: place.DivisionID, CityID: place.CityID,
					AddressLine: address, Coordinates: coords,
				},
				TimeZone: place.TimeZone, Currency: place.Currency,
				Policy: domain.DefaultBookingPolicy(), Reminders: domain.DefaultReminders(), Now: s.clock.Now(),
			})
			if err != nil {
				return err
			}
			if err := tx.InsertBusiness(ctx, b); err != nil {
				return err
			}
			// Same transaction (ctx carries it): no business without its OWNER.
			if _, err := s.members.AddMember(ctx, p, b.ID(), in.OwnerEmail, identity.RoleOwner); err != nil {
				return ownerError(err)
			}
			return tx.AppendAudit(ctx, s.audit(ctx, p, AuditBusinessCreated, b).Meta("city", place.CityID.String()))
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

func ownerError(err error) error {
	if errors.Is(err, identity.ErrAccountNotEligible) {
		return invalid("ownerEmail", CodeAccountNotEligible)
	}
	return err
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
