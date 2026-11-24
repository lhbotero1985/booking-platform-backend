package app

import (
	"github.com/jackc/pgx/v5/pgxpool"

	businesses "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/infrastructure/postgres"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
)

// NewCatalogService wires the catalog use cases with identity (authorization) and businesses
// (currency of the prices).
func NewCatalogService(pool *pgxpool.Pool, identity application.Identity, currencies businesses.Currencies) *application.Service {
	return application.NewService(postgres.NewStore(pool), identity, currencies, clock.System{})
}
