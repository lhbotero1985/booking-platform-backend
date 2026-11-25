package app

import (
	"github.com/jackc/pgx/v5/pgxpool"

	businesses "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/contract"
	catalog "github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/contract"
	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	professionals "github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/application"
	scheduling "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/infrastructure/postgres"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
)

// NewSchedulingService wires the scheduling use cases (T6.2, T6.3) with their store and the
// contracts they read: booking policy of public businesses, bookable services, the
// professionals' agendas (and their lock) and the customer's profile.
func NewSchedulingService(pool *pgxpool.Pool, authz identity.Authorizer, biz businesses.PublicBooking, cat catalog.Booking, profs ProfessionalsForScheduling, customers identity.Customers) *application.Service {
	return application.NewService(application.Deps{
		Store: postgres.NewStore(pool), Authz: authz, Businesses: biz, Catalog: cat, Professionals: profs, Bookable: profs,
		Customers: customers, Clock: clock.System{},
	})
}

// ProfessionalsForScheduling is what scheduling reads from the professionals module.
type ProfessionalsForScheduling interface {
	professionals.Agenda
	professionals.Bookable
}

// LinkAgendaGuard lets the professionals module refuse absences and schedules that would break
// active appointments (T6.7). Call once, before serving.
func LinkAgendaGuard(profs interface{ SetAgendaGuard(scheduling.AgendaGuard) }, guard scheduling.AgendaGuard) {
	profs.SetAgendaGuard(guard)
}
