package application

// Preferences and costs (T7.9): the customer chooses its channels (at least one) and whether it
// wants reminders, per account; each specialist profile turns its emails on or off (the
// specialist or the owner); the platform caps the WhatsApp messages of each business per month
// and of the whole platform per day (P16). Everything is read when a message is sent, so a change
// also applies to the reminders already planned. A skipped message costs nothing.

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	businesses "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/contract"
	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/domain"
	scheduling "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/contract"
)

// PreferencesStore keeps the preferences and counts the WhatsApp messages sent.
type PreferencesStore interface {
	// CustomerPreferences: the defaults (everything on) when the account never set them.
	CustomerPreferences(ctx context.Context, userID uuid.UUID) (domain.CustomerPreferences, error)
	SetCustomerPreferences(ctx context.Context, userID uuid.UUID, p domain.CustomerPreferences, at time.Time) error
	// ProfessionalEmailEnabled: true when never set.
	ProfessionalEmailEnabled(ctx context.Context, businessID, professionalID uuid.UUID) (bool, error)
	SetProfessionalEmailEnabled(ctx context.Context, businessID, professionalID uuid.UUID, on bool, at time.Time) error
	// WhatsAppMonthlyLimit: ok false when the platform never set one for the business.
	WhatsAppMonthlyLimit(ctx context.Context, businessID uuid.UUID) (limit int, ok bool, err error)
	SetWhatsAppMonthlyLimit(ctx context.Context, businessID uuid.UUID, limit int, at time.Time) error
	// CountWhatsAppSent counts the WhatsApp messages sent in [from, to), of one business or of
	// every business (uuid.Nil).
	CountWhatsAppSent(ctx context.Context, businessID uuid.UUID, from, to time.Time) (int, error)
}

// ErrInvalidLimit means the cap is outside 0–100000.
var ErrInvalidLimit = errors.New("notifications: invalid WhatsApp limit")

// unwanted tells why a message is not sent: the recipient's preferences or the WhatsApp caps.
func (s *Service) unwanted(ctx context.Context, m domain.Message, st scheduling.AppointmentState) (domain.SkipReason, error) {
	switch {
	case m.RecipientRole == domain.RecipientCustomer && st.CustomerUserID != uuid.Nil:
		p, err := s.d.Store.CustomerPreferences(ctx, st.CustomerUserID)
		if err != nil {
			return "", err
		}
		if reason := p.SkipFor(m); reason != "" {
			return reason, nil
		}
	case m.RecipientRole == domain.RecipientProfessional && m.Channel == domain.ChannelEmail:
		on, err := s.d.Store.ProfessionalEmailEnabled(ctx, m.BusinessID, m.ProfessionalID)
		if err != nil {
			return "", err
		}
		if !on {
			return domain.SkipProfessionalOptedOut, nil
		}
	}
	if m.Channel != domain.ChannelWhatsApp {
		return "", nil
	}
	return s.overCap(ctx, m.BusinessID)
}

// overCap checks the WhatsApp caps. They are soft: workers sending at the same moment may pass a
// cap by the number of messages in flight.
func (s *Service) overCap(ctx context.Context, businessID uuid.UUID) (domain.SkipReason, error) {
	now := s.d.Clock.Now()
	if daily := s.d.Policy.WhatsAppDailyLimit; daily > 0 {
		from, to := domain.DayOf(now)
		n, err := s.d.Store.CountWhatsAppSent(ctx, uuid.Nil, from, to)
		if err != nil {
			return "", err
		}
		if n >= daily {
			return domain.SkipDailyLimit, nil
		}
	}
	u, err := s.whatsAppUsage(ctx, businessID, now)
	if err != nil {
		return "", err
	}
	if u.Limited && u.UsedThisMonth >= u.MonthlyLimit {
		return domain.SkipMonthlyLimit, nil
	}
	return "", nil
}

// whatsAppUsage: the business's own cap, else the environment's default (0 there = no cap).
func (s *Service) whatsAppUsage(ctx context.Context, businessID uuid.UUID, now time.Time) (WhatsAppLimit, error) {
	limit, set, err := s.d.Store.WhatsAppMonthlyLimit(ctx, businessID)
	if err != nil {
		return WhatsAppLimit{}, err
	}
	if !set {
		limit = s.d.Policy.WhatsAppMonthlyLimit
	}
	from, to := domain.MonthOf(now)
	used, err := s.d.Store.CountWhatsAppSent(ctx, businessID, from, to)
	return WhatsAppLimit{MonthlyLimit: limit, Default: !set, Limited: set || limit > 0, UsedThisMonth: used, MonthStart: from}, err
}

