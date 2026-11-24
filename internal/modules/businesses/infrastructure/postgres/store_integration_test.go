//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/infrastructure/postgres"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/db"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/migrate"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/pgtest"
)

var pool *pgxpool.Pool

func TestMain(m *testing.M) {
	ctx := context.Background()
	c, err := pgtest.Start(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := func() int {
		defer c.Terminate()
		p, err := pgxpool.New(ctx, c.URL)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		defer p.Close()
		sqlDB := stdlib.OpenDBFromPool(p)
		defer sqlDB.Close()
		mg, err := migrate.New(sqlDB)
		if err == nil {
			_, err = mg.Up(ctx)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		pool = p
		return m.Run()
	}()
	os.Exit(code)
}

// Seed geography (migration 00005).
var (
	valle     = pgtest.SeedValleID
	cali      = pgtest.SeedCaliID
	antioquia = uuid.MustParse("01a20aae-7e5a-7f3e-953c-9ec690399d8d")
	medellin  = uuid.MustParse("01a20aae-7e61-7c3f-898e-2a023921bf54")
)

func newBusiness(t *testing.T, slug domain.Slug, city, division uuid.UUID, coords *domain.Coordinates) *domain.Business {
	t.Helper()
	b, err := domain.NewBusiness(domain.NewBusinessParams{
		ID: uuid.Must(uuid.NewV7()), Name: "Barbería " + string(slug), Slug: slug, Status: domain.StatusActive,
		ContactPhone: "+573001234567", Description: "Cortes clásicos",
		Location: domain.Location{CountryCode: "CO", DivisionID: division, CityID: city, AddressLine: "Calle 5 # 38-25", Coordinates: coords},
		TimeZone: "America/Bogota", Currency: "COP", Policy: domain.DefaultBookingPolicy(), Reminders: domain.DefaultReminders(),
		Now: time.Now().UTC().Truncate(time.Microsecond),
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func uniqueSlug(prefix string) domain.Slug {
	return domain.Slug(prefix + "-" + uuid.NewString()[:8])
}

func TestBusinessRoundTrip(t *testing.T) {
	ctx := t.Context()
	st := postgres.NewStore(pool)
	b := newBusiness(t, uniqueSlug("paisa"), cali, valle, &domain.Coordinates{Lat: 3.4516, Lng: -76.532})

	err := st.WithinTx(ctx, func(_ context.Context, tx application.Tx) error { return tx.InsertBusiness(ctx, b) })
	if err != nil {
		t.Fatal(err)
	}
	var got, bySlug *domain.Business
	err = st.WithinTx(ctx, func(_ context.Context, tx application.Tx) (err error) {
		if got, err = tx.BusinessByID(ctx, b.ID()); err != nil {
			return err
		}
		bySlug, err = tx.BusinessBySlug(ctx, domain.Slug(string(b.Slug())))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	gl, bl := got.Location(), b.Location()
	if got.Name() != b.Name() || gl.CountryCode != bl.CountryCode || gl.DivisionID != bl.DivisionID ||
		gl.CityID != bl.CityID || gl.AddressLine != bl.AddressLine || got.ContactPhone() != b.ContactPhone() ||
		got.Description() != b.Description() || got.Status() != b.Status() || got.Policy() != b.Policy() || got.Reminders() != b.Reminders() || got.TimeZone() != "America/Bogota" ||
		!got.CreatedAt().Equal(b.CreatedAt()) || bySlug.ID() != b.ID() {
		t.Errorf("round trip:\n got %+v\nwant %+v", got, b)
	}
	if c := got.Location().Coordinates; c == nil || c.Lat != 3.4516 || c.Lng != -76.532 {
		t.Errorf("coordinates = %+v", c)
	}
	// PostGIS location is derived from the coordinates.
	var wkt string
	if err := pool.QueryRow(ctx, `SELECT ST_AsText(location) FROM businesses WHERE id = $1`, b.ID()).Scan(&wkt); err != nil || wkt != "POINT(-76.532 3.4516)" {
		t.Errorf("location = %q, %v", wkt, err)
	}

	// Update (suspend + move within the country) and read back.
	b.Suspend(time.Now().UTC().Truncate(time.Microsecond))
	moved, err := domain.RehydrateBusiness(domain.NewBusinessParams{
		ID: b.ID(), Name: "Nuevo nombre", Slug: b.Slug(), Status: b.Status(), ContactPhone: "+576024861234",
		Location: domain.Location{CountryCode: "CO", DivisionID: antioquia, CityID: medellin, AddressLine: "Cra 70 # 1-1"},
		TimeZone: "America/Bogota", Currency: "COP", Policy: domain.BookingPolicy{SlotIntervalMinutes: 15, MaxAdvanceDays: 30},
		Reminders: domain.Reminders{HoursBefore: 2}, Now: b.CreatedAt(),
	}, b.CreatedAt(), b.UpdatedAt())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.WithinTx(ctx, func(_ context.Context, tx application.Tx) error { return tx.UpdateBusiness(ctx, moved) }); err != nil {
		t.Fatal(err)
	}
	_ = st.WithinTx(ctx, func(_ context.Context, tx application.Tx) (err error) {
		got, err = tx.BusinessByID(ctx, b.ID())
		return err
	})
	if got.Status() != domain.StatusSuspended || got.Location().CityID != medellin || got.Location().Coordinates != nil ||
		got.Policy().SlotIntervalMinutes != 15 || got.Reminders().Enabled || got.Name() != "Nuevo nombre" {
		t.Errorf("after update: %+v", got)
	}

	if err := st.WithinTx(ctx, func(_ context.Context, tx application.Tx) error {
		_, err := tx.BusinessByID(ctx, uuid.Must(uuid.NewV7()))
		return err
	}); !errors.Is(err, application.ErrNotFound) {
		t.Errorf("unknown id: %v", err)
	}
}

func TestSlugUniquenessAndAllocation(t *testing.T) {
	ctx := t.Context()
	st := postgres.NewStore(pool)
	slug := uniqueSlug("unica")
	insert := func(b *domain.Business) error {
		return st.WithinTx(ctx, func(_ context.Context, tx application.Tx) error { return tx.InsertBusiness(ctx, b) })
	}
	if err := insert(newBusiness(t, slug, cali, valle, nil)); err != nil {
		t.Fatal(err)
	}
	if err := insert(newBusiness(t, slug, cali, valle, nil)); !errors.Is(err, application.ErrSlugTaken) {
		t.Errorf("duplicate slug: %v, want ErrSlugTaken", err)
	}
	// citext: case-insensitive uniqueness (the domain only produces lower case, the index is the backstop).
	if _, err := pool.Exec(ctx, `UPDATE businesses SET slug = upper(slug) WHERE slug = $1`, string(slug)); err == nil {
		t.Error("upper-case slug accepted by the schema")
	}

	name := "Uñas Spa " + uuid.NewString()[:6]
	var first, second domain.Slug
	err := st.WithinTx(ctx, func(_ context.Context, tx application.Tx) (err error) {
		if first, err = application.AllocateSlug(ctx, tx, name); err != nil {
			return err
		}
		return tx.InsertBusiness(ctx, newBusiness(t, first, cali, valle, nil))
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = st.WithinTx(ctx, func(_ context.Context, tx application.Tx) (err error) {
		second, err = application.AllocateSlug(ctx, tx, name)
		return err
	})
	if second != first.WithSuffix(2) {
		t.Errorf("allocation after %q = %q, want the -2 suffix", first, second)
	}
}

func TestSchemaEnforcesLocationAndTenancy(t *testing.T) {
	ctx := t.Context()
	st := postgres.NewStore(pool)

	// City of another division (Medellín is in Antioquia, not Valle).
	bad := newBusiness(t, uniqueSlug("mal"), medellin, valle, nil)
	if err := st.WithinTx(ctx, func(_ context.Context, tx application.Tx) error { return tx.InsertBusiness(ctx, bad) }); db.SQLState(err) != db.CodeForeignKeyViolation {
		t.Errorf("city outside its division accepted: %v", err)
	}
	// Invalid values the domain would reject never get past the schema either.
	for name, q := range map[string]string{
		"phone not E.164":  `UPDATE businesses SET contact_phone = '3001234567' WHERE id = $1`,
		"slot interval 7":  `UPDATE businesses SET slot_interval_minutes = 7 WHERE id = $1`,
		"half coordinates": `UPDATE businesses SET latitude = 3.4 WHERE id = $1`,
		"unknown status":   `UPDATE businesses SET status = 'DELETED' WHERE id = $1`,
		"slug with --":     `UPDATE businesses SET slug = 'a--b' WHERE id = $1`,
	} {
		b := newBusiness(t, uniqueSlug("chk"), cali, valle, nil)
		if err := st.WithinTx(ctx, func(_ context.Context, tx application.Tx) error { return tx.InsertBusiness(ctx, b) }); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, q, b.ID()); db.SQLState(err) != db.CodeCheckViolation {
			t.Errorf("%s: %v, want check violation", name, err)
		}
	}

	// memberships.business_id now references businesses (RESTRICT on delete).
	var userID uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO users (id, email, password_hash, full_name, status, locale)
		VALUES (gen_random_uuid(), $1, '$argon2id$test', 'Ana', 'ACTIVE', 'es') RETURNING id`, "fk-"+uuid.NewString()[:8]+"@example.com").Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO memberships (id, user_id, business_id, role) VALUES (gen_random_uuid(), $1, gen_random_uuid(), 'OWNER')`, userID); db.SQLState(err) != db.CodeForeignKeyViolation {
		t.Errorf("membership of a nonexistent business: %v", err)
	}
	owned := newBusiness(t, uniqueSlug("owned"), cali, valle, nil)
	if err := st.WithinTx(ctx, func(_ context.Context, tx application.Tx) error { return tx.InsertBusiness(ctx, owned) }); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO memberships (id, user_id, business_id, role) VALUES (gen_random_uuid(), $1, $2, 'OWNER')`, userID, owned.ID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM businesses WHERE id = $1`, owned.ID()); db.SQLState(err) != db.CodeForeignKeyViolation {
		t.Errorf("deleting a business with members: %v", err)
	}
	// audit_log.business_id has no foreign key: denials of unknown businesses are kept.
	if _, err := pool.Exec(ctx, `INSERT INTO audit_log (id, occurred_at, business_id, action, outcome, metadata)
		VALUES (gen_random_uuid(), now(), gen_random_uuid(), 'authz.denied', 'DENIED', '{}')`); err != nil {
		t.Errorf("audit of an unknown business rejected: %v", err)
	}
}
