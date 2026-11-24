package app

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/subscriptions/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/subscriptions/infrastructure/postgres"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
)

// NewSubscriptionsService wires the subscriptions use cases (ADR-0018).
func NewSubscriptionsService(pool *pgxpool.Pool, authz application.Authorizer) *application.Service {
	return application.NewService(postgres.NewStore(pool), authz, clock.System{})
}
