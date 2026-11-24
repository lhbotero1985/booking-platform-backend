//go:build integration

package app

import (
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	bizapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/domain"
	geoapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/application"
	idapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application/apptest"
	identitycontract "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	idpg "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/infrastructure/postgres"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/pgtest"
)

// activeUser inserts a verified account directly (registration is covered elsewhere).
func activeUser(t *testing.T, pool *pgxpool.Pool, status string) (uuid.UUID, string) {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	email := "u-" + id.String()[24:] + "@example.com" // random tail (the head of a UUIDv7 is the time)
	if _, err := pool.Exec(t.Context(), `INSERT INTO users (id, email, password_hash, full_name, status, locale, email_verified_at)
		VALUES ($1, $2, '$argon2id$test', 'Dueño', $3, 'es', now())`, id, email, status); err != nil {
		t.Fatal(err)
	}
	return id, email
}

func count(t *testing.T, pool *pgxpool.Pool, q string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestBusinessOnboardingOnPostgres: the composition of cmd/api (identity + geography +
// businesses on one database). Business, OWNER membership and audit commit together.
func TestBusinessOnboardingOnPostgres(t *testing.T) {
	ctx := t.Context()
	pool := migratedPool(t)
	identity := idapp.NewService(idapp.Deps{
		Store: idpg.NewStore(pool, nil), Hasher: &apptest.Hasher{}, Mailer: &apptest.Mailer{}, Clock: clock.System{},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	geo := NewGeographyService(pool, identity)
	biz := NewBusinessesService(pool, identity, geo)
	admin := identitycontract.Principal{UserID: uuid.Must(uuid.NewV7()), IsPlatformAdmin: true}
	ownerID, ownerEmail := activeUser(t, pool, "ACTIVE")
	_, pendingEmail := activeUser(t, pool, "PENDING_VERIFICATION")

	in := bizapp.CreateInput{
		Name: "Barbería El Paisa", ContactPhone: "3001234567", CityID: pgtest.SeedCaliID,
		AddressLine: "Calle 5 # 38-25", OwnerEmail: ownerEmail,
	}
	b, err := biz.CreateBusiness(ctx, admin, in)
	if err != nil {
		t.Fatal(err)
	}
	if b.Status() != domain.StatusPendingReview || b.Location().DivisionID != valle || b.Currency() != "COP" {
		t.Errorf("business = %+v", b)
	}
	if n := count(t, pool, `SELECT count(*) FROM memberships WHERE business_id = $1 AND user_id = $2 AND role = 'OWNER' AND status = 'ACTIVE'`, b.ID(), ownerID); n != 1 {
		t.Errorf("owner memberships = %d", n)
	}
	if n := count(t, pool, `SELECT count(*) FROM audit_log WHERE business_id = $1 AND action IN ('business.created', 'membership.granted')`, b.ID()); n != 2 {
		t.Errorf("audit entries = %d, want business.created + membership.granted", n)
	}

	// The owner is not eligible: nothing of the business remains (one transaction).
	before := count(t, pool, `SELECT count(*) FROM businesses`)
	in.Name, in.OwnerEmail = "Peluquería Atómica", pendingEmail
	var verr *bizapp.ValidationError
	if _, err := biz.CreateBusiness(ctx, admin, in); !errors.As(err, &verr) || verr.Fields[0].Code != bizapp.CodeAccountNotEligible {
		t.Fatalf("unverified owner: %v", err)
	}
	if after := count(t, pool, `SELECT count(*) FROM businesses`); after != before {
		t.Errorf("business persisted without its owner: %d → %d", before, after)
	}
	if n := count(t, pool, `SELECT count(*) FROM audit_log WHERE action = 'business.created' AND metadata->>'city' = $1`, pgtest.SeedCaliID.String()); n != 1 {
		t.Errorf("business.created entries = %d, want only the committed one", n)
	}

	// Activation, membership visible to identity's authorizer, and location rule.
	if _, err := identity.RequireBusinessRole(ctx, identitycontract.Principal{UserID: ownerID}, b.ID(), identitycontract.RolesOwner...); err != nil {
		t.Errorf("owner role not visible to the authorizer: %v", err)
	}
	if got, err := biz.ActivateBusiness(ctx, admin, b.ID()); err != nil || got.Status() != domain.StatusActive {
		t.Fatalf("activate: %v %v", got, err)
	}
	if _, err := biz.SuspendBusiness(ctx, admin, b.ID()); err != nil {
		t.Fatal(err)
	}
	off := false
	if _, err := geo.UpdateCity(ctx, admin, pgtest.SeedCaliID, geoapp.CityPatch{Active: &off}); err != nil {
		t.Fatalf("deactivate Cali (business suspended): %v", err)
	}
	if _, err := biz.ActivateBusiness(ctx, admin, b.ID()); !errors.Is(err, bizapp.ErrLocationInactive) {
		t.Errorf("activation in an inactive city: %v", err)
	}
	on := true
	if _, err := geo.UpdateCity(ctx, admin, pgtest.SeedCaliID, geoapp.CityPatch{Active: &on}); err != nil {
		t.Fatal(err)
	}

	items, next, err := biz.ListBusinesses(ctx, admin, "SUSPENDED", pgtest.SeedCaliID, uuid.Nil, 10)
	if err != nil || len(items) != 1 || items[0].ID() != b.ID() || next != uuid.Nil {
		t.Errorf("list = %d items, next %v, %v", len(items), next, err)
	}
}
