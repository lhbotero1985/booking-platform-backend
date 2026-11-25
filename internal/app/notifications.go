package app

import (
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	businesses "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/contract"
	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/infrastructure/postgres"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/infrastructure/simulated"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/infrastructure/whatsapp"
	professionals "github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/contract"
	schedapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/application"
	scheduling "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/contract"
	schedpg "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/infrastructure/postgres"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/config"
)

// NewNotificationsService wires the notification use cases (T7.1). WhatsApp goes through the
// Cloud API of Meta when its credentials are configured (T7.2); otherwise through the simulated
// sender, and nothing leaves the server.
func NewNotificationsService(cfg config.Notifications, pool *pgxpool.Pool, jobs postgres.JobInserter, appointments SchedulingForNotifications,
	reminders application.ReminderSettings, logger *slog.Logger,
) *application.Service {
	wa := WhatsAppSender(cfg, logger)
	return application.NewService(application.Deps{
		Store:        postgres.NewStore(pool, jobs),
		Appointments: appointments,
		Replies:      appointments,
		TextSender:   wa,
		Reminders:    reminders,
		Senders:      map[domain.Channel]application.Sender{domain.ChannelWhatsApp: wa},
		Policy:       application.Policy{WhatsAppEnabled: cfg.WhatsAppEnabled, WhatsAppAllowlist: cfg.WhatsAppAllowlist},
		Clock:        clock.System{},
	})
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
