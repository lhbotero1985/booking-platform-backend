//go:build integration

package app

import (
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	bizapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/domain"
	idapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application/apptest"
	identitycontract "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	idpg "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/infrastructure/postgres"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/pgtest"
)

// TestSelfServiceOnboardingOnPostgres (ADR-0018): business ACTIVE + OWNER + 7-day demo commit
// together; at most OWNED_BUSINESS_LIMIT (2) owned businesses per account even under concurrent
// requests (P-11), each with its demo; the Idempotency-Key gives one business; demos expire with
// the real SQL of the hourly job.
func TestSelfServiceOnboardingOnPostgres(t *testing.T) {
	ctx := t.Context()
	pool := migratedPool(t)
	identity := idapp.NewService(idapp.Deps{
		Store: idpg.NewStore(pool, nil), Hasher: &apptest.Hasher{}, Mailer: &apptest.Mailer{}, Clock: clock.System{},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	subs := NewSubscriptionsService(pool, identity)
	biz := NewBusinessesService(pool, identity, NewGeographyService(pool, identity), subs)
	userID, _ := activeUser(t, pool, "ACTIVE")
	owner := identitycontract.Principal{UserID: userID}
	in := bizapp.BusinessInput{Name: "Barbería Autoservicio", ContactPhone: "3001234567", CityID: pgtest.SeedCaliID, AddressLine: "Calle 1"}

	// Three simultaneous requests of the same account: exactly two businesses are created.
	var wg sync.WaitGroup
	results := make([]error, 3)
	created := make([]*domain.Business, 3)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			created[i], results[i] = biz.CreateOwnBusiness(ctx, owner, in)
		}(i)
	}
	wg.Wait()
	var b *domain.Business
	ok, already := 0, 0
	for i, err := range results {
		switch {
		case err == nil:
			ok++
			if b == nil {
				b = created[i]
			}
		case errors.Is(err, identitycontract.ErrOwnedBusinessLimit):
			already++
		default:
			t.Fatalf("request %d: %v", i, err)
		}
	}
	if ok != 2 || already != 1 {
		t.Fatalf("concurrent self-service: %d created, %d refused; want 2 and 1", ok, already)
	}
	if n := count(t, pool, `SELECT count(*) FROM businesses b JOIN memberships m ON m.business_id = b.id
		JOIN subscriptions s ON s.business_id = b.id WHERE m.user_id = $1 AND m.role = 'OWNER' AND s.status = 'TRIALING'`, userID); n != 2 {
		t.Errorf("businesses owned with their demo = %d, want 2", n)
	}
	if b.Status() != domain.StatusActive {
		t.Errorf("status = %s, want ACTIVE", b.Status())
	}
	var status string
	var ends time.Time
	if err := pool.QueryRow(ctx, `SELECT status, trial_ends_at FROM subscriptions WHERE business_id = $1`, b.ID()).Scan(&status, &ends); err != nil {
		t.Fatal(err)
	}
	if status != "TRIALING" || time.Until(ends) < 7*24*time.Hour-time.Minute || time.Until(ends) > 7*24*time.Hour {
		t.Errorf("subscription = %s ending %v", status, ends)
	}
	if n := count(t, pool, `SELECT count(*) FROM audit_log WHERE business_id = $1 AND action IN
		('business.created', 'membership.granted', 'subscription.trial_started')`, b.ID()); n != 3 {
		t.Errorf("audit entries = %d, want 3", n)
	}
	if n := count(t, pool, `SELECT count(*) FROM businesses WHERE name = 'Barbería Autoservicio'`); n != 2 {
		t.Errorf("businesses with that name = %d: the refused request left a row behind", n)
	}
	if entitled, err := subs.Entitled(ctx, b.ID()); err != nil || !entitled {
		t.Errorf("entitled during the demo: %v %v", entitled, err)
	}

	// P-11: three simultaneous requests with the same Idempotency-Key give one business.
	keyUser, _ := activeUser(t, pool, "ACTIVE")
	keyed := in
	keyed.Name, keyed.IdempotencyKey = "Barbería Con Clave", "portal-create-0001"
	ids := make([]uuid.UUID, 3)
	for i := range ids {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			kb, err := biz.CreateOwnBusiness(ctx, identitycontract.Principal{UserID: keyUser}, keyed)
			if err != nil {
				t.Errorf("keyed request %d: %v", i, err)
				return
			}
			ids[i] = kb.ID()
		}(i)
	}
	wg.Wait()
	if ids[0] != ids[1] || ids[1] != ids[2] || ids[0] == uuid.Nil {
		t.Errorf("same key, businesses %v", ids)
	}
	if n := count(t, pool, `SELECT count(*) FROM businesses WHERE name = 'Barbería Con Clave'`); n != 1 {
		t.Errorf("businesses with the same key = %d, want 1", n)
	}
	keyed.Name = "Otro Nombre"
	if _, err := biz.CreateOwnBusiness(ctx, identitycontract.Principal{UserID: keyUser}, keyed); !errors.Is(err, bizapp.ErrIdempotencyKeyReused) {
		t.Errorf("same key, other data: %v", err)
	}

	// The OWNER manages it: moving to Medellín takes Antioquia as division (location FKs).
	medellin := uuid.MustParse("01a20aae-7e61-7c3f-898e-2a023921bf54")
	updated, err := biz.UpdateBusiness(ctx, owner, b.ID(), bizapp.UpdateInput{CityID: &medellin, AddressLine: ptrTo("Cra 70 # 1-1")})
	if err != nil || updated.Location().CityID != medellin {
		t.Fatalf("owner update: %v %v", updated, err)
	}
	var division uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT division_id FROM businesses WHERE id = $1`, b.ID()).Scan(&division); err != nil ||
		division != uuid.MustParse("01a20aae-7e5a-7f3e-953c-9ec690399d8d") {
		t.Errorf("division after moving = %v, %v", division, err)
	}
	m, err := biz.GetManaged(ctx, owner, b.ID())
	if err != nil || m.Subscription.Status != "TRIALING" || !m.Subscription.Entitled {
		t.Errorf("managed view: %+v %v", m.Subscription, err)
	}
	stranger, _ := activeUser(t, pool, "ACTIVE")
	if _, err := biz.UpdateBusiness(ctx, identitycontract.Principal{UserID: stranger}, b.ID(), bizapp.UpdateInput{Name: ptrTo("Ajena")}); !errors.Is(err, identitycontract.ErrNotFound) {
		t.Errorf("stranger update: %v, want ErrNotFound", err)
	}
	if n := count(t, pool, `SELECT count(*) FROM audit_log WHERE business_id = $1 AND action = 'business.updated'
		AND metadata->>'fields' = 'location'`, b.ID()); n != 1 {
		t.Errorf("business.updated entries = %d", n)
	}

	// The demo ends: the hourly job's SQL expires it.
	if _, err := pool.Exec(ctx, `UPDATE subscriptions SET trial_ends_at = now() - interval '1 minute' WHERE business_id = $1`, b.ID()); err != nil {
		t.Fatal(err)
	}
	n, err := subs.ExpireDueTrials(ctx)
	if err != nil || n < 1 {
		t.Fatalf("expire: %d %v", n, err)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM subscriptions WHERE business_id = $1`, b.ID()).Scan(&status); err != nil || status != "EXPIRED" {
		t.Errorf("after expiry: %s %v", status, err)
	}
	if entitled, _ := subs.Entitled(ctx, b.ID()); entitled {
		t.Error("an expired demo must not be entitled")
	}
	if n := count(t, pool, `SELECT count(*) FROM audit_log WHERE business_id = $1 AND action = 'subscription.expired'
		AND actor_user_id IS NULL AND metadata->>'source' = 'job'`, b.ID()); n != 1 {
		t.Errorf("expiry audit entries = %d", n)
	}
	// Deleting a business removes its subscription (CASCADE) — memberships still RESTRICT it.
	if _, err := pool.Exec(ctx, `DELETE FROM businesses WHERE id = $1`, b.ID()); err == nil {
		t.Error("a business with an OWNER must not be deletable")
	}
}

func ptrTo[T any](v T) *T { return &v }
