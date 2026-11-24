package domain

import (
	"time"

	"github.com/google/uuid"
)

// Status is the lifecycle state of a business (matches businesses_status_valid).
type Status string

// Statuses. Businesses are created by PLATFORM_ADMIN (P10); PENDING_REVIEW is kept for a
// future self-registration flow.
const (
	StatusPendingReview Status = "PENDING_REVIEW"
	StatusActive        Status = "ACTIVE"
	StatusSuspended     Status = "SUSPENDED"
)

// ParseStatus validates a stored or requested status.
func ParseStatus(s string) (Status, error) {
	switch st := Status(s); st {
	case StatusPendingReview, StatusActive, StatusSuspended:
		return st, nil
	default:
		return "", ErrInvalidStatus
	}
}

// BookingPolicy are the booking rules of a business (domain-model.md §4).
type BookingPolicy struct {
	SlotIntervalMinutes         int  // 5, 10, 15 or 20
	BufferMinutes               int  // 0–60, between appointments
	MinLeadMinutes              int  // 0–10080, minimum notice to book
	MaxAdvanceDays              int  // 1–365, booking horizon
	CancellationCutoffHours     int  // 0–168, the customer may cancel until N hours before
	RescheduleCutoffHours       int  // 0–168
	RequireCustomerConfirmation bool // reminder asks the customer to confirm
}

// DefaultBookingPolicy returns the documented defaults.
func DefaultBookingPolicy() BookingPolicy {
	return BookingPolicy{
		SlotIntervalMinutes: 10, BufferMinutes: 0, MinLeadMinutes: 60, MaxAdvanceDays: 60,
		CancellationCutoffHours: 2, RescheduleCutoffHours: 2, RequireCustomerConfirmation: true,
	}
}

// PolicyField names the first invalid field of a policy ("" when valid).
func (p BookingPolicy) PolicyField() string {
	switch {
	case p.SlotIntervalMinutes != 5 && p.SlotIntervalMinutes != 10 && p.SlotIntervalMinutes != 15 && p.SlotIntervalMinutes != 20:
		return "slotIntervalMinutes"
	case p.BufferMinutes < 0 || p.BufferMinutes > 60:
		return "bufferMinutes"
	case p.MinLeadMinutes < 0 || p.MinLeadMinutes > 10080:
		return "minLeadMinutes"
	case p.MaxAdvanceDays < 1 || p.MaxAdvanceDays > 365:
		return "maxAdvanceDays"
	case p.CancellationCutoffHours < 0 || p.CancellationCutoffHours > 168:
		return "cancellationCutoffHours"
	case p.RescheduleCutoffHours < 0 || p.RescheduleCutoffHours > 168:
		return "rescheduleCutoffHours"
	default:
		return ""
	}
}

// Validate returns ErrInvalidPolicy when a field is out of range.
func (p BookingPolicy) Validate() error {
	if p.PolicyField() != "" {
		return ErrInvalidPolicy
	}
	return nil
}

// Reminders configures appointment reminders (channels arrive with Phase 7).
type Reminders struct {
	Enabled     bool
	HoursBefore int // 1–72
}

// DefaultReminders returns reminders enabled 24 hours before.
func DefaultReminders() Reminders { return Reminders{Enabled: true, HoursBefore: 24} }

// Validate returns ErrInvalidReminder when HoursBefore is out of range.
func (r Reminders) Validate() error {
	if r.HoursBefore < 1 || r.HoursBefore > 72 {
		return ErrInvalidReminder
	}
	return nil
}

// Location places a business. CountryCode/DivisionID/CityID must be consistent (the city in
// the division, the division in the country): the application resolves them from the city and
// the schema enforces it with composite foreign keys.
type Location struct {
	CountryCode string
	DivisionID  uuid.UUID
	CityID      uuid.UUID
	AddressLine string
	Coordinates *Coordinates
}

// Business is the tenant root aggregate.
type Business struct {
	id           uuid.UUID
	name         string
	slug         Slug
	status       Status
	contactPhone string
	description  string
	location     Location
	timeZone     string
	currency     string
	policy       BookingPolicy
	reminders    Reminders
	createdAt    time.Time
	updatedAt    time.Time
}