// MyPreferences returns the caller's preferences.
func (s *Service) MyPreferences(ctx context.Context, p identity.Principal) (domain.CustomerPreferences, error) {
	return s.d.Store.CustomerPreferences(ctx, p.UserID)
}

// SetMyPreferences replaces the caller's preferences; domain.ErrNoChannel without a channel.
func (s *Service) SetMyPreferences(ctx context.Context, p identity.Principal, prefs domain.CustomerPreferences) error {
	if err := prefs.Validate(); err != nil {
		return err
	}
	return s.d.Store.SetCustomerPreferences(ctx, p.UserID, prefs, s.d.Clock.Now())
}

// authorizeProfessional lets the owner and the specialist itself; other members get
// ErrForbidden, everybody else (and an unknown profile) ErrNotFound.
func (s *Service) authorizeProfessional(ctx context.Context, p identity.Principal, businessID, professionalID uuid.UUID) error {
	role, err := s.d.Authz.RequireBusinessRole(ctx, p, businessID, identity.RolesStaff...)
	if err != nil {
		return err
	}
	account, err := s.d.Recipients.ProfessionalAccount(ctx, businessID, professionalID)
	switch {
	case err != nil:
		return err
	case account == uuid.Nil:
		return identity.ErrNotFound
	case role == identity.RoleOwner, account == p.UserID:
		return nil
	}
	return identity.ErrForbidden
}

// ProfessionalEmails tells whether a specialist receives its appointment emails.
func (s *Service) ProfessionalEmails(ctx context.Context, p identity.Principal, businessID, professionalID uuid.UUID) (bool, error) {
	if err := s.authorizeProfessional(ctx, p, businessID, professionalID); err != nil {
		return false, err
	}
	return s.d.Store.ProfessionalEmailEnabled(ctx, businessID, professionalID)
}

// ErrEmailRequired means the request did not say whether the emails are on.
var ErrEmailRequired = errors.New("notifications: email is required")

// SetProfessionalEmails turns a specialist's appointment emails on or off; nil (missing in the
// request) is ErrEmailRequired, checked after the authorization so a stranger learns nothing.
func (s *Service) SetProfessionalEmails(ctx context.Context, p identity.Principal, businessID, professionalID uuid.UUID, on *bool) error {
	if err := s.authorizeProfessional(ctx, p, businessID, professionalID); err != nil {
		return err
	}
	if on == nil {
		return ErrEmailRequired
	}
	return s.d.Store.SetProfessionalEmailEnabled(ctx, businessID, professionalID, *on, s.d.Clock.Now())
}

// WhatsAppLimit is a business's cap and the WhatsApp messages it sent this month.
type WhatsAppLimit struct {
	MonthlyLimit  int
	Default       bool // the platform never set one for the business
	Limited       bool // false: the default is 0, no cap in this environment
	UsedThisMonth int
	MonthStart    time.Time // UTC
}

// BusinessWhatsAppLimit returns a business's cap and usage (platform administrators);
// identity.ErrNotFound for an unknown business.
func (s *Service) BusinessWhatsAppLimit(ctx context.Context, p identity.Principal, businessID uuid.UUID) (WhatsAppLimit, error) {
	if err := s.requireAdminAndBusiness(ctx, p, businessID); err != nil {
		return WhatsAppLimit{}, err
	}
	return s.whatsAppUsage(ctx, businessID, s.d.Clock.Now())
}

// SetBusinessWhatsAppLimit sets a business's cap (0–100000; 0 = no WhatsApp) (platform
// administrators); nil or out of range is ErrInvalidLimit, after the authorization.
func (s *Service) SetBusinessWhatsAppLimit(ctx context.Context, p identity.Principal, businessID uuid.UUID, limit *int) error {
	if err := s.requireAdminAndBusiness(ctx, p, businessID); err != nil {
		return err
	}
	if limit == nil || *limit < 0 || *limit > domain.MaxWhatsAppMonthlyLimit {
		return ErrInvalidLimit
	}
	return s.d.Store.SetWhatsAppMonthlyLimit(ctx, businessID, *limit, s.d.Clock.Now())
}

func (s *Service) requireAdminAndBusiness(ctx context.Context, p identity.Principal, businessID uuid.UUID) error {
	if err := s.d.Authz.RequirePlatformAdmin(ctx, p); err != nil {
		return err
	}
	_, err := s.d.Reminders.BookingPolicyOf(ctx, businessID)
	if errors.Is(err, businesses.ErrBusinessNotFound) {
		return identity.ErrNotFound
	}
	return err
}
