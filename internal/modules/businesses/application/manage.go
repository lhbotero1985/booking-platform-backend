package application

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/domain"
	geography "github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/contract"
	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	subscriptions "github.com/lhbotero1985/booking-platform-backend/internal/modules/subscriptions/contract"
)

// AuditBusinessUpdated is audited with the changed fields (T4.4).
const AuditBusinessUpdated = "business.updated"

// More field error codes.
const (
	CodeInvalidPolicy    = "INVALID_POLICY"
	CodeInvalidReminder  = "INVALID_REMINDER"
	CodeCityOtherCountry = "CITY_OTHER_COUNTRY"
)

// ManagedBusiness is what the staff of a business sees: the business and its subscription.
type ManagedBusiness struct {
	Business     *domain.Business
	Subscription subscriptions.Summary
}

// GetManaged returns a business to its OWNERs and PROFESSIONALs (or PLATFORM_ADMIN).
// Another business answers 404, identical to a business that does not exist (ADR-0007).
func (s *Service) GetManaged(ctx context.Context, p identity.Principal, businessID uuid.UUID) (ManagedBusiness, error) {
	if _, err := s.authz.RequireBusinessRole(ctx, p, businessID, identity.RolesStaff...); err != nil {
		return ManagedBusiness{}, err
	}
	var out ManagedBusiness
	err := s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) (err error) {
		out.Business, err = tx.BusinessByID(ctx, businessID)
		return err
	})
	if err != nil {
		return ManagedBusiness{}, err
	}
	if out.Subscription, err = s.summaries.SummaryOf(ctx, businessID); err != nil {
		return ManagedBusiness{}, err
	}
	return out, nil
}

// CoordinatesPatch sets (Remove false) or removes the coordinates.
type CoordinatesPatch struct {
	Remove   bool
	Lat, Lng float64
}

// PolicyPatch changes the given booking policy fields (nil = unchanged).
type PolicyPatch struct {
	SlotIntervalMinutes, BufferMinutes, MinLeadMinutes, MaxAdvanceDays *int
	CancellationCutoffHours, RescheduleCutoffHours                     *int
	RequireCustomerConfirmation                                        *bool
}

// RemindersPatch changes the given reminder fields (nil = unchanged).
type RemindersPatch struct {
	Enabled     *bool
	HoursBefore *int
}

// UpdateInput is a partial change of a business by its OWNER (nil = unchanged). Moving to
// another city of the same country also takes that city's time zone.
type UpdateInput struct {
	Name, ContactPhone, Description, AddressLine *string
	CityID                                       *uuid.UUID
	Coordinates                                  *CoordinatesPatch
	Policy                                       *PolicyPatch
	Reminders                                    *RemindersPatch
}

// UpdateBusiness changes a business (OWNER or PLATFORM_ADMIN; a PROFESSIONAL only reads). Audited with the changed fields; a request that changes nothing is a no-op.
func (s *Service) UpdateBusiness(ctx context.Context, p identity.Principal, businessID uuid.UUID, in UpdateInput) (*domain.Business, error) {
	if _, err := s.authz.RequireBusinessRole(ctx, p, businessID, identity.RolesOwner...); err != nil {
		return nil, err
	}
	var b *domain.Business
	err := s.store.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		var err error
		if b, err = tx.BusinessByID(ctx, businessID); err != nil {
			return err
		}
		u, err := s.buildUpdate(ctx, b, in)
		if err != nil {
			return err
		}
		changed, err := b.Apply(u, s.clock.Now())
		if err != nil {
			return invalidDomain(err)
		}
		if len(changed) == 0 {
			return nil
		}
		if err := tx.UpdateBusiness(ctx, b); err != nil {
			return err
		}
		return tx.AppendAudit(ctx, s.audit(ctx, p, AuditBusinessUpdated, b).Meta("fields", strings.Join(changed, ",")))
	})
	if err != nil {
		return nil, err
	}
	return b, nil
}

