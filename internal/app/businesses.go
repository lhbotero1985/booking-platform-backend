package app

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/infrastructure/postgres"
	geocontract "github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/contract"
	identitycontract "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
)

// IdentityForBusinesses is what the businesses module needs from identity (contract only).
type IdentityForBusinesses interface {
	application.Authorizer
	identitycontract.MemberAdder
}

// NewBusinessesService wires the businesses use cases with identity (authorization, OWNER
// membership in the same transaction) and geography (city → division, country, time zone,
// currency).
func NewBusinessesService(pool *pgxpool.Pool, identity IdentityForBusinesses, places geocontract.Places) *application.Service {
	return application.NewService(application.Deps{
		Store: postgres.NewStore(pool), Authz: identity, Members: identity, Places: places, Clock: clock.System{},
	})
}
