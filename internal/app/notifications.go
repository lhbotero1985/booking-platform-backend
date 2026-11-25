package app

import (
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/infrastructure/postgres"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/infrastructure/simulated"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/infrastructure/whatsapp"
	schedapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/application"
	scheduling "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/contract"
	schedpg "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/infrastructure/postgres"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/config"
)

// NewNotificationsService wires the notification use cases (T7.1). WhatsApp goes through the
// Cloud API of Meta when its credentials are configured (T7.2); otherwise through the simulated
// sender, and nothing leaves the server.
func NewNotificationsService(cfg config.Notifications, pool *pgxpool.Pool, jobs postgres.JobInserter, appointments scheduling.Appointments, logger *slog.Logger) *application.Service {
	return application.NewService(application.Deps{
		Store:        postgres.NewStore(pool, jobs),
		Appointments: appointments,
		Senders:      map[domain.Channel]application.Sender{domain.ChannelWhatsApp: WhatsAppSender(cfg, logger)},
		Policy:       application.Policy{WhatsAppEnabled: cfg.WhatsAppEnabled, WhatsAppAllowlist: cfg.WhatsAppAllowlist},
		Clock:        clock.System{},
	})
}

// WhatsAppSender is the Meta client when the credentials are set, the simulated sender otherwise.
// Only the choice is logged, never the token.
func WhatsAppSender(cfg config.Notifications, logger *slog.Logger) application.Sender {
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
// appointment (no other module is read).
func NewSchedulingReader(pool *pgxpool.Pool, authz identity.Authorizer) scheduling.Appointments {
	return schedapp.NewService(schedapp.Deps{Store: schedpg.NewStore(pool), Authz: authz, Clock: clock.System{}})
}
