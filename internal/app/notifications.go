package app

import (
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/infrastructure/postgres"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/infrastructure/simulated"
	schedapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/application"
	scheduling "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/contract"
	schedpg "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/infrastructure/postgres"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/config"
)

// NewNotificationsService wires the notification use cases (T7.1). Until a provider is
// configured (T7.2) WhatsApp goes through the simulated sender: nothing leaves the server.
func NewNotificationsService(cfg config.Notifications, pool *pgxpool.Pool, jobs postgres.JobInserter, appointments scheduling.Appointments, logger *slog.Logger) *application.Service {
	return application.NewService(application.Deps{
		Store:        postgres.NewStore(pool, jobs),
		Appointments: appointments,
		Senders:      map[domain.Channel]application.Sender{domain.ChannelWhatsApp: simulated.New(logger)},
		Policy:       application.Policy{WhatsAppEnabled: cfg.WhatsAppEnabled, WhatsAppAllowlist: cfg.WhatsAppAllowlist},
		Clock:        clock.System{},
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
