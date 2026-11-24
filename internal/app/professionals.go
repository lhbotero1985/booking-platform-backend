package app

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/infrastructure/postgres"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
)

// NewProfessionalsService wires the professionals use cases with identity (authorization,
// membership by email in the same transaction, membership state of each profile).
func NewProfessionalsService(pool *pgxpool.Pool, identity application.Identity) *application.Service {
	return application.NewService(postgres.NewStore(pool), identity, clock.System{})
}