// NewBusinessParams are the validated inputs of a new business.
type NewBusinessParams struct {
	ID           uuid.UUID
	Name         string
	Slug         Slug
	Status       Status
	ContactPhone string // E.164
	Description  string
	Location     Location
	TimeZone     string
	Currency     string
	Policy       BookingPolicy
	Reminders    Reminders
	Now          time.Time
}

// NewBusiness creates a business after checking every invariant.
func NewBusiness(p NewBusinessParams) (*Business, error) {
	b := &Business{
		id: p.ID, name: p.Name, slug: p.Slug, status: p.Status, contactPhone: p.ContactPhone,
		description: p.Description, location: p.Location, timeZone: p.TimeZone, currency: p.Currency,
		policy: p.Policy, reminders: p.Reminders, createdAt: p.Now, updatedAt: p.Now,
	}
	if err := b.validate(); err != nil {
		return nil, err
	}
	return b, nil
}

// RehydrateBusiness rebuilds a stored business (already valid; checked again to fail closed if
// the schema and the code drift).
func RehydrateBusiness(p NewBusinessParams, createdAt, updatedAt time.Time) (*Business, error) {
	b, err := NewBusiness(p)
	if err != nil {
		return nil, err
	}
	b.createdAt, b.updatedAt = createdAt, updatedAt
	return b, nil
}

func (b *Business) validate() error {
	if b.id == uuid.Nil {
		return ErrInvalidID
	}
	if _, err := ParseName(b.name); err != nil {
		return err
	}
	if _, err := ParseSlug(string(b.slug)); err != nil {
		return err
	}
	if _, err := ParseStatus(string(b.status)); err != nil {
		return err
	}
	if b.contactPhone == "" || b.contactPhone[0] != '+' {
		return ErrInvalidPhone
	}
	if _, err := ParseDescription(b.description); err != nil {
		return err
	}
	l := b.location
	if len(l.CountryCode) != 2 || l.DivisionID == uuid.Nil || l.CityID == uuid.Nil {
		return ErrInvalidLocation
	}
	if _, err := ParseAddress(l.AddressLine); err != nil {
		return err
	}
	if l.Coordinates != nil {
		if _, err := NewCoordinates(l.Coordinates.Lat, l.Coordinates.Lng); err != nil {
			return err
		}
	}
	if _, err := ParseTimeZone(b.timeZone); err != nil {
		return err
	}
	if _, err := ParseCurrency(b.currency); err != nil {
		return err
	}
	if err := b.policy.Validate(); err != nil {
		return err
	}
	return b.reminders.Validate()
}

// Suspend hides the business from search and stops new bookings (idempotent).
func (b *Business) Suspend(now time.Time) {
	if b.status != StatusSuspended {
		b.status, b.updatedAt = StatusSuspended, now
	}
}

// Activate makes the business bookable and searchable (idempotent).
func (b *Business) Activate(now time.Time) {
	if b.status != StatusActive {
		b.status, b.updatedAt = StatusActive, now
	}
}

// IsBookable reports whether new bookings are accepted.
func (b *Business) IsBookable() bool { return b.status == StatusActive }

// ID is the business identifier (UUIDv7).
func (b *Business) ID() uuid.UUID { return b.id }

// Name is the display name.
func (b *Business) Name() string { return b.name }

// Slug is the public URL identifier.
func (b *Business) Slug() Slug { return b.slug }

// Status is the lifecycle state.
func (b *Business) Status() Status { return b.status }

// ContactPhone is the E.164 contact number.
func (b *Business) ContactPhone() string { return b.contactPhone }

// Description is the optional public description.
func (b *Business) Description() string { return b.description }

// Location is where the business is.
func (b *Business) Location() Location { return b.location }

// TimeZone is the IANA zone of the business.
func (b *Business) TimeZone() string { return b.timeZone }

// Currency is the ISO 4217 currency of its prices.
func (b *Business) Currency() string { return b.currency }

// Policy are the booking rules.
func (b *Business) Policy() BookingPolicy { return b.policy }

// Reminders configures appointment reminders.
func (b *Business) Reminders() Reminders { return b.reminders }

// CreatedAt is when the business was created.
func (b *Business) CreatedAt() time.Time { return b.createdAt }

// UpdatedAt is the last change.
func (b *Business) UpdatedAt() time.Time { return b.updatedAt }
