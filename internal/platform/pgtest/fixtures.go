//go:build integration

package pgtest

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

// Execer is satisfied by *pgxpool.Pool, *pgx.Conn and pgx.Tx.
type Execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Seed geography of migration 00005 (Cali, Valle del Cauca, Colombia).
var (
	SeedValleID = uuid.MustParse("01a20aae-7e56-796c-9de9-5409ce51e7d4")
	SeedCaliID  = uuid.MustParse("01a20aae-7e5d-73d7-b7d3-a11c5afb9655")
)

// InsertBusiness creates a minimal valid ACTIVE business in Cali with the given ID, for tests
// of other modules that need a tenant row (memberships reference businesses).
func InsertBusiness(ctx context.Context, db Execer, id uuid.UUID) error {
	slug := "test-" + strings.ReplaceAll(id.String(), "-", "")[:20]
	_, err := db.Exec(ctx, `INSERT INTO businesses (id, name, slug, status, contact_phone, country_code, division_id,
		city_id, address_line, time_zone, currency, created_at, updated_at)
		VALUES ($1, 'Test business', $2, 'ACTIVE', '+573001234567', 'CO', $3, $4, 'Calle 1 # 2-3', 'America/Bogota', 'COP', now(), now())`,
		id, slug, SeedValleID, SeedCaliID)
	return err
}
