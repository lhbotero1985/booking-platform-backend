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
func NewSchedulingService(pool *pgxpool.Pool, id IdentityForScheduling, biz BusinessesForScheduling, cat catalog.Booking, profs ProfessionalsForScheduling) *application.Service {
	return application.NewService(application.Deps{
		Store: postgres.NewStore(pool), Authz: id, Businesses: biz, Catalog: cat, Professionals: profs, Bookable: profs,
		Customers: id, Clock: clock.System{}, Schedules: profs, Currencies: biz, Memberships: id,
	})
}

// IdentityForScheduling is what scheduling reads from the identity module.
type IdentityForScheduling interface {
	identity.Authorizer
	identity.Customers
	identity.Memberships
}

// BusinessesForScheduling is what scheduling reads from the businesses module.
type BusinessesForScheduling interface {
	businesses.PublicBooking
	businesses.Currencies
}

// ProfessionalsForScheduling is what scheduling reads from the professionals module.
type ProfessionalsForScheduling interface {
	professionals.Agenda
	professionals.Bookable
	professionals.Schedules
}

// LinkAgendaGuard lets the professionals module refuse absences and schedules that would break
// active appointments (T6.7). Call once, before serving.
func LinkAgendaGuard(profs interface{ SetAgendaGuard(scheduling.AgendaGuard) }, guard scheduling.AgendaGuard) {
	profs.SetAgendaGuard(guard)
}
