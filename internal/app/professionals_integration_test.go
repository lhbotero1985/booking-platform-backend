//go:build integration

package app

import (
	"errors"
	"io"
	"log/slog"
	"testing"

	bizapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/application"
	idapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application/apptest"
	identitycontract "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	idpg "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/infrastructure/postgres"
	profapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/pgtest"
)

// TestProfessionalsOnPostgres: a barber added by email becomes a PROFESSIONAL member and gets a
// profile in one transaction; the OWNER can also be a professional; one profile per account and
// business; a removed member's profile is not bookable.
func TestProfessionalsOnPostgres(t *testing.T) {
	ctx := t.Context()
	pool := migratedPool(t)
	identity := idapp.NewService(idapp.Deps{
		Store: idpg.NewStore(pool, nil), Hasher: &apptest.Hasher{}, Mailer: &apptest.Mailer{}, Clock: clock.System{},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	biz := NewBusinessesService(pool, identity, NewGeographyService(pool, identity), NewSubscriptionsService(pool, identity))
	profs := NewProfessionalsService(pool, identity)

	ownerID, ownerEmail := activeUser(t, pool, "ACTIVE")
	barberID, barberEmail := activeUser(t, pool, "ACTIVE")
	owner := identitycontract.Principal{UserID: ownerID}
	b, err := biz.CreateOwnBusiness(ctx, owner, bizapp.BusinessInput{Name: "Barbería Equipo", ContactPhone: "3001234567", CityID: pgtest.SeedCaliID, AddressLine: "Calle 1"})
	if err != nil {
		t.Fatal(err)
	}

	v, err := profs.Create(ctx, owner, b.ID(), profapp.CreateInput{Email: barberEmail, DisplayName: "Juan"})
	if err != nil || v.UserID != barberID || !v.Bookable {
		t.Fatalf("create barber: %+v %v", v, err)
	}
	if n := count(t, pool, `SELECT count(*) FROM memberships WHERE business_id = $1 AND user_id = $2 AND role = 'PROFESSIONAL' AND status = 'ACTIVE'`, b.ID(), barberID); n != 1 {
		t.Errorf("barber membership rows = %d", n)
	}
	if _, err := profs.Create(ctx, owner, b.ID(), profapp.CreateInput{Email: ownerEmail, DisplayName: "El Dueño"}); err != nil {
		t.Fatalf("owner as professional: %v", err)
	}
	if n := count(t, pool, `SELECT count(*) FROM memberships WHERE business_id = $1 AND user_id = $2 AND role = 'OWNER' AND status = 'ACTIVE'`, b.ID(), ownerID); n != 1 {
		t.Error("the OWNER lost its role")
	}
	if _, err := profs.Create(ctx, owner, b.ID(), profapp.CreateInput{Email: barberEmail, DisplayName: "Otra vez"}); !errors.Is(err, profapp.ErrAlreadyExists) {
		t.Errorf("duplicate profile: %v", err)
	}
	// The same barber may work in another business too (one profile per business).
	otherOwner, _ := activeUser(t, pool, "ACTIVE")
	b2, err := biz.CreateOwnBusiness(ctx, identitycontract.Principal{UserID: otherOwner}, bizapp.BusinessInput{Name: "Otra Barbería", ContactPhone: "3001234567", CityID: pgtest.SeedCaliID, AddressLine: "Calle 2"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := profs.Create(ctx, identitycontract.Principal{UserID: otherOwner}, b2.ID(), profapp.CreateInput{Email: barberEmail, DisplayName: "Juan"}); err != nil {
		t.Errorf("same barber in a second business: %v", err)
	}

	list, err := profs.List(ctx, identitycontract.Principal{UserID: barberID}, b.ID())
	if err != nil || len(list) != 2 {
		t.Fatalf("barber lists: %d %v", len(list), err)
	}
	// Removing the barber from the business: the profile stays but is no longer bookable.
	if err := identity.RemoveMember(ctx, owner, b.ID(), barberID); err != nil {
		t.Fatal(err)
	}
	got, err := profs.Get(ctx, owner, b.ID(), v.ID)
	if err != nil || got.MemberActive || got.Bookable {
		t.Errorf("after removal: %+v %v", got, err)
	}
	if n := count(t, pool, `SELECT count(*) FROM audit_log WHERE business_id = $1 AND action = 'professional.created'`, b.ID()); n != 2 {
		t.Errorf("professional.created entries = %d", n)
	}
	// Schema: the profile belongs to its business (no cross-business reads by id).
	if n := count(t, pool, `SELECT count(*) FROM professionals WHERE business_id = $1 AND id = $2`, b2.ID(), v.ID); n != 0 {
		t.Error("a professional is visible through another business")
	}
}
