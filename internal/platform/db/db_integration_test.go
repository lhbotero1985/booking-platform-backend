//go:build integration

package db_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/lhbotero1985/booking-platform-backend/internal/platform/db"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/db/platformq"
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
		p, err := db.NewPool(ctx, db.PoolConfig{URL: c.URL, MaxConns: 10, ApplicationName: "db-test"})
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

func createTable(t *testing.T, ddl string) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), ddl); err != nil {
		t.Fatal(err)
	}
}

func count(t *testing.T, table string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM "+pgx.Identifier{table}.Sanitize()).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestPoolApplicationNameAndTimeouts(t *testing.T) {
	var app, stmt string
	if err := pool.QueryRow(t.Context(),
		`SELECT current_setting('application_name'), current_setting('statement_timeout')`).Scan(&app, &stmt); err != nil {
		t.Fatal(err)
	}
	if app != "db-test" || stmt != "15s" {
		t.Errorf("application_name = %q, statement_timeout = %q", app, stmt)
	}
}

func TestWithTxCommits(t *testing.T) {
	createTable(t, `CREATE TABLE tx_commit (id int PRIMARY KEY)`)
	err := db.WithTx(t.Context(), pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `INSERT INTO tx_commit VALUES (1), (2)`)
		return err
	})
	if err != nil || count(t, "tx_commit") != 2 {
		t.Fatalf("err = %v, rows = %d, want 2", err, count(t, "tx_commit"))
	}
}

func TestWithTxRollsBackOnError(t *testing.T) {
	createTable(t, `CREATE TABLE tx_error (id int PRIMARY KEY)`)
	boom := errors.New("business rule failed")
	err := db.WithTx(t.Context(), pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(t.Context(), `INSERT INTO tx_error VALUES (1)`); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
	if n := count(t, "tx_error"); n != 0 {
		t.Errorf("rows = %d after rollback, want 0", n)
	}
}

func TestWithTxRollsBackOnPanic(t *testing.T) {
	createTable(t, `CREATE TABLE tx_panic (id int PRIMARY KEY)`)
	func() {
		defer func() {
			if recover() == nil {
				t.Error("panic was swallowed; want it re-raised")
			}
		}()
		_ = db.WithTx(t.Context(), pool, func(tx pgx.Tx) error {
			if _, err := tx.Exec(t.Context(), `INSERT INTO tx_panic VALUES (1)`); err != nil {
				return err
			}
			panic("unexpected")
		})
	}()
	if n := count(t, "tx_panic"); n != 0 {
		t.Errorf("rows = %d after panic, want 0", n)
	}
}

// TestExclusionConstraintMapsToSQLState exercises the double-booking guard mechanism of
// ADR-0004 (btree_gist exclusion over a time range) and the error mapping used to turn it
// into HTTP 409.
func TestExclusionConstraintMapsToSQLState(t *testing.T) {
	createTable(t, `CREATE TABLE slot_probe (
		professional_id int NOT NULL,
		occupied tstzrange NOT NULL,
		CONSTRAINT slot_probe_no_overlap EXCLUDE USING gist (professional_id WITH =, occupied WITH &&)
	)`)
	ctx := t.Context()
	ins := `INSERT INTO slot_probe VALUES ($1, tstzrange($2::timestamptz, $3::timestamptz, '[)'))`

	if _, err := pool.Exec(ctx, ins, 1, "2026-12-01 09:00Z", "2026-12-01 09:40Z"); err != nil {
		t.Fatal(err)
	}
	// Touching ranges [09:00,09:40) and [09:40,10:20) do not overlap.
	if _, err := pool.Exec(ctx, ins, 1, "2026-12-01 09:40Z", "2026-12-01 10:20Z"); err != nil {
		t.Fatalf("adjacent slot rejected: %v", err)
	}
	// Another professional at the same time is fine.
	if _, err := pool.Exec(ctx, ins, 2, "2026-12-01 09:10Z", "2026-12-01 09:50Z"); err != nil {
		t.Fatalf("other professional rejected: %v", err)
	}
	// Overlap for the same professional must fail with 23P01.
	_, err := pool.Exec(ctx, ins, 1, "2026-12-01 09:30Z", "2026-12-01 10:00Z")
	if !db.IsExclusionViolation(err) || db.ConstraintName(err) != "slot_probe_no_overlap" {
		t.Fatalf("err = %v (sqlstate %q), want exclusion violation", err, db.SQLState(err))
	}
}

func TestUniqueViolationMapping(t *testing.T) {
	createTable(t, `CREATE TABLE uniq_probe (email citext PRIMARY KEY)`)
	if _, err := pool.Exec(t.Context(), `INSERT INTO uniq_probe VALUES ('Ana@Example.com')`); err != nil {
		t.Fatal(err)
	}
	// citext makes the comparison case-insensitive.
	_, err := pool.Exec(t.Context(), `INSERT INTO uniq_probe VALUES ('ana@example.COM')`)
	if !db.IsUniqueViolation(err) {
		t.Fatalf("err = %v, want unique violation", err)
	}
}

// TestSQLCGeneratedQuery runs the sqlc-generated query against the migrated database,
// proving the sqlc configuration (schema from Goose migrations, pgx/v5 driver) works end to end.
func TestSQLCGeneratedQuery(t *testing.T) {
	exts, err := platformq.New(pool).ListExtensions(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, e := range exts {
		if e.Version == "" {
			t.Errorf("extension %s has empty version", e.Name)
		}
		got[e.Name] = true
	}
	for _, want := range []string{"postgis", "btree_gist", "citext"} {
		if !got[want] {
			t.Errorf("ListExtensions() missing %s (got %v)", want, got)
		}
	}
}