// buildUpdate parses in against the current business, collecting every invalid field.
func (s *Service) buildUpdate(ctx context.Context, b *domain.Business, in UpdateInput) (domain.Update, error) {
	var verr ValidationError
	var u domain.Update
	if in.Name != nil {
		if v, err := domain.ParseName(*in.Name); err != nil {
			verr.add("name", required(*in.Name, CodeInvalidName))
		} else {
			u.Name = &v
		}
	}
	if in.Description != nil {
		if v, err := domain.ParseDescription(*in.Description); err != nil {
			verr.add("description", CodeInvalidDescription)
		} else {
			u.Description = &v
		}
	}
	if in.ContactPhone != nil {
		if v, err := domain.ParsePhone(*in.ContactPhone, b.Location().CountryCode); err != nil {
			verr.add("contactPhone", required(*in.ContactPhone, CodeInvalidPhone))
		} else {
			u.ContactPhone = &v
		}
	}

	loc, locChanged := b.Location(), false
	if in.AddressLine != nil {
		if v, err := domain.ParseAddress(*in.AddressLine); err != nil {
			verr.add("addressLine", required(*in.AddressLine, CodeInvalidAddress))
		} else {
			loc.AddressLine, locChanged = v, true
		}
	}
	if in.Coordinates != nil {
		locChanged = true
		if in.Coordinates.Remove {
			loc.Coordinates = nil
		} else if c, err := domain.NewCoordinates(in.Coordinates.Lat, in.Coordinates.Lng); err != nil {
			verr.add("coordinates", CodeInvalidCoordinates)
		} else {
			loc.Coordinates = &c
		}
	}
	if in.CityID != nil && *in.CityID != loc.CityID {
		place, err := s.places.PlaceOfCity(ctx, *in.CityID)
		switch {
		case errors.Is(err, geography.ErrCityNotFound):
			verr.add("cityId", CodeUnknownCity)
		case err != nil:
			return domain.Update{}, err
		case !place.Active:
			verr.add("cityId", CodeCityInactive)
		case place.CountryCode != loc.CountryCode:
			verr.add("cityId", CodeCityOtherCountry) // the currency belongs to the country
		default:
			loc.DivisionID, loc.CityID, locChanged = place.DivisionID, place.CityID, true
			tz := place.TimeZone
			u.TimeZone = &tz
		}
	}
	if locChanged {
		u.Location = &loc
	}

	if in.Policy != nil {
		p := b.Policy()
		setInt(&p.SlotIntervalMinutes, in.Policy.SlotIntervalMinutes)
		setInt(&p.BufferMinutes, in.Policy.BufferMinutes)
		setInt(&p.MinLeadMinutes, in.Policy.MinLeadMinutes)
		setInt(&p.MaxAdvanceDays, in.Policy.MaxAdvanceDays)
		setInt(&p.CancellationCutoffHours, in.Policy.CancellationCutoffHours)
		setInt(&p.RescheduleCutoffHours, in.Policy.RescheduleCutoffHours)
		if in.Policy.RequireCustomerConfirmation != nil {
			p.RequireCustomerConfirmation = *in.Policy.RequireCustomerConfirmation
		}
		if f := p.PolicyField(); f != "" {
			verr.add("bookingPolicy."+f, CodeInvalidPolicy)
		} else {
			u.Policy = &p
		}
	}
	if in.Reminders != nil {
		r := b.Reminders()
		setInt(&r.HoursBefore, in.Reminders.HoursBefore)
		if in.Reminders.Enabled != nil {
			r.Enabled = *in.Reminders.Enabled
		}
		if r.Validate() != nil {
			verr.add("reminders.hoursBefore", CodeInvalidReminder)
		} else {
			u.Reminders = &r
		}
	}
	return u, verr.orNil()
}

func setInt(dst *int, v *int) {
	if v != nil {
		*dst = *v
	}
}

// invalidDomain maps a domain invariant error that slipped past the field checks.
func invalidDomain(err error) error {
	if errors.Is(err, domain.ErrInvalidLocation) {
		return invalid("cityId", CodeCityOtherCountry)
	}
	return err
}
