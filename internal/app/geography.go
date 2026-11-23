package app

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/infrastructure/postgres"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
)

// NewGeographyService wires the geography use cases. authz is the identity module's authorizer
// (contract), which also audits denied attempts.
func NewGeographyService(pool *pgxpool.Pool, authz application.Authorizer) *application.Service {
	return application.NewService(postgres.NewStore(pool), authz, clock.System{})
}
