package app

import (
	businesses "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/contract"
	catalog "github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/contract"
	professionals "github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
)

// NewSchedulingService wires the scheduling use cases with the contracts they read (T6.2):
// booking policy of public businesses, bookable services and the professionals' agendas.
func NewSchedulingService(biz businesses.PublicBooking, cat catalog.Booking, profs professionals.Agenda) *application.Service {
	return application.NewService(application.Deps{Businesses: biz, Catalog: cat, Professionals: profs, Clock: clock.System{}})
}
