package app

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/infrastructure/postgres"
	geocontract "github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/contract"
	identitycontract "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	subscriptionscontract "github.com/lhbotero1985/booking-platform-backend/internal/modules/subscriptions/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
)

// IdentityForBusinesses is what the businesses module needs from identity (contract only).
type IdentityForBusinesses interface {
	application.Authorizer
	identitycontract.MemberAdder
	identitycontract.OwnershipClaimer
}

// NewBusinessesService wires the businesses use cases with identity (authorization, OWNER
// membership in the same transaction), geography (city → division, country, time zone,
// currency) and subscriptions (7-day demo in the same transaction).
func NewBusinessesService(pool *pgxpool.Pool, identity IdentityForBusinesses, places geocontract.Places, subs SubscriptionsForBusinesses) *application.Service {
	return application.NewService(application.Deps{
		Store: postgres.NewStore(pool), Authz: identity, Members: identity, Owners: identity, Trials: subs,
		Summaries: subs, Entitlements: subs, Places: places, Clock: clock.System{},
	})
}

// SubscriptionsForBusinesses is what the businesses module needs from subscriptions.
type SubscriptionsForBusinesses interface {
	subscriptionscontract.Trials
	subscriptionscontract.Summaries
	subscriptionscontract.Entitlements
}
