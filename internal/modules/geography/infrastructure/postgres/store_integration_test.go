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

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/application/geotest"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/infrastructure/postgres"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/audit"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
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

var admin = contract.Principal{UserID: uuid.Must(uuid.NewV7()), IsPlatformAdmin: true}

func newService() *application.Service {
	return application.NewService(postgres.NewStore(pool), geotest.Authorizer{}, nil, clock.System{})
}

func ptr[T any](v T) *T { return &v }

func TestSeedDataIsPresent(t *testing.T) {
	ctx := t.Context()
	svc := newService()
	cs, err := svc.ListCountries(ctx)
	if err != nil || len(cs) == 0 || cs[0].Code != "CO" || cs[0].DefaultCurrency != "COP" || cs[0].PhoneCode != "+57" {
		t.Fatalf("countries = %+v, %v", cs, err)
	}
	ds, err := svc.ListDivisions(ctx, "CO")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, d := range ds {
		got[d.Code] = d.Name
	}
	if got["CO-VAC"] != "Valle del Cauca" || got["CO-ANT"] != "Antioquia" {
		t.Fatalf("divisions = %+v", ds)
	}
	for div, want := range map[uuid.UUID][]string{
		geotest.ValleID:     {"Cali", "Yumbo"},
		geotest.AntioquiaID: {"Itagüí", "Medellín"},
	} {
		cities, err := svc.ListCities(ctx, div)
		if err != nil || len(cities) != 2 {
			t.Fatalf("cities of %s = %+v, %v", div, cities, err)
		}
		for i, c := range cities {
			if c.Name != want[i] || c.TimeZone != "America/Bogota" || !c.Active {
				t.Errorf("city %d = %+v, want %s in America/Bogota", i, c, want[i])
			}
			if _, err := domain.ParseTimeZone(c.TimeZone); err != nil {
				t.Errorf("seed time zone %q invalid: %v", c.TimeZone, err)
			}
		}
	}
}

func TestWritesConflictsAndAuditOnPostgres(t *testing.T) {
	ctx := t.Context()
	svc := newService()
	name := "Cauca " + uuid.NewString()[:8]

	d, err := svc.CreateDivision(ctx, admin, application.DivisionInput{CountryCode: "CO", Name: name, Type: "DEPARTMENT"})
	if err != nil {
		t.Fatal(err)
	}
	c, err := svc.CreateCity(ctx, admin, application.CityInput{DivisionID: d.ID, Name: "Popayán", TimeZone: "America/Bogota"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateCity(ctx, admin, c.ID, application.CityPatch{TimeZone: ptr("UTC"), Active: ptr(false)}); err != nil {
		t.Fatal(err)
	}

	// Unique indexes (case-insensitive names, ISO codes) surface as ErrConflict.
	if _, err := svc.CreateDivision(ctx, admin, application.DivisionInput{CountryCode: "CO", Name: "valle DEL cauca", Type: "DEPARTMENT"}); !errors.Is(err, application.ErrConflict) {
		t.Errorf("duplicate division name err = %v", err)
	}
	if _, err := svc.UpdateDivision(ctx, admin, d.ID, application.DivisionPatch{Code: ptr("CO-VAC")}); !errors.Is(err, application.ErrConflict) {
		t.Errorf("duplicate division code err = %v", err)
	}
	if _, err := svc.CreateCity(ctx, admin, application.CityInput{DivisionID: geotest.ValleID, Name: "CALI", TimeZone: "America/Bogota"}); !errors.Is(err, application.ErrConflict) {
		t.Errorf("duplicate city err = %v", err)
	}
	if _, err := svc.CreateCountry(ctx, admin, application.CountryInput{Code: "CO", Name: "x", PhoneCode: "+57", DefaultCurrency: "COP"}); !errors.Is(err, application.ErrConflict) {
		t.Errorf("duplicate country err = %v", err)
	}

	// Each successful write left exactly one audit row with the admin as actor.
	rows, err := pool.Query(ctx, `SELECT action, resource_id, metadata->>'fields' FROM audit_log
		WHERE actor_user_id = $1 AND resource_id IN ($2, $3) ORDER BY occurred_at, id`, admin.UserID, d.ID.String(), c.ID.String())
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var action, id string
		var fields *string
		if err := rows.Scan(&action, &id, &fields); err != nil {
			t.Fatal(err)
		}
		if fields != nil {
			action += "[" + *fields + "]"
		}
		got = append(got, action)
	}
	want := []string{application.AuditDivisionCreated, application.AuditCityCreated, application.AuditCityUpdated + "[active,timeZone]"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("audit = %v, want %v", got, want)
	}
}

// failingAuditStore makes AppendAudit fail so the write must roll back with it.
type failingAuditStore struct{ *postgres.Store }

func (s failingAuditStore) WithinTx(ctx context.Context, fn func(application.Tx) error) error {
	return s.Store.WithinTx(ctx, func(tx application.Tx) error { return fn(failingAuditTx{tx}) })
}

type failingAuditTx struct{ application.Tx }

func (failingAuditTx) AppendAudit(context.Context, *audit.Entry) error {
	return errors.New("audit unavailable")
}

func TestWriteRollsBackWhenAuditFails(t *testing.T) {
	ctx := t.Context()
	svc := application.NewService(failingAuditStore{postgres.NewStore(pool)}, geotest.Authorizer{}, nil, clock.NewFixed(time.Now()))
	name := "Rollback " + uuid.NewString()[:8]
	if _, err := svc.CreateDivision(ctx, admin, application.DivisionInput{CountryCode: "CO", Name: name, Type: "REGION"}); err == nil {
		t.Fatal("expected the audit failure to fail the write")
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM administrative_divisions WHERE name = $1`, name).Scan(&n); err != nil || n != 0 {
		t.Errorf("division persisted without its audit entry: n=%d err=%v", n, err)
	}
}

func TestSchemaConstraintsBackTheDomain(t *testing.T) {
	ctx := t.Context()
	for name, q := range map[string]string{
		"lower-case country":          `INSERT INTO countries VALUES ('co', 'x', '+57', 'COP', true, now(), now())`,
		"bad phone code":              `INSERT INTO countries VALUES ('ZZ', 'x', '57', 'COP', true, now(), now())`,
		"division code of other land": `INSERT INTO administrative_divisions VALUES (gen_random_uuid(), 'CO', 'MX-JAL', 'x', 'STATE', true, now(), now())`,
		"unknown division type":       `INSERT INTO administrative_divisions VALUES (gen_random_uuid(), 'CO', NULL, 'x', 'COUNTY', true, now(), now())`,
		"city of unknown division":    `INSERT INTO cities VALUES (gen_random_uuid(), gen_random_uuid(), 'x', 'UTC', true, now(), now())`,
		"empty city name":             `INSERT INTO cities VALUES (gen_random_uuid(), '01a20aae-7e56-796c-9de9-5409ce51e7d4', '', 'UTC', true, now(), now())`,
	} {
		if _, err := pool.Exec(ctx, q); err == nil {
			t.Errorf("%s: accepted by the schema", name)
		}
	}
}
