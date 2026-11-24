//go:build integration

package app

import (
	"fmt"
	"io"
	"log/slog"
	"testing"

	bizapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/application"
	idapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application/apptest"
	identitycontract "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	idpg "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/infrastructure/postgres"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/pgtest"
)

// TestPublicSearchOnPostgres: the search SQL (ILIKE with escaped wildcards, keyset on
// lower(name)) and the subscriptions filter, as wired in cmd/api.
func TestPublicSearchOnPostgres(t *testing.T) {
	ctx := t.Context()
	pool := migratedPool(t)
	identity := idapp.NewService(idapp.Deps{
		Store: idpg.NewStore(pool, nil), Hasher: &apptest.Hasher{}, Mailer: &apptest.Mailer{}, Clock: clock.System{},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	biz := NewBusinessesService(pool, identity, NewGeographyService(pool, identity), NewSubscriptionsService(pool, identity))

	names := []string{"Ñandú Barber", "alfa cortes", "Barbería 50% off", "Barbería Zeta", "Uñas Spa", "Vencida"}
	ids := map[string]string{}
	for _, n := range names {
		userID, _ := activeUser(t, pool, "ACTIVE")
		b, err := biz.CreateOwnBusiness(ctx, identitycontract.Principal{UserID: userID}, bizapp.BusinessInput{
			Name: n, ContactPhone: "3001234567", CityID: pgtest.SeedCaliID, AddressLine: "Calle 1",
		})
		if err != nil {
			t.Fatalf("%s: %v", n, err)
		}
		ids[n] = b.ID().String()
	}
	// One demo expired, one business suspended.
	if _, err := pool.Exec(ctx, `UPDATE subscriptions SET trial_ends_at = now() - interval '1 second' WHERE business_id = $1`, ids["Vencida"]); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE businesses SET status = 'SUSPENDED' WHERE id = $1`, ids["Barbería Zeta"]); err != nil {
		t.Fatal(err)
	}

	search := func(q bizapp.SearchQuery) string {
		t.Helper()
		var all []string
		for pages := 0; ; pages++ {
			items, next, err := biz.SearchPublic(ctx, q)
			if err != nil || pages > 10 {
				t.Fatalf("search %+v: %v", q, err)
			}
			for _, b := range items {
				all = append(all, b.Name())
			}
			if next == "" {
				return fmt.Sprint(all)
			}
			q.Cursor = next
		}
	}
	q := bizapp.SearchQuery{CityID: pgtest.SeedCaliID, Limit: 2}
	// Order follows the database collation (ñ sorts with n).
	if got := search(q); got != "[alfa cortes Barbería 50% off Ñandú Barber Uñas Spa]" {
		t.Errorf("paged search = %s (expired and suspended hidden, ordered by lower(name))", got)
	}
	for text, want := range map[string]string{
		"barb":     "[Barbería 50% off Ñandú Barber]",
		"50%":      "[Barbería 50% off]",
		"uñas_spa": "[]", // "_" is literal, not a one-character wildcard matching the space
		"ÑAS SP":   "[Uñas Spa]",
		"ÑANDÚ":    "[Ñandú Barber]",
	} {
		if got := search(bizapp.SearchQuery{CityID: pgtest.SeedCaliID, Text: text}); got != want {
			t.Errorf("q=%q: %s, want %s", text, got, want)
		}
	}
	if b, err := biz.GetPublic(ctx, "nandu-barber"); err != nil || b.Name() != "Ñandú Barber" {
		t.Errorf("by slug: %v %v", b, err)
	}
	for _, slug := range []string{"vencida", "barberia-zeta"} {
		if _, err := biz.GetPublic(ctx, slug); err == nil {
			t.Errorf("%s must not be public", slug)
		}
	}
}
