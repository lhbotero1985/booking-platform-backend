package app

import (
	"github.com/jackc/pgx/v5/pgxpool"

	bizapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/application"
	businesses "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/infrastructure/postgres"
	professionals "github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
)

// NewCatalogService wires the catalog use cases with identity (authorization) and businesses
// (currency of the prices) and professionals (professional of the business, T5.6).
func NewCatalogService(pool *pgxpool.Pool, identity application.Identity, currencies businesses.Currencies, profs professionals.Directory) *application.Service {
	return application.NewService(postgres.NewStore(pool), identity, currencies, profs, clock.System{})
}

// LinkPublicCatalog connects the public view (T5.7): the catalog resolves public slugs through
// businesses and bookable professionals through professionals, and the public search of
// businesses lists only those with something to book. Call once, before serving.
func LinkPublicCatalog(biz *bizapp.Service, catalog *application.Service, profs professionals.Bookable, directory businesses.PublicDirectory) {
	catalog.SetPublicDeps(application.PublicDeps{Directory: directory, Professionals: profs})
	biz.SetOfferings(catalog)
}
