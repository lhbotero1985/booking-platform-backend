package app

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	businesses "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/contract"
	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/infrastructure/email"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/infrastructure/postgres"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/infrastructure/simulated"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/infrastructure/whatsapp"
	professionals "github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/contract"
	schedapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/application"
	scheduling "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/contract"
	schedpg "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/infrastructure/postgres"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/config"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/mail"
)

// NewNotificationsService wires the notification use cases (T7.1). WhatsApp goes through the
// Cloud API of Meta when its credentials are configured (T7.2); otherwise through the simulated
// sender, and nothing leaves the server. Emails (T7.6) go through the SMTP server of the
// environment (Mailpit in QA).
func NewNotificationsService(cfg config.Config, pool *pgxpool.Pool, jobs postgres.JobInserter, appointments SchedulingForNotifications,
	reminders application.ReminderSettings, recipients application.Recipients, logger *slog.Logger,
) (*application.Service, error) {
	n := cfg.Notifications
	wa := WhatsAppSender(n, logger)
	mailer, err := mail.NewSMTP(mail.Config{
		Host: cfg.SMTP.Host, Port: cfg.SMTP.Port, From: cfg.SMTP.From,
		Username: cfg.SMTP.Username, Password: cfg.SMTP.Password, TLSMode: cfg.SMTP.TLSMode,
	})
	if err != nil {
		return nil, err
	}
	return application.NewService(application.Deps{
		Store:        postgres.NewStore(pool, jobs),
		Appointments: appointments,
		Replies:      appointments,
		TextSender:   wa,
		Reminders:    reminders,
		Recipients:   recipients,
		Senders: map[domain.Channel]application.Sender{
			domain.ChannelWhatsApp: wa,
			domain.ChannelEmail: email.New(mailer, email.Config{
				AppDownloadURL: cfg.AppDownloadURL, LinksBaseURL: cfg.PublicAPIURL, RescheduleURL: cfg.AppointmentRescheduleURL,
			}),
		},
		Policy: application.Policy{EmailEnabled: n.EmailEnabled, WhatsAppEnabled: n.WhatsAppEnabled, WhatsAppAllowlist: n.WhatsAppAllowlist},
		Clock:  clock.System{},
	}), nil
}

// NewEmailRecipients resolves the addresses of the appointment emails (T7.6): an account's email
// in identity, a professional's account in professionals.
func NewEmailRecipients(accounts identity.Accounts, profs professionals.Agenda) application.Recipients {
	return emailRecipients{accounts: accounts, profs: profs}
}

type emailRecipients struct {
	accounts identity.Accounts
	profs    professionals.Agenda
}

func (r emailRecipients) AccountEmail(ctx context.Context, userID uuid.UUID) (string, error) {
	return r.accounts.AccountEmail(ctx, userID)
}

func (r emailRecipients) ProfessionalEmail(ctx context.Context, businessID, professionalID uuid.UUID) (string, error) {
	userID, err := r.profs.ProfessionalUserID(ctx, businessID, professionalID)
	if errors.Is(err, professionals.ErrProfessionalNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return r.accounts.AccountEmail(ctx, userID)
}

// SchedulingForNotifications is what notifications reads and does in scheduling.
type SchedulingForNotifications interface {
	scheduling.Appointments
	scheduling.CustomerReplies
}

// WhatsAppDelivery sends templates and free-text answers.
type WhatsAppDelivery interface {
	application.Sender
	application.TextSender
}

// WhatsAppSender is the Meta client when the credentials are set, the simulated sender otherwise.
// Only the choice is logged, never the token.
func WhatsAppSender(cfg config.Notifications, logger *slog.Logger) WhatsAppDelivery {
	if !cfg.WhatsAppConfigured() {
		logger.Info("whatsapp sender", slog.String("provider", "simulated"), slog.Bool("enabled", cfg.WhatsAppEnabled))
		return simulated.New(logger)
	}
	logger.Info("whatsapp sender", slog.String("provider", "meta"), slog.Bool("enabled", cfg.WhatsAppEnabled),
		slog.String("graph_version", cfg.WhatsAppGraphVersion), slog.Int("allowlist", len(cfg.WhatsAppAllowlist)))
	return whatsapp.New(whatsapp.Config{
		BaseURL: cfg.WhatsAppAPIBaseURL, GraphVersion: cfg.WhatsAppGraphVersion,
		PhoneNumberID: cfg.WhatsAppPhoneNumberID, AccessToken: cfg.WhatsAppAccessToken,
	})
}

// LinkNotifications makes every recorded change of an appointment queue its messages, in the
// same transaction. Call once, before serving.
func LinkNotifications(sched interface {
	SetEventListener(scheduling.EventListener)
}, l scheduling.EventListener) {
	sched.SetEventListener(l)
}

// NewSchedulingReader is the part of scheduling the worker needs: the current state of an
// appointment with the names of its business and professional (T7.3), and the customer's answers
// to the reminder's buttons (T7.5).
func NewSchedulingReader(pool *pgxpool.Pool, authz identity.Authorizer, biz businesses.PublicBooking, profs professionals.Agenda) *schedapp.Service {
	return schedapp.NewService(schedapp.Deps{Store: schedpg.NewStore(pool), Authz: authz, Businesses: biz, Professionals: profs, Clock: clock.System{}})
}
