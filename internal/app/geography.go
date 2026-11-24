package app

import (
	"github.com/jackc/pgx/v5/pgxpool"

	bizpg "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/infrastructure/postgres"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/infrastructure/postgres"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
)

// NewGeographyService wires the geography use cases. authz is the identity module's authorizer
// (contract), which also audits denied attempts; the businesses store answers whether an area
// still has active businesses before it is deactivated.
func NewGeographyService(pool *pgxpool.Pool, authz application.Authorizer) *application.Service {
	return application.NewService(postgres.NewStore(pool), authz, bizpg.NewStore(pool), clock.System{})
}
